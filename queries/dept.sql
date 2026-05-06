-- name: ListDepts :many
SELECT id, parent_id, name, short_name, code, manager_id, sort_code,
       property_json, status, remark, create_at, update_at, create_by
FROM t_dept WHERE is_deleted = 0
ORDER BY COALESCE(sort_code, 0) ASC, create_at ASC;

-- name: GetDeptByID :one
SELECT id, parent_id, name, short_name, code, manager_id, sort_code,
       property_json, status, remark, create_at, update_at, create_by, update_by
FROM t_dept WHERE id = $1 AND is_deleted = 0;

-- name: CreateDept :exec
INSERT INTO t_dept (
    id, parent_id, name, short_name, code, manager_id, sort_code,
    property_json, status, remark, is_deleted, create_at, create_by
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, 0, NOW(), $11);

-- name: UpdateDept :exec
UPDATE t_dept SET
    parent_id     = COALESCE(sqlc.narg('parent_id'),     parent_id),
    name          = COALESCE(sqlc.narg('name'),          name),
    short_name    = COALESCE(sqlc.narg('short_name'),    short_name),
    code          = COALESCE(sqlc.narg('code'),          code),
    manager_id    = COALESCE(sqlc.narg('manager_id'),    manager_id),
    sort_code     = COALESCE(sqlc.narg('sort_code'),     sort_code),
    property_json = COALESCE(sqlc.narg('property_json'), property_json),
    status        = COALESCE(sqlc.narg('status'),        status),
    remark        = COALESCE(sqlc.narg('remark'),        remark),
    update_by     = $1
WHERE id = $2 AND is_deleted = 0;

-- name: SoftDeleteDept :exec
UPDATE t_dept SET is_deleted = 1, update_by = $1 WHERE id = $2 AND is_deleted = 0;
