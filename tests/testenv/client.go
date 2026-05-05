package testenv

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type Response struct {
	Status int
	Body   []byte
}

func (r Response) JSON(t *testing.T, dst any) {
	t.Helper()
	if err := json.Unmarshal(r.Body, dst); err != nil {
		t.Fatalf("decode json: %v\nbody: %s", err, r.Body)
	}
}

// GET issues a request and returns the Response. Failure is fatal.
func (s *Server) GET(t *testing.T, path string, headers ...[2]string) Response {
	t.Helper()
	return s.do(t, http.MethodGet, path, "", nil, headers)
}

func (s *Server) POST(t *testing.T, path, contentType string, body string, headers ...[2]string) Response {
	t.Helper()
	return s.do(t, http.MethodPost, path, contentType, strings.NewReader(body), headers)
}

func (s *Server) do(t *testing.T, method, path, contentType string, body io.Reader, headers [][2]string) Response {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, s.URL(path), body)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	for _, h := range headers {
		req.Header.Set(h[0], h[1])
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return Response{Status: resp.StatusCode, Body: b}
}
