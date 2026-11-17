package database_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/webbesoft/doorman/internal/analytics"
	"github.com/webbesoft/doorman/internal/auth"
	"github.com/webbesoft/doorman/internal/config"
	"github.com/webbesoft/doorman/internal/database"
	"github.com/webbesoft/doorman/internal/models"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// Only explicitly configured disposable databases are used. No development DSNs are read.
func TestBackendContracts(t *testing.T) {
	for _, provider := range []string{"postgres", "mysql"} {
		t.Run(provider, func(t *testing.T) {
			dsn := os.Getenv("DOORMAN_TEST_" + strings.ToUpper(provider) + "_DSN")
			if dsn == "" {
				t.Skip("disposable backend DSN not configured")
			}
			db, err := database.InitDB(config.Database{Provider: provider, URL: dsn})
			if err != nil {
				t.Fatal(err)
			}
			sqlDB, _ := db.DB()
			defer sqlDB.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if err := database.Migrate(ctx, db, strings.Repeat("k", 32)); err != nil {
				t.Fatal(err)
			}
			if err := database.CheckSchema(db); err != nil {
				t.Fatal(err)
			}
			var raw [16]byte
			if _, err := rand.Read(raw[:]); err != nil {
				t.Fatal(err)
			}
			id := hex.EncodeToString(raw[:])
			tracker := analytics.NewTracker(db, strings.Repeat("k", 32), []string{"example.com"}, nil, 1000000)
			event := analytics.Event{VisitID: id, Sequence: 1, URL: "https://example.com/backend-contract", DwellTime: 1, ActiveTime: 1, ScrollDepth: 11}
			errs := make(chan error, 8)
			var wg sync.WaitGroup
			for range 8 {
				wg.Add(1)
				go func() { defer wg.Done(); errs <- tracker.Record(ctx, event, "8.8.8.8", "Mozilla/5.0") }()
			}
			wg.Wait()
			close(errs)
			for err := range errs {
				if err != nil {
					t.Fatal(err)
				}
			}
			var count int64
			db.Model(&models.PageView{}).Where("id = ?", id).Count(&count)
			if count != 1 {
				t.Fatalf("duplicate visits=%d", count)
			}
			event.Sequence = 3
			event.DwellTime = 10
			event.ActiveTime = 5
			event.Final = true
			if err := tracker.Record(ctx, event, "8.8.8.8", "Mozilla/5.0"); err != nil {
				t.Fatal(err)
			}
			event.Sequence = 2
			event.DwellTime = 2
			event.ActiveTime = 1
			if err := tracker.Record(ctx, event, "8.8.8.8", "Mozilla/5.0"); err != nil {
				t.Fatal(err)
			}
			var persisted models.PageView
			db.Where("id = ?", id).First(&persisted)
			if persisted.DwellTime != 10 || !persisted.Final {
				t.Fatalf("stale update: %+v", persisted)
			}
			if _, err := analytics.NewReports(db).Snapshot(ctx, time.Now()); err != nil {
				t.Fatal("report query:", err)
			}
			hash, err := bcrypt.GenerateFromPassword([]byte("backend-fixture-password"), bcrypt.MinCost)
			if err != nil {
				t.Fatal(err)
			}
			user := models.User{Username: "contract-" + id, Password: string(hash)}
			if err := db.Create(&user).Error; err != nil {
				t.Fatal(err)
			}
			manager := auth.New(db, strings.Repeat("k", 32), false)
			token, err := manager.Login(ctx, user.Username, "backend-fixture-password")
			if err != nil {
				t.Fatal("serialized login:", err)
			}
			if err := database.ResetAdminPassword(ctx, db, user.Username, "changed-fixture-password"); err != nil {
				t.Fatal(err)
			}
			if _, err := manager.Authenticate(ctx, token); !errors.Is(err, auth.ErrUnauthorized) {
				t.Fatal("reset session retained access:", err)
			}
			if err := db.Delete(&user).Error; err != nil {
				t.Fatal(err)
			}
			// Remove only this fixture, preserving unrelated rows.
			if err := db.Transaction(func(tx *gorm.DB) error {
				if err := tx.Where("id = ?", id).Delete(&models.PageView{}).Error; err != nil {
					return err
				}
				return tx.Model(&models.StorageBudget{}).Where("id = ?", 1).Update("retained_views", gorm.Expr("retained_views - 1")).Error
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}
