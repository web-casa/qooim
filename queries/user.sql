-- name: GetUserByID :one
SELECT id, name, dept_id, gender, email, avatar, status, profile
FROM t_user
WHERE id = $1
  AND is_deleted = 0;
