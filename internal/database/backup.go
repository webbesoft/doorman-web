package database

import (
	"context"
	"database/sql"
	"errors"
	"gorm.io/gorm"
	"net/url"
	"os"
	"path/filepath"
)

// BackupSQLite writes a consistent snapshot without overwriting any existing file.
func BackupSQLite(ctx context.Context, db *gorm.DB, target string) error {
	if db.Dialector.Name() != "sqlite" {
		return errors.New("this backup command supports SQLite; use native backup tooling for the configured database")
	}
	absolute, err := filepath.Abs(target)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(absolute), 0700); err != nil {
		return err
	}
	file, err := os.OpenFile(absolute, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := db.WithContext(ctx).Exec("VACUUM INTO ?", absolute).Error; err != nil {
		os.Remove(absolute)
		return err
	}
	if err := VerifySQLiteBackup(ctx, absolute); err != nil {
		os.Remove(absolute)
		return err
	}
	return nil
}
func VerifySQLiteBackup(ctx context.Context, path string) error {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	u := url.URL{Scheme: "file", Path: absolute, RawQuery: "mode=ro&_query_only=true"}
	db, err := sql.Open("sqlite3", u.String())
	if err != nil {
		return err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	var result string
	if err := db.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&result); err != nil {
		return errors.New("backup could not be verified")
	}
	if result != "ok" {
		return errors.New("backup integrity check failed")
	}
	return nil
}
