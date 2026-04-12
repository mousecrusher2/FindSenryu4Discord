package main

import (
	"fmt"
	"os"

	"github.com/u16-io/FindSenryu4Discord/config"
	"github.com/u16-io/FindSenryu4Discord/db"
	"github.com/u16-io/FindSenryu4Discord/pkg/crypto"
	"github.com/u16-io/FindSenryu4Discord/pkg/logger"
)

func main() {
	conf, err := config.Load("config.toml")
	if err != nil {
		fmt.Printf("Failed to load config: %v\n", err)
		os.Exit(1)
	}

	logger.Init(logger.Config{
		Level:  conf.Log.Level,
		Format: conf.Log.Format,
	})

	if err := crypto.Init(conf.Encryption.Key); err != nil {
		logger.Error("Failed to initialize encryption", "error", err)
		os.Exit(1)
	}
	conf.Encryption.Key = ""

	logger.Info("Starting database migration",
		"db_driver", conf.Database.Driver,
		"encryption_enabled", crypto.IsEnabled(),
	)

	if err := db.Init(); err != nil {
		logger.Error("Failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := db.Migrate(); err != nil {
		logger.Error("Migration failed", "error", err)
		os.Exit(1)
	}

	logger.Info("Migration completed successfully")
}
