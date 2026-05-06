-- name: ListProjects :many
-- Optional filters: parent_id (exact), mode (exact), name (ILIKE).
-- Pass NULL for any filter you don't want to apply.
SELECT id, parent_id, name, survey, setting, status, mode, priority,
       create_at, update_at, create_by
FROM t_project
WHERE is_deleted = 0
  AND (sqlc.narg('parent_id')::varchar IS NULL OR parent_id = sqlc.narg('parent_id'))
  AND (sqlc.narg('mode')::varchar      IS NULL OR mode      = sqlc.narg('mode'))
  AND (sqlc.narg('name')::text         IS NULL OR name ILIKE '%' || sqlc.narg('name')::text || '%')
ORDER BY priority ASC, create_at DESC
LIMIT sqlc.arg('lim') OFFSET sqlc.arg('off');

-- name: CountProjects :one
SELECT COUNT(*)
FROM t_project
WHERE is_deleted = 0
  AND (sqlc.narg('parent_id')::varchar IS NULL OR parent_id = sqlc.narg('parent_id'))
  AND (sqlc.narg('mode')::varchar      IS NULL OR mode      = sqlc.narg('mode'))
  AND (sqlc.narg('name')::text         IS NULL OR name ILIKE '%' || sqlc.narg('name')::text || '%');

-- name: GetProjectByID :one
SELECT id, parent_id, name, survey, setting, status, mode, priority,
       create_at, create_by, update_at, update_by
FROM t_project
WHERE id = $1 AND is_deleted = 0;

-- name: CreateProject :exec
INSERT INTO t_project (
    id, parent_id, name, survey, setting, status, mode, priority,
    is_deleted, create_at, create_by
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, 0, NOW(), $9
);

-- name: UpdateProject :exec
UPDATE t_project SET
    parent_id = COALESCE(sqlc.narg('parent_id'),  parent_id),
    name      = COALESCE(sqlc.narg('name'),       name),
    survey    = COALESCE(sqlc.narg('survey'),     survey),
    setting   = COALESCE(sqlc.narg('setting'),    setting),
    status    = COALESCE(sqlc.narg('status'),     status),
    mode      = COALESCE(sqlc.narg('mode'),       mode),
    priority  = COALESCE(sqlc.narg('priority'),   priority),
    update_by = $1
WHERE id = $2 AND is_deleted = 0;

-- name: SoftDeleteProject :exec
UPDATE t_project SET is_deleted = 1, update_by = $1 WHERE id = $2 AND is_deleted = 0;
