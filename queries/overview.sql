-- name: UserOverviewCounts :one
-- Powers /api/userOverview — top-level numbers the SK dashboard shows.
-- One round-trip aggregate so the home screen renders fast.
SELECT
    (SELECT COUNT(*) FROM t_project   WHERE is_deleted = 0)            AS project_total,
    (SELECT COUNT(*) FROM t_project   WHERE is_deleted = 0 AND COALESCE(status, 0) = 1) AS project_published,
    (SELECT COUNT(*) FROM t_template  WHERE is_deleted = 0)            AS template_total,
    (SELECT COUNT(*) FROM t_repo)                                       AS repo_total,
    (SELECT COUNT(*) FROM t_answer    WHERE is_deleted = 0)            AS answer_total,
    (SELECT COUNT(*) FROM t_answer    WHERE is_deleted = 0 AND temp_save = 1) AS answer_finished,
    (SELECT COUNT(*) FROM t_user      WHERE is_deleted = 0)            AS user_total;
