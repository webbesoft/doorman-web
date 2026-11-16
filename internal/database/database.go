package database

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"time"

	mysqlconfig "github.com/go-sql-driver/mysql"
	"github.com/webbesoft/doorman/internal/config"
	"github.com/webbesoft/doorman/internal/models"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func InitDB(c config.Database) (*gorm.DB, error) {
	var dialector gorm.Dialector
	switch c.Provider {
	case "sqlite":
		path, err := filepath.Abs(c.Path)
		if err != nil {
			return nil, err
		}
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return nil, err
		}
		u := url.URL{Scheme: "file", Path: path}
		q := u.Query()
		q.Set("_foreign_keys", "on")
		q.Set("_journal_mode", "WAL")
		q.Set("_busy_timeout", "5000")
		u.RawQuery = q.Encode()
		dialector = sqlite.Open(u.String())
	case "postgres":
		dsn := c.URL
		if dsn == "" {
			port := c.Port
			if port == "" {
				port = "5432"
			}
			u := url.URL{Scheme: "postgres", User: url.UserPassword(c.User, c.Password), Host: net.JoinHostPort(c.Host, port), Path: "/" + c.Name}
			q := u.Query()
			q.Set("sslmode", c.SSLMode)
			q.Set("connect_timeout", "5")
			q.Set("TimeZone", "UTC")
			u.RawQuery = q.Encode()
			dsn = u.String()
		}
		dialector = postgres.Open(dsn)
	case "mysql":
		dsn := c.URL
		if dsn == "" {
			port := c.Port
			if port == "" {
				port = "3306"
			}
			mc := mysqlconfig.NewConfig()
			mc.User = c.User
			mc.Passwd = c.Password
			mc.Net = "tcp"
			mc.Addr = net.JoinHostPort(c.Host, port)
			mc.DBName = c.Name
			mc.ParseTime = true
			mc.Loc = time.UTC
			mc.TLSConfig = "true"
			mc.Timeout = 5 * time.Second
			mc.ReadTimeout = 10 * time.Second
			mc.WriteTimeout = 10 * time.Second
			dsn = mc.FormatDSN()
		}
		dialector = mysql.Open(dsn)
	default:
		return nil, errors.New("unsupported database provider")
	}
	db, err := gorm.Open(dialector, &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return nil, errors.New("database connection failed; check connection settings and availability")
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	if c.Provider == "sqlite" {
		sqlDB.SetMaxOpenConns(1)
		sqlDB.SetMaxIdleConns(1)
	} else {
		sqlDB.SetMaxOpenConns(10)
		sqlDB.SetMaxIdleConns(5)
		sqlDB.SetConnMaxLifetime(30 * time.Minute)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		sqlDB.Close()
		return nil, errors.New("database readiness check failed")
	}
	return db, nil
}

func EnsureDefaultAdmin(db *gorm.DB, username, password string) error {
	if username == "" {
		return errors.New("admin username is required")
	}
	var user models.User
	err := db.Where("username = ?", username).First(&user).Error
	if err == nil {
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("look up admin: %w", err)
	}
	if len(password) < 12 || len(password) > 72 {
		return errors.New("creating an admin requires a password containing 12 to 72 bytes")
	}
	hash, err := models.HashPassword(password)
	if err != nil {
		return err
	}
	return db.Create(&models.User{Username: username, Password: hash}).Error
}

func CleanupOldData(db *gorm.DB, retentionDays int) error {
	if retentionDays < 1 {
		return errors.New("retention must be positive")
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -retentionDays)

	dayCutoff := time.Now().UTC().AddDate(0, 0, 1-retentionDays).Format("2006-01-02")
	if err := deleteBatches(db, &models.PageView{}, "day < ?", dayCutoff); err != nil {
		return err
	}
	if err := deleteBatches(db, &models.PageVisit{}, "created_at < ?", cutoff); err != nil {
		return err
	}
	return deleteBatches(db, &models.Analytics{}, "created_at < ? AND id NOT IN (?)", cutoff, db.Model(&models.PageVisit{}).Select("analytics_id"))
}
func deleteBatches(db *gorm.DB, model any, condition string, args ...any) error {
	for {
		var ids []string
		if err := db.Model(model).Where(condition, args...).Limit(500).Pluck("id", &ids).Error; err != nil {
			return err
		}
		if len(ids) == 0 {
			return nil
		}
		if err := db.Where("id IN ?", ids).Delete(model).Error; err != nil {
			return err
		}
	}
}
