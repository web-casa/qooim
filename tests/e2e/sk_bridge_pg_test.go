//go:build pg

package e2e

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/web-casa/qooim/tests/testenv"
)

// rawGet bypasses testenv's auto-redirect-following client so we can
// observe a 302 response directly. The bridge returns 302 on auth
// failure; testenv's default client would silently follow to
// /console/login and we'd see the wrong status.
func rawGet(t *testing.T, s *testenv.Server, path string, headers ...[2]string) (int, string, string) {
	t.Helper()
	c := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.URL(path), nil)
	if err != nil {
		t.Fatalf("new req: %v", err)
	}
	for _, h := range headers {
		req.Header.Set(h[0], h[1])
	}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close()
	buf := make([]byte, 8192)
	n, _ := resp.Body.Read(buf)
	return resp.StatusCode, resp.Header.Get("Location"), string(buf[:n])
}

// TestSKBridgeAuthGate locks the bridge's auth requirement so a future
// refactor can't accidentally serve the page (and the embedded JWT)
// to anonymous callers.
func TestSKBridgeAuthGate(t *testing.T) {
	db := testenv.Postgres(t)
	s := testenv.NewServer(t, db)

	// 1. No cookie → redirect to login. Body must not contain a token.
	status, loc, body := rawGet(t, s, "/console/sk-bridge")
	if status != http.StatusFound {
		t.Fatalf("anon → expected 302, got %d", status)
	}
	if !strings.HasSuffix(loc, "/console/login") {
		t.Errorf("anon redirected to %q, want /console/login", loc)
	}
	if strings.Contains(body, "Bearer ") {
		t.Errorf("anon response body leaked a token")
	}

	// 2. Logged-in admin → 200, page contains a Bearer token.
	tok := login(t, s, "admin", "123456")
	status, _, body = rawGet(t, s, "/console/sk-bridge",
		[2]string{"Cookie", "qooim_console_session=" + tok})
	if status != http.StatusOK {
		t.Fatalf("authed → expected 200, got %d", status)
	}
	if !strings.Contains(body, "Bearer eyJ") {
		t.Errorf("authed response missing Bearer token")
	}
}

// TestSKBridgeOpenRedirect re-tests isSafeNext at the HTTP layer,
// not just the unit. The handler should coerce hostile `next` values
// to "/" rather than honouring them.
func TestSKBridgeOpenRedirect(t *testing.T) {
	db := testenv.Postgres(t)
	s := testenv.NewServer(t, db)
	tok := login(t, s, "admin", "123456")
	cookie := [2]string{"Cookie", "qooim_console_session=" + tok}

	cases := []struct {
		name string
		next string
		// destExpr is what the rendered `var dest = "..."` line should
		// contain after server-side sanitisation. "/" means the
		// dangerous value got coerced.
		destExpr string
	}{
		{"protocol_relative", "//evil.com", `var dest = "/"`},
		{"absolute_https", "https://evil.com", `var dest = "/"`},
		{"backslash", `/\evil.com`, `var dest = "/"`},
		{"javascript_scheme", "javascript:alert(1)", `var dest = "/"`},
		// Control chars in the URL must be percent-encoded — Go's
		// http.NewRequest rejects raw control bytes outright. We send
		// the percent-encoded form so the request actually leaves
		// the client; gin will decode it back before isSafeNext sees it.
		{"vertical_tab_encoded", "%2F%0B//evil.com", `var dest = "/"`},
		{"form_feed_encoded", "%2F%0C//evil.com", `var dest = "/"`},
		{"safe_path", "/admin/poster", `var dest = "/admin/poster"`},
		{"safe_with_query", "/admin?id=abc", `var dest = "/admin?id=abc"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, _, body := rawGet(t, s, "/console/sk-bridge?next="+tc.next, cookie)
			if status != http.StatusOK {
				t.Fatalf("status: %d", status)
			}
			if !strings.Contains(body, tc.destExpr) {
				// Print the actual rendered dest line so a regression
				// surfaces what the body looks like, not just "missing".
				idx := strings.Index(body, "var dest = ")
				if idx < 0 {
					t.Errorf("body has no `var dest =` line for next=%q", tc.next)
				} else {
					end := strings.IndexByte(body[idx:], '\n')
					if end < 0 {
						end = 80
					}
					t.Errorf("body missing %q for next=%q\n  actual: %s",
						tc.destExpr, tc.next, body[idx:idx+end])
				}
			}
		})
	}
}

// TestSKBridgeNonAdminBounced ensures a non-admin user with a valid
// console session can't reach the bridge — admin gate runs in
// requireAuth, applied to the whole authed group including bridge.
func TestSKBridgeNonAdminBounced(t *testing.T) {
	db := testenv.Postgres(t)
	s := testenv.NewServer(t, db)

	adminTok := login(t, s, "admin", "123456")
	adminBearer := [2]string{"Authorization", "Bearer " + adminTok}

	roleBody := mustJSON(t, map[string]any{
		"name":      "viewer-bridge",
		"code":      "viewer-bridge",
		"authority": "home",
		"status":    1,
	})
	r := s.POST(t, "/api/system/role/create", "application/json", roleBody, adminBearer)
	mustStatus(t, r, http.StatusOK, "role create")
	var role struct{ Data struct{ ID string } }
	r.JSON(t, &role)

	userBody := mustJSON(t, map[string]any{
		"username": "viewer-bridge",
		"password": "viewerpw1",
		"name":     "Viewer Bridge",
		"roleIds":  []string{role.Data.ID},
	})
	r = s.POST(t, "/api/system/user/create", "application/json", userBody, adminBearer)
	mustStatus(t, r, http.StatusOK, "user create")

	viewerTok := login(t, s, "viewer-bridge", "viewerpw1")
	status, _, body := rawGet(t, s, "/console/sk-bridge",
		[2]string{"Cookie", "qooim_console_session=" + viewerTok})
	if status != http.StatusForbidden {
		t.Errorf("viewer → expected 403, got %d (body=%s)", status, body)
	}
	if strings.Contains(body, "Bearer ") {
		t.Errorf("viewer response leaked a token")
	}
}
