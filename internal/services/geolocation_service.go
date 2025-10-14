package services

import (
	"encoding/json"
	"fmt"
	"net"
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
	DB             *gorm.DB
	geoCache       map[string]*GeoData
	geoCacheMux    sync.RWMutex
	rateLimiter    map[string][]time.Time
	rateLimiterMux sync.RWMutex
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
	if (! isPublicIP(net.ParseIP(ip))) {
		return &GeoData{}
	}
	
	g.geoCacheMux.RLock()
	cached, ok := g.geoCache[ipHash]
	g.geoCacheMux.RUnlock()
	if ok {
		return cached
	}

	// Check database for recent geo data
	var existingView models.Analytics
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

func isPublicIP(ip net.IP) bool {
	if ip == nil {
        return false
    }
    // Loopback, link-local unicast/multicast are not public
    if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
        return false
    }

    // IPv4 private ranges
    if ip4 := ip.To4(); ip4 != nil {
        switch {
        case ip4[0] == 10:
            return false
        case ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31:
            return false
        case ip4[0] == 192 && ip4[1] == 168:
            return false
        case ip4[0] == 169 && ip4[1] == 254: // link-local IPv4
            return false
        default:
            return true
        }
    }

    // IPv6: unique local addresses (fc00::/7) are not public
    if ip.To16() != nil {
        // check first byte for fc00::/7 (0b11111100 => 0xfc)
        if ip[0]&0xfe == 0xfc {
            return false
        }
        // fe80::/10 is link-local (IsLinkLocalUnicast already handled)
        return true
    }

    return false
}