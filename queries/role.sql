-- name: ListRoleCodesByUser :many
-- Return active role codes for a SysUser. Used to populate JWT claims
-- and (in P2+) to enforce permission gates.
SELECT r.code
FROM t_user_role ur
JOIN t_role r ON r.id = ur.role_id
WHERE ur.user_id = $1
  AND ur.user_type = 'SysUser'
  AND r.is_deleted = 0
  AND r.status = 1
ORDER BY r.code;

-- name: ListRoleAuthoritiesByUser :many
SELECT r.code, COALESCE(r.authority, '') AS authority
FROM t_user_role ur
JOIN t_role r ON r.id = ur.role_id
WHERE ur.user_id = $1
  AND ur.user_type = 'SysUser'
  AND r.is_deleted = 0
  AND r.status = 1;

-- name: ListRoles :many
-- Paged list of all roles (with their permission blob). Drives
-- /api/system/role/list. Filters: optional name (ILIKE), code (exact).
SELECT id, name, code, remark, authority, status, create_at, update_at, create_by
FROM t_role
WHERE is_deleted = 0
  AND (sqlc.narg('name')::text   IS NULL OR name ILIKE '%' || sqlc.narg('name')::text || '%')
  AND (sqlc.narg('code')::varchar IS NULL OR code = sqlc.narg('code'))
ORDER BY create_at DESC
LIMIT sqlc.arg('lim') OFFSET sqlc.arg('off');

-- name: CountRoles :one
SELECT COUNT(*) FROM t_role
WHERE is_deleted = 0
  AND (sqlc.narg('name')::text   IS NULL OR name ILIKE '%' || sqlc.narg('name')::text || '%')
  AND (sqlc.narg('code')::varchar IS NULL OR code = sqlc.narg('code'));

-- name: GetRoleByID :one
SELECT id, name, code, remark, authority, status, create_at, update_at, create_by, update_by
FROM t_role WHERE id = $1 AND is_deleted = 0;

-- name: CreateRole :exec
INSERT INTO t_role (id, name, code, remark, authority, status, is_deleted, create_at, create_by)
VALUES ($1, $2, $3, $4, $5, $6, 0, NOW(), $7);

-- name: UpdateRole :exec
UPDATE t_role SET
    name      = COALESCE(sqlc.narg('name'),      name),
    code      = COALESCE(sqlc.narg('code'),      code),
    remark    = COALESCE(sqlc.narg('remark'),    remark),
    authority = COALESCE(sqlc.narg('authority'), authority),
    status    = COALESCE(sqlc.narg('status'),    status),
    update_by = $1
WHERE id = $2 AND is_deleted = 0;

-- name: SoftDeleteRole :exec
UPDATE t_role SET is_deleted = 1, update_by = $1 WHERE id = $2 AND is_deleted = 0;

-- name: AllRoleAuthorities :many
-- Powers /api/system/permission/list: returns every active role's
-- authority column blob (which is itself a comma-separated list of
-- permission codes). The handler unions and de-duplicates in Go.
-- We query unpaged so deployments with hundreds of roles don't lose
-- entries past whatever page-size limit a paged variant would impose.
SELECT COALESCE(authority, '') AS authority
FROM t_role
WHERE is_deleted = 0 AND status = 1 AND authority IS NOT NULL AND authority <> '';
