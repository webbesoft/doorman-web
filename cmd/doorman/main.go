package main

import (
	"log"
	"os"

	"github.com/gorilla/sessions"
	"github.com/labstack/echo-contrib/session"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"gorm.io/gorm"

	assets "github.com/webbesoft/doorman"
	database "github.com/webbesoft/doorman/internal"
	"github.com/webbesoft/doorman/internal/handlers"
	authMiddleware "github.com/webbesoft/doorman/internal/middleware"
	"github.com/webbesoft/doorman/internal/services"
)

type App struct {
	DB *gorm.DB
}

func main() {
	db, err := database.InitDB()
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}

	app := &App{DB: db}

	// Create default admin user if doesn't exist
	if err := database.EnsureDefaultAdmin(app.DB); err != nil {
		log.Fatalf("failed to ensure default admin: %v", err)
	}

	// Initialize Echo
	e := echo.New()
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: []string{"*"},
	}))

	// Session middleware
	e.Use(session.Middleware(sessions.NewCookieStore([]byte(os.Getenv("DOORMAN_SESSION_SECRET")))))

	// extract real IP
	e.IPExtractor = echo.ExtractIPFromXFFHeader()

	// Initialize handlers
	h := &handlers.Handler{DB: app.DB}
	a := &handlers.AuthHandler{DB: app.DB}

	// Public routes (tracking)
	e.POST("/event", h.Track)

	// Auth routes
	e.GET("/login", a.LoginPage)
	e.POST("/login", a.Login)
	e.POST("/logout", a.Logout)

	e.StaticFS("/assets", echo.MustSubFS(&assets.Assets, "assets"))

	// Protected routes
	protected := e.Group("")
	protected.Use(authMiddleware.RequireAuth)
	protected.GET("/", h.Dashboard)
	protected.GET("/dashboard", h.Dashboard)

	// Static files
	e.Static("/static", "static")

	go services.StartCleanupRoutine(db)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Server starting on port %s", port)
	e.Logger.Fatal(e.Start(":" + port))

}
