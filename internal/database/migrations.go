package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/webbesoft/doorman/internal/models"
	"gorm.io/gorm"
)

type schemaMigration struct {
	Version   int `gorm:"primaryKey"`
	AppliedAt time.Time
}

var migrations = []func(*gorm.DB) error{
	func(db *gorm.DB) error {
		return db.AutoMigrate(&models.Analytics{}, &models.PageVisit{}, &models.User{}, &models.AuthSession{})
	},
}

// Migrate is an explicit deployment step. Only unapplied, versioned migrations run.
func Migrate(ctx context.Context, db *gorm.DB) error {
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
			if err := apply(tx); err != nil {
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
