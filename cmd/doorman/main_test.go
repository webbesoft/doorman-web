package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/webbesoft/doorman/internal/auth"
	"github.com/webbesoft/doorman/internal/config"
	"github.com/webbesoft/doorman/internal/models"
	"github.com/webbesoft/doorman/internal/observability"
	"github.com/webbesoft/doorman/internal/testutil"
	"golang.org/x/crypto/bcrypt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestHTTPAuthenticationAndCollectorPolicies(t *testing.T) {
	db := testutil.Open(t)
	hash, _ := bcrypt.GenerateFromPassword([]byte("test-password"), bcrypt.MinCost)
	user := models.User{Username: "owner", Password: string(hash)}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	e := newServer(config.Config{Environment: "production", SessionSecret: strings.Repeat("k", 32), AllowedHosts: []string{"example.com"}, MaxPageViews: 1000}, db, &observability.Metrics{})
	request := func(method, path, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "https://analytics.example"+path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if cookie != nil {
			req.AddCookie(cookie)
		}
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		return rec
	}
	if rec := request("GET", "/dashboard", "", nil); rec.Code != 303 {
		t.Fatal("dashboard exposed", rec.Code)
	}
	if rec := request("GET", "/metrics", "", nil); rec.Code != 303 {
		t.Fatal("metrics exposed", rec.Code)
	}
	form := url.Values{"username": {"owner"}, "password": {"test-password"}}.Encode()
	bad := httptest.NewRequest("POST", "https://analytics.example/login", strings.NewReader(form))
	bad.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	bad.Header.Set("Sec-Fetch-Site", "cross-site")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, bad)
	if rec.Code != 403 {
		t.Fatal("cross-site login accepted", rec.Code)
	}
	login := request("POST", "/login", form, nil)
	if login.Code != 303 {
		t.Fatal(login.Code, login.Body)
	}
	cookies := login.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatal("missing session")
	}
	cookie := cookies[0]
	if cookie.Name != auth.CookieName || !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteStrictMode {
		t.Fatal("unsafe cookie")
	}
	if rec := request("GET", "/dashboard", "", cookie); rec.Code != 200 || rec.Header().Get("Cache-Control") != "no-store" || rec.Header().Get("Content-Security-Policy") == "" {
		t.Fatal("dashboard response", rec.Code)
	}
	if rec := request("POST", "/logout", "", cookie); rec.Code != 303 {
		t.Fatal(rec.Code)
	}
	if rec := request("GET", "/dashboard", "", cookie); rec.Code != 303 {
		t.Fatal("logout token replay accepted")
	}
	// Login limiter uses the direct peer, so spoofed XFF cannot reset its bucket.
	for i := 0; i < 6; i++ {
		req := httptest.NewRequest("POST", "https://analytics.example/login", strings.NewReader("username=unknown&password=bad"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("X-Forwarded-For", string(rune('a'+i)))
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		if i == 5 && rec.Code != 429 {
			t.Fatal("login limiter bypassed", rec.Code)
		}
	}
	event := map[string]any{"visitId": "123456781234123412341234567890ab", "sequence": 1, "url": "https://example.com/page"}
	body, _ := json.Marshal(event)
	req := httptest.NewRequest("POST", "https://analytics.example/event", bytes.NewReader(body))
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("Origin", "https://example.com")
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != 204 || rec.Header().Get("Access-Control-Allow-Origin") == "" {
		t.Fatal("collector failed", rec.Code, rec.Body)
	}
	for _, path := range []string{"/livez", "/readyz", "/assets/js/t.js"} {
		if rec := request("GET", path, "", nil); rec.Code != 200 {
			t.Fatal(path, rec.Code)
		}
	}
}

func TestMetricsBearerAndCollectorOrigins(t *testing.T) {
	db := testutil.Open(t)
	token := strings.Repeat("m", 32)
	e := newServer(config.Config{Environment: "production", SessionSecret: strings.Repeat("k", 32), MetricsToken: token, AllowedHosts: []string{"example.com"}, MaxPageViews: 1000}, db, &observability.Metrics{})
	for _, tc := range []struct {
		token string
		want  int
	}{{token, 200}, {"wrong", 401}} {
		req := httptest.NewRequest("GET", "https://analytics.example/metrics", nil)
		req.Header.Set("Authorization", "Bearer "+tc.token)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		if rec.Code != tc.want {
			t.Fatalf("metrics status=%d", rec.Code)
		}
		if tc.want == 200 && !strings.Contains(rec.Body.String(), "doorman_max_page_views 1000") {
			t.Fatal("capacity gauge missing")
		}
	}
	req := httptest.NewRequest("POST", "https://analytics.example/event", strings.NewReader(`{}`))
	req.Header.Set("Origin", "https://evil.example")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != 403 {
		t.Fatal("unconfigured origin accepted", rec.Code)
	}
	req = httptest.NewRequest("OPTIONS", "https://analytics.example/event", nil)
	req.Header.Set("Origin", "https://example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "Content-Type")
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != 204 || rec.Header().Get("Access-Control-Allow-Origin") != "https://example.com" {
		t.Fatal("preflight failed", rec.Code)
	}
}

func TestStartupReclaimsExpiredImportedCapacity(t *testing.T) {
	db := testutil.Open(t)
	old := time.Now().UTC().AddDate(0, 0, -100)
	if err := db.Create(&models.PageView{ID: "legacy-1", URL: "https://example.com/old", Day: old.Format("2006-01-02"), CreatedAt: old, Legacy: true}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&models.StorageBudget{}).Where("id = ?", 1).Update("retained_views", 1).Error; err != nil {
		t.Fatal(err)
	}
	e, err := prepareServer(context.Background(), config.Config{Environment: "production", SessionSecret: strings.Repeat("k", 32), AllowedHosts: []string{"example.com"}, MaxPageViews: 1, RetentionDays: 90}, db, &observability.Metrics{})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("POST", "/event", strings.NewReader(`{"visitId":"123456781234123412341234567890ab","sequence":1,"url":"https://example.com/new"}`))
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != 204 {
		t.Fatalf("first event blocked by expired capacity: %d", rec.Code)
	}
	var budget models.StorageBudget
	if err := db.First(&budget, 1).Error; err != nil {
		t.Fatal(err)
	}
	if budget.RetainedViews != 1 {
		t.Fatal("storage limit accounting changed", budget.RetainedViews)
	}
}

func TestTrustedProxySpoofCannotChangeLimiterIdentity(t *testing.T) {
	db := testutil.Open(t)
	_, proxyRange, err := net.ParseCIDR("192.0.2.10/32")
	if err != nil {
		t.Fatal(err)
	}
	e := newServer(config.Config{Environment: "production", SessionSecret: strings.Repeat("k", 32), AllowedHosts: []string{"example.com"}, MaxPageViews: 1000, TrustedProxies: []*net.IPNet{proxyRange}}, db, &observability.Metrics{})
	forwarded := func(spoof, client string) *http.Request {
		inbound := httptest.NewRequest("POST", "https://analytics.example/login", strings.NewReader("username=unknown&password=long-fixture-password"))
		inbound.RemoteAddr = net.JoinHostPort(client, "54321")
		inbound.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		inbound.Header.Set("X-Forwarded-For", spoof)
		// Match Kamal's forward_headers path: preserve the prefix, then append the direct client.
		proxy := &httputil.ProxyRequest{In: inbound, Out: inbound.Clone(inbound.Context())}
		proxy.SetXForwarded()
		proxy.Out.RemoteAddr = "192.0.2.10:8080"
		return proxy.Out
	}
	for i := 0; i < 6; i++ {
		req := forwarded(fmt.Sprintf("203.0.113.%d", i+1), "198.51.100.20")
		if got := e.IPExtractor(req); got != "198.51.100.20" {
			t.Fatalf("spoof selected instead of verified client: %s", got)
		}
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		want := http.StatusSeeOther
		if i == 5 {
			want = http.StatusTooManyRequests
		}
		if rec.Code != want {
			t.Fatalf("request %d: status=%d want=%d", i, rec.Code, want)
		}
	}
	req := forwarded("203.0.113.99", "198.51.100.21")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatal("legitimate client lost its own bucket", rec.Code)
	}
	req = forwarded("203.0.113.99", "198.51.100.20")
	req.RemoteAddr = "198.51.100.200:8080"
	if got := e.IPExtractor(req); got != "198.51.100.200" {
		t.Fatal("untrusted peer can supply client identity", got)
	}
}
