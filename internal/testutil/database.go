package testutil

import (
	"context"
	"github.com/webbesoft/doorman/internal/config"
	"github.com/webbesoft/doorman/internal/database"
	"gorm.io/gorm"
	"path/filepath"
	"testing"
)

// Open uses the production SQLite configuration with a separate file for each test.
func Open(t testing.TB) *gorm.DB {
	t.Helper()
	db, err := database.InitDB(config.Database{Provider: "sqlite", Path: filepath.Join(t.TempDir(), "analytics.db")})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { sqlDB.Close() })
	if err := database.Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	return db
}
