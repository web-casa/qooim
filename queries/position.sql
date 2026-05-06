-- name: ListPositions :many
SELECT id, name, code, is_virtual, data_permission_type, property_json,
       create_at, update_at, create_by
FROM t_position
WHERE is_deleted = 0
  AND (sqlc.narg('name')::text   IS NULL OR name ILIKE '%' || sqlc.narg('name')::text || '%')
ORDER BY create_at DESC
LIMIT sqlc.arg('lim') OFFSET sqlc.arg('off');

-- name: CountPositions :one
SELECT COUNT(*) FROM t_position
WHERE is_deleted = 0
  AND (sqlc.narg('name')::text   IS NULL OR name ILIKE '%' || sqlc.narg('name')::text || '%');

-- name: GetPositionByID :one
SELECT id, name, code, is_virtual, data_permission_type, property_json,
       create_at, update_at, create_by, update_by
FROM t_position WHERE id = $1 AND is_deleted = 0;

-- name: CreatePosition :exec
INSERT INTO t_position (id, name, code, is_virtual, data_permission_type, property_json,
                        is_deleted, create_at, create_by)
VALUES ($1, $2, $3, $4, $5, $6, 0, NOW(), $7);

-- name: UpdatePosition :exec
UPDATE t_position SET
    name                 = COALESCE(sqlc.narg('name'),                 name),
    code                 = COALESCE(sqlc.narg('code'),                 code),
    is_virtual           = COALESCE(sqlc.narg('is_virtual'),           is_virtual),
    data_permission_type = COALESCE(sqlc.narg('data_permission_type'), data_permission_type),
    property_json        = COALESCE(sqlc.narg('property_json'),        property_json),
    update_by            = $1
WHERE id = $2 AND is_deleted = 0;

-- name: SoftDeletePosition :exec
UPDATE t_position SET is_deleted = 1, update_by = $1 WHERE id = $2 AND is_deleted = 0;
