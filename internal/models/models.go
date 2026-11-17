package models

import (
	"time"

	"golang.org/x/crypto/bcrypt"
)

type Analytics struct {
	ID        uint   `gorm:"primaryKey" json:"id"`
	URL       string `gorm:"not null;index" json:"url"`
	Referrer  string `json:"referrer"`
	UserAgent string `json:"user_agent"`
	IPHash    string `gorm:"index" json:"-"`

	Country string `gorm:"index" json:"country"`

	IsBot     bool   `gorm:"index;default:false" json:"is_bot"`
	BotScore  int    `json:"bot_score"`
	BotReason string `json:"bot_reason,omitempty"`

	PageVisits []PageVisit `gorm:"foreignKey:AnalyticsID"`

	CreatedAt time.Time `gorm:"index" json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type PageVisit struct {
	ID          uint   `gorm:"primaryKey"`
	AnalyticsID uint   `gorm:"index"`
	URL         string `gorm:"index"`

	IPHash string `gorm:"index" json:"-"`

	DwellTime   int `json:"dwell_time"`
	ActiveTime  int `json:"active_time"`
	ScrollDepth int `json:"scroll_depth"`

	CreatedAt time.Time `gorm:"index" json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type PageAnalytics struct {
	URL            string  `json:"url"`
	TotalViews     int64   `json:"total_views"`
	UniqueVisitors int64   `json:"unique_visitors"`
	AvgDwellTime   float64 `json:"avg_dwell_time"`
	AvgActiveTime  float64 `json:"avg_active_time"`
	AvgScrollDepth float64 `json:"avg_scroll_depth"`
	BounceRate     float64 `json:"bounce_rate"`
	EngagementRate float64 `json:"engagement_rate"`
}

type User struct {
	ID        uint   `gorm:"primaryKey"`
	Username  string `gorm:"unique;not null"`
	Password  string `gorm:"not null"`
	CreatedAt time.Time
}

func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), 14)
	return string(bytes), err
}

func CheckPasswordHash(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

// AuthSession stores a keyed hash of an opaque browser token, never the token itself.
type AuthSession struct {
	TokenHash  string    `gorm:"primaryKey;size:64"`
	UserID     uint      `gorm:"not null;index"`
	User       User      `gorm:"constraint:OnDelete:CASCADE"`
	ExpiresAt  time.Time `gorm:"index;not null"`
	LastSeenAt time.Time `gorm:"not null"`
}

// PageView represents one navigation. Heartbeats update this record atomically.
type PageView struct {
	ID          string    `gorm:"primaryKey;size:64"`
	SiteHost    string    `gorm:"size:253;not null;index:idx_page_view_site_day,priority:1"`
	Day         string    `gorm:"size:10;not null;index;index:idx_page_view_site_day,priority:2;index:idx_page_view_human_day,priority:2"`
	VisitorKey  string    `gorm:"size:64;not null;index"`
	URL         string    `gorm:"size:2048;not null"`
	Referrer    string    `gorm:"size:253"`
	Country     string    `gorm:"size:128"`
	DwellTime   int64     `gorm:"not null"`
	ActiveTime  int64     `gorm:"not null"`
	ScrollDepth int       `gorm:"not null"`
	Sequence    int64     `gorm:"not null"`
	Final       bool      `gorm:"not null"`
	IsBot       bool      `gorm:"not null;index;index:idx_page_view_human_day,priority:1"`
	Legacy      bool      `gorm:"not null"`
	CreatedAt   time.Time `gorm:"index"`
	UpdatedAt   time.Time
}

// StorageBudget serializes capacity reservations across application instances.
type StorageBudget struct {
	ID            uint  `gorm:"primaryKey"`
	RetainedViews int64 `gorm:"not null"`
}
