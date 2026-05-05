//go:build pg

package e2e

import (
	"net/http"
	"testing"

	"github.com/web-casa/qooim/tests/testenv"
)

// Verifies that the goose baseline + admin seed apply cleanly to a fresh
// PostgreSQL database, the seeded admin row exists, and /readyz pings the DB.
func TestPostgresBaseline(t *testing.T) {
	db := testenv.Postgres(t)

	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM t_account WHERE auth_account = 'admin'`).Scan(&n); err != nil {
		t.Fatalf("query t_account: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 seeded admin in t_account, got %d", n)
	}

	var sysName string
	if err := db.QueryRow(`SELECT name FROM t_sys_info WHERE id = '1'`).Scan(&sysName); err != nil {
		t.Fatalf("query t_sys_info: %v", err)
	}
	if sysName != "Qoo.IM" {
		t.Fatalf("sys_info.name = %q, want %q", sysName, "Qoo.IM")
	}

	s := testenv.NewServer(t, db)
	r := s.GET(t, "/readyz")
	if r.Status != http.StatusOK {
		t.Fatalf("readyz with db: status=%d body=%s", r.Status, r.Body)
	}
}
