package services

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/webbesoft/doorman/internal/models"
	"gorm.io/gorm"
)

type GeoData struct {
	Status     string `json:"status"`
	Country    string `json:"country"`
	RegionName string `json:"regionName"`
	City       string `json:"city"`
}

type GeoService struct {
	DB              *gorm.DB
	geoCache        map[string]*GeoData
	geoCacheMux     sync.RWMutex
	rateLimiter     map[string][]time.Time
	rateLimiterMux  sync.RWMutex
	getGeoFunc     func(ip string) (*GeoData, error)
}

// NewHandler creates a new handler with initialized caches
func NewGeoService(db *gorm.DB) *GeoService {
	return &GeoService{
		DB:          db,
		geoCache:    make(map[string]*GeoData),
		rateLimiter: make(map[string][]time.Time),
		getGeoFunc:  getGeoData,
	}
}

func getGeoData(ip string) (*GeoData, error) {
	url := fmt.Sprintf("http://ip-api.com/json/%s?fields=status,country,regionName,city", ip)

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var geo GeoData
	if err := json.NewDecoder(resp.Body).Decode(&geo); err != nil {
		return nil, err
	}

	if geo.Status != "success" {
		return nil, fmt.Errorf("geo lookup failed: %s", geo.Status)
	}

	return &geo, nil
}

func (g *GeoService) GetGeoDataCached(ip, ipHash string) *GeoData {
	g.geoCacheMux.RLock()
	cached, ok := g.geoCache[ipHash]
	g.geoCacheMux.RUnlock()
	if ok {
		return cached
	}

	// Check database for recent geo data
	var existingView models.PageView
	err := g.DB.Select("country, region, city").
		Where("ip_hash = ? AND country != '' AND created_at > ?", 
			ipHash, time.Now().Add(-24*time.Hour)).
		First(&existingView).Error

	if err == nil && existingView.Country != "" {
		geo := &GeoData{
			Country:    existingView.Country,
			RegionName: existingView.Region,
			City:       existingView.City,
		}
		
		g.geoCacheMux.Lock()
		g.geoCache[ipHash] = geo
		g.geoCacheMux.Unlock()
		
		return geo
	}

	geo, err := g.getGeoFunc(ip)
	if err != nil {
		return nil
	}

	g.geoCacheMux.Lock()
	g.geoCache[ipHash] = geo
	g.geoCacheMux.Unlock()

	return geo
}