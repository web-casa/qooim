-- name: GetUserByID :one
SELECT id, name, dept_id, gender, email, avatar, status, profile
FROM t_user
WHERE id = $1
  AND is_deleted = 0;

-- name: ListUsers :many
-- Paged sysuser list. Filters by name (ILIKE) and dept_id (exact).
SELECT id, name, dept_id, gender, phone, email, avatar, status, profile,
       create_at, update_at, create_by
FROM t_user
WHERE is_deleted = 0
  AND (sqlc.narg('name')::text    IS NULL OR name ILIKE '%' || sqlc.narg('name')::text || '%')
  AND (sqlc.narg('dept_id')::varchar IS NULL OR dept_id = sqlc.narg('dept_id'))
ORDER BY create_at DESC
LIMIT sqlc.arg('lim') OFFSET sqlc.arg('off');

-- name: CountUsers :one
SELECT COUNT(*) FROM t_user
WHERE is_deleted = 0
  AND (sqlc.narg('name')::text    IS NULL OR name ILIKE '%' || sqlc.narg('name')::text || '%')
  AND (sqlc.narg('dept_id')::varchar IS NULL OR dept_id = sqlc.narg('dept_id'));

-- name: ListUsersForConsole :many
-- Console-side hydrated user list: joins the login account (auth_account)
-- and the user's department name in one query so the table renderer
-- doesn't have to fan out an N+1 per row.
--
-- The account JOIN is wrapped in LATERAL ... LIMIT 1 to avoid
-- multiplying user rows when t_account has more than one active PWD
-- row per user_id (the schema has no uniqueness constraint there
-- yet — see migrations/00001_schema.sql). We pick the
-- earliest-created active account, which matches "the original
-- login" intuition.
SELECT u.id, u.name, u.dept_id, u.email, u.status, u.create_at,
       a.auth_account AS username,
       d.name         AS dept_name
FROM t_user u
LEFT JOIN LATERAL (
  SELECT auth_account
  FROM t_account
  WHERE user_id = u.id
    AND auth_type = 'PWD'
    AND is_deleted = 0
  ORDER BY create_at ASC
  LIMIT 1
) a ON TRUE
LEFT JOIN t_dept d
       ON d.id = u.dept_id
      AND d.is_deleted = 0
WHERE u.is_deleted = 0
  AND (sqlc.narg('name')::text       IS NULL OR u.name ILIKE '%' || sqlc.narg('name')::text || '%')
  AND (sqlc.narg('dept_id')::varchar IS NULL OR u.dept_id = sqlc.narg('dept_id'))
ORDER BY u.create_at DESC
LIMIT sqlc.arg('lim') OFFSET sqlc.arg('off');

-- name: CountAccountsByUsername :one
-- Backs /api/system/checkUsernameExist. Counts active rows so a
-- soft-deleted previous owner of the username doesn't shadow new
-- registrations.
SELECT COUNT(*) FROM t_account
WHERE auth_account = $1 AND auth_type = 'PWD' AND is_deleted = 0;

-- name: CreateUser :exec
INSERT INTO t_user (id, name, dept_id, gender, birthday, phone, email, avatar,
                    status, is_deleted, create_at, create_by, profile)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 0, NOW(), $10, $11);

-- name: UpdateUser :exec
UPDATE t_user SET
    name      = COALESCE(sqlc.narg('name'),    name),
    dept_id   = COALESCE(sqlc.narg('dept_id'), dept_id),
    gender    = COALESCE(sqlc.narg('gender'),  gender),
    birthday  = COALESCE(sqlc.narg('birthday'), birthday),
    phone     = COALESCE(sqlc.narg('phone'),   phone),
    email     = COALESCE(sqlc.narg('email'),   email),
    avatar    = COALESCE(sqlc.narg('avatar'),  avatar),
    status    = COALESCE(sqlc.narg('status'),  status),
    profile   = COALESCE(sqlc.narg('profile'), profile),
    update_by = $1
WHERE id = $2 AND is_deleted = 0;

-- name: SoftDeleteUser :exec
UPDATE t_user SET is_deleted = 1, update_by = $1 WHERE id = $2 AND is_deleted = 0;

-- name: CreateAccount :exec
-- Used together with CreateUser when an admin adds a sysuser. Stores
-- the bcrypt password hash; secret_salt stays NULL (bcrypt embeds its
-- own salt).
INSERT INTO t_account (id, user_type, user_id, auth_type, auth_account, auth_secret,
                       status, is_deleted, create_at, create_by)
VALUES ($1, 'SysUser', $2, 'PWD', $3, $4, 1, 0, NOW(), $5);

-- name: UpdateAccountSecret :exec
UPDATE t_account SET auth_secret = $1, update_by = $2
WHERE user_id = $3 AND user_type = 'SysUser' AND auth_type = 'PWD' AND is_deleted = 0;

-- name: SoftDeleteAccountForUser :exec
UPDATE t_account SET is_deleted = 1, update_by = $1
WHERE user_id = $2 AND user_type = 'SysUser' AND is_deleted = 0;

-- name: ListUserRolesByUserIDs :many
-- Batch role-binding lookup for the SK user list page (and any future
-- caller that needs to render N users at once). Returns a flat
-- (user_id, role_id) stream — caller groups by user_id in memory.
-- Replaces the per-row ListUserRoleIDs N+1 pattern.
SELECT user_id, role_id
FROM t_user_role
WHERE user_type = 'SysUser'
  AND user_id = ANY(sqlc.arg('user_ids')::varchar[]);

-- name: ListUserRoleIDs :many
SELECT role_id FROM t_user_role
WHERE user_id = $1 AND user_type = 'SysUser';

-- name: ReplaceUserRoles :exec
-- Wipes existing role bindings for a user and re-inserts new ones.
-- Wrapped in a tx by the service layer.
DELETE FROM t_user_role WHERE user_id = $1 AND user_type = 'SysUser';

-- name: AddUserRole :exec
INSERT INTO t_user_role (id, user_type, user_id, role_id, create_at, create_by)
VALUES ($1, 'SysUser', $2, $3, NOW(), $4);
