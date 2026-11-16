package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func environment(t *testing.T) {
	t.Helper()
	for _, key := range []string{"APP_ENV", "PORT", "DOORMAN_ADMIN_USER", "DOORMAN_ADMIN_PASSWORD", "DOORMAN_SESSION_SECRET", "DOORMAN_ALLOWED_HOSTS", "DOORMAN_TRUSTED_PROXIES", "DOORMAN_RETENTION_DAYS", "DOORMAN_GEO_API_KEY", "DB_PROVIDER", "DB_PATH", "DATABASE_URL", "DB_HOST", "DB_PORT", "DB_USER", "DB_PASSWORD", "DB_NAME", "DB_SSLMODE"} {
		t.Setenv(key, "")
	}
	t.Setenv("APP_ENV", "test")
	t.Setenv("DOORMAN_ADMIN_USER", "owner")
	t.Setenv("DOORMAN_SESSION_SECRET", strings.Repeat("s", 32))
}
func TestValidation(t *testing.T) {
	for _, tc := range []struct{ name, key, value string }{
		{"empty secret", "DOORMAN_SESSION_SECRET", ""}, {"weak secret", "DOORMAN_SESSION_SECRET", "short"},
		{"invalid provider", "DB_PROVIDER", "postgress"}, {"invalid port", "PORT", "-1"},
		{"invalid retention", "DOORMAN_RETENTION_DAYS", "0"}, {"invalid proxy", "DOORMAN_TRUSTED_PROXIES", "*"},
		{"invalid host", "DOORMAN_ALLOWED_HOSTS", "https://example.com"}, {"weak bootstrap password", "DOORMAN_ADMIN_PASSWORD", "admin123"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			environment(t)
			t.Setenv(tc.key, tc.value)
			if _, err := Load(); err == nil {
				t.Fatal("invalid configuration accepted")
			}
		})
	}
}
func TestLocalConfigurationLoadsBeforeUse(t *testing.T) {
	environment(t)
	t.Setenv("APP_ENV", "local")
	os.Unsetenv("DB_PATH")
	os.Unsetenv("DB_PROVIDER")
	t.Chdir(t.TempDir())
	if err := os.WriteFile(filepath.Join(".", ".env"), []byte("DB_PATH=from-dotenv.db\nDB_PROVIDER=sqlite\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Database.Path != "from-dotenv.db" {
		t.Fatal("database settings loaded too late")
	}
}
func TestProductionRequiresHosts(t *testing.T) {
	environment(t)
	t.Setenv("APP_ENV", "production")
	if _, err := Load(); err == nil {
		t.Fatal("unrestricted production collector accepted")
	}
}
