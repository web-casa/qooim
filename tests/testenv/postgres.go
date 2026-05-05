//go:build pg

// Postgres fixture for integration tests. Builds only with `-tags pg` so
// the default `go test ./...` stays fast and Docker-free.
//
// Two ways to run:
//
//	# spin up a fresh container via dockertest (needs Docker)
//	go test -tags pg ./tests/...
//
//	# point at an existing PG (e.g. the remote test DB)
//	QOOIM_TEST_DSN='postgresql://user:pass@host:port/db?sslmode=disable' \
//	  go test -tags pg ./tests/...
package testenv

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/ory/dockertest/v3"
	dc "github.com/ory/dockertest/v3/docker"
	"github.com/pressly/goose/v3"
)

// Postgres returns a *sql.DB pointing at a freshly-migrated PG instance.
// If QOOIM_TEST_DSN is set, that database is used (and reset before each test
// run). Otherwise dockertest spins up postgres:18-alpine.
func Postgres(t *testing.T) *sql.DB {
	t.Helper()

	if dsn := os.Getenv("QOOIM_TEST_DSN"); dsn != "" {
		return openExternal(t, dsn)
	}
	return openDocker(t)
}

func openExternal(t *testing.T, dsn string) *sql.DB {
	t.Helper()
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	if err := db.Ping(); err != nil {
		t.Fatalf("db.Ping: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	resetSchema(t, db)
	applyMigrations(t, db)
	return db
}

func openDocker(t *testing.T) *sql.DB {
	t.Helper()
	pool, err := dockertest.NewPool("")
	if err != nil {
		t.Fatalf("dockertest pool: %v", err)
	}
	if err := pool.Client.Ping(); err != nil {
		t.Skipf("docker not available and QOOIM_TEST_DSN not set: %v", err)
	}

	res, err := pool.RunWithOptions(&dockertest.RunOptions{
		Repository: "postgres",
		Tag:        "18-alpine",
		Env: []string{
			"POSTGRES_PASSWORD=postgres",
			"POSTGRES_USER=postgres",
			"POSTGRES_DB=qooim",
		},
	}, func(c *dc.HostConfig) {
		c.AutoRemove = true
		c.RestartPolicy = dc.RestartPolicy{Name: "no"}
	})
	if err != nil {
		t.Fatalf("run postgres: %v", err)
	}
	t.Cleanup(func() { _ = pool.Purge(res) })

	port := res.GetPort("5432/tcp")
	dsn := fmt.Sprintf("postgresql://postgres:postgres@localhost:%s/qooim?sslmode=disable", port)

	pool.MaxWait = 90 * time.Second
	var db *sql.DB
	if err := pool.Retry(func() error {
		var openErr error
		db, openErr = sql.Open("pgx", dsn)
		if openErr != nil {
			return openErr
		}
		return db.Ping()
	}); err != nil {
		t.Fatalf("postgres ping: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	applyMigrations(t, db)
	return db
}

func resetSchema(t *testing.T, db *sql.DB) {
	t.Helper()
	if _, err := db.Exec(`DROP SCHEMA public CASCADE; CREATE SCHEMA public;`); err != nil {
		t.Fatalf("reset schema: %v", err)
	}
}

func applyMigrations(t *testing.T, db *sql.DB) {
	t.Helper()
	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatalf("goose dialect: %v", err)
	}
	if err := goose.Up(db, migrationsDir()); err != nil {
		t.Fatalf("goose up: %v", err)
	}
}

func migrationsDir() string {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return "migrations"
	}
	dir := filepath.Join(filepath.Dir(thisFile), "..", "..", "migrations")
	if _, err := os.Stat(dir); err != nil {
		return "migrations"
	}
	return dir
}
