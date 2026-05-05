-- name: ProjectAnswerStats :one
-- Aggregated answer counters for a single project. PG's FILTER clause
-- avoids three separate COUNT queries.
SELECT
    COUNT(*)                                  AS total,
    COUNT(*) FILTER (WHERE temp_save = 1)     AS finished,
    COUNT(*) FILTER (WHERE temp_save = 0)     AS draft,
    COALESCE(AVG(exam_score), 0)::double precision AS avg_score
FROM t_answer
WHERE project_id = $1 AND is_deleted = 0;

-- name: AnswersForExportPage :many
-- Cursor-style pagination for the xlsx export. The service loops over
-- (offset, limit) chunks so peak memory stays bounded even when the
-- project has tens of thousands of answers.
SELECT id, project_id, answer, attachment, meta_info, temp_save,
       exam_info, exam_exercise_type, exam_score,
       create_at, create_by
FROM t_answer
WHERE project_id = $1 AND is_deleted = 0
ORDER BY create_at ASC
LIMIT $2 OFFSET $3;

-- name: ListExerciseProjects :many
-- Projects in exam/exercise mode with at least one finished answer.
-- Drives /api/exercises for the user-facing exercise overview.
SELECT
    p.id,
    p.name,
    p.mode,
    COUNT(a.id)                                       AS answer_count,
    COUNT(a.id) FILTER (WHERE a.temp_save = 1)         AS finished_count,
    COALESCE(AVG(a.exam_score), 0)::double precision   AS avg_score
FROM t_project p
LEFT JOIN t_answer a
       ON a.project_id = p.id AND a.is_deleted = 0
WHERE p.is_deleted = 0
  AND p.mode IN ('exam', 'exercise')
  AND COALESCE(p.status, 0) = 1
GROUP BY p.id, p.name, p.mode
ORDER BY p.create_at DESC;
