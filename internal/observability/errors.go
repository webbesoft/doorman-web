package observability

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"net"
	"syscall"

	mysql "github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5/pgconn"
	sqlite "github.com/mattn/go-sqlite3"
)

// ErrorClass preserves actionable diagnostics without logging SQL, URLs, credentials, or driver messages.
func ErrorClass(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "deadline_exceeded"
	}
	if errors.Is(err, context.Canceled) {
		return "request_canceled"
	}
	if errors.Is(err, syscall.ENOSPC) {
		return "disk_full"
	}
	if errors.Is(err, driver.ErrBadConn) {
		return "database_connection_lost"
	}
	var sqliteError sqlite.Error
	if errors.As(err, &sqliteError) {
		return fmt.Sprintf("sqlite_%d_%d", sqliteError.Code, sqliteError.ExtendedCode)
	}
	var pgError *pgconn.PgError
	if errors.As(err, &pgError) {
		return "postgres_" + pgError.Code
	}
	var mysqlError *mysql.MySQLError
	if errors.As(err, &mysqlError) {
		return fmt.Sprintf("mysql_%d", mysqlError.Number)
	}
	var networkError net.Error
	if errors.As(err, &networkError) {
		if networkError.Timeout() {
			return "network_timeout"
		}
		return "network_error"
	}
	return "internal_error"
}
