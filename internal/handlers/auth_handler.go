package handlers

import (
	"context"
	"net/http"

	"github.com/labstack/echo-contrib/session"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"

	"github.com/webbesoft/doorman/internal/models"
	"github.com/webbesoft/doorman/templates/pages"
)

type AuthHandler struct {
	DB *gorm.DB
}

// LoginPage renders the login page
func (a *AuthHandler) LoginPage(c echo.Context) error {
	errParam := c.QueryParam("error")
	var errMsg string
	switch errParam {
	case "invalid":
		errMsg = "Invalid username or password."
	case "missing":
		errMsg = "Please provide username and password."
	case "expired":
		errMsg = "Your session has expired. Please sign in again."
	default:
		// If an explicit message is provided via ?msg=... prefer that
		if m := c.QueryParam("msg"); m != "" {
			errMsg = m
		}
	}

	return pages.LoginPage(errMsg).Render(context.Background(), c.Response().Writer)
}

// Login handles user authentication
func (a *AuthHandler) Login(c echo.Context) error {
	username := c.FormValue("username")
	password := c.FormValue("password")

	if username == "" || password == "" {
		return c.Redirect(http.StatusFound, "/login?error=missing")
	}

	var user models.User
	if err := a.DB.Where("username = ?", username).First(&user).Error; err != nil {
		return c.Redirect(http.StatusFound, "/login?error=invalid")
	}

	if !models.CheckPasswordHash(password, user.Password) {
		return c.Redirect(http.StatusFound, "/login?error=invalid")
	}

	sess, _ := session.Get("session", c)
	sess.Values["user_id"] = user.ID
	sess.Save(c.Request(), c.Response())

	return c.Redirect(http.StatusFound, "/dashboard")
}

// Logout handles user logout
func (a *AuthHandler) Logout(c echo.Context) error {
	sess, _ := session.Get("session", c)
	sess.Values = make(map[interface{}]interface{})
	sess.Save(c.Request(), c.Response())
	return c.Redirect(http.StatusFound, "/login")
}
