-- name: GetAccountByLogin :one
-- Fetch an active login record for password-style authentication.
SELECT id, user_type, user_id, auth_type, auth_account, auth_secret, secret_salt, status
FROM t_account
WHERE auth_account = $1
  AND auth_type = $2
  AND is_deleted = 0
  AND status = 1
LIMIT 1;
