package handlers

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"time"

	"github.com/labstack/echo-contrib/session"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"

	"github.com/webbesoft/doorman/models"
	"github.com/webbesoft/doorman/templates/pages"
)

type Handler struct {
	DB *gorm.DB
}

// Track handles incoming analytics data
func (h *Handler) Track(c echo.Context) error {
	var req struct {
		URL      string `json:"url"`
		Referrer string `json:"referrer"`
	}

	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request"})
	}

	// Hash IP for GDPR compliance (no personal data stored)
	ip := c.RealIP()
	hasher := sha256.New()
	hasher.Write([]byte(ip))
	ipHash := fmt.Sprintf("%x", hasher.Sum(nil))

	pageView := models.PageView{
		URL:       req.URL,
		Referrer:  req.Referrer,
		UserAgent: c.Request().UserAgent(),
		IPHash:    ipHash,
		CreatedAt: time.Now(),
	}

	if err := h.DB.Create(&pageView).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to save data"})
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})

}

// LoginPage renders the login page
func (h *Handler) LoginPage(c echo.Context) error {
	return pages.LoginPage().Render(context.Background(), c.Response().Writer)
}

// Login handles user authentication
func (h *Handler) Login(c echo.Context) error {
	username := c.FormValue("username")
	password := c.FormValue("password")

	var user models.User
	if err := h.DB.Where("username = ?", username).First(&user).Error; err != nil {
		return c.Redirect(http.StatusFound, "/login?error=invalid")
	}

	if !models.CheckPasswordHash(password, user.Password) {
		return c.Redirect(http.StatusFound, "/login?error=invalid")
	}

	sess, _ := session.Get("session", c)
	sess.Values["user_id"] = user.ID
	sess.Save(c.Request(), c.Response())

	return c.Redirect(http.StatusFound, "/dashboard")
}

// Logout handles user logout
func (h *Handler) Logout(c echo.Context) error {
	sess, _ := session.Get("session", c)
	sess.Values = make(map[interface{}]interface{})
	sess.Save(c.Request(), c.Response())
	return c.Redirect(http.StatusFound, "/login")
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
