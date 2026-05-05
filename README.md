# exam-run

A Go rewrite of the [SurveyKing](https://github.com/javahuang/SurveyKing) survey & exam backend.
Reference Java source: `/home/ivmm/otheruse/surveyking`.

Status: **P0 (skeleton)** — the project boots, exposes health endpoints, runs an
in-process e2e test, and ships a `skctl` CLI. Database, business logic, and the
SurveyKing API surface land in P1–P4.

## Layout

```
cmd/server/          # HTTP server entry point
cmd/skctl/           # CLI: HTTP probes today, business commands in P1+
internal/
  api/               # gin router, middleware, handlers (per SK controller in P1+)
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
migrations/          # goose SQL migrations (00001 = SK baseline)
queries/             # sqlc input (P1+)
tests/
  testenv/           # in-process server + (build-tag) MySQL fixture
  e2e/               # http-level tests
  fixtures/          # request/response fixtures (P1+)
  snapshots/         # snapshot diffs vs. SurveyKing (P1+)
deploy/              # docker, systemd templates
```

## Quick start

```bash
make build           # builds bin/exam-run and bin/skctl
make run             # starts server on :18080 without a DB
# in another shell:
./bin/skctl --server http://localhost:18080 health
./bin/skctl --server http://localhost:18080 version
```

## Configuration

`internal/config` loads YAML (via `--config`/`EXAMRUN_CONFIG`) and overlays
env vars prefixed `EXAMRUN_`. Example:

```bash
EXAMRUN_HTTP_ADDR=":9090" \
EXAMRUN_DB_DSN="user:pass@tcp(localhost:3306)/examrun?parseTime=true&multiStatements=true" \
EXAMRUN_JWT_SECRET="$(openssl rand -hex 32)" \
./bin/exam-run
```

See `configs/example.yml` for the full set of keys.

## Testing

```bash
make test            # unit + in-process e2e (no Docker)
make e2e-mysql       # spins MySQL via dockertest, applies goose baseline
make lint            # golangci-lint
```

`tests/testenv` is the AI-driven testing entry point: it boots an
in-process server, lets you call `s.GET(t, "/path")`, and (with `-tags mysql`)
brings up a real MySQL with the SK schema applied.

## Migrations

Goose. The first migration (`migrations/00001_baseline.sql`) is the
SurveyKing v1.12.0 schema imported verbatim, including seed data
(default `admin` account, default roles, sys_info). Workflow tables
were never present in this SQL.

```bash
EXAMRUN_DB_DSN="user:pass@tcp(localhost:3306)/examrun?multiStatements=true&parseTime=true" \
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

- Original SurveyKing source (Java/Spring Boot 2.7) lives at
  `/home/ivmm/otheruse/surveyking`. P1+ ports these:
  - `SurveyServiceImpl.java` (1239 lines) — survey rendering & sampling
  - `AnswerServiceImpl.java` (755 lines) — answer write paths
  - `RandomSurveyProcessor.java` (291 lines) — random-question logic
- 11 controllers in `server/api/src/main/java/cn/surveyking/server/api/` map
  one-to-one to handlers under `internal/api/`.
