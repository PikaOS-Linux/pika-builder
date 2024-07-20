package packages

import (
	"compress/bzip2"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"pkbldr/config"
	"pkbldr/db"
	"pkbldr/deb"
	"slices"
	"strings"
	"time"

	"pault.ag/go/debian/version"

	"github.com/klauspost/compress/gzip"
	"github.com/surrealdb/surrealdb.go"
	"github.com/ulikunitz/xz"
)

var packagesSlice []PackageInfo = make([]PackageInfo, 0)
var updatedPackagesSlice []PackageInfo = make([]PackageInfo, 0)
var LastUpdateTime time.Time

var dbInstance *surrealdb.DB

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
	newPackagesSlice := make([]PackageInfo, 0)
	for _, v := range internalPackages {
		newPackagesSlice = append(newPackagesSlice, v)
	}
	slices.SortStableFunc(newPackagesSlice, func(a, b PackageInfo) int {
		if a.Name == b.Name {
			return 0
		}
		if a.Name > b.Name {
			return 1
		}
		return -1
	})

	for _, pkg2 := range newPackagesSlice {
		found := false
		for _, pkg := range packagesSlice {
			if pkg2.Name == pkg.Name {
				found = true
				if pkg.Status == Stale && pkg2.Status != Stale {
					pkg.Status = pkg2.Status
					pkg.Version = pkg2.Version
					pkg.PendingVersion = ""
					updatedPackagesSlice = append(updatedPackagesSlice, pkg)
				}
				if pkg.Status == Missing && pkg2.Status != Missing {
					pkg.PendingVersion = pkg2.PendingVersion
					pkg.Version = pkg2.Version
					pkg.Status = pkg2.Status
					updatedPackagesSlice = append(updatedPackagesSlice, pkg)
				}
				if pkg.Status == Missing && pkg2.Status == Missing {
					pkg.PendingVersion = pkg2.PendingVersion
					pkg.Version = pkg2.Version
					updatedPackagesSlice = append(updatedPackagesSlice, pkg)
				}
				if pkg.Status == Stale && pkg2.Status == Missing {
					pkg.PendingVersion = pkg2.PendingVersion
					pkg.Version = pkg2.Version
					pkg.Status = pkg2.Status
					updatedPackagesSlice = append(updatedPackagesSlice, pkg)
				}
				if (pkg2.Status == Stale || pkg2.Status == Missing) && (pkg.Status == Uptodate || pkg.Status == Stale || pkg.Status == Built || pkg.Status == Error) {
					pkg.PendingVersion = pkg2.PendingVersion
					pkg.Status = pkg2.Status
					updatedPackagesSlice = append(updatedPackagesSlice, pkg)
				}
			}
		}
		if !found {
			pkg2.LastBuildStatus = ""
			updatedPackagesSlice = append(updatedPackagesSlice, pkg2)
		}
	}
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

type PackageBuildQueue map[string][]PackageInfo

func GetBuildQueue() PackageBuildQueue {
	buildQueue := make(map[string][]PackageInfo)
	for _, pkg := range packagesSlice {
		if !(pkg.Status == Missing || pkg.Status == Stale) {
			continue
		}

		key := pkg.Source
		if key == "" {
			key = pkg.Name
		}
		existing, ok := buildQueue[key]
		if !ok {
			existing = make([]PackageInfo, 0)
		}
		existing = append(existing, pkg)
		buildQueue[key] = existing
	}
	return buildQueue
}

func UpdatePackage(pkg PackageInfo, updateDB bool) error {
	for i, v := range packagesSlice {
		if pkg.Name == v.Name {
			packagesSlice[i] = pkg
			if updateDB {
				err := saveSingleToDb(pkg)
				if err != nil {
					return err
				}
			}
			LastUpdateTime = time.Now()
			break
		}
	}
	if updateDB {
		err := saveSingleToDb(pkg)
		if err != nil {
			return err
		}
	}
	LastUpdateTime = time.Now()
	return nil
}

func IsBuilt(pkg PackageInfo) bool {
	for _, v := range packagesSlice {
		if pkg.Name == v.Name {
			if v.Status == Built {
				return true
			}
			break
		}
	}
	return false
}

func saveSingleToDb(pkg PackageInfo) error {
	if dbInstance == nil {
		var err error
		dbInstance, err = db.New()
		if err != nil {
			return err
		}
	}
	_, err := surrealdb.SmartMarshal(dbInstance.Update, pkg)
	if err != nil {
		fmt.Println(err)
		return err
	}
	timecont := TimeContainer{
		ID:   "lastupdatetime:`lastupdatetime`",
		Time: time.Now().Format("2006-01-02T15:04:05.999Z"),
	}
	_, err = surrealdb.SmartMarshal(dbInstance.Update, timecont)
	if err != nil {
		fmt.Println(err)
		return err
	}
	return nil
}

func SaveToDb() error {
	if dbInstance == nil {
		var err error
		dbInstance, err = db.New()
		if err != nil {
			return err
		}
	}

	for i, v := range updatedPackagesSlice {
		id := "packagestore:`" + v.Name + "`"
		v.ID = id
		updatedPackagesSlice[i] = v
	}

	for _, pkg := range updatedPackagesSlice {
		_, err := surrealdb.SmartMarshal(dbInstance.Update, pkg)
		if err != nil {
			fmt.Println(err)
			return err
		}
	}

	timecont := TimeContainer{
		ID:   "lastupdatetime:`lastupdatetime`",
		Time: time.Now().Format("2006-01-02T15:04:05.999Z"),
	}
	_, err := surrealdb.SmartMarshal(dbInstance.Update, timecont)
	if err != nil {
		fmt.Println(err)
		return err
	}

	return nil
}

func LoadFromDb() error {
	if dbInstance == nil {
		var err error
		dbInstance, err = db.New()
		if err != nil {
			slog.Error(err.Error())
			return nil
		}
	}
	packages, err := surrealdb.SmartUnmarshal[[]PackageInfo](dbInstance.Select("packagestore"))
	if err != nil {
		slog.Error(err.Error())
		return nil
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
	timecont, err := surrealdb.SmartUnmarshal[TimeContainer](dbInstance.Select("lastupdatetime:`lastupdatetime`"))
	if err != nil {
		slog.Error(err.Error())
		return nil
	}
	LastUpdateTime, err = time.Parse("2006-01-02T15:04:05.999Z", timecont.Time)
	if err != nil {
		slog.Error(err.Error())
		return nil
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
				pk, ok := internalPackages[k]
				if !ok {
					internalPackages[k] = v
					continue
				}
				matchedVer, _ := version.Parse(pk.Version)
				extVer, _ := version.Parse(v.Version)
				cmpVal := version.Compare(extVer, matchedVer)
				if cmpVal >= 0 {
					internalPackages[k] = v
				}
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
			return -1
		}
		return 1
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
			newStatus := Missing
			v.Status = newStatus
			internalPackages[k] = v
		}
	}
}

func ProcessStalePackages(internalPackages map[string]PackageInfo, externalPackages map[string]PackageInfo) {
	for k, v := range externalPackages {
		matchedPackage, ok := internalPackages[k]
		if !ok || matchedPackage.Status == Missing {
			continue
		}

		splitver := strings.Split(v.Version, "+b")
		matchedVer, _ := version.Parse(matchedPackage.Version)
		extVer, _ := version.Parse(splitver[0])
		cmpVal := version.Compare(matchedVer, extVer)
		if cmpVal < 0 {
			matchedPackage.Status = Stale
			matchedPackage.PendingVersion = extVer.String()
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

		if stanza["Section"] == "debian-installer" {
			continue
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

		pk, ok := packages[stanza["Package"]]
		if ok {
			matchedVer, _ := version.Parse(pk.Version)
			cmpVal := version.Compare(ver, matchedVer)
			if cmpVal < 0 {
				continue
			}
		}

		packages[stanza["Package"]] = PackageInfo{
			Name:         stanza["Package"],
			Version:      ver.String(),
			Source:       stanza["Source"],
			Architecture: stanza["Architecture"],
			Description:  stanza["Description"],
			Status:       Uptodate,
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
		switch v.Status {
		case Stale:
			count.Stale++
		case Missing:
			count.Missing++
		case Built:
			count.Built++
		case Uptodate:
			count.Built++
		case Error:
			count.Error++
		case Queued:
			count.Queued++
		case Building:
			count.Building++
		}
		if v.LastBuildStatus == Error {
			count.Error++
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
	// Source of the package
	Source string `json:"source"`
	// Architecture of the package
	Architecture string `json:"architecture"`
	// Description of the package
	Description string `json:"description"`
	// Status of the package
	Status PackageStatus `json:"statusinfo"`
	// Last built version
	LastBuildVersion string `json:"lastbuiltversion"`
	// Number of build attempts since last successful build
	BuildAttempts int `json:"buildattempts"`
	// Pending Version
	PendingVersion string `json:"pendingversion"`
	// Last Built Status
	LastBuildStatus PackageStatus `json:"buildstatusinfo"`
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
	// Package is upto date
	Uptodate PackageStatus = "Uptodate"
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
