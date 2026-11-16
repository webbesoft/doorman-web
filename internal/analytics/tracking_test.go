package analytics_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/webbesoft/doorman/internal/analytics"
	"github.com/webbesoft/doorman/internal/models"
	"github.com/webbesoft/doorman/internal/testutil"
)

func event(id int) analytics.Event {
	return analytics.Event{VisitID: fmt.Sprintf("%032x", id), Sequence: 1, URL: "https://example.com/page", DwellTime: 120, ActiveTime: 100, ScrollDepth: 90}
}
func TestSeparateNavigationsAndOutOfOrderHeartbeats(t *testing.T) {
	db := testutil.Open(t)
	tracker := analytics.NewTracker(db, strings.Repeat("k", 32), []string{"example.com"}, nil)
	ctx := context.Background()
	for _, id := range []int{1, 2} {
		if err := tracker.Record(ctx, event(id), "8.8.8.8", "Mozilla/5.0"); err != nil {
			t.Fatal(err)
		}
	}
	later := event(1)
	later.Sequence = 3
	later.DwellTime = 150
	later.ActiveTime = 130
	later.ScrollDepth = 100
	later.Final = true
	if err := tracker.Record(ctx, later, "8.8.8.8", "Mozilla/5.0"); err != nil {
		t.Fatal(err)
	}
	stale := event(1)
	stale.Sequence = 2
	stale.DwellTime = 1
	stale.ActiveTime = 0
	stale.ScrollDepth = 0
	for range 2 {
		if err := tracker.Record(ctx, stale, "8.8.8.8", "Mozilla/5.0"); err != nil {
			t.Fatal(err)
		}
	}
	var count int64
	db.Model(&models.PageView{}).Count(&count)
	if count != 2 {
		t.Fatalf("visits=%d", count)
	}
	var view models.PageView
	db.Where("id = ?", later.VisitID).First(&view)
	if view.DwellTime != 150 || view.ActiveTime != 130 || view.ScrollDepth != 100 || !view.Final || view.Sequence != 3 {
		t.Fatalf("stale event corrupted visit: %+v", view)
	}
	later.URL = "https://example.com/other"
	if err := tracker.Record(ctx, later, "8.8.8.8", "Mozilla/5.0"); !errors.Is(err, analytics.ErrVisitConflict) {
		t.Fatalf("conflict=%v", err)
	}
}
func TestConcurrentDuplicateDeliveriesCreateOneVisit(t *testing.T) {
	db := testutil.Open(t)
	tracker := analytics.NewTracker(db, strings.Repeat("k", 32), nil, nil)
	var wg sync.WaitGroup
	errs := make(chan error, 16)
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- tracker.Record(context.Background(), event(1), "8.8.8.8", "Mozilla/5.0")
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var count int64
	db.Model(&models.PageView{}).Count(&count)
	if count != 1 {
		t.Fatalf("visits=%d", count)
	}
}
func TestRejectInvalidMeasurementsAndHosts(t *testing.T) {
	tracker := analytics.NewTracker(testutil.Open(t), strings.Repeat("k", 32), []string{"example.com"}, nil)
	for _, change := range []func(*analytics.Event){func(e *analytics.Event) { e.URL = "https://evil.example/" }, func(e *analytics.Event) { e.DwellTime = -1 }, func(e *analytics.Event) { e.ActiveTime = 1000 }, func(e *analytics.Event) { e.ScrollDepth = 101 }, func(e *analytics.Event) { e.VisitID = "" }, func(e *analytics.Event) { e.Sequence = 0 }} {
		e := event(1)
		change(&e)
		if err := tracker.Record(context.Background(), e, "8.8.8.8", "Mozilla/5.0"); !errors.Is(err, analytics.ErrInvalidEvent) {
			t.Fatalf("invalid event accepted: %v", err)
		}
	}
}
func TestVisitorKeysAreScopedToSiteAndUTCDay(t *testing.T) {
	key := []byte(strings.Repeat("k", 32))
	a := analytics.VisitorKey(key, "a.example", "2026-11-16", "8.8.8.8")
	for _, b := range []string{analytics.VisitorKey(key, "b.example", "2026-11-16", "8.8.8.8"), analytics.VisitorKey(key, "a.example", "2026-11-17", "8.8.8.8")} {
		if a == b {
			t.Fatal("visitor key correlates across sites or days")
		}
	}
}
