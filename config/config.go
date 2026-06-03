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
	secretDiscordToken          = "findsenryu-discord-token"
	secretDiscordPlaying        = "findsenryu-discord-playing"
	secretPGHost                = "findsenryu-pghost"
	secretPGDatabase            = "findsenryu-pgdatabase"
	secretPGUser                = "findsenryu-pguser"
	secretPGPassword            = "findsenryu-pgpassword"
	secretPGSSLMode             = "findsenryu-pgsslmode"
	secretLogLevel              = "findsenryu-log-level"
	secretLogFormat             = "findsenryu-log-format"
	secretAdminOwnerIDs         = "findsenryu-admin-owner-ids"
	secretAdminGuildID          = "findsenryu-admin-guild-id"
	secretAdminContactChannelID = "findsenryu-admin-contact-channel-id"
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
	Admin    AdminConfig
}

// DiscordConfig holds Discord-related configuration.
type DiscordConfig struct {
	Token   string
	Playing string
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
	Level  string
	Format string
}

// AdminConfig holds admin-related configuration.
type AdminConfig struct {
	OwnerIDs         []string
	GuildID          string
	ContactChannelID string
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
	c.Log.Format = "text"
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
	if err := loadAdminSecrets(c, dir); err != nil {
		return err
	}

	return nil
}

func loadDiscordSecrets(c *Config, dir string) error {
	var err error

	if c.Discord.Token, err = readSecret(dir, secretDiscordToken); err != nil {
		return err
	}
	c.Discord.Playing, err = readOptionalSecret(dir, secretDiscordPlaying)
	if err != nil {
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
	if c.Log.Format, err = readOptionalSecretWithDefault(dir, secretLogFormat, c.Log.Format); err != nil {
		return err
	}

	return nil
}

func loadAdminSecrets(c *Config, dir string) error {
	var err error

	if c.Admin.OwnerIDs, err = readOptionalListSecret(dir, secretAdminOwnerIDs); err != nil {
		return err
	}
	if c.Admin.GuildID, err = readOptionalSecret(dir, secretAdminGuildID); err != nil {
		return err
	}
	if c.Admin.ContactChannelID, err = readOptionalSecret(dir, secretAdminContactChannelID); err != nil {
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
		return "", fmt.Errorf("required podman secret %q is missing or empty", name)
	}
	return value, nil
}

func readOptionalSecret(dir, name string) (string, error) {
	value, _, err := readSecretFile(dir, name)
	return value, err
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

func readOptionalListSecret(dir, name string) ([]string, error) {
	value, exists, err := readSecretFile(dir, name)
	if err != nil || !exists || value == "" {
		return nil, err
	}
	return splitList(value), nil
}

func readSecretFile(dir, name string) (string, bool, error) {
	body, err := os.ReadFile(filepath.Join(dir, name))
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("failed to read podman secret %q: %w", name, err)
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
	if c.Admin.GuildID != "" && len(c.Admin.OwnerIDs) == 0 {
		fmt.Fprintln(os.Stderr, "<4>WARNING: admin guild id is set but admin owner ids are empty; admin commands will be registered but unusable")
	}
	return nil
}

func splitList(value string) []string {
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\n' || r == '\t'
	})
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
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

// GetConf returns the loaded configuration.
func GetConf() *Config {
	if conf == nil {
		var err error
		conf, err = Load()
		if err != nil {
			fmt.Fprintf(os.Stderr, "<3>Failed to load config: %v\n", err)
			os.Exit(1)
		}
	}
	return conf
}
