package repo

import (
	"database/sql"
	"fmt"

	// pgx's database/sql wrapper — registers the "pgx" driver name we
	// pass to sql.Open.
	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/web-casa/qooim/internal/config"
)

// Open returns a *sql.DB backed by jackc/pgx/v5's database/sql driver.
// DSN must be a libpq URL, e.g.
//
//	postgresql://user:pass@host:5432/db?sslmode=disable
func Open(cfg config.DB) (*sql.DB, error) {
	if cfg.DSN == "" {
		return nil, fmt.Errorf("db.dsn is empty")
	}
	db, err := sql.Open("pgx", cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("sql.Open: %w", err)
	}
	if cfg.MaxOpenConns > 0 {
		db.SetMaxOpenConns(cfg.MaxOpenConns)
	}
	if cfg.MaxIdleConns > 0 {
		db.SetMaxIdleConns(cfg.MaxIdleConns)
	}
	if cfg.ConnMaxLifetime > 0 {
		db.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("db.Ping: %w", err)
	}
	return db, nil
}
