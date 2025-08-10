package models

import (
	"time"

	"golang.org/x/crypto/bcrypt"
)

type PageView struct {
ID uint `gorm:"primaryKey" json:"id"`
URL string `gorm:"not null" json:"url"`
Referrer string `json:"referrer"`
UserAgent string `json:"user_agent"`
IPHash string `gorm:"index" json:"-"` // Hashed IP for GDPR compliance
CreatedAt time.Time `json:"created_at"`
}

type User struct {
ID uint `gorm:"primaryKey"`
Username string `gorm:"unique;not null"`
Password string `gorm:"not null"`
CreatedAt time.Time
}

func HashPassword(password string) (string, error) {
bytes, err := bcrypt.GenerateFromPassword([]byte(password), 14)
return string(bytes), err
}

func CheckPasswordHash(password, hash string) bool {
err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
return err == nil
}