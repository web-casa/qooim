//go:build pg

package e2e

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"

	"github.com/web-casa/qooim/tests/testenv"
)

// TestP2CRUD exercises every write path (create/get/update/soft-delete)
// for projects/repos/templates and the file upload/download/delete cycle.
// All routes require a JWT — we log in once with the seeded admin and
// reuse the bearer for everything.
func TestP2CRUD(t *testing.T) {
	db := testenv.Postgres(t)
	s := testenv.NewServer(t, db)

	tok := login(t, s, "admin", "123456")
	bearer := [2]string{"Authorization", "Bearer " + tok}

	t.Run("project", func(t *testing.T) {
		// Create.
		body := mustJSON(t, map[string]any{
			"name":     "alpha",
			"mode":     "survey",
			"priority": 100,
		})
		r := s.POST(t, "/api/projects", "application/json", body, bearer)
		mustStatus(t, r, http.StatusCreated, "POST /projects")
		var created struct{ ID string }
		r.JSON(t, &created)
		if created.ID == "" {
			t.Fatal("expected id from create")
		}

		// Get.
		r = s.GET(t, "/api/projects/"+created.ID, bearer)
		mustStatus(t, r, http.StatusOK, "GET /projects/:id")
		if !strings.Contains(string(r.Body), `"name":"alpha"`) {
			t.Fatalf("missing name in get body: %s", r.Body)
		}

		// Update name + status.
		r = s.Do(t, http.MethodPut, "/api/projects/"+created.ID, "application/json",
			strings.NewReader(mustJSON(t, map[string]any{"name": "alpha-2", "status": 1})),
			bearer)
		mustStatus(t, r, http.StatusNoContent, "PUT /projects/:id")

		r = s.GET(t, "/api/projects/"+created.ID, bearer)
		if !strings.Contains(string(r.Body), `"name":"alpha-2"`) {
			t.Fatalf("update did not persist: %s", r.Body)
		}

		// Soft delete.
		r = s.Do(t, http.MethodDelete, "/api/projects/"+created.ID, "", nil,
			bearer)
		mustStatus(t, r, http.StatusNoContent, "DELETE /projects/:id")

		r = s.GET(t, "/api/projects/"+created.ID, bearer)
		mustStatus(t, r, http.StatusNotFound, "GET after delete must 404")
	})

	t.Run("repo", func(t *testing.T) {
		body := mustJSON(t, map[string]any{"name": "main-repo", "mode": "survey"})
		r := s.POST(t, "/api/repos", "application/json", body, bearer)
		mustStatus(t, r, http.StatusCreated, "POST /repos")
		var created struct{ ID string }
		r.JSON(t, &created)

		r = s.GET(t, "/api/repos/"+created.ID, bearer)
		mustStatus(t, r, http.StatusOK, "GET /repos/:id")

		r = s.Do(t, http.MethodDelete, "/api/repos/"+created.ID, "", nil, bearer)
		mustStatus(t, r, http.StatusNoContent, "DELETE /repos/:id (hard)")

		r = s.GET(t, "/api/repos/"+created.ID, bearer)
		mustStatus(t, r, http.StatusNotFound, "GET after hard delete must 404")
	})

	t.Run("template", func(t *testing.T) {
		body := mustJSON(t, map[string]any{
			"name":          "Q1",
			"question_type": "Radio",
			"mode":          "survey",
			"template":      `{"options":["A","B"]}`,
		})
		r := s.POST(t, "/api/templates", "application/json", body, bearer)
		mustStatus(t, r, http.StatusCreated, "POST /templates")
		var created struct{ ID string }
		r.JSON(t, &created)

		r = s.Do(t, http.MethodDelete, "/api/templates/"+created.ID, "", nil, bearer)
		mustStatus(t, r, http.StatusNoContent, "DELETE /templates/:id")

		r = s.GET(t, "/api/templates/"+created.ID, bearer)
		mustStatus(t, r, http.StatusNotFound, "GET after soft-delete must 404")
	})

	t.Run("file_upload_download", func(t *testing.T) {
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		fw, err := mw.CreateFormFile("file", "hello.txt")
		if err != nil {
			t.Fatal(err)
		}
		_, _ = fw.Write([]byte("hello qooim"))
		_ = mw.Close()

		r := s.Do(t, http.MethodPost, "/api/files", mw.FormDataContentType(), &buf,
			bearer)
		mustStatus(t, r, http.StatusCreated, "POST /files")
		var up struct {
			ID           string `json:"id"`
			OriginalName string `json:"original_name"`
			FileName     string `json:"file_name"`
			FilePath     string `json:"file_path"`
		}
		r.JSON(t, &up)
		if up.ID == "" || up.OriginalName != "hello.txt" {
			t.Fatalf("unexpected upload result: %+v", up)
		}

		r = s.GET(t, "/api/files/"+up.ID, bearer)
		mustStatus(t, r, http.StatusOK, "GET /files/:id (download)")
		if !bytes.Equal(r.Body, []byte("hello qooim")) {
			t.Fatalf("downloaded content mismatch: %q", r.Body)
		}

		r = s.Do(t, http.MethodDelete, "/api/files/"+up.ID, "", nil, bearer)
		mustStatus(t, r, http.StatusNoContent, "DELETE /files/:id")
	})
}

func login(t *testing.T, s *testenv.Server, account, password string) string {
	t.Helper()
	body := `{"account":"` + account + `","password":"` + password + `"}`
	r := s.POST(t, "/api/auth/login", "application/json", body)
	if r.Status != http.StatusOK {
		t.Fatalf("login: status=%d body=%s", r.Status, r.Body)
	}
	var out struct{ Token string }
	r.JSON(t, &out)
	if out.Token == "" {
		t.Fatal("login returned empty token")
	}
	return out.Token
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(b)
}

func mustStatus(t *testing.T, r testenv.Response, want int, label string) {
	t.Helper()
	if r.Status != want {
		t.Fatalf("%s: status=%d want=%d body=%s", label, r.Status, want, r.Body)
	}
}
