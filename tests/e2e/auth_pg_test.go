//go:build pg

package e2e

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/web-casa/qooim/tests/testenv"
)

// TestAuthAndLists exercises the full P1 read path against a fresh PG:
// login with admin/123456, /me with the resulting token, and each list
// endpoint. All four lists should be empty (we don't seed demo content)
// but must return a well-formed pagination envelope.
func TestAuthAndLists(t *testing.T) {
	db := testenv.Postgres(t)
	s := testenv.NewServer(t, db)

	// Bad creds first — must not leak existence.
	bad := s.POST(t, "/api/auth/login", "application/json",
		`{"account":"admin","password":"wrong"}`)
	if bad.Status != http.StatusUnauthorized {
		t.Fatalf("bad creds: status=%d body=%s", bad.Status, bad.Body)
	}

	// Good creds.
	r := s.POST(t, "/api/auth/login", "application/json",
		`{"account":"admin","password":"123456"}`)
	if r.Status != http.StatusOK {
		t.Fatalf("login: status=%d body=%s", r.Status, r.Body)
	}
	var login struct {
		Token     string `json:"token"`
		Principal struct {
			UserID   string   `json:"uid"`
			Username string   `json:"name"`
			Roles    []string `json:"roles"`
		} `json:"principal"`
	}
	r.JSON(t, &login)
	if login.Token == "" {
		t.Fatal("expected non-empty token")
	}
	if login.Principal.Username != "Admin" {
		t.Fatalf("principal.name = %q, want Admin", login.Principal.Username)
	}
	if len(login.Principal.Roles) != 1 || login.Principal.Roles[0] != "admin" {
		t.Fatalf("roles = %v, want [admin]", login.Principal.Roles)
	}

	bearer := [2]string{"Authorization", "Bearer " + login.Token}

	// /me — protected route, with token.
	me := s.GET(t, "/api/me", bearer)
	if me.Status != http.StatusOK {
		t.Fatalf("/me: status=%d body=%s", me.Status, me.Body)
	}

	// /me — protected route, without token.
	noTok := s.GET(t, "/api/me")
	if noTok.Status != http.StatusUnauthorized {
		t.Fatalf("/me without token: status=%d", noTok.Status)
	}

	// All four list endpoints empty + well-formed envelope.
	for _, path := range []string{"/api/projects", "/api/repos", "/api/templates", "/api/dashboards"} {
		r := s.GET(t, path, bearer)
		if r.Status != http.StatusOK {
			t.Fatalf("%s: status=%d body=%s", path, r.Status, r.Body)
		}
		var env struct {
			Items    []json.RawMessage `json:"items"`
			Total    int               `json:"total"`
			Page     int               `json:"page"`
			PageSize int               `json:"page_size"`
		}
		r.JSON(t, &env)
		if env.Page != 1 || env.PageSize != 20 {
			t.Fatalf("%s: page/page_size = %d/%d", path, env.Page, env.PageSize)
		}
		if len(env.Items) != 0 || env.Total != 0 {
			t.Fatalf("%s: expected empty list, got items=%d total=%d", path, len(env.Items), env.Total)
		}
	}

	// Pagination flags propagate.
	rp := s.GET(t, "/api/projects?page=2&page_size=5", bearer)
	if rp.Status != http.StatusOK {
		t.Fatalf("paginated: status=%d", rp.Status)
	}
	var paged struct {
		Page     int `json:"page"`
		PageSize int `json:"page_size"`
	}
	rp.JSON(t, &paged)
	if paged.Page != 2 || paged.PageSize != 5 {
		t.Fatalf("paginated query echo: page=%d page_size=%d", paged.Page, paged.PageSize)
	}
}
