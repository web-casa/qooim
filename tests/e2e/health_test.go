package e2e

import (
	"net/http"
	"testing"

	"github.com/web-casa/qooim/tests/testenv"
)

func TestHealthz(t *testing.T) {
	s := testenv.NewServer(t, nil)

	r := s.GET(t, "/healthz")
	if r.Status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", r.Status, r.Body)
	}
	var body struct {
		Status string `json:"status"`
	}
	r.JSON(t, &body)
	if body.Status != "ok" {
		t.Fatalf("status field = %q, want %q", body.Status, "ok")
	}
}

func TestReadyzWithoutDB(t *testing.T) {
	s := testenv.NewServer(t, nil)
	r := s.GET(t, "/readyz")
	if r.Status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", r.Status, r.Body)
	}
}

func TestVersion(t *testing.T) {
	s := testenv.NewServer(t, nil)
	r := s.GET(t, "/api/version")
	if r.Status != http.StatusOK {
		t.Fatalf("status = %d, want 200", r.Status)
	}
	var body struct {
		Name    string `json:"name"`
		Env     string `json:"env"`
		Version string `json:"version"`
	}
	r.JSON(t, &body)
	if body.Name != "Qoo.IM" {
		t.Fatalf("name = %q, want %q", body.Name, "Qoo.IM")
	}
	if body.Env != "test" {
		t.Fatalf("env = %q, want %q", body.Env, "test")
	}
}
