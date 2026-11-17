package middleware

import (
	"crypto/sha256"
	"github.com/labstack/echo/v4"
	"golang.org/x/time/rate"
	"net/http"
	"sync"
	"time"
)

type clientBucket struct {
	limiter *rate.Limiter
	seen    time.Time
}
type Limits struct {
	mu        sync.Mutex
	clients   map[[32]byte]*clientBucket
	global    *rate.Limiter
	perClient rate.Limit
	burst     int
	slots     chan struct{}
	nextSweep time.Time
}

func NewLimits(perMinute, burst, concurrency int, globalPerSecond rate.Limit) *Limits {
	return &Limits{clients: make(map[[32]byte]*clientBucket), global: rate.NewLimiter(globalPerSecond, max(1, int(globalPerSecond))), perClient: rate.Limit(float64(perMinute) / 60), burst: burst, slots: make(chan struct{}, concurrency)}
}
func (l *Limits) Middleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		reject := func() error {
			c.Response().Header().Set("Retry-After", "60")
			return c.NoContent(http.StatusTooManyRequests)
		}
		if !l.global.Allow() {
			return reject()
		}
		now := time.Now()
		key := sha256.Sum256([]byte(c.RealIP()))
		l.mu.Lock()
		if now.After(l.nextSweep) {
			for k, b := range l.clients {
				if now.Sub(b.seen) > 10*time.Minute {
					delete(l.clients, k)
				}
			}
			l.nextSweep = now.Add(time.Minute)
		}
		bucket := l.clients[key]
		if bucket == nil {
			if len(l.clients) >= 10000 {
				l.mu.Unlock()
				return reject()
			}
			bucket = &clientBucket{limiter: rate.NewLimiter(l.perClient, l.burst)}
			l.clients[key] = bucket
		}
		bucket.seen = now
		allowed := bucket.limiter.Allow()
		l.mu.Unlock()
		if !allowed {
			return reject()
		}
		select {
		case l.slots <- struct{}{}:
			defer func() { <-l.slots }()
			return next(c)
		default:
			c.Response().Header().Set("Retry-After", "1")
			return c.NoContent(http.StatusServiceUnavailable)
		}
	}
}
