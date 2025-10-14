package models

import (
	"time"

	"golang.org/x/crypto/bcrypt"
)

type PageView struct {
	ID uint `gorm:"primaryKey" json:"id"`
	URL string `gorm:"not null;index:idx_iphash_url,unique" json:"url"`
	Referrer string `json:"referrer"`
	UserAgent string `json:"user_agent"`
	IPHash string `gorm:"index:idx_iphash_url,unique" json:"-"`
	
	DwellTime   int `json:"dwell_time"`
	ActiveTime  int `json:"active_time"`
	ScrollDepth int `json:"scroll_depth"`
	
	Country string `gorm:"index" json:"country"`
	Region  string `json:"region"`
	City    string `json:"city"`

	IsBot bool `gorm:"index;default:false" json:"is_bot"`
	BotScore int `json:"bot_score"`
	BotReason string `json:"bot_reason,omitempty"`

	CreatedAt time.Time `json:"created_at"`
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
	ID uint `gorm:"primaryKey"`
	Username string `gorm:"unique;not null"`
	Password string `gorm:"not null"`
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