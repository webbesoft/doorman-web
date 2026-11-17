package auth

import (
	"context"
	"errors"
	"github.com/webbesoft/doorman/internal/config"
	"github.com/webbesoft/doorman/internal/database"
	"github.com/webbesoft/doorman/internal/models"
	"github.com/webbesoft/doorman/internal/testutil"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"strings"
	"testing"
	"time"
)

func TestSessionLifecycle(t *testing.T) {
	db := testutil.Open(t)
	hash, _ := bcrypt.GenerateFromPassword([]byte("test-password"), bcrypt.MinCost)
	user := models.User{Username: "owner", Password: string(hash)}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	manager := New(db, strings.Repeat("k", 32), true)
	ctx := context.Background()
	token, err := manager.Login(ctx, "owner", "test-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Authenticate(ctx, token); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Authenticate(ctx, token+"x"); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("tampered token accepted")
	}
	if err := manager.Logout(ctx, token); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Authenticate(ctx, token); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("logged-out token was replayed")
	}
	token, err = manager.Login(ctx, "owner", "test-password")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Delete(&user).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Authenticate(ctx, token); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("deleted account retains access")
	}
	cookie := manager.Cookie(token)
	if !cookie.Secure || !cookie.HttpOnly || cookie.MaxAge != int(Lifetime.Seconds()) {
		t.Fatalf("unsafe cookie flags: %+v", cookie)
	}
	if manager.Cookie("").MaxAge != -1 {
		t.Fatal("logout cookie not expired")
	}
}
func TestSessionIdleAndAbsoluteExpiry(t *testing.T) {
	for _, elapsed := range []time.Duration{IdleTimeout + time.Second, Lifetime + time.Second} {
		t.Run(elapsed.String(), func(t *testing.T) {
			db := testutil.Open(t)
			hash, _ := bcrypt.GenerateFromPassword([]byte("test-password"), bcrypt.MinCost)
			db.Create(&models.User{Username: "owner", Password: string(hash)})
			manager := New(db, strings.Repeat("k", 32), false)
			now := time.Now()
			manager.now = func() time.Time { return now }
			token, err := manager.Login(context.Background(), "owner", "test-password")
			if err != nil {
				t.Fatal(err)
			}
			if elapsed > Lifetime {
				for active := IdleTimeout / 2; active < Lifetime; active += IdleTimeout / 2 {
					manager.now = func() time.Time { return now.Add(active) }
					if _, err := manager.Authenticate(context.Background(), token); err != nil {
						t.Fatalf("active session expired before its absolute lifetime: %v", err)
					}
				}
			}
			manager.now = func() time.Time { return now.Add(elapsed) }
			if _, err := manager.Authenticate(context.Background(), token); !errors.Is(err, ErrUnauthorized) {
				t.Fatal("expired session accepted")
			}
		})
	}
}

func TestLegacyShortPasswordCannotAuthenticate(t *testing.T) {
	db := testutil.Open(t)
	hash, _ := bcrypt.GenerateFromPassword([]byte("admin123"), bcrypt.MinCost)
	if err := db.Create(&models.User{Username: "legacy-admin", Password: string(hash)}).Error; err != nil {
		t.Fatal(err)
	}
	manager := New(db, strings.Repeat("k", 32), false)
	if _, err := manager.Login(context.Background(), "legacy-admin", "admin123"); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("legacy weak password accepted")
	}
}

func TestPasswordResetCannotBeOvertakenByOldLogin(t *testing.T) {
	db := testutil.Open(t)
	hash, err := bcrypt.GenerateFromPassword([]byte("old-password-fixture"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.User{Username: "owner", Password: string(hash)}).Error; err != nil {
		t.Fatal(err)
	}
	var files []struct{ Name, File string }
	if err := db.Raw("PRAGMA database_list").Scan(&files).Error; err != nil {
		t.Fatal(err)
	}
	var path string
	for _, file := range files {
		if file.Name == "main" {
			path = file.File
		}
	}
	resetDB, err := database.InitDB(config.Database{Provider: "sqlite", Path: path})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := resetDB.DB()
	defer sqlDB.Close()
	manager := New(db, strings.Repeat("k", 32), false)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	creating, resume := make(chan struct{}), make(chan struct{})
	if err := db.Callback().Create().Before("gorm:create").Register("test:pause_session_insert", func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Name == "AuthSession" {
			close(creating)
			select {
			case <-resume:
			case <-ctx.Done():
				tx.AddError(ctx.Err())
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
	defer db.Callback().Create().Remove("test:pause_session_insert")
	type result struct {
		token string
		err   error
	}
	outcome := make(chan result, 1)
	go func() {
		token, err := manager.Login(ctx, "owner", "old-password-fixture")
		outcome <- result{token, err}
	}()
	select {
	case <-creating:
	case <-ctx.Done():
		t.Fatal("login did not reach session insertion")
	}
	resetErr := database.ResetAdminPassword(ctx, resetDB, "owner", "new-password-fixture")
	close(resume)
	login := <-outcome
	if resetErr != nil {
		t.Fatal(resetErr)
	}
	if login.err == nil {
		if _, err := manager.Authenticate(ctx, login.token); !errors.Is(err, ErrUnauthorized) {
			t.Fatal("old-password login created a usable session after reset")
		}
	}
}
