-- ----- t_user_book (a.k.a. wrong-question book / favourites) -----

-- name: ListUserBooks :many
-- The book is per (create_by, type) where type=1 is wrong questions
-- and type=2 is favourites. Filter by repo_id when the UI is showing
-- a single repo's pool.
SELECT id, name, template_id, wrong_times, correct_times, note, status, type,
       repo_id, is_marked, create_at, update_at, create_by
FROM t_user_book
WHERE (sqlc.narg('create_by')::varchar IS NULL OR create_by = sqlc.narg('create_by'))
  AND (sqlc.narg('repo_id')::varchar   IS NULL OR repo_id   = sqlc.narg('repo_id'))
  AND (sqlc.narg('type')::int          IS NULL OR type      = sqlc.narg('type'))
ORDER BY create_at DESC
LIMIT sqlc.arg('lim') OFFSET sqlc.arg('off');

-- name: CountUserBooks :one
SELECT COUNT(*) FROM t_user_book
WHERE (sqlc.narg('create_by')::varchar IS NULL OR create_by = sqlc.narg('create_by'))
  AND (sqlc.narg('repo_id')::varchar   IS NULL OR repo_id   = sqlc.narg('repo_id'))
  AND (sqlc.narg('type')::int          IS NULL OR type      = sqlc.narg('type'));

-- name: CreateUserBook :exec
INSERT INTO t_user_book (id, name, template_id, wrong_times, correct_times, note,
                         status, type, repo_id, is_marked, create_at, create_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW(), $11);

-- name: UpdateUserBook :exec
UPDATE t_user_book SET
    name          = COALESCE(sqlc.narg('name'),          name),
    template_id   = COALESCE(sqlc.narg('template_id'),   template_id),
    wrong_times   = COALESCE(sqlc.narg('wrong_times'),   wrong_times),
    correct_times = COALESCE(sqlc.narg('correct_times'), correct_times),
    note          = COALESCE(sqlc.narg('note'),          note),
    status        = COALESCE(sqlc.narg('status'),        status),
    type          = COALESCE(sqlc.narg('type'),          type),
    repo_id       = COALESCE(sqlc.narg('repo_id'),       repo_id),
    is_marked     = COALESCE(sqlc.narg('is_marked'),     is_marked),
    update_by     = $1
WHERE id = $2;

-- name: DeleteUserBook :exec
DELETE FROM t_user_book WHERE id = $1;
