package observability

import (
	"github.com/labstack/echo/v4"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPreflightDoesNotCountAsAnEvent(t *testing.T) {
	var metrics Metrics
	e := echo.New()
	e.Use(metrics.Observe)
	e.OPTIONS("/event", func(c echo.Context) error { return c.NoContent(http.StatusNoContent) })
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodOptions, "/event", nil))
	if rec.Code != 204 {
		t.Fatal(rec.Code)
	}
	if metrics.accepted.Load() != 0 || metrics.rejected.Load() != 0 || metrics.failed.Load() != 0 {
		t.Fatal("preflight changed event counters")
	}
	if metrics.requests.Load() != 1 {
		t.Fatal("preflight missing from HTTP request count")
	}
}
