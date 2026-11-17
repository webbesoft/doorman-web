package services

import (
	"context"
	"github.com/webbesoft/doorman/internal/database"
	"github.com/webbesoft/doorman/internal/models"
	"github.com/webbesoft/doorman/internal/observability"
	"gorm.io/gorm"
	"log/slog"
	"time"
)

func StartCleanupRoutine(ctx context.Context, db *gorm.DB, retentionDays int, metrics *observability.Metrics) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		job, cancel := context.WithTimeout(ctx, 30*time.Second)
		err := database.CleanupOldData(db.WithContext(job), retentionDays)
		if err == nil {
			err = db.WithContext(job).Where("expires_at < ? OR last_seen_at < ?", time.Now().UTC(), time.Now().UTC().Add(-30*time.Minute)).Delete(&models.AuthSession{}).Error
		}
		metrics.Cleanup(err)
		cancel()
		if err != nil && ctx.Err() == nil {
			slog.Error("maintenance failed", "error_class", observability.ErrorClass(err))
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
