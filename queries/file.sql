-- name: GetFileByID :one
SELECT id, original_name, file_name, file_path, thumb_file_path, storage_type, shared,
       create_at, create_by, update_at, update_by
FROM t_file WHERE id = $1 AND is_deleted = 0;

-- name: CreateFile :exec
INSERT INTO t_file (
    id, original_name, file_name, file_path, thumb_file_path, storage_type, shared,
    is_deleted, create_at, create_by
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, 0, NOW(), $8
);

-- name: SoftDeleteFile :exec
UPDATE t_file SET is_deleted = 1, update_by = $1 WHERE id = $2 AND is_deleted = 0;
