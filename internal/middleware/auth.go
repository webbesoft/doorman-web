package middleware

import (
	"errors"
	"github.com/labstack/echo/v4"
	"github.com/webbesoft/doorman/internal/auth"
	"net/http"
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
				return echo.NewHTTPError(http.StatusServiceUnavailable, "Authentication is temporarily unavailable")
			}
			c.Set("user", user)
			return next(c)
		}
	}
}
