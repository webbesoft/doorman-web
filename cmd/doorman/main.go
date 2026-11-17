package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	assets "github.com/webbesoft/doorman"
	"github.com/webbesoft/doorman/internal/analytics"
	"github.com/webbesoft/doorman/internal/auth"
	"github.com/webbesoft/doorman/internal/config"
	"github.com/webbesoft/doorman/internal/database"
	"github.com/webbesoft/doorman/internal/geolocation"
	"github.com/webbesoft/doorman/internal/handlers"
	authMiddleware "github.com/webbesoft/doorman/internal/middleware"
	"github.com/webbesoft/doorman/internal/models"
	"github.com/webbesoft/doorman/internal/observability"
	"github.com/webbesoft/doorman/internal/services"
	"gorm.io/gorm"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))
	if err := run(); err != nil {
		slog.Error("application stopped", "error", err)
		os.Exit(1)
	}
}
func run() error {
	migrate := flag.Bool("migrate", false, "Apply pending database migrations and exit")
	reset := flag.Bool("reset-admin-password", false, "Reset the configured admin password and revoke sessions")
	check := flag.Bool("healthcheck", false, "Check local HTTP readiness and exit")
	backup := flag.String("backup", "", "Write and verify a new SQLite backup file, then exit")
	verify := flag.String("verify-backup", "", "Verify a SQLite backup without opening the live database")
	flag.Parse()
	modes := 0
	for _, enabled := range []bool{*migrate, *reset, *check, *backup != "", *verify != ""} {
		if enabled {
			modes++
		}
	}
	if modes > 1 || flag.NArg() > 0 {
		return errors.New("select one command using -migrate, -reset-admin-password, -backup, -verify-backup, or -healthcheck")
	}
	if *verify != "" {
		return database.VerifySQLiteBackup(context.Background(), *verify)
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if *check {
		client := http.Client{Timeout: 3 * time.Second}
		resp, err := client.Get("http://127.0.0.1:" + cfg.Port + "/readyz")
		if err != nil {
			return errors.New("readiness check failed")
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return errors.New("application is not ready")
		}
		return nil
	}
	db, err := database.InitDB(cfg.Database)
	if err != nil {
		return err
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if *migrate {
		return database.Migrate(ctx, db, cfg.SessionSecret)
	}
	if *backup != "" {
		return database.BackupSQLite(ctx, db, *backup)
	}
	if cfg.Environment == "local" {
		if err := database.Migrate(ctx, db, cfg.SessionSecret); err != nil {
			return err
		}
	}
	if err := database.CheckSchema(db); err != nil {
		return err
	}
	if *reset {
		return database.ResetAdminPassword(ctx, db, cfg.AdminUser, cfg.AdminPassword)
	}
	if err := database.EnsureDefaultAdmin(db, cfg.AdminUser, cfg.AdminPassword); err != nil {
		return err
	}
	metrics := &observability.Metrics{}
	e, err := prepareServer(ctx, cfg, db, metrics)
	if err != nil {
		return err
	}
	workerDone := make(chan struct{})
	go func() { defer close(workerDone); services.StartCleanupRoutine(ctx, db, cfg.RetentionDays, metrics) }()
	serverErr := make(chan error, 1)
	go func() { serverErr <- e.Start(":" + cfg.Port) }()
	select {
	case err := <-serverErr:
		stop()
		<-workerDone
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err = e.Shutdown(shutdown)
	if err != nil {
		_ = e.Close()
	}
	<-workerDone
	return err
}
func prepareServer(ctx context.Context, cfg config.Config, db *gorm.DB, metrics *observability.Metrics) (*echo.Echo, error) {
	cleanup, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := database.CleanupOldData(db.WithContext(cleanup), cfg.RetentionDays); err != nil {
		return nil, fmt.Errorf("startup retention cleanup failed: %w", err)
	}
	return newServer(cfg, db, metrics), nil
}
func newServer(cfg config.Config, db *gorm.DB, metrics *observability.Metrics) *echo.Echo {
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true
	e.Server.ReadHeaderTimeout = 5 * time.Second
	e.Server.ReadTimeout = 10 * time.Second
	e.Server.WriteTimeout = 15 * time.Second
	e.Server.IdleTimeout = 60 * time.Second
	e.Server.MaxHeaderBytes = 16 << 10
	e.Use(metrics.Observe)
	e.Use(middleware.Recover())
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			ctx, cancel := context.WithTimeout(c.Request().Context(), 8*time.Second)
			defer cancel()
			c.SetRequest(c.Request().WithContext(ctx))
			headers := c.Response().Header()
			headers.Set("X-Content-Type-Options", "nosniff")
			headers.Set("X-Frame-Options", "DENY")
			headers.Set("Referrer-Policy", "no-referrer")
			headers.Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; object-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
			if cfg.SecureCookies() {
				headers.Set("Strict-Transport-Security", "max-age=31536000")
			}
			return next(c)
		}
	})
	e.Use(middleware.BodyLimit("16K"))
	e.IPExtractor = echo.ExtractIPDirect()
	if len(cfg.TrustedProxies) > 0 {
		options := []echo.TrustOption{echo.TrustLoopback(false), echo.TrustLinkLocal(false), echo.TrustPrivateNet(false)}
		for _, network := range cfg.TrustedProxies {
			options = append(options, echo.TrustIPRange(network))
		}
		e.IPExtractor = echo.ExtractIPFromXFFHeader(options...)
	}
	e.HTTPErrorHandler = func(err error, c echo.Context) {
		code := http.StatusInternalServerError
		message := "Request failed"
		var he *echo.HTTPError
		if errors.As(err, &he) {
			code = he.Code
			if code < 500 {
				message = fmt.Sprint(he.Message)
			}
		}
		if code >= 500 {
			slog.Error("request failed", "route", c.Path(), "status", code, "error_class", observability.ErrorClass(err))
		}
		if !c.Response().Committed {
			_ = c.JSON(code, map[string]string{"error": message})
		}
	}
	manager := auth.New(db, cfg.SessionSecret, cfg.SecureCookies())
	a := &handlers.AuthHandler{Auth: manager}
	geo := geolocation.NewGeoService(cfg.GeoAPIKey)
	h := &handlers.Handler{Tracker: analytics.NewTracker(db, cfg.SessionSecret, cfg.AllowedHosts, geo, cfg.MaxPageViews), Reports: analytics.NewReports(db)}
	protection := echo.WrapMiddleware(http.NewCrossOriginProtection().Handler)
	eventLimits := authMiddleware.NewLimits(120, 60, 32, 200)
	loginLimits := authMiddleware.NewLimits(5, 5, 2, 10)
	allowedOrigin := func(origin string) bool {
		if len(cfg.AllowedHosts) == 0 {
			return true
		}
		u, err := url.Parse(origin)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil {
			return false
		}
		for _, host := range cfg.AllowedHosts {
			if u.Hostname() == host {
				return true
			}
		}
		return false
	}
	cors := middleware.CORSWithConfig(middleware.CORSConfig{AllowOriginFunc: func(origin string) (bool, error) { return allowedOrigin(origin), nil }, AllowMethods: []string{http.MethodPost}, AllowHeaders: []string{echo.HeaderContentType}, MaxAge: 3600})
	collectorOrigin := func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if origin := c.Request().Header.Get("Origin"); origin != "" && !allowedOrigin(origin) {
				return c.NoContent(http.StatusForbidden)
			}
			return next(c)
		}
	}
	e.POST("/event", h.Track, eventLimits.Middleware, collectorOrigin, cors)
	e.OPTIONS("/event", func(c echo.Context) error { return c.NoContent(http.StatusNoContent) }, cors)
	e.GET("/login", a.LoginPage)
	e.POST("/login", a.Login, protection, loginLimits.Middleware)
	e.POST("/logout", a.Logout, protection)
	e.StaticFS("/assets", echo.MustSubFS(&assets.Assets, "assets"))
	protected := e.Group("", authMiddleware.RequireAuth(manager))
	reportLimits := authMiddleware.NewLimits(60, 10, 4, 20)
	protected.GET("/", h.Dashboard, reportLimits.Middleware)
	protected.GET("/dashboard", h.Dashboard, reportLimits.Middleware)
	e.GET("/metrics", func(c echo.Context) error {
		var budget models.StorageBudget
		if err := db.WithContext(c.Request().Context()).First(&budget, 1).Error; err != nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable)
		}
		c.Response().Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		if _, err := fmt.Fprintf(c.Response(), "doorman_retained_page_views %d\ndoorman_max_page_views %d\ndoorman_geo_lookup_failures_total %d\ndoorman_geo_rate_limited_total %d\n", budget.RetainedViews, cfg.MaxPageViews, geo.LookupFailures(), geo.RateLimitedLookups()); err != nil {
			return err
		}
		return metrics.Handler(c)
	}, authMiddleware.MetricsAccess(manager, cfg.MetricsToken))
	e.GET("/livez", func(c echo.Context) error { return c.NoContent(http.StatusOK) })
	e.GET("/readyz", func(c echo.Context) error {
		ctx, cancel := context.WithTimeout(c.Request().Context(), time.Second)
		defer cancel()
		sqlDB, err := db.DB()
		if err != nil {
			return c.NoContent(http.StatusServiceUnavailable)
		}
		if err := sqlDB.PingContext(ctx); err != nil {
			return c.NoContent(http.StatusServiceUnavailable)
		}
		return c.NoContent(http.StatusOK)
	})
	return e
}
