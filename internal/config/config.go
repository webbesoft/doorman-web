package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Database struct{ Provider, Path, URL, Host, Port, User, Password, Name, SSLMode string }
type Config struct {
	Environment, Port, AdminUser, AdminPassword, SessionSecret, GeoAPIKey string
	Database                                                              Database
	AllowedHosts                                                          []string
	TrustedProxies                                                        []*net.IPNet
	RetentionDays                                                         int
}

func Load() (Config, error) {
	if os.Getenv("APP_ENV") == "local" {
		if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
			return Config{}, fmt.Errorf("load local configuration: %w", err)
		}
	}
	get := func(key, fallback string) string {
		if value := os.Getenv(key); value != "" {
			return value
		}
		return fallback
	}
	c := Config{
		Environment: get("APP_ENV", "production"), Port: get("PORT", "8080"),
		AdminUser: strings.TrimSpace(os.Getenv("DOORMAN_ADMIN_USER")), AdminPassword: os.Getenv("DOORMAN_ADMIN_PASSWORD"),
		SessionSecret: os.Getenv("DOORMAN_SESSION_SECRET"), GeoAPIKey: os.Getenv("DOORMAN_GEO_API_KEY"),
		Database: Database{Provider: get("DB_PROVIDER", "sqlite"), Path: get("DB_PATH", "analytics.db"),
			URL: os.Getenv("DATABASE_URL"), Host: os.Getenv("DB_HOST"), Port: os.Getenv("DB_PORT"),
			User: os.Getenv("DB_USER"), Password: os.Getenv("DB_PASSWORD"), Name: os.Getenv("DB_NAME"), SSLMode: get("DB_SSLMODE", "verify-full")},
	}
	var err error
	c.RetentionDays, err = strconv.Atoi(get("DOORMAN_RETENTION_DAYS", "90"))
	if err != nil || c.RetentionDays < 1 || c.RetentionDays > 3650 {
		return Config{}, errors.New("DOORMAN_RETENTION_DAYS must be between 1 and 3650")
	}
	for _, host := range strings.Split(os.Getenv("DOORMAN_ALLOWED_HOSTS"), ",") {
		host = strings.ToLower(strings.TrimSpace(host))
		if host == "" {
			continue
		}
		u, err := url.Parse("https://" + host)
		if err != nil || u.Hostname() != host || u.Port() != "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
			return Config{}, errors.New("DOORMAN_ALLOWED_HOSTS must contain comma-separated hostnames without schemes, ports, or paths")
		}
		c.AllowedHosts = append(c.AllowedHosts, host)
	}
	for _, cidr := range strings.Split(os.Getenv("DOORMAN_TRUSTED_PROXIES"), ",") {
		if strings.TrimSpace(cidr) == "" {
			continue
		}
		_, network, err := net.ParseCIDR(strings.TrimSpace(cidr))
		if err != nil {
			return Config{}, errors.New("DOORMAN_TRUSTED_PROXIES must contain valid CIDR ranges")
		}
		c.TrustedProxies = append(c.TrustedProxies, network)
	}
	if c.Environment != "local" && c.Environment != "production" && c.Environment != "test" {
		return Config{}, errors.New("APP_ENV must be local, test, or production")
	}
	port, err := strconv.Atoi(c.Port)
	if err != nil || port < 1 || port > 65535 {
		return Config{}, errors.New("PORT must be between 1 and 65535")
	}
	if len(c.SessionSecret) < 32 {
		return Config{}, errors.New("DOORMAN_SESSION_SECRET must contain at least 32 bytes")
	}
	if c.AdminUser == "" || len(c.AdminUser) > 128 {
		return Config{}, errors.New("DOORMAN_ADMIN_USER must contain 1 to 128 bytes")
	}
	if c.AdminPassword != "" && (len(c.AdminPassword) < 12 || len(c.AdminPassword) > 72) {
		return Config{}, errors.New("DOORMAN_ADMIN_PASSWORD must contain 12 to 72 bytes")
	}
	if c.Environment == "production" && len(c.AllowedHosts) == 0 {
		return Config{}, errors.New("DOORMAN_ALLOWED_HOSTS is required in production")
	}
	if c.Database.Provider == "pg" {
		c.Database.Provider = "postgres"
	}
	switch c.Database.Provider {
	case "sqlite":
	case "postgres", "mysql":
		if c.Database.URL == "" && (c.Database.Host == "" || c.Database.User == "" || c.Database.Name == "") {
			return Config{}, errors.New("database connection settings are incomplete")
		}
	default:
		return Config{}, errors.New("DB_PROVIDER must be sqlite, postgres, or mysql")
	}
	return c, nil
}
func (c Config) SecureCookies() bool { return c.Environment != "local" && c.Environment != "test" }
