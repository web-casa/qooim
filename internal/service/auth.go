// Package service holds the business-logic layer that sits between HTTP
// handlers and the sqlc-generated repo. P1 only has the auth flow + four
// read-only list services here.
package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/web-casa/qooim/internal/auth"
	"github.com/web-casa/qooim/internal/repo/db"
)

// AuthService verifies credentials and mints JWTs.
type AuthService struct {
	q      db.Querier
	issuer *auth.Issuer
}

func NewAuthService(q db.Querier, issuer *auth.Issuer) *AuthService {
	return &AuthService{q: q, issuer: issuer}
}

// LoginResult is what the HTTP handler renders to the client.
type LoginResult struct {
	Token     string         `json:"token"`
	Principal auth.Principal `json:"principal"`
}

// Login checks the password and returns a signed JWT with the user's roles.
// On invalid credentials it returns auth.ErrBadCredentials so callers can
// decide what to log without leaking detail to the client.
func (s *AuthService) Login(ctx context.Context, account, password string) (*LoginResult, error) {
	a, err := s.q.GetAccountByLogin(ctx, db.GetAccountByLoginParams{
		AuthAccount: account,
		AuthType:    "PWD",
	})
	if err != nil {
		// sql.ErrNoRows from sqlc surfaces here; map it to bad creds so
		// "user does not exist" and "wrong password" look identical to
		// the client.
		return nil, auth.ErrBadCredentials
	}
	if !a.AuthSecret.Valid {
		return nil, auth.ErrBadCredentials
	}
	if err := auth.VerifyPassword(a.AuthSecret.String, password); err != nil {
		return nil, err
	}

	user, err := s.q.GetUserByID(ctx, a.UserID)
	if err != nil {
		return nil, fmt.Errorf("load user: %w", err)
	}

	codes, err := s.q.ListRoleCodesByUser(ctx, a.UserID)
	if err != nil {
		return nil, fmt.Errorf("load roles: %w", err)
	}

	username := user.Name
	p := auth.Principal{
		UserID:   user.ID,
		Username: username,
		Roles:    codes,
	}
	token, err := s.issuer.Sign(p)
	if err != nil {
		return nil, fmt.Errorf("sign jwt: %w", err)
	}
	return &LoginResult{Token: token, Principal: p}, nil
}

// MeResult is returned by GET /api/me.
type MeResult struct {
	Principal auth.Principal `json:"principal"`
	Email     string         `json:"email,omitempty"`
	Avatar    string         `json:"avatar,omitempty"`
	Profile   string         `json:"profile,omitempty"`
}

func (s *AuthService) Me(ctx context.Context, p auth.Principal) (*MeResult, error) {
	user, err := s.q.GetUserByID(ctx, p.UserID)
	if err != nil {
		return nil, fmt.Errorf("load user: %w", err)
	}
	out := &MeResult{Principal: p}
	if user.Email.Valid {
		out.Email = user.Email.String
	}
	if user.Avatar.Valid {
		out.Avatar = user.Avatar.String
	}
	if user.Profile.Valid {
		out.Profile = user.Profile.String
	}
	return out, nil
}

// IsBadCredentials reports whether the error came from a failed login attempt.
func IsBadCredentials(err error) bool { return errors.Is(err, auth.ErrBadCredentials) }
