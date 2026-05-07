//go:build pg

package e2e

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/web-casa/qooim/tests/testenv"
)

// TestFileServingHeaders pins the response headers from /api/file?id=
// after the Gate-4 hardening landed:
//
//   - X-Content-Type-Options: nosniff is ALWAYS set (defence in depth
//     against an unanticipated MIME being sniffed past the declared
//     Content-Type).
//   - shouldForceDownload() returns Content-Disposition: attachment
//     for non-image/non-pdf/non-audio/non-video types. Images and PDFs
//     remain inline so the answer page can preview them.
func TestFileServingHeaders(t *testing.T) {
	db := testenv.Postgres(t)
	s := testenv.NewServer(t, db)

	upload := func(filename string, body []byte) string {
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		fw, _ := mw.CreateFormFile("file", filename)
		_, _ = fw.Write(body)
		_ = mw.Close()
		r := s.Do(t, http.MethodPost, "/api/public/upload", mw.FormDataContentType(), &buf)
		mustStatus(t, r, http.StatusOK, "upload "+filename)
		var resp struct {
			Data struct{ ID string } `json:"data"`
		}
		r.JSON(t, &resp)
		if resp.Data.ID == "" {
			t.Fatalf("missing id in upload response")
		}
		return resp.Data.ID
	}

	rawHead := func(t *testing.T, path string) http.Header {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, s.URL(path), nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("get %s: %v", path, err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("unexpected status %d on %s", resp.StatusCode, path)
		}
		return resp.Header
	}

	t.Run("png_inline_with_nosniff", func(t *testing.T) {
		id := upload("pic.png", []byte("\x89PNG\r\n\x1a\n"))
		h := rawHead(t, "/api/file?id="+id)
		if got := h.Get("X-Content-Type-Options"); got != "nosniff" {
			t.Errorf("X-Content-Type-Options = %q, want nosniff", got)
		}
		if got := h.Get("Content-Disposition"); got != "" {
			t.Errorf("png served with Content-Disposition=%q (should be inline)", got)
		}
		if !strings.HasPrefix(h.Get("Content-Type"), "image/") {
			t.Errorf("png Content-Type=%q, want image/*", h.Get("Content-Type"))
		}
	})

	t.Run("pdf_inline_with_nosniff", func(t *testing.T) {
		id := upload("doc.pdf", []byte("%PDF-1.4\n"))
		h := rawHead(t, "/api/file?id="+id)
		if got := h.Get("X-Content-Type-Options"); got != "nosniff" {
			t.Errorf("nosniff missing on pdf")
		}
		if got := h.Get("Content-Disposition"); got != "" {
			t.Errorf("pdf served with Content-Disposition=%q (should be inline)", got)
		}
	})

	t.Run("txt_force_download_with_nosniff", func(t *testing.T) {
		id := upload("notes.txt", []byte("hello"))
		h := rawHead(t, "/api/file?id="+id)
		if got := h.Get("X-Content-Type-Options"); got != "nosniff" {
			t.Errorf("nosniff missing on txt")
		}
		if !strings.HasPrefix(h.Get("Content-Disposition"), "attachment") {
			t.Errorf("txt should force-download; got Content-Disposition=%q", h.Get("Content-Disposition"))
		}
	})
}
