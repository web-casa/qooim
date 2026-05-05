-- name: ListDashboards :many
SELECT id, "key", "type", project_id, setting, create_at, update_at, create_by
FROM t_dashboard
ORDER BY create_at DESC
LIMIT $1 OFFSET $2;

-- name: CountDashboards :one
SELECT COUNT(*) FROM t_dashboard;
