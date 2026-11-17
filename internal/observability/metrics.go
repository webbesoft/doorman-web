package observability

import (
	"fmt"
	"github.com/labstack/echo/v4"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"
)

type Metrics struct {
	accepted, rejected, failed, requests, inflight, durationNS atomic.Int64
	cleanupFailures                                            atomic.Int64
	lastCleanup                                                atomic.Int64
}

func (m *Metrics) Observe(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		started := time.Now()
		m.inflight.Add(1)
		defer m.inflight.Add(-1)
		err := next(c)
		if err != nil {
			c.Error(err)
		}
		elapsed := time.Since(started)
		status := c.Response().Status
		m.requests.Add(1)
		m.durationNS.Add(elapsed.Nanoseconds())
		if c.Path() == "/event" && c.Request().Method == http.MethodPost {
			if status == http.StatusNoContent {
				m.accepted.Add(1)
			} else if status < 500 {
				m.rejected.Add(1)
			} else {
				m.failed.Add(1)
			}
		}
		if c.Path() != "/livez" && c.Path() != "/readyz" {
			slog.Info("http request", "method", c.Request().Method, "route", c.Path(), "status", status, "duration_ms", elapsed.Milliseconds())
		}
		return nil
	}
}
func (m *Metrics) Cleanup(err error) {
	if err != nil {
		m.cleanupFailures.Add(1)
	} else {
		m.lastCleanup.Store(time.Now().Unix())
	}
}
func (m *Metrics) Handler(c echo.Context) error {
	c.Response().Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	_, err := fmt.Fprintf(c.Response(), "doorman_http_requests_total %d\ndoorman_http_inflight %d\ndoorman_http_duration_seconds_sum %.6f\ndoorman_events_accepted_total %d\ndoorman_events_rejected_total %d\ndoorman_events_failed_total %d\ndoorman_cleanup_failures_total %d\ndoorman_cleanup_last_success_timestamp_seconds %d\n", m.requests.Load(), m.inflight.Load(), float64(m.durationNS.Load())/1e9, m.accepted.Load(), m.rejected.Load(), m.failed.Load(), m.cleanupFailures.Load(), m.lastCleanup.Load())
	return err
}
