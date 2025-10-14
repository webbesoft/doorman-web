package handlers

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/webbesoft/doorman/internal/models"
	"github.com/webbesoft/doorman/internal/services"
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

	fmt.Println(req)

	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request"})
	}

	// Hash IP for GDPR compliance (no personal data stored)
	ip := c.RealIP()
	hasher := sha256.New()
	hasher.Write([]byte(ip))
	ipHash := fmt.Sprintf("%x", hasher.Sum(nil))

	// TODO: add rate limiting per IP

	userAgent := c.Request().UserAgent()

	isBotUA := isBot(userAgent)

	pageView := models.PageView{
		URL:       req.URL,
		Referrer:  req.Referrer,
		UserAgent: userAgent,
		IPHash:    ipHash,
		DwellTime: req.DwellTime,
		ActiveTime: req.ActiveTime,
		ScrollDepth: req.ScrollDepth,
		CreatedAt: time.Now(),
		IsBot: isBotUA,
	}

	// calculate bot-iness
	if !isBotUA && req.DwellTime > 0 {
		score, reason := calculateBotiness(&pageView)
		pageView.BotScore = score
		pageView.BotReason = reason

		// flag high bot likelihood score
		if score > 50 {
			pageView.IsBot = true
		}
	}

	geoService := services.NewGeoService(h.DB)
	geo := geoService.GetGeoDataCached(ip, ipHash)
	if geo != nil {

	}

	if err := h.DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "url"}, {Name: "ip_hash"}},
		UpdateAll: true,
	}).Create(&pageView).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to save data"})
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// Dashboard renders the analytics dashboard
func (h *Handler) Dashboard(c echo.Context) error {
	// Get total page views
	var totalViews int64
	h.DB.Model(&models.PageView{}).Count(&totalViews)

	// Get unique visitors (unique IP hashes)
	var uniqueVisitors int64
	h.DB.Model(&models.PageView{}).Distinct("ip_hash").Count(&uniqueVisitors)

	// Get top pages
	var topPages []struct {
		URL   string
		Count int64
	}
	h.DB.Model(&models.PageView{}).
		Select("url, COUNT(*) as count").
		Group("url").
		Order("count DESC").
		Limit(10).
		Scan(&topPages)

	// Get top referrers
	var topReferrers []struct {
		Referrer string
		Count    int64
	}
	h.DB.Model(&models.PageView{}).
		Select("referrer, COUNT(*) as count").
		Where("referrer != ''").
		Group("referrer").
		Order("count DESC").
		Limit(10).
		Scan(&topReferrers)

	// Get recent views for chart (last 7 days)
	var dailyViews []struct {
		Date  string
		Count int64
	}
	h.DB.Model(&models.PageView{}).
		Select("DATE(created_at) as date, COUNT(*) as count").
		Where("created_at >= ?", time.Now().AddDate(0, 0, -7)).
		Group("DATE(created_at)").
		Order("date").
		Scan(&dailyViews)

	return pages.DashboardPage(topReferrers, topPages, dailyViews, uniqueVisitors, totalViews).Render(context.Background(), c.Response().Writer)
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

func calculateBotiness(pv *models.PageView) (int, string) {
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
	return  score, reasonStr
}