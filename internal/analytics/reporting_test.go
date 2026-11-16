package analytics_test

import (
	"context"
	"fmt"
	"github.com/webbesoft/doorman/internal/analytics"
	"github.com/webbesoft/doorman/internal/models"
	"github.com/webbesoft/doorman/internal/testutil"
	"testing"
	"time"
)

func TestReportsUseConsistentDatesAndFractionalAverages(t *testing.T) {
	db := testutil.Open(t)
	now := time.Date(2026, 11, 16, 23, 0, 0, 0, time.UTC)
	for i, view := range []models.PageView{
		{Day: "2026-11-16", DwellTime: 1, ScrollDepth: 10, VisitorKey: "visitor", Referrer: "search.example", Country: "Zimbabwe"},
		{Day: "2026-11-16", DwellTime: 2, ScrollDepth: 11, VisitorKey: "visitor", Referrer: "search.example", Country: "Zimbabwe"},
		{Day: "2026-11-10", DwellTime: 3, ScrollDepth: 12, VisitorKey: "visitor-previous-day"},
		{Day: "2026-11-09", DwellTime: 10000, ScrollDepth: 100, VisitorKey: "outside-window"},
		{Day: "2026-11-16", DwellTime: 10000, ScrollDepth: 100, VisitorKey: "bot", IsBot: true},
	} {
		view.ID = fmt.Sprint(i)
		view.URL = "https://example.com/page"
		view.SiteHost = "example.com"
		if err := db.Create(&view).Error; err != nil {
			t.Fatal(err)
		}
	}
	s, err := analytics.NewReports(db).Snapshot(context.Background(), now)
	if err != nil {
		t.Fatal(err)
	}
	if s.Metrics.TotalPageVisits != 3 || s.Metrics.UniqueVisitors != 2 || s.Metrics.AvgDwellTime != 2 {
		t.Fatalf("metrics=%+v", s.Metrics)
	}
	if len(s.DailyStats) != 7 || s.DailyStats[1].PageVisits != 0 || s.DailyStats[6].PageVisits != 2 {
		t.Fatalf("days=%+v", s.DailyStats)
	}
	if s.TopPages[0].AvgScroll != 11 || s.TopPages[0].AvgDwellTime != 2 {
		t.Fatal(s.TopPages)
	}
	// Restrict to fractional averages to reproduce the previous integer-scan failure.
	db.Where("day != ?", "2026-11-16").Delete(&models.PageView{})
	s, err = analytics.NewReports(db).Snapshot(context.Background(), now)
	if err != nil {
		t.Fatal(err)
	}
	if s.TopPages[0].AvgDwellTime != 1.5 || s.TopPages[0].AvgScroll != 10.5 {
		t.Fatal(s.TopPages)
	}
	sqlDB, _ := db.DB()
	sqlDB.Close()
	if _, err := analytics.NewReports(db).Snapshot(context.Background(), now); err == nil {
		t.Fatal("database failure rendered as an empty report")
	}
}
