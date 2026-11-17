package handlers

import (
	"bytes"
	"encoding/json"
	"github.com/labstack/echo/v4"
	"github.com/webbesoft/doorman/internal/analytics"
	"github.com/webbesoft/doorman/internal/models"
	"github.com/webbesoft/doorman/internal/testutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTrackValidatesBeforePersisting(t *testing.T) {
	db := testutil.Open(t)
	h := &Handler{Tracker: analytics.NewTracker(db, strings.Repeat("k", 32), []string{"example.com"}, nil, 1000000)}
	e := echo.New()
	e.POST("/event", h.Track)
	valid := analytics.Event{VisitID: "123456781234123412341234567890ab", Sequence: 1, URL: "https://example.com/page?token=secret#private", DwellTime: 5, ActiveTime: 3, ScrollDepth: 40}
	body, _ := json.Marshal(valid)
	req := httptest.NewRequest(http.MethodPost, "/event", bytes.NewReader(body))
	req.Header.Set("User-Agent", "Mozilla/5.0")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != 204 {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body)
	}
	var view models.PageView
	if err := db.First(&view).Error; err != nil {
		t.Fatal(err)
	}
	if view.URL != "https://example.com/page" {
		t.Fatal("sensitive URL stored:", view.URL)
	}
	for _, body := range []string{"null", "not-json", `{}`, string(body) + ` {}`, strings.Repeat("x", 17000)} {
		req := httptest.NewRequest("POST", "/event", strings.NewReader(body))
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		if rec.Code != 400 && rec.Code != 413 {
			t.Fatalf("invalid payload status=%d", rec.Code)
		}
	}
	var count int64
	db.Model(&models.PageView{}).Count(&count)
	if count != 1 {
		t.Fatalf("invalid data persisted: %d", count)
	}
}
