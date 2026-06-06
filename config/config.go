package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const secretDir = "/run/secrets"

const (
	secretDiscordToken = "discord-token"
	secretPGHost       = "pg-host"
	secretPGDatabase   = "pg-database"
	secretPGUser       = "pg-user"
	secretPGPassword   = "pg-password"
	secretPGSSLMode    = "pg-sslmode"
	secretLogLevel     = "log-level"
)

var (
	conf *Config
	once sync.Once
)

// Config holds all configuration.
type Config struct {
	Discord  DiscordConfig
	Database DatabaseConfig
	Log      LogConfig
}

// DiscordConfig holds Discord-related configuration.
type DiscordConfig struct {
	Token string
}

// DatabaseConfig holds database configuration.
type DatabaseConfig struct {
	Host     string
	Name     string
	User     string
	Password string
	SSLMode  string
	DSN      string
}

// LogConfig holds logging configuration.
type LogConfig struct {
	Level string
}

// Load loads configuration from Podman secret files mounted under /run/secrets.
func Load() (*Config, error) {
	var loadErr error
	once.Do(func() {
		c := &Config{}
		setDefaults(c)

		if err := loadSecrets(c, secretDir); err != nil {
			loadErr = err
			return
		}
		if err := validate(c); err != nil {
			loadErr = err
			return
		}
		conf = c
	})

	return conf, loadErr
}

// LoadMigration loads only the configuration needed by the migration command.
func LoadMigration() (*Config, error) {
	var loadErr error
	once.Do(func() {
		c := &Config{}
		setDefaults(c)

		if err := loadDatabaseSecrets(c, secretDir); err != nil {
			loadErr = err
			return
		}
		if err := loadLogSecrets(c, secretDir); err != nil {
			loadErr = err
			return
		}
		conf = c
	})

	return conf, loadErr
}

func setDefaults(c *Config) {
	c.Log.Level = "info"
}

func loadSecrets(c *Config, dir string) error {
	if err := loadDiscordSecrets(c, dir); err != nil {
		return err
	}
	if err := loadDatabaseSecrets(c, dir); err != nil {
		return err
	}
	if err := loadLogSecrets(c, dir); err != nil {
		return err
	}
	return nil
}

func loadDiscordSecrets(c *Config, dir string) error {
	var err error

	if c.Discord.Token, err = readSecret(dir, secretDiscordToken); err != nil {
		return err
	}

	return nil
}

func loadDatabaseSecrets(c *Config, dir string) error {
	var err error

	if c.Database.Host, err = readSecret(dir, secretPGHost); err != nil {
		return err
	}
	if c.Database.Name, err = readSecret(dir, secretPGDatabase); err != nil {
		return err
	}
	if c.Database.User, err = readSecret(dir, secretPGUser); err != nil {
		return err
	}
	if c.Database.Password, err = readSecret(dir, secretPGPassword); err != nil {
		return err
	}
	if c.Database.SSLMode, err = readSecret(dir, secretPGSSLMode); err != nil {
		return err
	}
	c.Database.DSN = buildPostgresDSN(c.Database)

	return nil
}

func loadLogSecrets(c *Config, dir string) error {
	var err error

	if c.Log.Level, err = readOptionalSecretWithDefault(dir, secretLogLevel, c.Log.Level); err != nil {
		return err
	}

	return nil
}

func readSecret(dir, name string) (string, error) {
	value, exists, err := readSecretFile(dir, name)
	if err != nil {
		return "", err
	}
	if !exists || value == "" {
		return "", fmt.Errorf("required secret file %q is missing or empty", name)
	}
	return value, nil
}

func readOptionalSecretWithDefault(dir, name, defaultValue string) (string, error) {
	value, exists, err := readSecretFile(dir, name)
	if err != nil {
		return "", err
	}
	if !exists || value == "" {
		return defaultValue, nil
	}
	return value, nil
}

func readSecretFile(dir, name string) (string, bool, error) {
	body, err := os.ReadFile(filepath.Join(dir, name))
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("failed to read secret file %q: %w", name, err)
	}
	return strings.TrimSpace(string(body)), true, nil
}

func validate(c *Config) error {
	if c.Discord.Token == "" {
		return errors.New("discord token is required")
	}
	if c.Database.DSN == "" {
		return errors.New("postgres configuration is required")
	}
	return nil
}

func buildPostgresDSN(c DatabaseConfig) string {
	parts := []string{
		"host=" + quotePostgresValue(c.Host),
		"dbname=" + quotePostgresValue(c.Name),
		"user=" + quotePostgresValue(c.User),
		"password=" + quotePostgresValue(c.Password),
		"sslmode=" + quotePostgresValue(c.SSLMode),
	}
	return strings.Join(parts, " ")
}

func quotePostgresValue(value string) string {
	var b strings.Builder
	b.WriteByte('\'')
	for _, r := range value {
		if r == '\\' || r == '\'' {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	b.WriteByte('\'')
	return b.String()
}
