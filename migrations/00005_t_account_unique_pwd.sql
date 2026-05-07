-- +goose Up
-- +goose StatementBegin

-- Prevent the same user from having more than one ACTIVE PWD account.
-- Without this, list views had to LATERAL/DISTINCT-pick a canonical
-- row to avoid multiplying users (see queries/user.sql
-- ListUsersForConsole). The LATERAL stays in place as defence in
-- depth — partial unique indexes are not enforced on already-broken
-- data, only on new writes.
--
-- We only constrain `is_deleted = 0` rows so a soft-deleted historic
-- account doesn't conflict with the current one for the same user.
CREATE UNIQUE INDEX IF NOT EXISTS "uniq_t_account_user_pwd_active"
  ON "t_account" (user_id, auth_type)
  WHERE is_deleted = 0;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS "uniq_t_account_user_pwd_active";
-- +goose StatementEnd
