package auth

import (
	"errors"
	"testing"
)

// SK's seeded admin hash; canonical for the bootstrap "123456" password.
const seededAdminHash = "$2a$10$vZk9P3XtbD2KrdLbQYPvBuPAkkUda0OlkDg7io1Q6VEtfFPig/tqO"

func TestVerifyPassword_SeededAdmin(t *testing.T) {
	if err := VerifyPassword(seededAdminHash, "123456"); err != nil {
		t.Fatalf("admin/123456 should verify: %v", err)
	}
}

func TestVerifyPassword_BadPassword(t *testing.T) {
	err := VerifyPassword(seededAdminHash, "wrong")
	if !errors.Is(err, ErrBadCredentials) {
		t.Fatalf("want ErrBadCredentials, got %v", err)
	}
}

func TestVerifyPassword_EmptyInputs(t *testing.T) {
	for _, tc := range []struct{ hash, pw string }{
		{"", "x"}, {"x", ""}, {"", ""},
	} {
		if err := VerifyPassword(tc.hash, tc.pw); !errors.Is(err, ErrBadCredentials) {
			t.Fatalf("VerifyPassword(%q,%q) = %v, want ErrBadCredentials", tc.hash, tc.pw, err)
		}
	}
}

func TestHashAndVerifyRoundtrip(t *testing.T) {
	h, err := HashPassword("hunter2")
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyPassword(h, "hunter2"); err != nil {
		t.Fatalf("roundtrip verify: %v", err)
	}
	if err := VerifyPassword(h, "nope"); !errors.Is(err, ErrBadCredentials) {
		t.Fatalf("wrong password should fail; got %v", err)
	}
}
