-- name: ListRepos :many
SELECT id, name, description, category, mode, shared, tag, priority, is_practice,
       create_at, update_at, create_by
FROM t_repo
ORDER BY COALESCE(priority, 1000) ASC, create_at DESC
LIMIT $1 OFFSET $2;

-- name: CountRepos :one
SELECT COUNT(*) FROM t_repo;

-- name: GetRepoByID :one
SELECT id, name, description, category, mode, shared, tag, priority, setting, is_practice,
       create_at, create_by, update_at, update_by
FROM t_repo WHERE id = $1;

-- name: CreateRepo :exec
INSERT INTO t_repo (
    id, name, description, category, mode, shared, tag, priority, setting, is_practice,
    create_at, create_by
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW(), $11
);

-- name: UpdateRepo :exec
UPDATE t_repo SET
    name        = COALESCE(sqlc.narg('name'),        name),
    description = COALESCE(sqlc.narg('description'), description),
    category    = COALESCE(sqlc.narg('category'),    category),
    mode        = COALESCE(sqlc.narg('mode'),        mode),
    shared      = COALESCE(sqlc.narg('shared'),      shared),
    tag         = COALESCE(sqlc.narg('tag'),         tag),
    priority    = COALESCE(sqlc.narg('priority'),    priority),
    setting     = COALESCE(sqlc.narg('setting'),     setting),
    is_practice = COALESCE(sqlc.narg('is_practice'), is_practice),
    update_by   = $1
WHERE id = $2;

-- name: DeleteRepo :exec
-- t_repo has no is_deleted column in SK, so delete is hard.
DELETE FROM t_repo WHERE id = $1;
