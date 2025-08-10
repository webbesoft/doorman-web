package main

import (
	"log"
	"os"

	"github.com/gorilla/sessions"
	"github.com/labstack/echo-contrib/session"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/tmunongo/doorman-go/handlers"
	authMiddleware "github.com/tmunongo/doorman-go/middleware"
	"github.com/tmunongo/doorman-go/models"
)

func main() {
	// Initialize database
	db, err := gorm.Open(sqlite.Open("analytics.db"), &gorm.Config{})

	// Check if there was an initial error
	if err != nil {
		file, createErr := os.Create("analytics.db")
		if createErr != nil {
			panic("failed to create database file")
		}
		file.Close()

		db, err = gorm.Open(sqlite.Open("analytics.db"), &gorm.Config{})
		if err != nil {
			panic("failed to connect to database after creation")
		}
	}

	// Auto migrate
	db.AutoMigrate(&models.PageView{}, &models.User{})

	// Create default admin user if doesn't exist
	var user models.User
	if err := db.Where("username = ?", "admin").First(&user).Error; err == gorm.ErrRecordNotFound {
		hashedPassword, _ := models.HashPassword(os.Getenv("ADMIN_PASS"))
		defaultUser := models.User{
			Username: os.Getenv("ADMIN_USER"),
			Password: hashedPassword,
		}
		db.Create(&defaultUser)
	}

	// Initialize Echo
	e := echo.New()
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(middleware.CORS())

	// Session middleware
	e.Use(session.Middleware(sessions.NewCookieStore([]byte(os.Getenv("DOORMAN_SESSION_SECRET")))))

	// Initialize handlers
	h := &handlers.Handler{DB: db}

	// Public routes (tracking)
	e.POST("/track", h.Track)
	e.GET("/t.js", h.ServeTracker)

	// Auth routes
	e.GET("/login", h.LoginPage)
	e.POST("/login", h.Login)
	e.POST("/logout", h.Logout)

	// Protected routes
	protected := e.Group("")
	protected.Use(authMiddleware.RequireAuth)
	protected.GET("/", h.Dashboard)
	protected.GET("/dashboard", h.Dashboard)

	// Static files
	e.Static("/static", "static")

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Server starting on port %s", port)
	e.Logger.Fatal(e.Start(":" + port))

}
