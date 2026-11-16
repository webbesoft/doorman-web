package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/http"
	"time"

	"github.com/webbesoft/doorman/internal/models"
	"gorm.io/gorm"
)

const CookieName = "doorman_session"
const Lifetime = 12 * time.Hour
const IdleTimeout = 30 * time.Minute

var ErrUnauthorized = errors.New("invalid credentials or session")

type Manager struct {
	db     *gorm.DB
	key    []byte
	secure bool
	now    func() time.Time
}

func New(db *gorm.DB, secret string, secure bool) *Manager {
	return &Manager{db: db, key: []byte(secret), secure: secure, now: time.Now}
}
func (m *Manager) hash(token string) string {
	h := hmac.New(sha256.New, m.key)
	h.Write([]byte("session:" + token))
	return hex.EncodeToString(h.Sum(nil))
}
func (m *Manager) Login(ctx context.Context, username, password string) (string, error) {
	var user models.User
	err := m.db.WithContext(ctx).Where("username = ?", username).First(&user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", ErrUnauthorized
		}
		return "", err
	}
	if !models.CheckPasswordHash(password, user.Password) {
		return "", ErrUnauthorized
	}
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(raw[:])
	now := m.now().UTC()
	s := models.AuthSession{TokenHash: m.hash(token), UserID: user.ID, ExpiresAt: now.Add(Lifetime), LastSeenAt: now}
	if err := m.db.WithContext(ctx).Create(&s).Error; err != nil {
		return "", err
	}
	return token, nil
}
func (m *Manager) Authenticate(ctx context.Context, token string) (models.User, error) {
	if len(token) != 43 {
		return models.User{}, ErrUnauthorized
	}
	now := m.now().UTC()
	var user models.User
	err := m.db.WithContext(ctx).Model(&models.User{}).Select("users.*").Joins("JOIN auth_sessions ON auth_sessions.user_id = users.id").
		Where("auth_sessions.token_hash = ? AND auth_sessions.expires_at > ? AND auth_sessions.last_seen_at > ?", m.hash(token), now, now.Add(-IdleTimeout)).First(&user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return user, ErrUnauthorized
		}
		return user, err
	}
	if err := m.db.WithContext(ctx).Model(&models.AuthSession{}).Where("token_hash = ? AND last_seen_at < ?", m.hash(token), now.Add(-time.Minute)).Update("last_seen_at", now).Error; err != nil {
		return models.User{}, err
	}
	return user, nil
}
func (m *Manager) Logout(ctx context.Context, token string) error {
	return m.db.WithContext(ctx).Where("token_hash = ?", m.hash(token)).Delete(&models.AuthSession{}).Error
}
func (m *Manager) Cookie(token string) *http.Cookie {
	maxAge := int(Lifetime.Seconds())
	if token == "" {
		maxAge = -1
	}
	return &http.Cookie{Name: CookieName, Value: token, Path: "/", Secure: m.secure, HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: maxAge}
}
