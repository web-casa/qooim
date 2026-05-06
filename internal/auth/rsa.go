package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"sync"
)

// LoginKeyPair holds the RSA keys used by the SK login form. The
// frontend bundle pulls the public key from /api/system.publicKey,
// RSA-encrypts the password with it, and we decrypt server-side
// before the bcrypt compare. The keypair is generated once per
// process start; tokens minted under it are short-lived (24h JWT)
// so a process restart that rotates the key is safe.
type LoginKeyPair struct {
	priv      *rsa.PrivateKey
	publicB64 string
}

var (
	defaultPair     *LoginKeyPair
	defaultPairOnce sync.Once
	defaultPairErr  error
)

// DefaultLoginKeyPair returns a process-wide singleton.
func DefaultLoginKeyPair() (*LoginKeyPair, error) {
	defaultPairOnce.Do(func() {
		defaultPair, defaultPairErr = NewLoginKeyPair(1024)
	})
	return defaultPair, defaultPairErr
}

// NewLoginKeyPair generates a fresh RSA keypair. SK uses 1024-bit;
// not strong enough for crown-jewel data, but matches the legacy
// frontend's hard-coded fallback so we stay bug-compatible. The
// password the form sends is never longer than ~120 bytes so the
// PKCS#1 v1.5 padding fits easily.
func NewLoginKeyPair(bits int) (*LoginKeyPair, error) {
	priv, err := rsa.GenerateKey(rand.Reader, bits)
	if err != nil {
		return nil, fmt.Errorf("rsa.GenerateKey: %w", err)
	}
	pubDER, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("marshal public: %w", err)
	}
	return &LoginKeyPair{
		priv:      priv,
		publicB64: base64.StdEncoding.EncodeToString(pubDER),
	}, nil
}

// PublicKeyB64 returns the X.509-encoded public key as base64,
// matching the SK frontend's expectation (the JSEncrypt instance is
// fed PEM-stripped base64).
func (k *LoginKeyPair) PublicKeyB64() string { return k.publicB64 }

// Decrypt undoes the frontend's RSA-PKCS1v15 encryption of the login
// password. Returns the plain string. Rejects empty input.
func (k *LoginKeyPair) Decrypt(ciphertextB64 string) (string, error) {
	if ciphertextB64 == "" {
		return "", errors.New("empty ciphertext")
	}
	ct, err := base64.StdEncoding.DecodeString(ciphertextB64)
	if err != nil {
		return "", fmt.Errorf("base64: %w", err)
	}
	plain, err := rsa.DecryptPKCS1v15(rand.Reader, k.priv, ct)
	if err != nil {
		return "", fmt.Errorf("rsa decrypt: %w", err)
	}
	return string(plain), nil
}

// LooksLikeRSACiphertext is a cheap heuristic: a base64 blob whose
// decoded length matches the modulus size (~128 bytes for 1024-bit).
// We use this to decide whether to even try Decrypt — plaintext
// passwords typed via curl shouldn't trigger an attempt that produces
// nonsense.
func LooksLikeRSACiphertext(s string) bool {
	if len(s) < 100 || len(s) > 1024 {
		return false
	}
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return false
	}
	// 1024-bit RSA → 128 bytes; allow 2048 too just in case.
	return len(b) == 128 || len(b) == 256
}
