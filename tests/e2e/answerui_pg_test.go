//go:build pg

package e2e

import (
	"net/http"
	"strings"
	"testing"

	"github.com/web-casa/qooim/tests/testenv"
)

// TestAnswerUIDemoMode locks the demo route's contract:
//
//   - Demo content only renders for the explicit pid="demo" + ?demo=1
//     pair. Random project IDs return 404 — no fixture leak.
//   - The demo page references both question types the spike covers
//     (radio + upload).
//
// We can't easily exercise the actual fetch() submit path from a Go
// test (it's a JS XHR), but we cover the contract surface that the
// JS depends on: the rendered HTML's structure.
func TestAnswerUIDemoMode(t *testing.T) {
	db := testenv.Postgres(t)
	// Default test env (env="test") has DemoMode on.
	s := testenv.NewServer(t, db)

	t.Run("demo_route_renders", func(t *testing.T) {
		r := s.GET(t, "/answerui/demo?demo=1")
		mustStatus(t, r, http.StatusOK, "demo page")
		body := string(r.Body)
		for _, want := range []string{
			"Qoo.IM Console 答题器 spike",
			`type="radio"`, // radio question
			`type="file"`,  // upload question
			"/api/public/saveAnswer",
			"/api/public/upload",
		} {
			if !strings.Contains(body, want) {
				t.Errorf("demo body missing %q", want)
			}
		}
	})

	t.Run("random_id_404s", func(t *testing.T) {
		// DemoMode + random pid + ?demo=1 → still 404 (must match pid="demo")
		r := s.GET(t, "/answerui/01abc?demo=1")
		mustStatus(t, r, http.StatusNotFound, "random id with demo flag")
	})

	t.Run("demo_pid_without_flag_404s", func(t *testing.T) {
		// pid="demo" but no ?demo=1 → not found, no leak.
		r := s.GET(t, "/answerui/demo")
		mustStatus(t, r, http.StatusNotFound, "demo pid without flag")
	})
}

// TestAnswerUIDemoOffInProd ensures the prod build (DemoMode=false)
// never serves the fixture, even with the magic pid + flag pair.
func TestAnswerUIDemoOffInProd(t *testing.T) {
	db := testenv.Postgres(t)
	s := testenv.NewServer(t, db, testenv.WithEnv("prod"))

	r := s.GET(t, "/answerui/demo?demo=1")
	mustStatus(t, r, http.StatusNotFound, "demo route with DemoMode=false")
	if strings.Contains(string(r.Body), "答题器 spike") {
		t.Errorf("prod build leaked demo content")
	}
}
