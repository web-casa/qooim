-- name: ListTemplates :many
SELECT id, repo_id, serial_no, name, question_type, mode, category, tag,
       priority, preview_url, shared, create_at, update_at, create_by
FROM t_template
WHERE is_deleted = 0
ORDER BY COALESCE(priority, 1000) ASC, create_at DESC
LIMIT $1 OFFSET $2;

-- name: CountTemplates :one
SELECT COUNT(*) FROM t_template WHERE is_deleted = 0;
