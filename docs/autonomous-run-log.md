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
