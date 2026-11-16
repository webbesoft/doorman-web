package auth

import (
	"context"
	"errors"
	"github.com/webbesoft/doorman/internal/models"
	"github.com/webbesoft/doorman/internal/testutil"
	"golang.org/x/crypto/bcrypt"
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
			manager.now = func() time.Time { return now.Add(elapsed) }
			if _, err := manager.Authenticate(context.Background(), token); !errors.Is(err, ErrUnauthorized) {
				t.Fatal("expired session accepted")
			}
		})
	}
}
