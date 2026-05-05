-- name: GetPublishedSurvey :one
-- Public survey-render query: returns only the data needed to render a
-- published, non-deleted survey to an unauthenticated visitor. Drafts
-- (status=0) and soft-deleted projects are invisible here.
SELECT id, name, survey, setting, mode, status
FROM t_project
WHERE id = $1 AND is_deleted = 0 AND COALESCE(status, 0) = 1;

-- name: GetPartnerByUID :one
-- Look up a project partner by its short uid (the URL-token surrogate).
-- Used by the partner-token middleware to identify a participant
-- without forcing login.
SELECT id, project_id, type, status, user_id, user_name, group_id
FROM t_project_partner
WHERE uid = $1
LIMIT 1;
