package auth

import (
	"errors"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"github.com/web-casa/qooim/internal/httpx"
)

const ContextKey = "auth.principal"

type Principal struct {
	UserID   string   `json:"uid"`
	Username string   `json:"name"`
	Roles    []string `json:"roles"`
}

type Claims struct {
	Principal
	jwt.RegisteredClaims
}

type Issuer struct {
	secret    []byte
	issuer    string
	expiresIn time.Duration
}

func NewIssuer(secret, issuer string, expiresIn time.Duration) *Issuer {
	return &Issuer{secret: []byte(secret), issuer: issuer, expiresIn: expiresIn}
}

func (i *Issuer) Sign(p Principal) (string, error) {
	return i.SignWithTTL(p, i.expiresIn)
}

// SignWithTTL is like Sign but lets callers override the token's
// lifetime. The console uses this so the bearer it writes into
// localStorage (via the SK bridge) can't outlive the cookie that
// minted it. The default Sign() retains the issuer-wide TTL.
func (i *Issuer) SignWithTTL(p Principal, ttl time.Duration) (string, error) {
	if len(i.secret) == 0 {
		return "", errors.New("jwt secret not configured")
	}
	now := time.Now()
	claims := Claims{
		Principal: p,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    i.issuer,
			Subject:   p.UserID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return tok.SignedString(i.secret)
}

func (i *Issuer) Parse(token string) (*Principal, error) {
	if len(i.secret) == 0 {
		return nil, errors.New("jwt secret not configured")
	}
	claims := &Claims{}
	_, err := jwt.ParseWithClaims(token, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return i.secret, nil
	})
	if err != nil {
		return nil, err
	}
	return &claims.Principal, nil
}

// Middleware extracts and verifies a Bearer token.
// On success, the *Principal is stashed in the gin context under ContextKey.
// On failure, responds with 401.
func (i *Issuer) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		raw := c.GetHeader("Authorization")
		if raw == "" {
			httpx.Unauthorized(c, "missing Authorization header")
			return
		}
		parts := strings.SplitN(raw, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			httpx.Unauthorized(c, "invalid Authorization header")
			return
		}
		p, err := i.Parse(parts[1])
		if err != nil {
			httpx.Unauthorized(c, "invalid or expired token")
			return
		}
		c.Set(ContextKey, p)
		c.Next()
	}
}

// FromContext retrieves the authenticated principal stored by Middleware.
func FromContext(c *gin.Context) (*Principal, bool) {
	v, ok := c.Get(ContextKey)
	if !ok {
		return nil, false
	}
	p, ok := v.(*Principal)
	return p, ok
}
