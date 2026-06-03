package main

import (
	"fmt"
	"os"

	"github.com/mousecrusher2/FindSenryu4Discord/config"
	"github.com/mousecrusher2/FindSenryu4Discord/db"
	"github.com/mousecrusher2/FindSenryu4Discord/pkg/logger"
)

func main() {
	conf, err := config.LoadMigration()
	if err != nil {
		fmt.Fprintf(os.Stderr, "<3>Failed to load config: %v\n", err)
		os.Exit(1)
	}

	logger.Init(logger.Config{
		Level:  conf.Log.Level,
		Format: conf.Log.Format,
	})

	logger.Info("Starting database migration",
		"db_driver", "postgres",
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
