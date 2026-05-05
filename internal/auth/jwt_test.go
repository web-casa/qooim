package auth

import (
	"testing"
	"time"
)

func TestIssuer_SignParseRoundtrip(t *testing.T) {
	i := NewIssuer("test-secret-32-bytes-long-padding!!", "qooim", time.Hour)
	tok, err := i.Sign(Principal{UserID: "u1", Username: "alice", Roles: []string{"admin", "user"}})
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	got, err := i.Parse(tok)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got.UserID != "u1" || got.Username != "alice" {
		t.Fatalf("principal mismatch: %+v", got)
	}
	if len(got.Roles) != 2 || got.Roles[0] != "admin" || got.Roles[1] != "user" {
		t.Fatalf("roles mismatch: %v", got.Roles)
	}
}

func TestIssuer_RejectsTamperedToken(t *testing.T) {
	i := NewIssuer("k1", "qooim", time.Hour)
	tok, err := i.Sign(Principal{UserID: "u1"})
	if err != nil {
		t.Fatal(err)
	}
	// Flip the final character — invalid signature.
	bad := tok[:len(tok)-1] + "A"
	if _, err := i.Parse(bad); err == nil {
		t.Fatal("expected parse error on tampered token")
	}
}

func TestIssuer_RejectsExpired(t *testing.T) {
	i := NewIssuer("k1", "qooim", -time.Hour) // already expired at sign time
	tok, err := i.Sign(Principal{UserID: "u1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := i.Parse(tok); err == nil {
		t.Fatal("expected parse error on expired token")
	}
}

func TestIssuer_RequiresSecret(t *testing.T) {
	i := NewIssuer("", "qooim", time.Hour)
	if _, err := i.Sign(Principal{UserID: "u1"}); err == nil {
		t.Fatal("Sign should require a secret")
	}
}
