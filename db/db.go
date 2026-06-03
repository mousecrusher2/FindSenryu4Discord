package db

import (
	"embed"
	"errors"
	"sync"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database"
	pgmigrate "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jinzhu/gorm"
	"github.com/mousecrusher2/FindSenryu4Discord/config"
	"github.com/mousecrusher2/FindSenryu4Discord/model"
	"github.com/mousecrusher2/FindSenryu4Discord/pkg/logger"

	// PostgreSQL driver for Gorm
	_ "github.com/lib/pq"
)

var (
	DB   *gorm.DB
	once sync.Once
)

//go:embed migrations/postgres/*.sql
var postgresMigrations embed.FS

// Init initializes the database connection
func Init() error {
	var initErr error
	once.Do(func() {
		initErr = initDB()
	})
	return initErr
}

func initDB() error {
	conf := config.GetConf()

	var err error
	DB, err = gorm.Open("postgres", conf.Database.DSN)
	if err != nil {
		logger.Error("Failed to connect to PostgreSQL", "error", err)
		return err
	}
	logger.Info("Connected to PostgreSQL database")

	// Configure connection pool
	sqlDB := DB.DB()
	if sqlDB != nil {
		sqlDB.SetMaxOpenConns(25)
		sqlDB.SetMaxIdleConns(5)
	}

	logger.Debug("Database connection established")

	return nil
}

// Migrate runs all schema and data migrations using golang-migrate.
// SQL migration files are embedded for PostgreSQL.
// It must be called after Init().
func Migrate() error {
	if DB == nil {
		return errors.New("database not initialized; call Init() first")
	}

	sqlDB := DB.DB()

	var sourceDriver source.Driver
	var dbDriver database.Driver

	sourceDriver, err := iofs.New(postgresMigrations, "migrations/postgres")
	if err != nil {
		return err
	}
	dbDriver, err = pgmigrate.WithInstance(sqlDB, &pgmigrate.Config{})
	if err != nil {
		return err
	}

	m, err := migrate.NewWithInstance("iofs", sourceDriver, "postgres", dbDriver)
	if err != nil {
		return err
	}

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return err
	}

	logger.Info("Database migration completed")

	return nil
}

// Close closes the database connection
func Close() error {
	if DB != nil {
		logger.Info("Closing database connection")
		if err := DB.Close(); err != nil {
			logger.Error("Failed to close database connection", "error", err)
			return err
		}
		logger.Info("Database connection closed")
	}
	return nil
}

// IsConnected returns true if database is connected
func IsConnected() bool {
	if DB == nil {
		return false
	}
	sqlDB := DB.DB()
	if sqlDB == nil {
		return false
	}
	return sqlDB.Ping() == nil
}

// GetDB returns the database instance
func GetDB() *gorm.DB {
	return DB
}

// Stats returns database statistics
type Stats struct {
	SenryuCount       int64
	MutedChannelCount int64
	OptOutCount       int64
	IsConnected       bool
}

// GetStats returns database statistics
func GetStats() Stats {
	stats := Stats{
		IsConnected: IsConnected(),
	}

	if DB != nil {
		DB.Model(&model.Senryu{}).Count(&stats.SenryuCount)
		DB.Model(&model.MutedChannel{}).Count(&stats.MutedChannelCount)
		DB.Model(&model.DetectionOptOut{}).Count(&stats.OptOutCount)
	}

	return stats
}
