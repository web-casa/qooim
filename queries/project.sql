-- name: ListProjects :many
SELECT id, parent_id, name, status, mode, priority, create_at, update_at, create_by
FROM t_project
WHERE is_deleted = 0
ORDER BY priority ASC, create_at DESC
LIMIT $1 OFFSET $2;

-- name: CountProjects :one
SELECT COUNT(*) FROM t_project WHERE is_deleted = 0;
