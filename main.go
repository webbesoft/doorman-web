package main

import (
	"log"
	"os"

	"github.com/gorilla/sessions"
	"github.com/joho/godotenv"
	"github.com/labstack/echo-contrib/session"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/tmunongo/doorman/handlers"
	authMiddleware "github.com/tmunongo/doorman/middleware"
	"github.com/tmunongo/doorman/models"
)

func main() {
	db, err := gorm.Open(sqlite.Open("analytics.db"), &gorm.Config{})
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}

	// Auto migrate
	db.AutoMigrate(&models.PageView{}, &models.User{})

	if os.Getenv("APP_ENV") == "local" {
		err := godotenv.Load()

		if err != nil {
			log.Fatal("Error loading .env file")
		}
	}

	// Create default admin user if doesn't exist
	var user models.User
	if err := db.Where("username = ?", "admin").First(&user).Error; err == gorm.ErrRecordNotFound {
		log.Println(os.Getenv("ADMIN_PASSWORD"))

		hashedPassword, _ := models.HashPassword(os.Getenv("ADMIN_PASSWORD"))
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
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: []string{"*"},
	}))

	// Session middleware
	e.Use(session.Middleware(sessions.NewCookieStore([]byte(os.Getenv("DOORMAN_SESSION_SECRET")))))

	// Initialize handlers
	h := &handlers.Handler{DB: db}

	// Public routes (tracking)
	e.POST("/event", h.Track)

	// Auth routes
	e.GET("/login", h.LoginPage)
	e.POST("/login", h.Login)
	e.POST("/logout", h.Logout)

	e.Static("/assets", "assets")

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
