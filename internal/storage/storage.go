// Package storage abstracts where uploaded files live. P2 ships only the
// local-disk backend; an S3/OSS backend can drop in alongside it later.
package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ErrNotFound is returned by Open when the key has no stored object.
var ErrNotFound = errors.New("storage: not found")

// Storage is the interface every backend implements.
type Storage interface {
	// Save streams content to the given key. Returns the canonical path
	// the backend stored it under (for Local that's the relative key, for
	// S3 it's the full s3:// URI).
	Save(ctx context.Context, key string, r io.Reader) (string, error)
	Open(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
	Kind() string // "local" | "s3" | ...
}

// Local stores objects under a single root directory.
type Local struct {
	root string
}

func NewLocal(root string) (*Local, error) {
	if root == "" {
		return nil, errors.New("local storage: root is empty")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir root: %w", err)
	}
	return &Local{root: abs}, nil
}

func (l *Local) Kind() string { return "local" }

// resolve produces a safe absolute path under root, refusing keys that
// escape the root via "..".
func (l *Local) resolve(key string) (string, error) {
	clean := filepath.Clean("/" + key)
	clean = strings.TrimPrefix(clean, "/")
	if clean == "" || strings.Contains(clean, "..") {
		return "", fmt.Errorf("storage: invalid key %q", key)
	}
	return filepath.Join(l.root, clean), nil
}

func (l *Local) Save(ctx context.Context, key string, r io.Reader) (string, error) {
	p, err := l.resolve(key)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return "", fmt.Errorf("mkdir: %w", err)
	}
	// Stream to a sibling temp file then atomically rename, so a copy
	// error or a crash can't leave a half-written object at the final key.
	tmp, err := os.CreateTemp(filepath.Dir(p), filepath.Base(p)+".tmp.*")
	if err != nil {
		return "", err
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpName) }
	if _, err := io.Copy(tmp, r); err != nil {
		_ = tmp.Close()
		cleanup()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return "", err
	}
	if err := os.Rename(tmpName, p); err != nil {
		cleanup()
		return "", fmt.Errorf("rename: %w", err)
	}
	return key, nil
}

func (l *Local) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	p, err := l.resolve(key)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(p)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotFound
	}
	return f, err
}

func (l *Local) Delete(ctx context.Context, key string) error {
	p, err := l.resolve(key)
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
