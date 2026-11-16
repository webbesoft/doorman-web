package analytics

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/webbesoft/doorman/internal/geolocation"
	"github.com/webbesoft/doorman/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrInvalidEvent = errors.New("invalid tracking event")
var ErrVisitConflict = errors.New("visit ID already belongs to a different page")

type Event struct {
	VisitID     string `json:"visitId"`
	Sequence    int64  `json:"sequence"`
	URL         string `json:"url"`
	Referrer    string `json:"referrer"`
	DwellTime   int64  `json:"dwellTime"`
	ActiveTime  int64  `json:"activeTime"`
	ScrollDepth int    `json:"scrollDepth"`
	Final       bool   `json:"final"`
}

type Tracker struct {
	db     *gorm.DB
	secret []byte
	hosts  map[string]bool
	geo    *geolocation.GeoService
}

func NewTracker(db *gorm.DB, secret string, hosts []string, geo *geolocation.GeoService) *Tracker {
	allowed := make(map[string]bool, len(hosts))
	for _, host := range hosts {
		allowed[host] = true
	}
	return &Tracker{db: db, secret: []byte(secret), hosts: allowed, geo: geo}
}

func NormalizeURL(raw string) (string, string, error) {
	if len(raw) == 0 || len(raw) > 2048 {
		return "", "", ErrInvalidEvent
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Hostname() == "" || u.User != nil || len(u.Hostname()) > 253 {
		return "", "", ErrInvalidEvent
	}
	host := strings.ToLower(u.Hostname())
	u.Host = strings.ToLower(u.Host)
	u.RawQuery = ""
	u.ForceQuery = false
	u.Fragment = ""
	u.RawFragment = ""
	if u.Path == "" {
		u.Path = "/"
	}
	return u.String(), host, nil
}
func ReferrerHost(raw string) string {
	if len(raw) > 2048 {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || len(u.Hostname()) > 253 {
		return ""
	}
	return strings.ToLower(u.Hostname())
}
func VisitorKey(secret []byte, site, day, ip string) string {
	h := hmac.New(sha256.New, secret)
	h.Write([]byte("visitor:" + site + "\x00" + day + "\x00" + ip))
	return hex.EncodeToString(h.Sum(nil))
}
func (t *Tracker) Record(ctx context.Context, event Event, ip, userAgent string) error {
	id := strings.ReplaceAll(event.VisitID, "-", "")
	decoded, err := hex.DecodeString(id)
	if err != nil || len(decoded) != 16 || len(event.VisitID) > 36 || event.Sequence < 1 || event.Sequence > 1000000 || event.DwellTime < 0 || event.DwellTime > 86400 || event.ActiveTime < 0 || event.ActiveTime > event.DwellTime || event.ScrollDepth < 0 || event.ScrollDepth > 100 || len(event.Referrer) > 2048 {
		return ErrInvalidEvent
	}
	canonical, host, err := NormalizeURL(event.URL)
	if err != nil {
		return err
	}
	if len(t.hosts) > 0 && !t.hosts[host] {
		return ErrInvalidEvent
	}
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return ErrInvalidEvent
	}
	now := time.Now().UTC()
	day := now.Format("2006-01-02")
	key := VisitorKey(t.secret, host, day, parsedIP.String())
	// Existing heartbeats do not need enrichment. Only a new navigation can look it up.
	var existing models.PageView
	err = t.db.WithContext(ctx).Select("id", "url").Where("id = ?", strings.ToLower(id)).First(&existing).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if err == nil && existing.URL != canonical {
		return ErrVisitConflict
	}
	country := ""
	if errors.Is(err, gorm.ErrRecordNotFound) {
		country = t.geo.GetGeoDataCached(ctx, parsedIP.String(), key).Country
	}
	view := models.PageView{ID: strings.ToLower(id), SiteHost: host, Day: day, VisitorKey: key, URL: canonical, Referrer: ReferrerHost(event.Referrer), Country: country,
		DwellTime: event.DwellTime, ActiveTime: event.ActiveTime, ScrollDepth: event.ScrollDepth, Sequence: event.Sequence, Final: event.Final, IsBot: IsBot(userAgent), CreatedAt: now, UpdatedAt: now}
	return t.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&view)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 1 {
			return nil
		}
		// A sequence guard rejects stale and duplicate deliveries; maxima preserve cumulative measurements.
		result = tx.Model(&models.PageView{}).Where("id = ? AND url = ? AND sequence < ?", view.ID, canonical, event.Sequence).Updates(map[string]any{
			"dwell_time":   gorm.Expr("CASE WHEN dwell_time < ? THEN ? ELSE dwell_time END", event.DwellTime, event.DwellTime),
			"active_time":  gorm.Expr("CASE WHEN active_time < ? THEN ? ELSE active_time END", event.ActiveTime, event.ActiveTime),
			"scroll_depth": gorm.Expr("CASE WHEN scroll_depth < ? THEN ? ELSE scroll_depth END", event.ScrollDepth, event.ScrollDepth),
			"sequence":     event.Sequence, "final": gorm.Expr("final OR ?", event.Final), "is_bot": gorm.Expr("is_bot OR ?", view.IsBot), "updated_at": now,
		})
		if result.Error != nil {
			return result.Error
		}
		// Recheck the invariant inside the transaction, including concurrent first writes.
		var persisted models.PageView
		if err := tx.Select("url").Where("id = ?", view.ID).First(&persisted).Error; err != nil {
			return err
		}
		if persisted.URL != canonical {
			return ErrVisitConflict
		}
		return nil
	})
}
func IsBot(userAgent string) bool {
	ua := strings.ToLower(userAgent)
	if ua == "" {
		return true
	}
	for _, pattern := range []string{"bot", "crawler", "spider", "scraper", "python-requests", "go-http-client", "curl/", "wget/", "pingdom", "uptime"} {
		if strings.Contains(ua, pattern) {
			return true
		}
	}
	return false
}
