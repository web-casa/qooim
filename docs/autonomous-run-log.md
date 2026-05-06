# Autonomous run log

A running log of every non-trivial decision, shelf, or TODO made while
P2–P5 + the final review + the DigitalOcean smoke test were executed
without the user in the loop. Browse this first when reviewing the
overnight run.

Format:

```
## P{N} — short title
Started: ISO-8601
Finished: ISO-8601
Outcome: ok | blockers shelved | partial

### Decisions
- ...

### Shelved (blockers / out-of-scope)
- ...

### Codex review
- verdict, then any nits worth flagging
```

Anything marked `SHELVED` here means there's a TODO in the code with the
same wording so search will find both.

---

## Run kickoff

- 2026-05-06 — user delegates P2/P3/P4/P5 + final review + DO smoke test.
- Budget: 60 min hard cap on the DO droplet.
- Blocker policy: shelve + record, keep going on whatever is unblocked.
- `.env` at /home/ivmm/exam-run/.env (gitignored), holds `doapi=<DO API token>`. Token verified against /v2/account → 200 OK before P2 began.
- Codex review per phase; failures fall back to a self-review pass.

---

## P2 — write CRUD + file upload
Started: 2026-05-06 01:17 (HKT)
Outcome: ok (Codex review found 0 blockers, 3 nits — all fixed)

### Decisions
- **IDs**: ULID, 26 chars, drops into varchar(64). Coexists with SK's snowflake numeric IDs in the same tables.
- **DTO layer added** (`internal/domain/dto.go`). sqlc keeps Null* wrappers at the persistence layer (truthful), service/API maps to plain JSON-friendly DTOs. Decided to add new files rather than override sqlc types globally so the boundary stays explicit.
- **Soft delete**: projects + templates + files are soft-deleted (is_deleted=1). Repos are hard-deleted because t_repo lacks an is_deleted column in SK.
- **File storage atomic write**: write to `<key>.tmp.*` then rename, so crashes can't leave half-files. (Codex P2 nit.)
- **Content-Disposition** uses `mime.FormatMediaType` to escape user-supplied filenames properly. (Codex P2 nit.)
- **Permission gating**: P2 keeps every authed user able to read/write any project/repo/template. SK's partner-scoped permissions land in P3 with the answer flow that needs them.

### Shelved
- None for P2.

---

## P3 — survey rendering + answer submission
Started: 2026-05-06 ~01:35 (HKT)
Outcome: ok (Codex review found 0 blockers, several nits — time-format nit fixed; rest noted as P4+ watch)

### Decisions
- **Public render** (`GET /api/survey/:projectId`) only returns rows with `status=1 AND is_deleted=0`. Drafts 404 — deliberately leak nothing about their existence.
- **Answer survey snapshot is server-side**: the survey JSON copied into t_answer.survey is read from t_project at submit time, not taken from the client. A dishonest client can't claim a different survey shape than what was published.
- **Partner token**: `?t=<uid>` resolves a t_project_partner row best-effort. If absent or unknown, the answer is recorded as create_by="guest". When a partner is matched, create_by is set to the partner's user_id (or partner.id if no user_id).
- **meta_info JSON**: stores `{ip, user_agent}` only. SK consumers may expect more — punted to P4 if reports show empty fields.
- **Exam mode**: when survey.mode=="exam", the answer row gets exam_exercise_type='O' (the SK "online" code). Other modes leave it null.

### Shelved (with TODOs in code)
- **Random sampling** (SK's `RandomSurveyProcessor` 291L). Public render currently returns the survey JSON verbatim; sampling is needed for exam mode where each test-taker should get a different subset. Tagged in survey.go header comment.
- **Captcha** (anji-plus slider). The anji-plus protocol is a multi-day port; P3 accepts `captcha_token` in the body and ignores it. README warns this is dev-only behaviour.
- **Partner permission gate**: we look up the partner row but don't reject answers based on group_id/data_permission/already-answered status. Cross-project token attribution is also possible (partner uid is unique but isn't validated against projectId). All of this lives behind admin-tooling (P4+).
- **Quotas / dedupe / rate limiting**: open public POST. P3 cut.

### Codex review
- 0 blockers. Nits captured: time-format inconsistency (fixed → time.Time + RFC3339), partner cross-project attribution (TODO), exam_exercise_type='O' is unverifiable from this repo (kept as best guess).

---

## P4 — XLSX export/import, project report, exercise overview
Started: 2026-05-06 ~01:55 (HKT)
Outcome: ok (4 Codex must-fix items addressed; final test still green)

### Decisions
- **Streaming export**: paginated SQL (LIMIT 1000 OFFSET N) loop in service so peak memory stays bounded on huge projects. excelize's StreamWriter then writes rows in O(1) memory.
- **Buffered HTTP write for export**: instead of streaming straight to gin's ResponseWriter, we render to a bytes.Buffer first, so a mid-stream DB failure can return a clean 500 instead of leaving a half-xlsx + an unreliable trailer header. Tradeoff: one whole xlsx in memory per concurrent download — fine for P4 numbers, would need rework if many simultaneous huge exports become a thing.
- **Import semantics**: empty rows (all whitespace) are silently skipped. Non-empty rows missing the required `name` column now error with `row N: 'name' is required`, instead of being silently dropped.
- **Workbook always closed**: `excel.Writer.Flush` defers `Close()` so excelize resources don't leak when Write fails.
- **Avg-score query**: `COALESCE(AVG(...), 0)::double precision` so sqlc emits float64 instead of NullFloat64. The aggregation is non-null (zero on empty set); callers that want to distinguish "no answers" from "average is zero" should consult `total`.

### Codex review
- Codex flagged 4 must-fix items; all addressed in the same commit:
  1. Export was materialising the whole result set → switched to paginated query.
  2. Import skipped non-empty rows missing `name` → now errors.
  3. Excelize workbook not always closed on error paths → Flush defers Close, plus a Close() helper.
  4. X-Export-Error trailer leaked + unreliable → removed; export pre-renders to a buffer.
- Style nits left as-is (5-line `template_id.go` indirection; nullableX helpers parallel to domain.nsX). Captured here for future hygiene PRs.

### Shelved
- True streaming HTTP export (chunked transfer + reliable error reporting). Deferred until report sizes warrant a job-queue + signed-URL pattern.

---

## P5 — AI chat (SSE + OpenAI-compatible provider)
Started: 2026-05-06 ~02:25 (HKT)
Outcome: ok (Codex review tool returned an empty stub instead of findings → self-review per the user's fallback rule)

### Decisions
- **Generic provider, SiliconFlow as default**. `OpenAICompatible` POSTs to `/v1/chat/completions` with `stream=true`. Drop in any OpenAI-compatible URL by setting `QOOIM_AI_PROVIDER=openai` + `QOOIM_AI_BASE_URL`.
- **404 when disabled**. Hidden endpoint design: when no token / module disabled, the route returns `404 not_found` so the existence of the feature isn't leaked to unauthorised users.
- **Test injection point**. `Server.SetAIProvider(ai.Provider)` lets tests swap the provider after construction; routes resolve `s.aiSvc` per-request so the mutation is observed live.
- **HTTP timeout**: header timeout only. Streaming responses can run minutes; an overall `Timeout` on http.Client would chop them. We use `Transport.ResponseHeaderTimeout` for first-byte timing.
- **Done semantics**: provider emits content first, then a separate `Delta{Done:true}` when `finish_reason` is set. Callers can therefore aggregate content without inspecting Done.
- **No history persistence**. P5 just proxies — no server-side conversation store. SK had a `ConversationCacheService`; we'll add one only when product needs it.
- **No provider token in this run**: real SiliconFlow integration is testable manually via `QOOIM_AI_ENABLED=true QOOIM_AI_TOKEN=...`. Tests use a fake provider.

### Self-review (Codex tool returned without findings)
Walked the diff myself, fixed two:
- handleAIChat had a dead `errors.Is(err, ai.ErrDisabled)` branch after we'd already sent 200 + headers. The disabled case is caught before any byte hits the wire; removed the dead branch + dropped the `errors` import.
- `Server.SetAIProvider` now has a doc comment explaining it's safe post-construction (handler resolves `s.aiSvc` per-request).

### Shelved
- Per-user / per-token rate limiting on /api/ai/chat. None in P5.
- 1MB max line buffer on the SSE scanner — sufficient for chat content; some tool-call payloads can exceed this on other providers. If we add tool calling, raise the limit.
- Conversation persistence (history, user-mode threads, costs).

---

## Final cross-cutting review (P2..P5)
Started: 2026-05-06 ~02:55 (HKT)
Outcome: ok (Codex tool returned empty again → self-review, 2 ops fixes folded into the deploy commit)

### Self-review notes
Codex returned an empty stub for the final round; I walked the diff myself.
- **Service ErrNotFound mapping**: identical pattern across all P2..P5 services (`if errors.Is(err, sql.ErrNoRows) { return ErrNotFound }`). No drift.
- **Handler error mapping**: each handler maps ErrNotFound→404, validation→400, others→500. Consistent.
- **DTO discipline**: domain/dto.go covers projects/repos/templates/files/dashboards. Answer / survey / AI use ad-hoc structs in service/. They have different shapes (no audit columns on AI deltas, e.g.); leaving them as-is is the right call vs forcing a uniform package layout.
- **Logging**: `s.logger.Error("topic.action", "err", err)` is the dominant pattern; partner-lookup uses Warn (best-effort). Acceptable.
- **Public routes**: /api/version, GET /api/survey/:projectId, POST /api/survey/:projectId/answer, POST /api/auth/login. All intentional; documented.
- **Cookie routes**: none. JWT-only API surface — no CSRF concern.
- **Schema vs sqlc input**: schema/schema.sql includes the property_json rename; matches what migrations 00001+00003 produce. ✓
- **Pool sizing** (max_open=25, max_idle=5, lifetime=30m) is reasonable for the smallest VPS.

### Ops fixes folded in (before DO smoke test)
- **JWT secret startup validation** (`cmd/server/main.go:validateConfig`). Server now refuses to start in env=prod/production when JWT secret is empty or equal to the example default `change-me-in-production`.
- **Storage local_root resolved to abs path at startup** so logs show the directory the server will actually write to (systemd's CWD is `/`).

Verified: `make build` / `make test` clean; bad prod configs now fail with a clear "config:" error before binding the port.

---

## DO VPS smoke test
Started: 2026-05-06 ~03:05 (HKT)
Outcome: ok (1 issue found + fixed in-place; droplet destroyed; total cost ~$0.003)

### Run
- Region sgp1, size s-1vcpu-1gb ($0.00893/h), Ubuntu 24.04 LTS.
- Droplet 569187055 at 157.230.44.73 — provisioned in ~30s.
- Cost: ~21 min uptime × $0.00893/h ≈ $0.003.
- DB: the same Zeabur PG used by the e2e fixture (no new database created — kept the run idempotent).

### Issue found
**The repo is private**. `create.sh` originally baked `git clone
https://github.com/web-casa/qooim` into cloud-init; the unauthenticated
clone failed with "could not read Username for 'https://github.com'".
**Fix**: rewritten `create.sh` to cross-compile a linux/amd64 binary
locally (`GOOS=linux GOARCH=amd64 go build`) and `scp` it + the
migrations directory to the droplet, with cloud-init only handling the
apt + Go install. Verified by re-running the install steps over SSH on
the live droplet:
- `qooim-server` running under systemd as expected.
- `/healthz`, `/readyz`, `/api/version`, login, `/me`, all four list
  endpoints, project create/get/delete, public survey render, guest
  answer submit, report aggregation, xlsx export (PK header verified),
  and AI chat 404 (no provider) — every route on the surface answered
  as expected.

### Cleanup
- Droplet `DELETE /v2/droplets/569187055` — 204; subsequent GET → 404.
- Ephemeral SSH key `DELETE /v2/account/keys/56122008` — 204.
- /tmp/qooim-do scratch dir removed.

### Deliverables
- `deploy/digitalocean/create.sh` — provision + scp + systemd + wait for /healthz.
- `deploy/digitalocean/destroy.sh` — droplet + ssh-key teardown.
- `deploy/digitalocean/smoke.sh` — walks every endpoint surface against an IP.
- `deploy/digitalocean/README.md` — recipe + tunables + hardening checklist for a long-lived install.

---

## Run summary — wake-up brief

**6 phases shipped, all green, all pushed.**

```
a96b5dc deploy: DigitalOcean smoke + permanent recipe
491b88a ops: fail-fast config validation (jwt secret + storage root)
e76f9ce P5: AI chat module — OpenAI-compatible provider over SSE
4bd2821 P4: xlsx export/import + project report + exercise overview
bfb2c6c P3: survey rendering + answer submission
c46ec3a P2: write CRUD (project/repo/template) + file upload
```

(P1 carry-overs landed earlier in 78d46ed/ddd1565/cc2ce5d.)

**Codex review behaviour**
- P2: clean (3 nits, all fixed in same commit).
- P3: clean (1 time-format nit fixed; partner-token edge cases captured here as future work).
- P4: 4 must-fix items (paginated streaming export, import error vs silent skip, excelize close on error, X-Export-Error trailer). All fixed in same commit.
- P5: Codex tool returned an empty review stub → fell back to self-review per your rule. Found a dead branch + missing comment, both fixed.
- Final cross-cutting: same Codex empty-stub behaviour → self-review. Folded in JWT-secret + storage-root startup validation.

**Things you may want to revisit**
- Codex agent tool came back empty twice (P5 + final). The earlier P0/P1/P2/P3/P4 calls all worked, so the tool is functional but appears to silently no-op some of the time. If you have insight into what controls that I'd appreciate it; for now the self-review fallback caught issues that mattered.
- I did not change the GitHub repo's visibility. The DO smoke flow assumed it was public the first try; once that failed I switched to an scp-based deploy and that's what `deploy/digitalocean/create.sh` now does. If you want the simpler "git clone in cloud-init" recipe back, make the repo public or add a deploy key.
- Captcha and SK's RandomSurveyProcessor are still shelved (per the kickoff). Survey/answer submit currently has no rate limiting. The autonomous-run-log.md "Shelved" sections cite where each TODO lives in code.
- Default admin password is still SK's `123456` — change it in the t_account row before the first real user shows up. The seed migration already warns about this in a comment.

---

## Source-based VPS path validated (post-public flip)

After the user flipped the repo to public, I re-ran
`create-from-source.sh` to verify the cloud-init `git clone` path. The
first attempt failed with two real issues that the binary-upload path
had silently dodged:

1. **`HOME` unset → Go module cache disabled**. cloud-init runs
   `runcmd` as root with a stripped env. Go 1.26 errors with
   "module cache not found: neither GOMODCACHE nor GOPATH is set".
   Fix: `export HOME=/root` at the top of the install script.
2. **`go build` OOM-killed on `s-1vcpu-1gb` (1024 MiB RAM)**. The
   ugorji/codec compile alone peaks above the cap. Fix: cloud-init
   creates a 2 GiB swapfile, and the build uses `-p 1` to cap parallel
   compiler steps. Adds ~30s but holds RSS under the swap window.

Re-tested end-to-end on droplet 569248393 at 159.223.63.19. After the
two fixes the smoke harness passed all 16 checks (PASS on /healthz,
/readyz, version, login, /me, four lists, project lifecycle, public
render, guest answer, report, xlsx PK header, AI 404, delete). Droplet
destroyed afterwards; total run cost ~$0.005.
