package database_test

import (
	"context"
	"github.com/webbesoft/doorman/internal/database"
	"github.com/webbesoft/doorman/internal/models"
	"github.com/webbesoft/doorman/internal/testutil"
	"testing"
	"time"
)

func TestAdminBootstrapSurvivesRestart(t *testing.T) {
	db := testutil.Open(t)
	if err := database.EnsureDefaultAdmin(db, "owner", "a-long-test-password"); err != nil {
		t.Fatal(err)
	}
	if err := database.EnsureDefaultAdmin(db, "owner", ""); err != nil {
		t.Fatal("restart failed:", err)
	}
	var count int64
	db.Model(&models.User{}).Count(&count)
	if count != 1 {
		t.Fatalf("admins=%d", count)
	}
	if err := database.EnsureDefaultAdmin(db, "other", ""); err == nil {
		t.Fatal("empty password accepted")
	}
	sqlDB, _ := db.DB()
	sqlDB.Close()
	if err := database.EnsureDefaultAdmin(db, "owner", ""); err == nil {
		t.Fatal("database lookup failure ignored")
	}
}
func TestRetentionRemovesChildrenAndKeepsRecentVisits(t *testing.T) {
	db := testutil.Open(t)
	old := time.Now().UTC().AddDate(0, 0, -100)
	a := models.Analytics{URL: "/old", CreatedAt: old}
	db.Create(&a)
	db.Create(&models.PageVisit{AnalyticsID: a.ID, URL: a.URL, CreatedAt: old})
	b := models.Analytics{URL: "/returning", CreatedAt: old}
	db.Create(&b)
	db.Create(&models.PageVisit{AnalyticsID: b.ID, URL: b.URL, CreatedAt: time.Now()})
	if err := database.CleanupOldData(db, 90); err != nil {
		t.Fatal(err)
	}
	var parents, children int64
	db.Model(&models.Analytics{}).Count(&parents)
	db.Model(&models.PageVisit{}).Count(&children)
	if parents != 1 || children != 1 {
		t.Fatalf("analytics=%d visits=%d", parents, children)
	}
	if err := database.Migrate(context.Background(), db); err != nil {
		t.Fatal("repeat migration:", err)
	}
	if err := database.CheckSchema(db); err != nil {
		t.Fatal(err)
	}
}
func TestSQLiteConfigurationAndPersistence(t *testing.T) {
	db := testutil.Open(t)
	var foreign int
	var journal string
	db.Raw("PRAGMA foreign_keys").Scan(&foreign)
	db.Raw("PRAGMA journal_mode").Scan(&journal)
	sqlDB, _ := db.DB()
	if foreign != 1 || journal != "wal" || sqlDB.Stats().MaxOpenConnections != 1 {
		t.Fatalf("foreign=%d journal=%s", foreign, journal)
	}
}
