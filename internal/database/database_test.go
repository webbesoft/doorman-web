package database_test

import (
	"context"
	"github.com/webbesoft/doorman/internal/config"
	"github.com/webbesoft/doorman/internal/database"
	"github.com/webbesoft/doorman/internal/models"
	"github.com/webbesoft/doorman/internal/testutil"
	"os"
	"path/filepath"
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
	if err := database.Migrate(context.Background(), db, "test-migration-secret-32-bytes-long"); err != nil {
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

func TestLegacyMigrationPreservesRowsAndRedactsURLs(t *testing.T) {
	db, err := database.InitDB(config.Database{Provider: "sqlite", Path: filepath.Join(t.TempDir(), "legacy.db")})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	if err := db.AutoMigrate(&models.Analytics{}, &models.PageVisit{}, &models.User{}); err != nil {
		t.Fatal(err)
	}
	a := models.Analytics{URL: "https://example.com/page?token=private#secret", IPHash: "old-ip-hash", Referrer: "https://search.example/?email=private", Country: "Zimbabwe"}
	db.Create(&a)
	db.Create(&models.PageVisit{AnalyticsID: a.ID, URL: a.URL, IPHash: a.IPHash, DwellTime: 12, ActiveTime: 5, ScrollDepth: 80})
	for range 2 {
		if err := database.Migrate(context.Background(), db, "test-migration-secret-32-bytes-long"); err != nil {
			t.Fatal(err)
		}
	}
	var view models.PageView
	if err := db.First(&view).Error; err != nil {
		t.Fatal(err)
	}
	if !view.Legacy || view.URL != "https://example.com/page" || view.Referrer != "search.example" || view.DwellTime != 12 || view.VisitorKey == a.IPHash {
		t.Fatalf("migration=%+v", view)
	}
	var count int64
	db.Model(&models.PageView{}).Count(&count)
	if count != 1 {
		t.Fatal("repeat migration duplicated history")
	}
	db.Model(&models.PageVisit{}).Count(&count)
	if count != 1 {
		t.Fatal("original records modified")
	}
}

func TestPageViewRetentionUsesCalendarDays(t *testing.T) {
	db := testutil.Open(t)
	today := time.Now().UTC()
	old := today.AddDate(0, 0, -90)
	for _, view := range []models.PageView{
		{ID: "keep", Day: today.Format("2006-01-02"), URL: "https://example.com/"},
		{ID: "delete", Day: old.Format("2006-01-02"), URL: "https://example.com/"},
	} {
		if err := db.Create(&view).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := database.CleanupOldData(db, 90); err != nil {
		t.Fatal(err)
	}
	var views []models.PageView
	db.Find(&views)
	if len(views) != 1 || views[0].ID != "keep" {
		t.Fatal(views)
	}
}

func TestVerifiedBackupCanBeRestoredAndNeverOverwrites(t *testing.T) {
	db := testutil.Open(t)
	ctx := context.Background()
	user := models.User{Username: "backup-owner", Password: "already-hashed-fixture"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	backup := filepath.Join(t.TempDir(), "backup.db")
	if err := database.BackupSQLite(ctx, db, backup); err != nil {
		t.Fatal(err)
	}
	if err := database.BackupSQLite(ctx, db, backup); err == nil {
		t.Fatal("existing backup overwritten")
	}
	if err := database.VerifySQLiteBackup(ctx, backup); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(backup)
	if err != nil {
		t.Fatal(err)
	}
	restore := filepath.Join(t.TempDir(), "restored.db")
	if err := os.WriteFile(restore, contents, 0600); err != nil {
		t.Fatal(err)
	}
	restored, err := database.InitDB(config.Database{Provider: "sqlite", Path: restore})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := restored.DB()
	defer sqlDB.Close()
	var found models.User
	if err := restored.Where("username = ?", user.Username).First(&found).Error; err != nil {
		t.Fatal(err)
	}
	if found.Password != user.Password {
		t.Fatal("restored data differs")
	}
	if err := database.CheckSchema(restored); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(backup)
	if info.Mode().Perm() != 0600 {
		t.Fatal("backup is not private")
	}
}

func TestLegacyRetentionUsesSameCalendarCutoff(t *testing.T) {
	db := testutil.Open(t)
	now := time.Now().UTC()
	yesterdayEnd := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).Add(-time.Nanosecond)
	parent := models.Analytics{URL: "https://example.com/", CreatedAt: yesterdayEnd}
	if err := db.Create(&parent).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.PageVisit{AnalyticsID: parent.ID, URL: parent.URL, CreatedAt: yesterdayEnd}).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.CleanupOldData(db, 1); err != nil {
		t.Fatal(err)
	}
	var parents, children int64
	db.Model(&models.Analytics{}).Count(&parents)
	db.Model(&models.PageVisit{}).Count(&children)
	if parents != 0 || children != 0 {
		t.Fatal("expired sensitive legacy rows survived the calendar cutoff")
	}
}
