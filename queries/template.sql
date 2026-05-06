-- name: ListTemplates :many
SELECT id, repo_id, serial_no, name, question_type, mode, category, tag,
       priority, preview_url, shared, create_at, update_at, create_by
FROM t_template
WHERE is_deleted = 0
ORDER BY COALESCE(priority, 1000) ASC, create_at DESC
LIMIT $1 OFFSET $2;

-- name: CountTemplates :one
SELECT COUNT(*) FROM t_template WHERE is_deleted = 0;

-- name: GetTemplateByID :one
SELECT id, repo_id, serial_no, name, question_type, template, mode, category, tag,
       priority, preview_url, shared, create_at, create_by, update_at, update_by
FROM t_template WHERE id = $1 AND is_deleted = 0;

-- name: CreateTemplate :exec
INSERT INTO t_template (
    id, repo_id, serial_no, name, question_type, template, mode, category, tag,
    priority, preview_url, shared, is_deleted, create_at, create_by
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, 0, NOW(), $13
);

-- name: UpdateTemplate :exec
UPDATE t_template SET
    repo_id       = COALESCE(sqlc.narg('repo_id'),       repo_id),
    serial_no     = COALESCE(sqlc.narg('serial_no'),     serial_no),
    name          = COALESCE(sqlc.narg('name'),          name),
    question_type = COALESCE(sqlc.narg('question_type'), question_type),
    template      = COALESCE(sqlc.narg('template'),      template),
    mode          = COALESCE(sqlc.narg('mode'),          mode),
    category      = COALESCE(sqlc.narg('category'),      category),
    tag           = COALESCE(sqlc.narg('tag'),           tag),
    priority      = COALESCE(sqlc.narg('priority'),      priority),
    preview_url   = COALESCE(sqlc.narg('preview_url'),   preview_url),
    shared        = COALESCE(sqlc.narg('shared'),        shared),
    update_by     = $1
WHERE id = $2 AND is_deleted = 0;

-- name: SoftDeleteTemplate :exec
UPDATE t_template SET is_deleted = 1, update_by = $1 WHERE id = $2 AND is_deleted = 0;

-- name: DistinctTemplateCategories :many
-- Powers /api/template/listCategory. Returns each non-empty category
-- exactly once across the whole table (the previous heuristic walked
-- only the first 200 rows and missed values further in).
SELECT DISTINCT category FROM t_template
WHERE is_deleted = 0 AND category IS NOT NULL AND category <> ''
ORDER BY category;

-- name: DistinctTemplateTagBlobs :many
-- Returns the comma-separated tag column for every undeleted template;
-- the service splits and dedupes in Go because PG's string_to_array +
-- DISTINCT plumbing here doesn't add enough value to justify the
-- extra SQL.
SELECT tag FROM t_template
WHERE is_deleted = 0 AND tag IS NOT NULL AND tag <> '';
