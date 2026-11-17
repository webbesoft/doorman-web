package middleware

import (
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"github.com/labstack/echo/v4"
	"github.com/webbesoft/doorman/internal/auth"
	"net/http"
	"strings"
)

func RequireAuth(manager *auth.Manager) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			c.Response().Header().Set("Cache-Control", "no-store")
			cookie, err := c.Cookie(auth.CookieName)
			if err != nil {
				return c.Redirect(http.StatusSeeOther, "/login")
			}
			user, err := manager.Authenticate(c.Request().Context(), cookie.Value)
			if errors.Is(err, auth.ErrUnauthorized) {
				c.SetCookie(manager.Cookie(""))
				return c.Redirect(http.StatusSeeOther, "/login?error=expired")
			}
			if err != nil {
				return echo.NewHTTPError(http.StatusServiceUnavailable, "Authentication is temporarily unavailable").SetInternal(err)
			}
			c.Set("user", user)
			return next(c)
		}
	}
}

// MetricsAccess allows a dedicated scraper token or an authenticated dashboard session.
func MetricsAccess(manager *auth.Manager, token string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		browser := RequireAuth(manager)(next)
		return func(c echo.Context) error {
			if token != "" && strings.HasPrefix(c.Request().Header.Get("Authorization"), "Bearer ") {
				want := sha256.Sum256([]byte(token))
				actual := sha256.Sum256([]byte(strings.TrimPrefix(c.Request().Header.Get("Authorization"), "Bearer ")))
				if subtle.ConstantTimeCompare(want[:], actual[:]) != 1 {
					return c.NoContent(http.StatusUnauthorized)
				}
				c.Response().Header().Set("Cache-Control", "no-store")
				return next(c)
			}
			return browser(c)
		}
	}
}
