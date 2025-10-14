package services

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/webbesoft/doorman/internal/models"
)

// helper to create an in-memory DB
func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)

	err = db.AutoMigrate(&models.PageView{})
	assert.NoError(t, err)
	return db
}

// mock server for ip-api.com
func newMockGeoServer(status, country, region, city string) *httptest.Server {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := GeoData{
			Status:     status,
			Country:    country,
			RegionName: region,
			City:       city,
		}
		_ = json.NewEncoder(w).Encode(resp)
	})
	return httptest.NewServer(handler)
}

// Override getGeoData in tests (use package-level variable)
var originalGetGeoData = getGeoData

func restoreGetGeoData() { getGeoData = originalGetGeoData }

// --- TESTS ---

func TestGeoService_CacheHit(t *testing.T) {
	db := setupTestDB(t)
	service := NewGeoService(db)

	ip := "8.8.8.8"
	ipHash := "hash-8888"

	// Prepopulate cache
	service.geoCache[ipHash] = &GeoData{
		Country:    "USA",
		RegionName: "CA",
		City:       "Mountain View",
	}

	geo := service.GetGeoDataCached(ip, ipHash)
	assert.NotNil(t, geo)
	assert.Equal(t, "USA", geo.Country)
}

func TestGeoService_FromDatabase(t *testing.T) {
	db := setupTestDB(t)
	service := NewGeoService(db)

	ip := "1.1.1.1"
	ipHash := "hash-1111"

	// Insert existing PageView record
	db.Create(&models.PageView{
		IPHash:    ipHash,
		Country:   "Germany",
		CreatedAt: time.Now(),
	})

	geo := service.GetGeoDataCached(ip, ipHash)
	assert.NotNil(t, geo)
	assert.Equal(t, "Germany", geo.Country)

	// Should be cached now
	geo2 := service.GetGeoDataCached(ip, ipHash)
	assert.Equal(t, geo, geo2)
}

func TestGeoService_FromAPI(t *testing.T) {
	db := setupTestDB(t)
	service := NewGeoService(db)

	server := newMockGeoServer("success", "France", "Île-de-France", "Paris")
	defer server.Close()

	// Temporarily override getGeoData to use mock server
	service.getGeoFunc = func(ip string) (*GeoData, error) {
		resp, err := http.Get(server.URL)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		var geo GeoData
		_ = json.NewDecoder(resp.Body).Decode(&geo)
		return &geo, nil
	}
	defer restoreGetGeoData()

	ip := "2.2.2.2"
	ipHash := "hash-2222"

	geo := service.GetGeoDataCached(ip, ipHash)
	assert.NotNil(t, geo)
	assert.Equal(t, "France", geo.Country)
	assert.Equal(t, "Paris", geo.City)
}

func TestGeoService_APIError(t *testing.T) {
	db := setupTestDB(t)
	service := NewGeoService(db)

	server := newMockGeoServer("fail", "", "", "")
	defer server.Close()

	service.getGeoFunc = func(ip string) (*GeoData, error) {
		resp, err := http.Get(server.URL)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		var geo GeoData
		_ = json.NewDecoder(resp.Body).Decode(&geo)
		if geo.Status != "success" {
			return nil, fmt.Errorf("geo lookup failed")
		}
		return &geo, nil
	}
	defer restoreGetGeoData()

	ip := "3.3.3.3"
	ipHash := "hash-3333"

	geo := service.GetGeoDataCached(ip, ipHash)
	assert.Nil(t, geo)
}

func TestGeoService_LocalIP(t *testing.T) {
	db := setupTestDB(t)
	service := NewGeoService(db)

	ip := "127.0.0.1"
	ipHash := "local-127"

	service.getGeoFunc = func(ip string) (*GeoData, error) {
		return nil, fmt.Errorf("local IPs cannot be geolocated")
	}
	defer restoreGetGeoData()

	geo := service.GetGeoDataCached(ip, ipHash)
	assert.Nil(t, geo, "local IPs should not return geo data")
}
