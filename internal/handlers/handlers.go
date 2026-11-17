package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/webbesoft/doorman/internal/analytics"
	"github.com/webbesoft/doorman/templates/pages"
)

type Handler struct {
	Tracker *analytics.Tracker
	Reports *analytics.Reports
}
type TrackRequest = analytics.Event

func (h *Handler) Track(c echo.Context) error {
	c.Request().Body = http.MaxBytesReader(c.Response(), c.Request().Body, 16<<10)
	var event *analytics.Event
	decoder := json.NewDecoder(c.Request().Body)
	if err := decoder.Decode(&event); err != nil || event == nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return c.NoContent(http.StatusRequestEntityTooLarge)
		}
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid event"})
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid event"})
	}
	err := h.Tracker.Record(c.Request().Context(), *event, c.RealIP(), c.Request().UserAgent())
	if errors.Is(err, analytics.ErrInvalidEvent) {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid event"})
	}
	if errors.Is(err, analytics.ErrVisitConflict) {
		return c.JSON(http.StatusConflict, map[string]string{"error": "Visit ID belongs to a different page"})
	}
	if errors.Is(err, analytics.ErrCapacity) {
		c.Response().Header().Set("Retry-After", "3600")
		return echo.NewHTTPError(http.StatusServiceUnavailable, "Event storage capacity reached").SetInternal(err)
	}
	if err != nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "Event storage is unavailable").SetInternal(err)
	}
	return c.NoContent(http.StatusNoContent)
}
func (h *Handler) Dashboard(c echo.Context) error {
	snapshot, err := h.Reports.Snapshot(c.Request().Context(), time.Now())
	if err != nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "Reports are temporarily unavailable").SetInternal(err)
	}
	return pages.DashboardPage(snapshot.TopReferrers, snapshot.TopPages, snapshot.DailyStats, snapshot.TopCountries, snapshot.Metrics).Render(c.Request().Context(), c.Response().Writer)
}
