-- ----- t_comm_dict -----

-- name: ListDicts :many
SELECT id, code, name, remark, dict_type, create_at, update_at, create_by
FROM t_comm_dict
WHERE (sqlc.narg('name')::text  IS NULL OR name ILIKE '%' || sqlc.narg('name')::text || '%')
  AND (sqlc.narg('code')::varchar IS NULL OR code = sqlc.narg('code'))
ORDER BY create_at DESC
LIMIT sqlc.arg('lim') OFFSET sqlc.arg('off');

-- name: CountDicts :one
SELECT COUNT(*) FROM t_comm_dict
WHERE (sqlc.narg('name')::text  IS NULL OR name ILIKE '%' || sqlc.narg('name')::text || '%')
  AND (sqlc.narg('code')::varchar IS NULL OR code = sqlc.narg('code'));

-- name: GetDictByID :one
SELECT id, code, name, remark, dict_type, create_at, update_at, create_by, update_by
FROM t_comm_dict WHERE id = $1;

-- name: CreateDict :exec
INSERT INTO t_comm_dict (id, code, name, remark, dict_type, create_at, create_by)
VALUES ($1, $2, $3, $4, $5, NOW(), $6);

-- name: UpdateDict :exec
UPDATE t_comm_dict SET
    code      = COALESCE(sqlc.narg('code'),      code),
    name      = COALESCE(sqlc.narg('name'),      name),
    remark    = COALESCE(sqlc.narg('remark'),    remark),
    dict_type = COALESCE(sqlc.narg('dict_type'), dict_type),
    update_by = $1
WHERE id = $2;

-- name: DeleteDict :exec
DELETE FROM t_comm_dict WHERE id = $1;

-- name: DeleteDictItemsByCode :exec
-- Cascade helper: when an admin deletes a t_comm_dict row, drop its
-- items too.
DELETE FROM t_comm_dict_item WHERE dict_code = $1;

-- ----- t_comm_dict_item -----

-- name: ListDictItems :many
SELECT id, dict_code, item_name, item_value, item_order, item_level,
       parent_item_value, create_at, update_at, create_by
FROM t_comm_dict_item
WHERE (sqlc.narg('dict_code')::varchar IS NULL OR dict_code = sqlc.narg('dict_code'))
  AND (sqlc.narg('parent_item_value')::varchar IS NULL OR parent_item_value = sqlc.narg('parent_item_value'))
  AND (sqlc.narg('item_name')::text IS NULL OR item_name ILIKE '%' || sqlc.narg('item_name')::text || '%')
ORDER BY COALESCE(item_order, 0) ASC, create_at ASC
LIMIT sqlc.arg('lim') OFFSET sqlc.arg('off');

-- name: CountDictItems :one
SELECT COUNT(*) FROM t_comm_dict_item
WHERE (sqlc.narg('dict_code')::varchar IS NULL OR dict_code = sqlc.narg('dict_code'))
  AND (sqlc.narg('parent_item_value')::varchar IS NULL OR parent_item_value = sqlc.narg('parent_item_value'))
  AND (sqlc.narg('item_name')::text IS NULL OR item_name ILIKE '%' || sqlc.narg('item_name')::text || '%');

-- name: CreateDictItem :exec
INSERT INTO t_comm_dict_item (id, dict_code, item_name, item_value, item_order,
                              item_level, parent_item_value, create_at, create_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, NOW(), $8);

-- name: UpdateDictItem :exec
UPDATE t_comm_dict_item SET
    dict_code         = COALESCE(sqlc.narg('dict_code'),         dict_code),
    item_name         = COALESCE(sqlc.narg('item_name'),         item_name),
    item_value        = COALESCE(sqlc.narg('item_value'),        item_value),
    item_order        = COALESCE(sqlc.narg('item_order'),        item_order),
    item_level        = COALESCE(sqlc.narg('item_level'),        item_level),
    parent_item_value = COALESCE(sqlc.narg('parent_item_value'), parent_item_value),
    update_by         = $1
WHERE id = $2;

-- name: DeleteDictItem :exec
DELETE FROM t_comm_dict_item WHERE id = $1;

