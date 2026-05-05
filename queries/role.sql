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
-- Returns the comma-separated authority strings for a user's roles.
-- Splitting/parsing happens in Go because SK stored authorities as a
-- single varchar(3000) blob per role.
SELECT r.code, COALESCE(r.authority, '') AS authority
FROM t_user_role ur
JOIN t_role r ON r.id = ur.role_id
WHERE ur.user_id = $1
  AND ur.user_type = 'SysUser'
  AND r.is_deleted = 0
  AND r.status = 1;
