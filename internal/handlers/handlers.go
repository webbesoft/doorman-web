package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"gorm.io/gorm"

	"github.com/webbesoft/doorman/internal/models"
	"github.com/webbesoft/doorman/internal/services"
	"github.com/webbesoft/doorman/internal/types"
	"github.com/webbesoft/doorman/templates/pages"
)

type Handler struct {
	DB *gorm.DB
}

var botPatterns = []string{
	"bot", "crawler", "spider", "scraper", "scraping",
	"googlebot", "bingbot", "ahrefsbot", "semrushbot",
	"gptbot", "claude-web", "chatgpt-user", "anthropic-ai",
	"pingdom", "uptime", "monitor", "check",
	"facebookexternalhit", "twitterbot", "linkedinbot",
	"slackbot", "discordbot", "telegrambot",
	"python-requests", "go-http-client", "curl", "wget",
}

type TrackRequest struct {
	URL         string `json:"url"`
	Referrer    string `json:"referrer"`
	DwellTime   int    `json:"dwellTime"`
	ActiveTime  int    `json:"activeTime"`
	ScrollDepth int    `json:"scrollDepth"`
	Final       bool   `json:"final"`
}


// Track handles incoming analytics data
func (h *Handler) Track(c echo.Context) error {
	var req TrackRequest

	body, err := io.ReadAll(c.Request().Body)
	if err != nil {
		c.Logger().Errorf("Failed to read body: %v", err)
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
	}

	if len(body) == 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Empty request body"})
	}

	if err := json.Unmarshal(body, &req); err != nil {
		var inner string
		if err2 := json.Unmarshal(body, &inner); err2 == nil {
			if err3 := json.Unmarshal([]byte(inner), &req); err3 != nil {
				c.Logger().Errorf("Failed to parse inner JSON: %v", err3)
				return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid JSON"})
			}
		} else {
			trimmed := strings.TrimSpace(string(body))
			if strings.HasPrefix(trimmed, "{") {
				if err4 := json.Unmarshal([]byte(trimmed), &req); err4 != nil {
					c.Logger().Errorf("Failed to unmarshal trimmed JSON: %v", err4)
					return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid JSON"})
				}
			} else {
				c.Logger().Errorf("Failed to parse JSON: %v", err)
				return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid JSON"})
			}
		}
	}

	// Hash IP for GDPR compliance (no personal data stored)
	ip := c.RealIP()
	hasher := sha256.New()
	hasher.Write([]byte(ip))
	ipHash := fmt.Sprintf("%x", hasher.Sum(nil))

	c.Logger().Debugf("Processing request from IP hash: %s", ipHash[:8]+"...")

	userAgent := c.Request().UserAgent()
	isBotUA := isBot(userAgent)

	geoService := services.NewGeoService(h.DB)
	geo := geoService.GetGeoDataCached(ip, ipHash)

	var existingAnalytic models.Analytics
	err = h.DB.
		Where("ip_hash = ? AND url = ?", ipHash, req.URL).
		First(&existingAnalytic).Error

	var analytic models.Analytics

	if errors.Is(err, gorm.ErrRecordNotFound) {
		analytic = models.Analytics{
			IPHash:    ipHash,
			URL:       req.URL,
			Country:   geo.Country,
			Region:    geo.RegionName,
			City:      geo.City,
			UserAgent: userAgent,
			Referrer:  req.Referrer,
			IsBot:     isBotUA,
			CreatedAt: time.Now(),
		}

		if err := h.DB.Create(&analytic).Error; err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to save analytics"})
		}
	} else if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Database error"})
	} else {
		analytic = existingAnalytic
	}

	// Create page visit record
	fmt.Printf("Analytic ID: %d, URL: %s, IPHash: %s\n", analytic.ID, req.URL, ipHash)
	var pv models.PageVisit
	pvErr := h.DB.
		Where("ip_hash = ? AND url = ? AND analytics_id = ?", ipHash, req.URL, analytic.ID).
		Order("created_at DESC").
		First(&pv).Error

	if errors.Is(pvErr, gorm.ErrRecordNotFound) {
		// Create new page visit for first heartbeat
		pv = models.PageVisit{
			IPHash:      ipHash,
			URL:         req.URL,
			AnalyticsID: analytic.ID,
			DwellTime:   req.DwellTime,
			ActiveTime:  req.ActiveTime,
			ScrollDepth: req.ScrollDepth,
			CreatedAt:   time.Now(),
		}

		if err := h.DB.Create(&pv).Error; err != nil {
			c.Logger().Errorf("Failed to create page visit: %v", err)
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to save page visit"})
		}
	} else if pvErr != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Database error"})
	} else {
		pv.DwellTime = req.DwellTime
		pv.ActiveTime = req.ActiveTime
		pv.ScrollDepth = req.ScrollDepth
		pv.UpdatedAt = time.Now()

		if err := h.DB.Save(&pv).Error; err != nil {
			c.Logger().Errorf("Failed to update page visit: %v", err)
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to update page visit"})
		}
	}

	// Calculate bot score if not a bot UA and has dwell time
	if !isBotUA && req.DwellTime > 0 {
		score, reason := calculateBotiness(&pv)
		analytic.BotScore = score
		analytic.BotReason = reason

		// Update analytic if high bot score
		if score > 50 {
			analytic.IsBot = true
			analytic.BotScore = score
			analytic.BotReason = reason
			if err := h.DB.Save(&analytic).Error; err != nil {
				c.Logger().Warnf("Failed to update analytics bot score: %v", err)
			}
		}
	}

	return c.NoContent(http.StatusNoContent)
}

// Dashboard renders the analytics dashboard
func (h *Handler) Dashboard(c echo.Context) error {
	metrics := h.getOverallMetrics()

	topPages := h.getTopPages()

	topReferrers := h.getTopReferrers()

	dailyStats := h.getDailyStats()

	topCountries := h.getTopCountries()

	return pages.DashboardPage(
		topReferrers,
		topPages,
		dailyStats,
		topCountries,
		metrics,
	).Render(context.Background(), c.Response().Writer)
}

func (h *Handler) getOverallMetrics() types.DashboardMetrics {
	var metrics types.DashboardMetrics

	h.DB.Model(&models.PageVisit{}).Count(&metrics.TotalPageVisits)

	h.DB.Model(&models.Analytics{}).Count(&metrics.TotalAnalytics)

	h.DB.Model(&models.Analytics{}).
		Distinct("ip_hash").
		Count(&metrics.UniqueVisitors)

	var avgMetrics struct {
		AvgDwellTime   float64
		AvgScrollDepth float64
	}
	h.DB.Model(&models.PageVisit{}).
		Select("AVG(dwell_time) as avg_dwell_time, AVG(scroll_depth) as avg_scroll_depth").
		Where("dwell_time > 0").
		Scan(&avgMetrics)

	metrics.AvgDwellTime = avgMetrics.AvgDwellTime
	metrics.AvgScrollDepth = avgMetrics.AvgScrollDepth

	// Bot percentage
	var botCount int64
	h.DB.Model(&models.Analytics{}).
		Where("is_bot = ?", true).
		Count(&botCount)

	if metrics.TotalAnalytics > 0 {
		metrics.BotPercentage = float64(botCount) / float64(metrics.TotalAnalytics) * 100
	}

	return metrics
}

func (h *Handler) getTopPages() []types.TopPage {
	var topPages []types.TopPage

	h.DB.Model(&models.PageVisit{}).
		Select(`
			url,
			COUNT(*) as visits,
			AVG(dwell_time) as avg_dwell_time,
			AVG(scroll_depth) as avg_scroll
		`).
		Where("url != ''").
		Group("url").
		Order("visits DESC").
		Limit(10).
		Scan(&topPages)

	return topPages
}

func (h *Handler) getTopReferrers() []types.TopReferrer {
	var topReferrers []types.TopReferrer

	h.DB.Model(&models.Analytics{}).
		Select("COALESCE(NULLIF(referrer, ''), 'Direct') as referrer, COUNT(*) as count").
		Group("referrer").
		Order("count DESC").
		Limit(10).
		Scan(&topReferrers)

	return topReferrers
}

func (h *Handler) getDailyStats() []types.DailyStats {
	var dailyStats []types.DailyStats

	h.DB.Raw(`
		SELECT 
			DATE(pv.created_at) as date,
			COUNT(DISTINCT pv.id) as page_visits,
			COUNT(DISTINCT a.ip_hash) as unique_users,
			AVG(pv.dwell_time) as avg_dwell_time
		FROM page_visits pv
		JOIN analytics a ON pv.analytics_id = a.id
		WHERE pv.created_at >= ?
		GROUP BY DATE(pv.created_at)
		ORDER BY date ASC
	`, time.Now().AddDate(0, 0, -7)).Scan(&dailyStats)

	return dailyStats
}

func (h *Handler) getTopCountries() []types.CountryStats {
	var topCountries []types.CountryStats

	h.DB.Model(&models.Analytics{}).
		Select("COALESCE(NULLIF(country, ''), 'Unknown') as country, COUNT(*) as count").
		Group("country").
		Order("count DESC").
		Limit(10).
		Scan(&topCountries)

	return topCountries
}

func isBot(userAgent string) bool {
	ua := strings.ToLower(userAgent)
	for _, pattern := range botPatterns {
		if strings.Contains(ua, strings.ToLower(pattern)) {
			return true
		}
	}

	// empty ua likely bot
	return ua == ""
}

func calculateBotiness(pv *models.PageVisit) (int, string) {
	score := 0
	reasons := []string{}

	if pv.DwellTime > 0 && pv.ActiveTime == 0 {
		score += 30
		reasons = append(reasons, "no_active_time")
	}

	// instant scroll to bottom
	if pv.ScrollDepth == 100 && pv.ActiveTime < 2 {
		score += 20
		reasons = append(reasons, "instant_scroll")
	}

	// speed reader
	if pv.ScrollDepth > 80 && pv.DwellTime < 3 {
		score += 25
		reasons = append(reasons, "too_fast")
	}

	// no scroll
	if pv.ScrollDepth == 0 && pv.DwellTime > 10 {
		score += 15
		reasons = append(reasons, "no_scroll_long_dwell")
	}

	reasonStr := strings.Join(reasons, ",")
	return score, reasonStr
}
