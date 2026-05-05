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
