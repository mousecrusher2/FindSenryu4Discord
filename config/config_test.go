package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSecretsBuildsPostgresDSN(t *testing.T) {
	dir := t.TempDir()
	writeSecret(t, dir, secretDiscordToken, "discord-token")
	writeSecret(t, dir, secretPGHost, "db.example.com")
	writeSecret(t, dir, secretPGDatabase, "findsenryu")
	writeSecret(t, dir, secretPGUser, "senryu")
	writeSecret(t, dir, secretPGPassword, "pa'ss\\word")
	writeSecret(t, dir, secretPGSSLMode, "verify-full")

	c := &Config{}
	setDefaults(c)
	if err := loadSecrets(c, dir); err != nil {
		t.Fatalf("loadSecrets failed: %v", err)
	}

	want := "host='db.example.com' dbname='findsenryu' user='senryu' password='pa\\'ss\\\\word' sslmode='verify-full'"
	if c.Database.DSN != want {
		t.Fatalf("DSN = %q, want %q", c.Database.DSN, want)
	}
}

func TestMigrationConfigDoesNotRequireDiscordSecret(t *testing.T) {
	dir := t.TempDir()
	writeSecret(t, dir, secretPGHost, "db.example.com")
	writeSecret(t, dir, secretPGDatabase, "findsenryu")
	writeSecret(t, dir, secretPGUser, "senryu")
	writeSecret(t, dir, secretPGPassword, "password")
	writeSecret(t, dir, secretPGSSLMode, "verify-full")

	c := &Config{}
	setDefaults(c)
	if err := loadDatabaseSecrets(c, dir); err != nil {
		t.Fatalf("loadDatabaseSecrets failed: %v", err)
	}
	if err := loadLogSecrets(c, dir); err != nil {
		t.Fatalf("loadLogSecrets failed: %v", err)
	}

	if c.Discord.Token != "" {
		t.Fatalf("Discord token = %q, want empty", c.Discord.Token)
	}
	if c.Database.DSN == "" {
		t.Fatal("Database DSN is empty")
	}
}

func writeSecret(t *testing.T, dir, name, value string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(value), 0o600); err != nil {
		t.Fatalf("failed to write secret %s: %v", name, err)
	}
}
