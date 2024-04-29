package packages

import (
	"compress/bzip2"
	"fmt"
	"io"
	"net/http"
	"pkbldr/config"
	"pkbldr/db"
	"pkbldr/deb"
	"slices"
	"strings"
	"time"

	"github.com/goccy/go-json"

	"pault.ag/go/debian/version"

	"github.com/klauspost/compress/gzip"
	"github.com/surrealdb/surrealdb.go"
	"github.com/ulikunitz/xz"
)

var packagesSlice []PackageInfo = make([]PackageInfo, 0)
var LastUpdateTime time.Time

func GetPackagesSlice() []PackageInfo {
	return packagesSlice
}

func ProcessPackages() error {
	var internalPackages = make(map[string]PackageInfo)
	var externalPackages = make(map[string]PackageInfo)
	err := LoadInternalPackages(internalPackages)
	if err != nil {
		return err
	}
	err = LoadExternalPackages(externalPackages)
	if err != nil {
		return err
	}
	ProcessStalePackages(internalPackages, externalPackages)
	ProcessMissingPackages(internalPackages, externalPackages)
	packagesSlice = make([]PackageInfo, 0)
	for _, v := range internalPackages {
		packagesSlice = append(packagesSlice, v)
	}
	slices.SortStableFunc(packagesSlice, func(a, b PackageInfo) int {
		if a.Name == b.Name {
			return 0
		}
		if a.Name > b.Name {
			return 1
		}
		return -1
	})
	LastUpdateTime = time.Now()
	err = SaveToDb()
	if err != nil {
		return err
	}
	err = LoadFromDb()
	if err != nil {
		return err
	}
	return nil
}

type packs struct {
	ID   string        `json:"id"`
	Pkgs []PackageInfo `json:"pkgs"`
}

func UpdatePackage(pkg PackageInfo) {
	for i, v := range packagesSlice {
		if pkg.Name == v.Name {
			packagesSlice[i] = pkg
			LastUpdateTime = time.Now()
			break
		}
	}
}

func SaveToDb() error {
	database, err := db.New()
	if err != nil {
		return err
	}
	defer database.Close()

	for i, v := range packagesSlice {
		id := "packages:`" + v.Name + "`"
		v.ID = id
		packagesSlice[i] = v
	}

	packsMarshaled, err := json.Marshal(packagesSlice)
	if err != nil {
		fmt.Println(err)
		return err
	}

	_, err = database.Query("REMOVE TABLE packages", nil)
	if err != nil {
		fmt.Println(err)
		return err
	}

	_, err = database.Query("INSERT INTO packages "+string(packsMarshaled), nil)
	if err != nil {
		fmt.Println(err)
		return err
	}

	timecont := TimeContainer{
		ID:   "lastupdatetime:`lastupdatetime`",
		Time: time.Now().Format("2006-01-02T15:04:05.999Z"),
	}
	_, err = surrealdb.SmartMarshal(database.Update, timecont)
	if err != nil {
		fmt.Println(err)
		return err
	}

	return nil
}

func LoadFromDb() error {
	database, err := db.New()
	if err != nil {
		return err
	}
	defer database.Close()
	packages, err := surrealdb.SmartUnmarshal[[]PackageInfo](database.Select("packages"))
	if err != nil {
		return err
	}
	packagesSlice = packages
	slices.SortStableFunc(packagesSlice, func(a, b PackageInfo) int {
		if a.Name == b.Name {
			return 0
		}
		if a.Name > b.Name {
			return 1
		}
		return -1
	})
	timecont, err := surrealdb.SmartUnmarshal[TimeContainer](database.Select("lastupdatetime:`lastupdatetime`"))
	if err != nil {
		return err
	}
	LastUpdateTime, err = time.Parse("2006-01-02T15:04:05.999Z", timecont.Time)
	if err != nil {
		return err
	}
	return nil
}

func LoadInternalPackages(internalPackages map[string]PackageInfo) error {
	localPackageFile := config.Configs.LocalPackageFiles
	slices.SortStableFunc(localPackageFile, func(a, b config.PackageFile) int {
		if a.Priority == b.Priority {
			return 0
		}
		if a.Priority < b.Priority {
			return 1
		}
		return -1
	})

	for _, pkg := range config.Configs.LocalPackageFiles {
		for _, repo := range pkg.Subrepos {
			packages, err := fetchPackageFile(pkg, repo)
			if err != nil {
				return err
			}
			for k, v := range packages {
				internalPackages[k] = v
			}
		}
	}

	return nil
}

func LoadExternalPackages(externalPackages map[string]PackageInfo) error {
	externalPackageFile := config.Configs.ExternalPackageFiles
	slices.SortStableFunc(externalPackageFile, func(a, b config.PackageFile) int {
		if a.Priority == b.Priority {
			return 0
		}
		if a.Priority < b.Priority {
			return 1
		}
		return -1
	})

	for _, pkg := range config.Configs.ExternalPackageFiles {
		for _, repo := range pkg.Subrepos {
			packages, err := fetchPackageFile(pkg, repo)
			if err != nil {
				return err
			}
			for k, v := range packages {
				externalPackages[k] = v
			}
		}
	}

	return nil
}

func ProcessMissingPackages(internalPackages map[string]PackageInfo, externalPackages map[string]PackageInfo) {
	for k, v := range externalPackages {
		_, ok := internalPackages[k]
		if !ok {
			newStatus := Status{
				Status: Missing,
			}
			v.Status = newStatus
			internalPackages[k] = v
		}
	}
}

func ProcessStalePackages(internalPackages map[string]PackageInfo, externalPackages map[string]PackageInfo) {
	for k, v := range externalPackages {
		matchedPackage, ok := internalPackages[k]
		if !ok || matchedPackage.Status.Status == Missing {
			continue
		}

		splitver := strings.Split(v.Version, "+b")
		matchedVer, _ := version.Parse(matchedPackage.Version)
		extVer, _ := version.Parse(splitver[0])
		cmpVal := version.Compare(matchedVer, extVer)
		if cmpVal < 0 {
			newStatus := Status{
				Status:     Stale,
				NewVersion: v.Version,
			}
			matchedPackage.Status = newStatus
			internalPackages[k] = matchedPackage
		}
	}
}

func fetchPackageFile(pkg config.PackageFile, selectedRepo string) (map[string]PackageInfo, error) {
	resp, err := http.Get(pkg.Url + selectedRepo + "/" + pkg.Packagepath + "." + pkg.Compression)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	rdr := io.Reader(resp.Body)
	if pkg.Compression == "bz2" {
		r := bzip2.NewReader(resp.Body)
		rdr = r
	}
	if pkg.Compression == "xz" {
		r, err := xz.NewReader(resp.Body)
		if err != nil {
			return nil, err
		}
		rdr = r
	}
	if pkg.Compression == "gz" {
		r, err := gzip.NewReader(resp.Body)
		if err != nil {
			return nil, err
		}
		rdr = r
	}

	packages := make(map[string]PackageInfo)
	sreader := deb.NewControlFileReader(rdr, false, false)
	for {
		stanza, err := sreader.ReadStanza()
		if err != nil || stanza == nil {
			break
		}

		useWhitelist := pkg.UseWhitelist && len(pkg.Whitelist) > 0
		if useWhitelist {
			contained := nameContains(stanza["Package"], pkg.Whitelist)
			if !contained {
				continue
			}
		}

		broken := nameContains(stanza["Package"], pkg.Blacklist)
		if broken {
			continue
		}

		ver, err := version.Parse(stanza["Version"])
		if err != nil {
			return nil, err
		}

		packages[stanza["Package"]] = PackageInfo{
			Name:         stanza["Package"],
			Version:      ver.String(),
			Architecture: stanza["Architecture"],
			Description:  stanza["Description"],
			Status: Status{
				Status: Built,
			},
		}
	}

	return packages, nil
}

func nameContains(name string, match []string) bool {
	for _, m := range match {
		if strings.Contains(name, m) {
			return true
		}
	}
	return false
}

func GetPackagesCount() PackagesCount {
	count := PackagesCount{
		Stale:    0,
		Missing:  0,
		Built:    0,
		Error:    0,
		Queued:   0,
		Building: 0,
	}
	for _, v := range packagesSlice {
		switch v.Status.Status {
		case Stale:
			count.Stale++
		case Missing:
			count.Missing++
		case Built:
			count.Built++
		case Error:
			count.Error++
		case Queued:
			count.Queued++
		case Building:
			count.Building++
		}
	}
	return count
}

type PackagesCount struct {
	Stale    int
	Missing  int
	Built    int
	Error    int
	Queued   int
	Building int
}

type PackageInfo struct {
	ID string `json:"id"`
	// Name of the package
	Name string `json:"name"`
	// Version of the package
	Version string `json:"version"`
	// Architecture of the package
	Architecture string `json:"architecture"`
	// Description of the package
	Description string `json:"description"`
	// Status of the package
	Status Status `json:"statusinfo"`
}

type PackageStatus string

const (
	// Package is built
	Built PackageStatus = "Built"
	// Package is stale
	Stale PackageStatus = "Stale"
	// Package build errored out
	Error PackageStatus = "Error"
	// Package is queued for building
	Queued PackageStatus = "Queued"
	// Package is being built
	Building PackageStatus = "Building"
	// Package is being missing
	Missing PackageStatus = "Missing"
)

type TimeContainer struct {
	ID   string `json:"id"`
	Time string `json:"time"`
}

type Status struct {
	// Status of the package
	Status PackageStatus `json:"status"`
	// Error message
	Error string `json:"error"`
	// New version of the package
	NewVersion string `json:"newversion"`
}
