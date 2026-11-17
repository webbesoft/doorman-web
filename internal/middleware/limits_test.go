package middleware

import (
	"github.com/labstack/echo/v4"
	"net/http/httptest"
	"testing"
)

func TestLimitsBoundClientsAndConcurrency(t *testing.T) {
	limits := NewLimits(1, 1, 1, 100)
	e := echo.New()
	handler := limits.Middleware(func(c echo.Context) error { return c.NoContent(204) })
	for i, want := range []int{204, 429} {
		req := httptest.NewRequest("POST", "/event", nil)
		rec := httptest.NewRecorder()
		if err := handler(e.NewContext(req, rec)); err != nil {
			t.Fatal(err)
		}
		if rec.Code != want {
			t.Fatalf("request %d status=%d", i, rec.Code)
		}
	}
	// A full work queue fails quickly rather than accumulating blocked requests.
	limits = NewLimits(10, 10, 1, 100)
	limits.slots <- struct{}{}
	req := httptest.NewRequest("POST", "/event", nil)
	rec := httptest.NewRecorder()
	if err := limits.Middleware(func(c echo.Context) error { t.Fatal("handler ran at capacity"); return nil })(e.NewContext(req, rec)); err != nil {
		t.Fatal(err)
	}
	if rec.Code != 503 {
		t.Fatal("queue was unbounded", rec.Code)
	}
}
