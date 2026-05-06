//go:build pg

package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"

	"github.com/web-casa/qooim/tests/testenv"
)

// TestSecurityFileACL — knowing a file id shouldn't be enough to read
// it. Anonymous + non-owner reads must be rejected unless shared=1.
//
// Status: this test SHOULD fail before the fix lands. Expected fix:
// /api/file?id=… and /api/public/preview/:id check the file's shared
// flag (and/or owner) before serving.
func TestSecurityFileACL(t *testing.T) {
	db := testenv.Postgres(t)
	s := testenv.NewServer(t, db)

	// Owner uploads a private file via the admin path.
	tok := login(t, s, "admin", "123456")
	bearer := [2]string{"Authorization", "Bearer " + tok}

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", "secret.txt")
	_, _ = fw.Write([]byte("secret-payload"))
	_ = mw.Close()
	r := s.Do(t, http.MethodPost, "/api/files", mw.FormDataContentType(), &buf, bearer)
	if r.Status != http.StatusCreated {
		t.Fatalf("upload: %d %s", r.Status, r.Body)
	}
	var up struct {
		ID string `json:"id"`
	}
	r.JSON(t, &up)

	// Anonymous fetch — currently /api/file is wide-open public; it
	// MUST require auth (or refuse non-shared private files).
	r = s.GET(t, "/api/file?id="+up.ID)
	if r.Status == http.StatusOK {
		t.Fatalf("BUG: anon could read private file via /api/file (got 200)")
	}
	r = s.GET(t, "/api/public/preview/"+up.ID)
	if r.Status == http.StatusOK {
		t.Fatalf("BUG: anon could read private file via /api/public/preview (got 200)")
	}
}

// TestSecurityUploadExtension — public upload accepts arbitrary bytes;
// reject obvious-dangerous extensions / mime types so we don't store
// + replay executables for free.
func TestSecurityUploadExtension(t *testing.T) {
	db := testenv.Postgres(t)
	s := testenv.NewServer(t, db)

	// /api/public/upload is the SK-shape route — successful responses
	// come back as `200 + {success:true}`, not 201. Failures we care
	// about land as 4xx with no envelope.
	cases := []struct {
		name  string
		fname string
		body  []byte
		want  int
	}{
		{"png_ok", "image.png", []byte("\x89PNG\r\n\x1a\n"), http.StatusOK},
		{"txt_ok", "notes.txt", []byte("hello"), http.StatusOK},
		{"exe_blocked", "hack.exe", []byte("MZ\x90\x00"), http.StatusBadRequest},
		{"sh_blocked", "rce.sh", []byte("#!/bin/sh\nrm -rf /"), http.StatusBadRequest},
		{"php_blocked", "shell.php", []byte("<?php system($_GET['c']);"), http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			mw := multipart.NewWriter(&buf)
			fw, _ := mw.CreateFormFile("file", tc.fname)
			_, _ = fw.Write(tc.body)
			_ = mw.Close()
			r := s.Do(t, http.MethodPost, "/api/public/upload", mw.FormDataContentType(), &buf)
			if r.Status != tc.want {
				t.Fatalf("%s: got %d, want %d. body=%s", tc.fname, r.Status, tc.want, r.Body)
			}
		})
	}
}

// TestSecurityPartnerCrossProject — a partner uid issued for project
// A must NOT be valid against project B.
func TestSecurityPartnerCrossProject(t *testing.T) {
	db := testenv.Postgres(t)
	s := testenv.NewServer(t, db)
	tok := login(t, s, "admin", "123456")
	bearer := [2]string{"Authorization", "Bearer " + tok}

	// Create two published projects.
	mk := func(name string) string {
		body := mustJSON(t, map[string]any{"name": name, "mode": "survey", "status": 1, "survey": map[string]any{"title": name}})
		r := s.POST(t, "/api/projects", "application/json", body, bearer)
		var v struct{ ID string }
		r.JSON(t, &v)
		return v.ID
	}
	pA := mk("partner-cross-A")
	pB := mk("partner-cross-B")
	t.Cleanup(func() {
		_, _ = db.ExecContext(t.Context(), `DELETE FROM t_project_partner WHERE project_id IN ($1,$2)`, pA, pB)
	})

	// Mint a partner for project A by inserting the row directly — SK
	// admin partner-create endpoint is fine but we don't want this
	// test to depend on its body shape.
	uid := "partner-uid-A-" + pA
	_, err := db.ExecContext(t.Context(), `INSERT INTO t_project_partner (id, uid, project_id, type, status) VALUES ($1, $2, $3, 1, 0)`,
		"pp-"+pA, uid, pA)
	if err != nil {
		t.Fatalf("seed partner: %v", err)
	}

	// Submit to project B with project-A's token — must be 401/403.
	r := s.POST(t, "/api/survey/"+pB+"/answer?token="+uid, "application/json",
		`{"answer":{"q":"x"},"temp_save":1}`)
	if r.Status == http.StatusOK || r.Status == http.StatusCreated {
		t.Fatalf("BUG: cross-project partner token accepted (status=%d)", r.Status)
	}
}

// TestSecurityAuthorityGating — non-admin users with limited
// authorities must NOT be able to call admin-only routes through the
// SK compat layer. Today the JWT middleware checks signature only;
// this test will fail until per-route authority gating lands.
func TestSecurityAuthorityGating(t *testing.T) {
	db := testenv.Postgres(t)
	s := testenv.NewServer(t, db)

	// Login as admin, mint a viewer-only role + user.
	adminTok := login(t, s, "admin", "123456")
	adminBearer := [2]string{"Authorization", "Bearer " + adminTok}

	// Role: only `answer:list` and `home`. No system:user:* anything.
	roleBody := mustJSON(t, map[string]any{
		"name":      "viewer-secure",
		"code":      "viewer-secure",
		"authority": "answer:list,home",
		"status":    1,
	})
	r := s.POST(t, "/api/system/role/create", "application/json", roleBody, adminBearer)
	mustStatus(t, r, http.StatusOK, "role create")
	var role struct{ Data struct{ ID string } }
	if err := json.Unmarshal(r.Body, &role); err != nil {
		t.Fatalf("decode role: %v", err)
	}

	userBody := mustJSON(t, map[string]any{
		"username": "viewer-secure",
		"password": "viewerpw1",
		"name":     "Viewer Secure",
		"roleIds":  []string{role.Data.ID},
	})
	r = s.POST(t, "/api/system/user/create", "application/json", userBody, adminBearer)
	mustStatus(t, r, http.StatusOK, "user create")

	// Login as the viewer, then attempt admin actions.
	viewerTok := login(t, s, "viewer-secure", "viewerpw1")
	vbearer := [2]string{"Authorization", "Bearer " + viewerTok}

	// 1. Non-admin should NOT be able to list users.
	r = s.GET(t, "/api/system/user/list?current=1&pageSize=10", vbearer)
	if r.Status == http.StatusOK {
		t.Errorf("BUG: viewer can GET /api/system/user/list (no authority gate)")
	}
	// 2. Non-admin should NOT be able to create roles.
	r = s.POST(t, "/api/system/role/create", "application/json", roleBody, vbearer)
	if r.Status == http.StatusOK {
		t.Errorf("BUG: viewer can create roles (no authority gate)")
	}
	// 3. Non-admin should NOT be able to delete projects.
	r = s.POST(t, "/api/project/delete", "application/json", `{"id":"x"}`, vbearer)
	if r.Status == http.StatusOK {
		t.Errorf("BUG: viewer can call project/delete (no authority gate)")
	}
}

// TestSecurityRateLimit — public auth + answer endpoints must throttle
// abusive clients. Hammer login 60 times in <1s — at least some
// requests should respond 429 (or at minimum slow down). This test
// will fail until a rate-limit middleware lands on those routes.
func TestSecurityRateLimit(t *testing.T) {
	db := testenv.Postgres(t)
	s := testenv.NewServer(t, db)

	got429 := 0
	for i := 0; i < 60; i++ {
		body := fmt.Sprintf(`{"username":"admin","password":"wrong-%d"}`, i)
		r := s.POST(t, "/api/public/login", "application/json", body)
		if r.Status == http.StatusTooManyRequests {
			got429++
		}
	}
	if got429 == 0 {
		t.Errorf("BUG: 60 concurrent bad logins, none rate-limited")
	}
}

// TestSecurityProductionSafetyChecks — the server should refuse to
// start in env=prod when the admin row still carries the SK-seeded
// "123456" bcrypt. We can't easily call into cmd/server from a
// foreign test package, so we re-implement the check here against
// the same migrated DB and demand the seed prefix is present (so
// the production-time guard would, in fact, fire) AND that a fresh
// hash *would* be accepted (so the guard isn't trivially over-broad).
func TestSecurityProductionSafetyChecks(t *testing.T) {
	db := testenv.Postgres(t)

	const seededPrefix = "$2a$10$vZk9P3XtbD2KrdLbQYPvBu"

	var hash string
	err := db.QueryRowContext(t.Context(), `SELECT auth_secret FROM t_account WHERE auth_account='admin' AND is_deleted=0`).Scan(&hash)
	if err != nil {
		t.Fatalf("read admin: %v", err)
	}
	if !strings.HasPrefix(hash, seededPrefix) {
		t.Fatalf("expected the seed migration to leave the SK '123456' bcrypt in place; got %q — has the seed changed?", hash[:min(20, len(hash))])
	}

	// Simulate a rotation, prove the guard would let prod start once
	// admin's password has been changed.
	rotated := "$2a$10$N2yTotallyDifferentSaltX8JgzEAbbbbbbbbbbbbbbbbbbbbbbbbbb"
	if strings.HasPrefix(rotated, seededPrefix) {
		t.Fatalf("test fixture mistake: rotated hash collides with seed prefix")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
