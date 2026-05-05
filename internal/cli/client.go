package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// httpClient is a thin wrapper around http.Client that handles base URL,
// auth token, JSON encoding, and surfaces server error bodies as Go errors.
type httpClient struct {
	base  string
	token string
	c     *http.Client
}

func newClient(base, token string) *httpClient {
	return &httpClient{
		base:  strings.TrimRight(base, "/"),
		token: token,
		c:     &http.Client{Timeout: 30 * time.Second},
	}
}

func (h *httpClient) get(ctx context.Context, path string, query url.Values, dst any) error {
	full := h.base + path
	if len(query) > 0 {
		full += "?" + query.Encode()
	}
	return h.do(ctx, http.MethodGet, full, nil, dst)
}

func (h *httpClient) post(ctx context.Context, path string, body, dst any) error {
	var buf io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal body: %w", err)
		}
		buf = bytes.NewReader(b)
	}
	return h.do(ctx, http.MethodPost, h.base+path, buf, dst)
}

func (h *httpClient) do(ctx context.Context, method, url string, body io.Reader, dst any) error {
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if h.token != "" {
		req.Header.Set("Authorization", "Bearer "+h.token)
	}
	resp, err := h.c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		var apiErr struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		}
		_ = json.Unmarshal(raw, &apiErr)
		if apiErr.Message != "" {
			return fmt.Errorf("%s (%d %s)", apiErr.Message, resp.StatusCode, apiErr.Code)
		}
		return fmt.Errorf("http %d: %s", resp.StatusCode, string(raw))
	}
	if dst == nil {
		return nil
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		return fmt.Errorf("decode response: %w (body=%s)", err, raw)
	}
	return nil
}

// tokenStore handles persisting the JWT to ~/.config/qooim/token. It also
// honours the QOOIM_TOKEN env var so CI / scripts can inject a token without
// touching the filesystem.
type tokenStore struct{}

func (tokenStore) load() string {
	if t := os.Getenv("QOOIM_TOKEN"); t != "" {
		return t
	}
	p, err := tokenPath()
	if err != nil {
		return ""
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func (tokenStore) save(token string) error {
	p, err := tokenPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	return os.WriteFile(p, []byte(token), 0o600)
}

func (tokenStore) clear() error {
	p, err := tokenPath()
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func tokenPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "qooim", "token"), nil
}
