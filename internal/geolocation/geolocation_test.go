package geolocation

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestGeolocationDisabledAndFailuresAreUnknown(t *testing.T) {
	g := NewGeoService("")
	if data := g.GetGeoDataCached(context.Background(), "8.8.8.8", "key"); data.Country != "" {
		t.Fatal(data)
	}
	calls := 0
	g.lookup = func(context.Context, string) (GeoData, error) { calls++; return GeoData{}, errors.New("unavailable") }
	for range 2 {
		if data := g.GetGeoDataCached(context.Background(), "8.8.8.8", "key"); data.Country != "" {
			t.Fatal(data)
		}
	}
	if calls != 1 {
		t.Fatalf("negative result not cached: calls=%d", calls)
	}
}
func TestGeolocationCachesAndSkipsPrivateIPs(t *testing.T) {
	g := NewGeoService("")
	calls := 0
	g.lookup = func(context.Context, string) (GeoData, error) { calls++; return GeoData{Country: "Zimbabwe"}, nil }
	for range 2 {
		if data := g.GetGeoDataCached(context.Background(), "8.8.8.8", "key"); data.Country != "Zimbabwe" {
			t.Fatal(data)
		}
	}
	if calls != 1 {
		t.Fatal("cache missed")
	}
	for _, ip := range []string{"127.0.0.1", "::1", "10.0.0.1", "fc00::1", "invalid"} {
		if data := g.GetGeoDataCached(context.Background(), ip, ip); data.Country != "" {
			t.Fatal(ip)
		}
	}
	if calls != 1 {
		t.Fatal("private IP disclosed")
	}
	g.mu.Lock()
	g.cache["expired"] = geoEntry{data: GeoData{Country: "old"}, expires: time.Now().Add(-time.Minute)}
	g.mu.Unlock()
}
