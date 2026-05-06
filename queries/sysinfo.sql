-- name: GetDefaultSysInfo :one
-- Returns the singleton system-info row. SK seeded id='1'; we tolerate
-- any id but prefer the row marked is_default=1.
SELECT id, name, description, avatar, locale, version,
       setting, ai_setting, register_info, is_default,
       create_at, update_at, create_by, update_by
FROM t_sys_info
ORDER BY (is_default = 1) DESC, create_at ASC
LIMIT 1;

-- name: UpdateDefaultSysInfo :exec
UPDATE t_sys_info SET
    name          = COALESCE(sqlc.narg('name'),          name),
    description   = COALESCE(sqlc.narg('description'),   description),
    avatar        = COALESCE(sqlc.narg('avatar'),        avatar),
    locale        = COALESCE(sqlc.narg('locale'),        locale),
    version       = COALESCE(sqlc.narg('version'),       version),
    setting       = COALESCE(sqlc.narg('setting'),       setting),
    ai_setting    = COALESCE(sqlc.narg('ai_setting'),    ai_setting),
    register_info = COALESCE(sqlc.narg('register_info'), register_info),
    update_by     = $1
WHERE id = $2;
