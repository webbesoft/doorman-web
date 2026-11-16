package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	assets "github.com/webbesoft/doorman"
	"github.com/webbesoft/doorman/internal/auth"
"github.com/webbesoft/doorman/internal/analytics"
	"github.com/webbesoft/doorman/internal/config"
	"github.com/webbesoft/doorman/internal/database"
	"github.com/webbesoft/doorman/internal/handlers"
	authMiddleware "github.com/webbesoft/doorman/internal/middleware"
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
	flag.Parse()
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
		return database.Migrate(ctx, db)
	}
	if cfg.Environment == "local" {
		if err := database.Migrate(ctx, db); err != nil {
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
	e := newServer(cfg, db)
	workerDone := make(chan struct{})
	go func() { defer close(workerDone); services.StartCleanupRoutine(ctx, db, cfg.RetentionDays) }()
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
func newServer(cfg config.Config, db *gorm.DB) *echo.Echo {
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true
	e.Server.ReadHeaderTimeout = 5 * time.Second
	e.Server.ReadTimeout = 10 * time.Second
	e.Server.WriteTimeout = 15 * time.Second
	e.Server.IdleTimeout = 60 * time.Second
	e.Server.MaxHeaderBytes = 16 << 10
	e.Use(middleware.Recover())
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
			slog.Error("request failed", "route", c.Path(), "status", code)
		}
		if !c.Response().Committed {
			_ = c.JSON(code, map[string]string{"error": message})
		}
	}
	manager := auth.New(db, cfg.SessionSecret, cfg.SecureCookies())
	a := &handlers.AuthHandler{Auth: manager}
	h := &handlers.Handler{Tracker: analytics.NewTracker(db,cfg.SessionSecret,cfg.AllowedHosts,services.NewGeoService(cfg.GeoAPIKey)), Reports: analytics.NewReports(db)}
	protection := echo.WrapMiddleware(http.NewCrossOriginProtection().Handler)
	e.POST("/event", h.Track, middleware.CORSWithConfig(middleware.CORSConfig{AllowOrigins: []string{"*"}, AllowMethods: []string{http.MethodPost}, AllowHeaders: []string{echo.HeaderContentType}}))
	e.GET("/login", a.LoginPage)
	e.POST("/login", a.Login, protection)
	e.POST("/logout", a.Logout, protection)
	e.StaticFS("/assets", echo.MustSubFS(&assets.Assets, "assets"))
	protected := e.Group("", authMiddleware.RequireAuth(manager))
	protected.GET("/", h.Dashboard)
	protected.GET("/dashboard", h.Dashboard)
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
