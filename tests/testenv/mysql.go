//go:build mysql

// MySQL fixture for integration tests. Builds only with `-tags mysql` so
// `go test ./...` stays fast and Docker-free by default. Runs via
//
//	go test -tags mysql ./tests/...
//
// Requires a reachable Docker daemon (docker / OrbStack / colima).
package testenv

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/ory/dockertest/v3"
	dc "github.com/ory/dockertest/v3/docker"
	"github.com/pressly/goose/v3"
)

// MySQL spawns a throwaway MySQL container, runs goose migrations against
// it, and returns a *sql.DB. The container is destroyed on test cleanup.
func MySQL(t *testing.T) *sql.DB {
	t.Helper()
	pool, err := dockertest.NewPool("")
	if err != nil {
		t.Fatalf("dockertest pool: %v", err)
	}
	if err := pool.Client.Ping(); err != nil {
		t.Skipf("docker not available: %v", err)
	}

	res, err := pool.RunWithOptions(&dockertest.RunOptions{
		Repository: "mysql",
		Tag:        "8.0",
		Env: []string{
			"MYSQL_ROOT_PASSWORD=root",
			"MYSQL_DATABASE=examrun",
		},
	}, func(c *dc.HostConfig) {
		c.AutoRemove = true
		c.RestartPolicy = dc.RestartPolicy{Name: "no"}
	})
	if err != nil {
		t.Fatalf("run mysql: %v", err)
	}
	t.Cleanup(func() { _ = pool.Purge(res) })

	port := res.GetPort("3306/tcp")
	dsn := fmt.Sprintf("root:root@tcp(localhost:%s)/examrun?multiStatements=true&parseTime=true", port)

	pool.MaxWait = 90 * time.Second
	var db *sql.DB
	if err := pool.Retry(func() error {
		var openErr error
		db, openErr = sql.Open("mysql", dsn)
		if openErr != nil {
			return openErr
		}
		return db.Ping()
	}); err != nil {
		t.Fatalf("mysql ping: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if err := goose.SetDialect("mysql"); err != nil {
		t.Fatalf("goose dialect: %v", err)
	}
	if err := goose.Up(db, migrationsDir()); err != nil {
		t.Fatalf("goose up: %v", err)
	}
	return db
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
