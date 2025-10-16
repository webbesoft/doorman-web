package database

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/joho/godotenv"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/webbesoft/doorman/internal/models"
)

func InitDB() (*gorm.DB, error) {
	provider := os.Getenv("DB_PROVIDER")

	db, err := connectByProvider(provider)
	if err != nil {
		return nil, err
	}

	if err := db.AutoMigrate(&models.Analytics{}, &models.PageVisit{}, &models.User{}); err != nil {
		return nil, err
	}

	return db, nil
}

func EnsureDefaultAdmin(db *gorm.DB) error {
	if os.Getenv("APP_ENV") == "local" {
		if err := godotenv.Load(); err != nil {
			log.Println("Warning: .env not loaded:", err)
		}
	}

	var user models.User
	if err := db.Where("username = ?", "admin").First(&user).Error; err == gorm.ErrRecordNotFound {
		log.Println("Creating default admin; password from ADMIN_PASSWORD env var")
		hashedPassword, _ := models.HashPassword(os.Getenv("ADMIN_PASSWORD"))
		defaultUser := models.User{
			Username: os.Getenv("ADMIN_USER"),
			Password: hashedPassword,
		}
		return db.Create(&defaultUser).Error
	}

	return nil
}

func connectByProvider(provider string) (*gorm.DB, error) {
	switch provider {
	case "postgres", "pg":
		// Prefer full DATABASE_URL
		dsn := os.Getenv("DATABASE_URL")
		if dsn == "" {
			host := os.Getenv("DB_HOST")
			port := os.Getenv("DB_PORT")
			user := os.Getenv("DB_USER")
			password := os.Getenv("DB_PASSWORD")
			dbname := os.Getenv("DB_NAME")
			sslmode := os.Getenv("DB_SSLMODE")
			if sslmode == "" {
				sslmode = "disable"
			}
			dsn = fmt.Sprintf(
				"host=%s user=%s password=%s dbname=%s port=%s sslmode=%s TimeZone=UTC",
				host, user, password, dbname, port, sslmode,
			)
		}
		db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})

		return db, err

	case "mysql":
		// Prefer full DATABASE_URL if provided
		dsn := os.Getenv("DATABASE_URL")
		if dsn == "" {
			host := os.Getenv("DB_HOST")
			port := os.Getenv("DB_PORT")
			if port == "" {
				port = "3306"
			}
			user := os.Getenv("DB_USER")
			password := os.Getenv("DB_PASSWORD")
			dbname := os.Getenv("DB_NAME")
			dsn = fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local",
				user, password, host, port, dbname)
		}
		db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})

		return db, err

	default:
		// Default to SQLite. DB_PATH env can override the file path.
		path := os.Getenv("DB_PATH")
		if path == "" {
			path = "analytics.db"
		}
		if provider != "" {
			log.Printf("DB_PROVIDER=%s not recognized, defaulting to sqlite", provider)
		}
		db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})

		return db, err
	}
}

// remove page views older than retention period
func CleanupOldData(db *gorm.DB, retentionDays int) error {
	cutoff := time.Now().AddDate(0, 0, -retentionDays)

	result := db.Where("created_at < ?", cutoff).Delete(&models.Analytics{})
	if result.Error != nil {
		return result.Error
	}

	log.Printf("Cleaned up %d old page views (older than %d days)", result.RowsAffected, retentionDays)
	return nil
}
