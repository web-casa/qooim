package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func newLoginCmd() *cobra.Command {
	var account, password string
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate against the server and persist the JWT",
		RunE: func(cmd *cobra.Command, _ []string) error {
			base, _ := cmd.Flags().GetString("server")
			if account == "" {
				return fmt.Errorf("--account is required")
			}
			if password == "" {
				p, err := readPassword()
				if err != nil {
					return err
				}
				password = p
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
			defer cancel()

			cli := newClient(base, "")
			var res struct {
				Token     string `json:"token"`
				Principal struct {
					UserID   string   `json:"uid"`
					Username string   `json:"name"`
					Roles    []string `json:"roles"`
				} `json:"principal"`
			}
			if err := cli.post(ctx, "/api/auth/login", map[string]string{
				"account":  account,
				"password": password,
			}, &res); err != nil {
				return err
			}
			if err := (tokenStore{}).save(res.Token); err != nil {
				return fmt.Errorf("save token: %w", err)
			}
			fmt.Printf("logged in as %s (uid=%s, roles=%v)\n",
				res.Principal.Username, res.Principal.UserID, res.Principal.Roles)
			return nil
		},
	}
	cmd.Flags().StringVarP(&account, "account", "a", "", "login account (required)")
	cmd.Flags().StringVarP(&password, "password", "p", "", "password (prompted if omitted)")
	return cmd
}

func newLogoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Forget the persisted JWT",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := (tokenStore{}).clear(); err != nil {
				return err
			}
			fmt.Println("logged out")
			return nil
		},
	}
}

func newMeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "me",
		Short: "Print the current user (GET /api/me)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			base, _ := cmd.Flags().GetString("server")
			tok := tokenStore{}.load()
			if tok == "" {
				return fmt.Errorf("not logged in; run `qooim login`")
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), 10*time.Second)
			defer cancel()
			cli := newClient(base, tok)
			var res any
			if err := cli.get(ctx, "/api/me", nil, &res); err != nil {
				return err
			}
			b, _ := json.MarshalIndent(res, "", "  ")
			fmt.Println(string(b))
			return nil
		},
	}
}

func readPassword() (string, error) {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return "", fmt.Errorf("--password is required when stdin is not a terminal")
	}
	fmt.Fprint(os.Stderr, "Password: ")
	b, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
