-- +goose Up
-- Carry-over fixes from the Codex P0 review.
--
-- 1. Recreate every qooim_set_update_at trigger with a WHEN clause so
--    update_at only bumps when the row actually changes — matching MySQL's
--    ON UPDATE CURRENT_TIMESTAMP semantics. PG's BEFORE UPDATE without a
--    guard fires on any UPDATE, even no-ops, which would diverge from SK.
--
-- 2. Rename t_user_position.propertyJson → property_json so we don't have
--    to double-quote a mixed-case identifier in every query downstream.
--    All other SK audit columns use snake_case; this column was the lone
--    outlier (and isn't read by P0 yet, so renaming is safe).

-- +goose StatementBegin
DO $do$
DECLARE
  t text;
BEGIN
  FOR t IN
    SELECT unnest(ARRAY[
      't_account','t_answer','t_dashboard','t_dept','t_file',
      't_position','t_project','t_project_partner','t_repo','t_repo_partner',
      't_role','t_sys_info','t_template','t_user','t_user_book',
      't_user_position','t_user_role'
    ])
  LOOP
    EXECUTE format('DROP TRIGGER IF EXISTS %I ON %I', t || '_set_update_at', t);
    EXECUTE format(
      'CREATE TRIGGER %I BEFORE UPDATE ON %I FOR EACH ROW '
      'WHEN (OLD.* IS DISTINCT FROM NEW.*) '
      'EXECUTE FUNCTION qooim_set_update_at()',
      t || '_set_update_at', t);
  END LOOP;
END $do$;
-- +goose StatementEnd

ALTER TABLE "t_user_position" RENAME COLUMN "propertyJson" TO "property_json";

-- +goose Down
ALTER TABLE "t_user_position" RENAME COLUMN "property_json" TO "propertyJson";

-- +goose StatementBegin
DO $do$
DECLARE
  t text;
BEGIN
  FOR t IN
    SELECT unnest(ARRAY[
      't_account','t_answer','t_dashboard','t_dept','t_file',
      't_position','t_project','t_project_partner','t_repo','t_repo_partner',
      't_role','t_sys_info','t_template','t_user','t_user_book',
      't_user_position','t_user_role'
    ])
  LOOP
    EXECUTE format('DROP TRIGGER IF EXISTS %I ON %I', t || '_set_update_at', t);
    EXECUTE format(
      'CREATE TRIGGER %I BEFORE UPDATE ON %I FOR EACH ROW '
      'EXECUTE FUNCTION qooim_set_update_at()',
      t || '_set_update_at', t);
  END LOOP;
END $do$;
-- +goose StatementEnd
