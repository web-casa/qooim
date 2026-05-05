-- name: ListRepos :many
SELECT id, name, description, category, mode, shared, tag, priority, is_practice,
       create_at, update_at, create_by
FROM t_repo
ORDER BY COALESCE(priority, 1000) ASC, create_at DESC
LIMIT $1 OFFSET $2;

-- name: CountRepos :one
SELECT COUNT(*) FROM t_repo;
