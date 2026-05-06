-- ----- t_project_partner -----

-- name: ListProjectPartners :many
SELECT id, uid, project_id, type, status, user_id, user_name, group_id,
       data_permission, initial_value, create_at, update_at, create_by
FROM t_project_partner
WHERE (sqlc.narg('project_id')::varchar IS NULL OR project_id = sqlc.narg('project_id'))
  AND (sqlc.narg('user_name')::text   IS NULL OR user_name ILIKE '%' || sqlc.narg('user_name')::text || '%')
ORDER BY create_at DESC
LIMIT sqlc.arg('lim') OFFSET sqlc.arg('off');

-- name: CountProjectPartners :one
SELECT COUNT(*) FROM t_project_partner
WHERE (sqlc.narg('project_id')::varchar IS NULL OR project_id = sqlc.narg('project_id'))
  AND (sqlc.narg('user_name')::text   IS NULL OR user_name ILIKE '%' || sqlc.narg('user_name')::text || '%');

-- name: CreateProjectPartner :exec
INSERT INTO t_project_partner (id, uid, project_id, type, status, user_id, user_name,
                                group_id, data_permission, initial_value, create_at, create_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW(), $11);

-- name: DeleteProjectPartner :exec
DELETE FROM t_project_partner WHERE id = $1;

-- ----- t_repo_partner -----

-- name: ListRepoPartners :many
SELECT id, repo_id, user_id, create_at, update_at, create_by
FROM t_repo_partner
WHERE (sqlc.narg('repo_id')::varchar IS NULL OR repo_id = sqlc.narg('repo_id'))
ORDER BY create_at DESC
LIMIT sqlc.arg('lim') OFFSET sqlc.arg('off');

-- name: CountRepoPartners :one
SELECT COUNT(*) FROM t_repo_partner
WHERE (sqlc.narg('repo_id')::varchar IS NULL OR repo_id = sqlc.narg('repo_id'));

-- name: CreateRepoPartner :exec
INSERT INTO t_repo_partner (id, repo_id, user_id, create_at, create_by)
VALUES ($1, $2, $3, NOW(), $4);

-- name: DeleteRepoPartner :exec
DELETE FROM t_repo_partner WHERE id = $1;
