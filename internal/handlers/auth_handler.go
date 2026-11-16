package handlers

import (
	"errors"
	"github.com/labstack/echo/v4"
	"github.com/webbesoft/doorman/internal/auth"
	"github.com/webbesoft/doorman/templates/pages"
	"net/http"
)

type AuthHandler struct{ Auth *auth.Manager }

func (a *AuthHandler) LoginPage(c echo.Context) error {
	c.Response().Header().Set("Cache-Control", "no-store")
	messages := map[string]string{"invalid": "Invalid username or password.", "missing": "Please provide username and password.", "expired": "Your session has expired. Please sign in again."}
	return pages.LoginPage(messages[c.QueryParam("error")]).Render(c.Request().Context(), c.Response().Writer)
}
func (a *AuthHandler) Login(c echo.Context) error {
	username, password := c.FormValue("username"), c.FormValue("password")
	if username == "" || password == "" {
		return c.Redirect(http.StatusSeeOther, "/login?error=missing")
	}
	if len(username) > 128 || len(password) > 72 {
		return c.Redirect(http.StatusSeeOther, "/login?error=invalid")
	}
	token, err := a.Auth.Login(c.Request().Context(), username, password)
	if errors.Is(err, auth.ErrUnauthorized) {
		return c.Redirect(http.StatusSeeOther, "/login?error=invalid")
	}
	if err != nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "Authentication is temporarily unavailable")
	}
	if old, err := c.Cookie(auth.CookieName); err == nil {
		if err := a.Auth.Logout(c.Request().Context(), old.Value); err != nil {
			return err
		}
	}
	c.SetCookie(a.Auth.Cookie(token))
	return c.Redirect(http.StatusSeeOther, "/dashboard")
}
func (a *AuthHandler) Logout(c echo.Context) error {
	if cookie, err := c.Cookie(auth.CookieName); err == nil {
		if err := a.Auth.Logout(c.Request().Context(), cookie.Value); err != nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "Sign out is temporarily unavailable")
		}
	}
	c.SetCookie(a.Auth.Cookie(""))
	return c.Redirect(http.StatusSeeOther, "/login")
}
