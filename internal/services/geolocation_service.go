package services

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
	"golang.org/x/time/rate"
)

type GeoData struct {
	Status  string `json:"status"`
	Country string `json:"country"`
}
type geoEntry struct {
	data    GeoData
	expires time.Time
}
type GeoService struct {
	mu      sync.Mutex
	cache   map[string]geoEntry
	group   singleflight.Group
	limiter *rate.Limiter
	lookup  func(context.Context, string) (GeoData, error)
}

// Geolocation is disabled unless an HTTPS commercial provider key is configured.
func NewGeoService(apiKey string) *GeoService {
	g := &GeoService{cache: make(map[string]geoEntry), limiter: rate.NewLimiter(rate.Every(1500*time.Millisecond), 1)}
	if apiKey != "" {
		client := &http.Client{Timeout: time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		g.lookup = func(ctx context.Context, ip string) (GeoData, error) {
			u := url.URL{Scheme: "https", Host: "pro.ip-api.com", Path: "/json/" + ip}
			q := u.Query()
			q.Set("fields", "status,country")
			q.Set("key", apiKey)
			u.RawQuery = q.Encode()
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
			if err != nil {
				return GeoData{}, err
			}
			resp, err := client.Do(req)
			if err != nil {
				return GeoData{}, errors.New("geolocation unavailable")
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				return GeoData{}, errors.New("geolocation unavailable")
			}
			var data GeoData
			if err := json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&data); err != nil {
				return GeoData{}, err
			}
			if data.Status != "success" || len(data.Country) > 128 {
				return GeoData{}, errors.New("invalid geolocation result")
			}
			return data, nil
		}
	}
	return g
}

// Unknown countries are a normal result and never prevent ingestion.
func (g *GeoService) GetGeoDataCached(ctx context.Context, ip, key string) GeoData {
	parsed := net.ParseIP(ip)
	if g == nil || g.lookup == nil || parsed == nil || !parsed.IsGlobalUnicast() || parsed.IsPrivate() {
		return GeoData{}
	}
	value, _, _ := g.group.Do(key, func() (any, error) {
		now := time.Now()
		g.mu.Lock()
		entry, ok := g.cache[key]
		g.mu.Unlock()
		if ok && entry.expires.After(now) {
			return entry.data, nil
		}
		if !g.limiter.Allow() {
			return GeoData{}, nil
		}
		data, err := g.lookup(ctx, ip)
		ttl := 24 * time.Hour
		if err != nil {
			data = GeoData{}
			ttl = 5 * time.Minute
		}
		g.mu.Lock()
		if len(g.cache) >= 4096 {
			for k, v := range g.cache {
				if !v.expires.After(now) {
					delete(g.cache, k)
				}
			}
		}
		if len(g.cache) < 4096 {
			g.cache[key] = geoEntry{data: data, expires: now.Add(ttl)}
		}
		g.mu.Unlock()
		return data, nil
	})
	return value.(GeoData)
}
