package database

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/webbesoft/doorman/internal/analytics"
	"github.com/webbesoft/doorman/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type schemaMigration struct {
	Version   int `gorm:"primaryKey"`
	AppliedAt time.Time
}

var migrations = []func(*gorm.DB, string) error{
	func(db *gorm.DB, _ string) error {
		return db.AutoMigrate(&models.Analytics{}, &models.PageVisit{}, &models.User{}, &models.AuthSession{})
	},
	importLegacyViews,
}

// Migrate is an explicit deployment step. Only unapplied, versioned migrations run.
func Migrate(ctx context.Context, db *gorm.DB, secret string) error {
	db = db.WithContext(ctx)
	if err := db.AutoMigrate(&schemaMigration{}); err != nil {
		return err
	}
	for i, apply := range migrations {
		version := i + 1
		var count int64
		if err := db.Model(&schemaMigration{}).Where("version = ?", version).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			continue
		}
		if err := db.Transaction(func(tx *gorm.DB) error {
			if err := apply(tx, secret); err != nil {
				return err
			}
			return tx.Create(&schemaMigration{Version: version, AppliedAt: time.Now().UTC()}).Error
		}); err != nil {
			return fmt.Errorf("migration %d failed: %w", version, err)
		}
	}
	return nil
}
func CheckSchema(db *gorm.DB) error {
	var latest int
	if err := db.Model(&schemaMigration{}).Select("COALESCE(MAX(version), 0)").Scan(&latest).Error; err != nil || latest != len(migrations) {
		return errors.New("database schema requires migration; run doorman -migrate before starting")
	}
	return nil
}
func ResetAdminPassword(ctx context.Context, db *gorm.DB, username, password string) error {
	if len(password) < 12 || len(password) > 72 {
		return errors.New("new password must contain 12 to 72 bytes")
	}
	hash, err := models.HashPassword(password)
	if err != nil {
		return err
	}
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var user models.User
		if err := tx.Where("username = ?", username).First(&user).Error; err != nil {
			return err
		}
		if err := tx.Model(&user).Update("password", hash).Error; err != nil {
			return err
		}
		return tx.Where("user_id = ?", user.ID).Delete(&models.AuthSession{}).Error
	})
}

// Legacy rows are copied without modifying the original tables. Previously lost visits remain unrecoverable.
func importLegacyViews(db *gorm.DB, secret string) error {
	if err := db.AutoMigrate(&models.PageView{}); err != nil {
		return err
	}
	type legacyRow struct {
		ID                             uint
		URL, IPHash, Referrer, Country string
		DwellTime, ActiveTime          int64
		ScrollDepth                    int
		IsBot                          bool
		CreatedAt, UpdatedAt           time.Time
	}
	var after uint
	for {
		var rows []legacyRow
		if err := db.Table("page_visits AS pv").Select("pv.id, pv.url, pv.ip_hash, pv.dwell_time, pv.active_time, pv.scroll_depth, pv.created_at, pv.updated_at, a.referrer, a.country, a.is_bot").Joins("LEFT JOIN analytics a ON a.id = pv.analytics_id").Where("pv.id > ?", after).Order("pv.id").Limit(500).Scan(&rows).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		for _, row := range rows {
			canonical, host, err := analytics.NormalizeURL(row.URL)
			if err != nil {
				raw := "https://legacy.invalid/invalid-url"
				if strings.HasPrefix(row.URL, "/") {
					raw = "https://legacy.invalid" + row.URL
				}
				canonical, host, err = analytics.NormalizeURL(raw)
				if err != nil {
					canonical, host = "https://legacy.invalid/invalid-url", "legacy.invalid"
				}
			}
			created := row.CreatedAt.UTC()
			day := created.Format("2006-01-02")
			view := models.PageView{ID: fmt.Sprintf("legacy-%d", row.ID), SiteHost: host, Day: day, VisitorKey: analytics.VisitorKey([]byte(secret), "legacy:"+host, day, row.IPHash), URL: canonical, Referrer: analytics.ReferrerHost(row.Referrer), Country: row.Country,
				DwellTime: max(0, min(row.DwellTime, 86400)), ScrollDepth: max(0, min(row.ScrollDepth, 100)), IsBot: row.IsBot, Legacy: true, Final: true, Sequence: 1, CreatedAt: created, UpdatedAt: row.UpdatedAt.UTC()}
			view.ActiveTime = max(0, min(row.ActiveTime, view.DwellTime))
			if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&view).Error; err != nil {
				return err
			}
		}
		after = rows[len(rows)-1].ID
	}
}
