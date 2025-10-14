package handlers

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/webbesoft/doorman/internal/models"
)

func newTestHandler(t *testing.T) (*Handler, func()) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}

	if err := db.AutoMigrate(&models.PageView{}); err != nil {
		t.Fatalf("auto migrate failed: %v", err)
	}

	cleanup := func() {
		sqlDB, err := db.DB()
		if err == nil {
			sqlDB.Close()
		}
	}

	return &Handler{DB: db}, cleanup
}

func TestTrack_Success(t *testing.T) {
	h, cleanup := newTestHandler(t)
	defer cleanup()

	e := echo.New()

	payload := map[string]string{
		"url":      "/test-page",
		"referrer": "https://example.com",
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/event", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req.RemoteAddr = "1.2.3.4:5678"

	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.Track(c); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 got %d body=%s", rec.Code, rec.Body.String())
	}

	var resp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid json response: %v", err)
	}
	if resp["status"] != "ok" {
		t.Fatalf("unexpected response body: %v", resp)
	}

	var pv models.PageView
	if err := h.DB.First(&pv).Error; err != nil {
		t.Fatalf("expected a page view in db: %v", err)
	}

	if pv.URL != payload["url"] {
		t.Errorf("expected url %q got %q", payload["url"], pv.URL)
	}
	if pv.Referrer != payload["referrer"] {
		t.Errorf("expected referrer %q got %q", payload["referrer"], pv.Referrer)
	}

	ip := "1.2.3.4"
	sum := sha256.Sum256([]byte(ip))
	expectedHash := fmt.Sprintf("%x", sum[:])
	if pv.IPHash != expectedHash {
		t.Errorf("expected ip hash %q got %q", expectedHash, pv.IPHash)
	}
}

func TestTrack_BadRequest(t *testing.T) {
	h, cleanup := newTestHandler(t)
	defer cleanup()

	e := echo.New()

	req := httptest.NewRequest(http.MethodPost, "/event", bytes.NewReader([]byte("not-json")))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req.RemoteAddr = "1.2.3.4:5678"

	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.Track(c); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400 got %d body=%s", rec.Code, rec.Body.String())
	}

	var resp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid json response: %v", err)
	}
	if resp["error"] == "" {
		t.Fatalf("expected error message in response, got: %v", resp)
	}
}
