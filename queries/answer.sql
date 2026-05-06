-- name: CreateAnswer :exec
INSERT INTO t_answer (
    id, project_id, survey, answer, meta_info, attachment, temp_save,
    exam_info, exam_exercise_type, exam_score, repo_id,
    is_deleted, create_at, create_by
) VALUES (
    $1, $2, $3, $4, $5, $6, $7,
    $8, $9, $10, $11,
    0, NOW(), $12
);

-- name: GetAnswerByID :one
SELECT id, project_id, survey, answer, attachment, meta_info, temp_save,
       exam_info, exam_exercise_type, exam_score, repo_id,
       create_at, create_by, update_at, update_by
FROM t_answer
WHERE id = $1 AND is_deleted = 0;

-- name: ListAnswersByProject :many
SELECT id, project_id, temp_save, exam_score, exam_exercise_type,
       create_at, create_by, update_at
FROM t_answer
WHERE project_id = $1 AND is_deleted = 0
ORDER BY create_at DESC
LIMIT $2 OFFSET $3;

-- name: CountAnswersByProject :one
SELECT COUNT(*) FROM t_answer WHERE project_id = $1 AND is_deleted = 0;

-- name: SoftDeleteAnswer :exec
UPDATE t_answer SET is_deleted = 1, update_by = $1 WHERE id = $2 AND is_deleted = 0;

-- name: UpdateAnswerInPlace :execrows
-- Used by saveAnswer's resume flow: when the client returns a previously
-- issued answerId, we patch the existing draft instead of creating a
-- new row. Returns the number of rows touched so the service can fall
-- back to insert if the id is stale (deleted or never existed).
UPDATE t_answer SET
    survey     = COALESCE(sqlc.narg('survey'),     survey),
    answer     = COALESCE(sqlc.narg('answer'),     answer),
    attachment = COALESCE(sqlc.narg('attachment'), attachment),
    meta_info  = COALESCE(sqlc.narg('meta_info'),  meta_info),
    temp_save  = COALESCE(sqlc.narg('temp_save'),  temp_save),
    exam_score = COALESCE(sqlc.narg('exam_score'), exam_score),
    update_by  = $1
WHERE id = $2 AND is_deleted = 0;
