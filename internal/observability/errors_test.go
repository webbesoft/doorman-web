package observability

import (
	mysql "github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/labstack/echo/v4"
	"strings"
	"testing"
)

func TestDriverDiagnosticsExcludeSensitiveMessages(t *testing.T) {
	for _, err := range []error{&pgconn.PgError{Code: "23505", Message: "private URL and password"}, &mysql.MySQLError{Number: 1045, Message: "private URL and password"}} {
		class := ErrorClass(echo.NewHTTPError(503).SetInternal(err))
		if strings.Contains(class, "private") || class == "internal_error" {
			t.Fatal("unsafe or missing diagnostic", class)
		}
	}
}
