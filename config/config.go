package config

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

var Users map[string]User
var Configs Config

type User struct {
	Username     string `json:"username"`
	PasswordHash string `json:"passwordHash"`
}

// Struct representing an individual package file entries
type PackageFile struct {
	Name         string   `json:"name"`
	Url          string   `json:"url"`
	Subrepos     []string `json:"subrepos"`
	Priority     int      `json:"priority"`
	UseWhitelist bool     `json:"usewhitelist"`
	Whitelist    []string `json:"whitelist"`
	Blacklist    []string `json:"blacklist"`
	Packagepath  string   `json:"packagepath"`
	Compression  string   `json:"compression"`
}

// Struct for the overall configuration
type Config struct {
	SurrealHost          string        `json:"surrealHost"`
	SurrealPort          int           `json:"surrealPort"`
	SurrealUsername      string        `json:"surrealUsername"`
	SurrealPassword      string        `json:"surrealPassword"`
	TemporalUrl          string        `json:"temporalUrl"`
	UpstreamFallback     bool          `json:"upstreamFallback"`
	LocalPackageFiles    []PackageFile `json:"localPackageFiles"`
	ExternalPackageFiles []PackageFile `json:"externalPackageFiles"`
	LTOBlocklist         []string      `json:"ltoBlocklist"`
	DeboutputDir         string        `json:"deboutputDir"`
	Salt                 string        `json:"salt"`
}

func Init() error {
	err := loadUsers()
	if err != nil {
		return err
	}

	err = loadConfig()
	if err != nil {
		return err
	}

	return nil
}

func loadUsers() error {
	jsonFile, err := os.Open("users.json")
	if err != nil {
		fmt.Println(err)
		return err
	}
	defer jsonFile.Close()

	byteValue, _ := io.ReadAll(jsonFile)

	var users []User
	err = json.Unmarshal(byteValue, &users)
	if err != nil {
		fmt.Println(err)
		return err
	}

	var usersMap = make(map[string]User, len(users))
	for _, user := range users {
		usersMap[user.Username] = user
	}

	Users = usersMap

	return nil
}

func loadConfig() error {
	jsonFile, err := os.Open("config.json")
	if err != nil {
		fmt.Println(err)
		return err
	}
	defer jsonFile.Close()

	byteValue, _ := io.ReadAll(jsonFile)

	var config Config
	err = json.Unmarshal(byteValue, &config)
	if err != nil {
		fmt.Println(err)
		return err
	}

	Configs = config

	return nil
}
