package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

const secretDir = "/run/secrets"

const (
	secretDiscordToken          = "findsenryu-discord-token"
	secretDiscordPlaying        = "findsenryu-discord-playing"
	secretDiscordWelcomeEnabled = "findsenryu-discord-welcome-enabled"
	secretDatabaseDSN           = "findsenryu-database-dsn"
	secretLogLevel              = "findsenryu-log-level"
	secretLogFormat             = "findsenryu-log-format"
	secretAdminOwnerIDs         = "findsenryu-admin-owner-ids"
	secretAdminGuildID          = "findsenryu-admin-guild-id"
	secretAdminLogChannelID     = "findsenryu-admin-log-channel-id"
	secretAdminReportChannelID  = "findsenryu-admin-report-channel-id"
	secretAdminContactChannelID = "findsenryu-admin-contact-channel-id"
	secretServerEnabled         = "findsenryu-server-enabled"
	secretServerPort            = "findsenryu-server-port"
	secretEncryptionKey         = "findsenryu-encryption-key"
)

var (
	conf *Config
	once sync.Once
)

// Config holds all configuration.
type Config struct {
	Discord    DiscordConfig
	Database   DatabaseConfig
	Log        LogConfig
	Admin      AdminConfig
	Server     ServerConfig
	Encryption EncryptionConfig
}

// DiscordConfig holds Discord-related configuration.
type DiscordConfig struct {
	Token          string
	Playing        string
	WelcomeEnabled *bool
}

// DatabaseConfig holds database configuration.
type DatabaseConfig struct {
	DSN string
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
	LogChannelID     string
	ReportChannelID  string
	ContactChannelID string
}

// ServerConfig holds HTTP server configuration.
type ServerConfig struct {
	Port    int
	Enabled *bool
}

// EncryptionConfig holds encryption configuration for senryu data.
type EncryptionConfig struct {
	Key string
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

func boolPtr(v bool) *bool { return &v }

func setDefaults(c *Config) {
	c.Discord.WelcomeEnabled = boolPtr(true)
	c.Log.Level = "info"
	c.Log.Format = "text"
	c.Server.Port = 9090
	c.Server.Enabled = boolPtr(true)
}

func loadSecrets(c *Config, dir string) error {
	var err error

	if c.Discord.Token, err = readSecret(dir, secretDiscordToken); err != nil {
		return err
	}
	c.Discord.Playing, err = readOptionalSecret(dir, secretDiscordPlaying)
	if err != nil {
		return err
	}
	if c.Discord.WelcomeEnabled, err = readOptionalBoolSecret(dir, secretDiscordWelcomeEnabled, c.Discord.WelcomeEnabled); err != nil {
		return err
	}

	if c.Database.DSN, err = readSecret(dir, secretDatabaseDSN); err != nil {
		return err
	}

	if c.Log.Level, err = readOptionalSecretWithDefault(dir, secretLogLevel, c.Log.Level); err != nil {
		return err
	}
	if c.Log.Format, err = readOptionalSecretWithDefault(dir, secretLogFormat, c.Log.Format); err != nil {
		return err
	}

	if c.Admin.OwnerIDs, err = readOptionalListSecret(dir, secretAdminOwnerIDs); err != nil {
		return err
	}
	if c.Admin.GuildID, err = readOptionalSecret(dir, secretAdminGuildID); err != nil {
		return err
	}
	if c.Admin.LogChannelID, err = readOptionalSecret(dir, secretAdminLogChannelID); err != nil {
		return err
	}
	if c.Admin.ReportChannelID, err = readOptionalSecret(dir, secretAdminReportChannelID); err != nil {
		return err
	}
	if c.Admin.ContactChannelID, err = readOptionalSecret(dir, secretAdminContactChannelID); err != nil {
		return err
	}

	if c.Server.Enabled, err = readOptionalBoolSecret(dir, secretServerEnabled, c.Server.Enabled); err != nil {
		return err
	}
	if c.Server.Port, err = readOptionalIntSecret(dir, secretServerPort, c.Server.Port); err != nil {
		return err
	}

	c.Encryption.Key, err = readOptionalSecret(dir, secretEncryptionKey)
	return err
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

func readOptionalBoolSecret(dir, name string, defaultValue *bool) (*bool, error) {
	value, exists, err := readSecretFile(dir, name)
	if err != nil {
		return nil, err
	}
	if !exists || value == "" {
		return defaultValue, nil
	}

	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return nil, fmt.Errorf("invalid boolean podman secret %q: %w", name, err)
	}
	return boolPtr(parsed), nil
}

func readOptionalIntSecret(dir, name string, defaultValue int) (int, error) {
	value, exists, err := readSecretFile(dir, name)
	if err != nil {
		return 0, err
	}
	if !exists || value == "" {
		return defaultValue, nil
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("invalid integer podman secret %q: %w", name, err)
	}
	return parsed, nil
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
		return errors.New("postgres dsn is required")
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

// IsWelcomeEnabled returns whether the welcome message feature is enabled.
func (c *DiscordConfig) IsWelcomeEnabled() bool {
	if c.WelcomeEnabled == nil {
		return true
	}
	return *c.WelcomeEnabled
}

// IsEnabled returns whether the health server is enabled.
func (c *ServerConfig) IsEnabled() bool {
	if c.Enabled == nil {
		return true
	}
	return *c.Enabled
}
