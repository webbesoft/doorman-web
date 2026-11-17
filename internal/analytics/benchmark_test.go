package analytics_test

import (
	"context"
	"fmt"
	"github.com/webbesoft/doorman/internal/analytics"
	"github.com/webbesoft/doorman/internal/models"
	"github.com/webbesoft/doorman/internal/testutil"
	"strings"
	"testing"
	"time"
)

func BenchmarkRecord(b *testing.B) {
	db := testutil.Open(b)
	tracker := analytics.NewTracker(db, strings.Repeat("k", 32), nil, nil, 1000000)
	ctx := context.Background()
	i := 0
	for b.Loop() {
		i++
		if err := tracker.Record(ctx, event(i), "8.8.8.8", "Mozilla/5.0"); err != nil {
			b.Fatal(err)
		}
	}
}
func BenchmarkReportTenThousandViews(b *testing.B) {
	db := testutil.Open(b)
	now := time.Now().UTC()
	views := make([]models.PageView, 10000)
	for i := range views {
		views[i] = models.PageView{ID: fmt.Sprint(i), SiteHost: "example.com", Day: now.AddDate(0, 0, -i%7).Format("2006-01-02"), URL: fmt.Sprintf("https://example.com/%d", i%50), VisitorKey: fmt.Sprint(i % 1000), DwellTime: int64(i % 120), ScrollDepth: i % 100}
	}
	if err := db.CreateInBatches(views, 500).Error; err != nil {
		b.Fatal(err)
	}
	reports := analytics.NewReports(db)
	ctx := context.Background()
	for b.Loop() {
		if _, err := reports.Snapshot(ctx, now); err != nil {
			b.Fatal(err)
		}
	}
}
