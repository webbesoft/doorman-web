package analytics

import (
	"context"
	"database/sql"
	"time"

	"github.com/webbesoft/doorman/internal/models"
	"github.com/webbesoft/doorman/internal/types"
	"gorm.io/gorm"
)

type Snapshot struct {
	Metrics      types.DashboardMetrics
	TopPages     []types.TopPage
	TopReferrers []types.TopReferrer
	DailyStats   []types.DailyStats
	TopCountries []types.CountryStats
}

type Reports struct{ db *gorm.DB }

func NewReports(db *gorm.DB) *Reports { return &Reports{db: db} }

// Snapshot uses seven UTC calendar days, including today, for every displayed metric.
func (r *Reports) Snapshot(ctx context.Context, now time.Time) (Snapshot, error) {
	now = now.UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	start := today.AddDate(0, 0, -6)
	var s Snapshot
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		query := func() *gorm.DB {
			return tx.Model(&models.PageView{}).Where("day >= ? AND day <= ? AND is_bot = ?", start.Format("2006-01-02"), today.Format("2006-01-02"), false)
		}
		if err := query().Count(&s.Metrics.TotalPageVisits).Error; err != nil {
			return err
		}
		if err := query().Distinct("visitor_key").Count(&s.Metrics.UniqueVisitors).Error; err != nil {
			return err
		}
		var averages struct{ AvgDwellTime, AvgScrollDepth float64 }
		if err := query().Select("COALESCE(AVG(dwell_time),0) AS avg_dwell_time, COALESCE(AVG(scroll_depth),0) AS avg_scroll_depth").Scan(&averages).Error; err != nil {
			return err
		}
		s.Metrics.AvgDwellTime = averages.AvgDwellTime
		s.Metrics.AvgScrollDepth = averages.AvgScrollDepth
		if err := query().Select("url, COUNT(*) AS visits, AVG(dwell_time) AS avg_dwell_time, AVG(scroll_depth) AS avg_scroll").Group("url").Order("visits DESC, url ASC").Limit(10).Scan(&s.TopPages).Error; err != nil {
			return err
		}
		if err := query().Select("COALESCE(NULLIF(referrer,''),'Direct') AS referrer, COUNT(*) AS count").Group("referrer").Order("count DESC, referrer ASC").Limit(10).Scan(&s.TopReferrers).Error; err != nil {
			return err
		}
		if err := query().Select("COALESCE(NULLIF(country,''),'Unknown') AS country, COUNT(*) AS count").Group("country").Order("count DESC, country ASC").Limit(10).Scan(&s.TopCountries).Error; err != nil {
			return err
		}
		var days []types.DailyStats
		if err := query().Select("day AS date, COUNT(*) AS page_visits, COUNT(DISTINCT visitor_key) AS unique_users, AVG(dwell_time) AS avg_dwell_time").Group("day").Order("day ASC").Scan(&days).Error; err != nil {
			return err
		}
		byDate := make(map[string]types.DailyStats, len(days))
		for _, day := range days {
			byDate[day.Date] = day
		}
		for i := 0; i < 7; i++ {
			date := start.AddDate(0, 0, i).Format("2006-01-02")
			day := byDate[date]
			day.Date = date
			s.DailyStats = append(s.DailyStats, day)
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	return s, err
}
