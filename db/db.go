package db

import (
	"fmt"
	"pkbldr/config"
	"time"

	"github.com/surrealdb/surrealdb.go"
)

func New() (*surrealdb.DB, error) {
	db, err := surrealdb.New(fmt.Sprintf("ws://%s:%d/rpc", config.Configs.SurrealHost, config.Configs.SurrealPort), surrealdb.WithTimeout(600*time.Second), surrealdb.UseWriteCompression(true))
	if err != nil {
		return nil, err
	}

	authData := map[string]interface{}{
		"user": config.Configs.SurrealUsername,
		"pass": config.Configs.SurrealPassword,
	}
	if _, err = db.Signin(authData); err != nil {
		return nil, err
	}

	if _, err = db.Use("pikabldr", "packages"); err != nil {
		return nil, err
	}

	return db, nil
}
