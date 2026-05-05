//go:build mysql

package e2e

import (
	"net/http"
	"testing"

	"github.com/ivmm/exam-run/tests/testenv"
)

// Verifies that the goose baseline applies cleanly to a fresh MySQL,
// the seeded admin row exists, and /readyz pings the DB.
func TestMySQLBaseline(t *testing.T) {
	db := testenv.MySQL(t)

	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM t_account WHERE auth_account = 'admin'`).Scan(&n); err != nil {
		t.Fatalf("query t_account: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 seeded admin in t_account, got %d", n)
	}

	s := testenv.NewServer(t, db)
	r := s.GET(t, "/readyz")
	if r.Status != http.StatusOK {
		t.Fatalf("readyz with db: status=%d body=%s", r.Status, r.Body)
	}
}
