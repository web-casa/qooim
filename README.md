# Qoo.IM

A Go rewrite of the [SurveyKing](https://github.com/javahuang/SurveyKing) survey & exam backend, on PostgreSQL 18.
Reference Java source: `/home/ivmm/otheruse/surveyking`.

Status: **P0 (skeleton)** — the project boots, exposes health endpoints, runs an
in-process e2e test, ships a `qooim` CLI, and applies the SurveyKing schema
(translated to PostgreSQL) via goose. P1–P4 add the business logic.

## Layout

```
cmd/server/          # qooim-server entry point (HTTP)
cmd/qooim/           # qooim CLI entry point
internal/
  api/               # gin router, middleware, handlers
  cli/               # cobra commands for the qooim CLI
  service/           # business logic
  repo/              # database access (sqlc-generated + custom queries land in P1)
  domain/            # entities & DTOs
  auth/              # JWT + URL-token middleware
  storage/           # file storage abstraction
  excel/             # import/export
  i18n/              # localized messages
  config/            # viper-based config loader
  logger/            # slog wrappers
  httpx/             # shared response helpers
migrations/          # goose SQL migrations (00001 = SK baseline, ported to PG)
queries/             # sqlc input (P1+)
tests/
  testenv/           # in-process server + (build-tag) Postgres fixture
  e2e/               # http-level tests
  fixtures/          # request/response fixtures (P1+)
  snapshots/         # snapshot diffs vs. SurveyKing (P1+)
deploy/              # docker, systemd templates
```

## Quick start

```bash
make build           # builds bin/qooim-server and bin/qooim
make run             # starts server on :18080 without a DB
# in another shell:
./bin/qooim --server http://localhost:18080 health
./bin/qooim --server http://localhost:18080 version
```

## Configuration

`internal/config` loads YAML (via `--config`/`QOOIM_CONFIG`) and overlays
env vars prefixed `QOOIM_`. **Never** commit a real DSN to `configs/`.

```bash
QOOIM_HTTP_ADDR=":9090" \
QOOIM_DB_DSN="postgresql://user:pass@host:5432/qooim?sslmode=disable" \
QOOIM_JWT_SECRET="$(openssl rand -hex 32)" \
./bin/qooim-server
```

See `configs/example.yml` for the full set of keys.

## Testing

```bash
make test            # unit + in-process e2e (no DB)
make e2e-pg          # Postgres-backed integration tests (Docker or QOOIM_TEST_DSN)
make lint            # golangci-lint
```

`tests/testenv` is the AI-driven testing entry point: it boots an
in-process server, lets you call `s.GET(t, "/path")`, and (with `-tags pg`)
brings up a real Postgres with the schema applied.

## Migrations

Goose. `migrations/00001_baseline.sql` is the SurveyKing v1.12.0 schema
**ported to PostgreSQL 18**: `tinyint(1)` → `boolean`, `longtext` → `text`,
`ON UPDATE CURRENT_TIMESTAMP` → trigger, JSON-shaped columns → `jsonb`.
Workflow tables were never present in the SK SQL.

```bash
QOOIM_DB_DSN="postgresql://user:pass@host:5432/qooim?sslmode=disable" \
make migrate-up
```

## Roadmap

| Phase | Scope |
|-------|-------|
| P0 | Skeleton, CI, tests, baseline schema |
| P1 | Read-only routes (Project / Repo / Template / Dashboard list) + JWT auth |
| P2 | Write CRUD (project, repo, template, file upload) |
| P3 | **Core**: survey rendering, random sampling, answer submission, URL token, captcha |
| P4 | Excel import/export, reports, exam/exercise |
| Later | AI module (SSE, providers) |

Out of scope: Flowable workflow (was already disabled in SK), multi-tenant.

## Reference

The Java SurveyKing source (Spring Boot 2.7) lives at
`/home/ivmm/otheruse/surveyking`. P1+ ports these:

- `SurveyServiceImpl.java` (1239 lines) — survey rendering & sampling
- `AnswerServiceImpl.java` (755 lines) — answer write paths
- `RandomSurveyProcessor.java` (291 lines) — random-question logic

11 controllers in `server/api/src/main/java/cn/surveyking/server/api/` map
one-to-one to handlers under `internal/api/`.
