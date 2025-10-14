package services

import (
	"log"
	"time"

	database "github.com/webbesoft/doorman/internal"
	"gorm.io/gorm"
)

func StartCleanupRoutine(db *gorm.DB) {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	// Run immediately on startup
	runCleanup(db)

	for range ticker.C {
		runCleanup(db)
	}
}

func runCleanup(db *gorm.DB) {
	log.Println("Running scheduled cleanup...")

	// Clean up data older than 90 days
	if err := database.CleanupOldData(db, 90); err != nil {
		log.Printf("Cleanup failed: %v", err)
	}

	// TODO: Clear in-memory caches older than 24 hours

	log.Println("Cleanup completed")
}