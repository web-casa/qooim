package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/spf13/cobra"
)

func newHealthCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "health",
		Short: "Probe /healthz and /readyz on the server",
		RunE: func(cmd *cobra.Command, _ []string) error {
			base, _ := cmd.Flags().GetString("server")
			ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
			defer cancel()
			for _, path := range []string{"/healthz", "/readyz"} {
				body, status, err := httpGet(ctx, base+path)
				if err != nil {
					return fmt.Errorf("%s: %w", path, err)
				}
				fmt.Printf("%s -> %d %s\n", path, status, body)
			}
			return nil
		},
	}
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print server /api/version (and skctl's own build version)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			fmt.Printf("skctl: %s\n", Version)
			base, _ := cmd.Flags().GetString("server")
			ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
			defer cancel()
			body, _, err := httpGet(ctx, base+"/api/version")
			if err != nil {
				return fmt.Errorf("server: %w", err)
			}
			var v map[string]any
			if err := json.Unmarshal([]byte(body), &v); err == nil {
				fmt.Printf("server: %v\n", v)
			} else {
				fmt.Printf("server: %s\n", body)
			}
			return nil
		},
	}
}

func httpGet(ctx context.Context, url string) (string, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", 0, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", resp.StatusCode, err
	}
	return string(b), resp.StatusCode, nil
}
