-- +goose Up
-- Qoo.IM baseline schema, ported from SurveyKing v1.12.0 init-mysql.sql.
-- Hand-translated to PostgreSQL 18:
--   * tinyint -> smallint (preserves numeric semantics from SK code)
--   * longtext/text -> text; datetime/timestamp -> timestamptz
--   * ON UPDATE CURRENT_TIMESTAMP replaced by trigger qooim_set_update_at
--   * KEY/UNIQUE KEY split into CREATE [UNIQUE] INDEX statements
--   * Workflow (Flowable) tables were never present in this SQL
-- Seed data lives in 00002_seed_admin.sql (kept minimal; no SK demo content).

CREATE TABLE "t_account" (
  "id" varchar(64) NOT NULL,
  "user_type" varchar(100) NOT NULL DEFAULT 'SysUser',
  "user_id" varchar(64) NOT NULL,
  "auth_type" varchar(20) NOT NULL DEFAULT 'PWD',
  "auth_account" varchar(100) NOT NULL,
  "auth_secret" varchar(64) DEFAULT NULL,
  "secret_salt" varchar(32) DEFAULT NULL,
  "status" int NOT NULL DEFAULT 1,
  "is_deleted" smallint NOT NULL DEFAULT 0,
  "create_at" timestamptz NOT NULL DEFAULT NOW(),
  "create_by" varchar(256) DEFAULT NULL,
  "update_at" timestamptz NULL DEFAULT NULL,
  "update_by" varchar(256) DEFAULT NULL,
  PRIMARY KEY (id)
);
CREATE TABLE "t_answer" (
  "id" varchar(64) NOT NULL,
  "project_id" varchar(64) NOT NULL,
  "temp_answer" text,
  "survey" text,
  "answer" text,
  "attachment" varchar(1024) DEFAULT NULL,
  "meta_info" text,
  "temp_save" int DEFAULT NULL,
  "exam_info" text,
  "exam_exercise_type" varchar(4) DEFAULT NULL,
  "exam_score" real DEFAULT NULL,
  "is_deleted" smallint NOT NULL DEFAULT 0,
  "create_at" timestamptz NOT NULL DEFAULT NOW(),
  "create_by" varchar(256) DEFAULT NULL,
  "update_at" timestamptz NULL DEFAULT NULL,
  "update_by" varchar(256) DEFAULT NULL,
  "repo_id" varchar(256) DEFAULT NULL,
  PRIMARY KEY (id)
);
CREATE TABLE "t_comm_dict" (
  "id" varchar(64) NOT NULL,
  "code" varchar(256) DEFAULT NULL,
  "name" varchar(256) DEFAULT NULL,
  "remark" varchar(256) DEFAULT NULL,
  "dict_type" int DEFAULT 1,
  "create_at" timestamptz DEFAULT NULL,
  "create_by" varchar(256) DEFAULT NULL,
  "update_at" timestamptz DEFAULT NULL,
  "update_by" varchar(256) DEFAULT NULL,
  PRIMARY KEY (id)
);
CREATE TABLE "t_comm_dict_item" (
  "id" varchar(64) NOT NULL,
  "dict_code" varchar(256) DEFAULT NULL,
  "item_name" varchar(256) DEFAULT NULL,
  "item_value" varchar(256) NOT NULL,
  "item_order" int DEFAULT NULL,
  "item_level" int DEFAULT NULL,
  "parent_item_value" varchar(64) DEFAULT NULL,
  "create_at" timestamptz DEFAULT NULL,
  "create_by" varchar(256) DEFAULT NULL,
  "update_at" timestamptz DEFAULT NULL,
  "update_by" varchar(256) DEFAULT NULL,
  PRIMARY KEY (id,item_value)
);
CREATE TABLE "t_dashboard" (
  "id" varchar(64) NOT NULL,
  "key" varchar(256) NOT NULL,
  "type" int DEFAULT NULL,
  "project_id" varchar(64) DEFAULT NULL,
  "setting" varchar(1024) DEFAULT NULL,
  "create_at" timestamptz NOT NULL DEFAULT NOW(),
  "create_by" varchar(256) DEFAULT NULL,
  "update_at" timestamptz NULL DEFAULT NULL,
  "update_by" varchar(256) DEFAULT NULL,
  PRIMARY KEY (id)
);
CREATE TABLE "t_dept" (
  "id" varchar(64) NOT NULL,
  "parent_id" varchar(64) NOT NULL,
  "name" varchar(64) DEFAULT NULL,
  "short_name" varchar(64) NOT NULL,
  "code" varchar(64) DEFAULT NULL,
  "manager_id" varchar(64) DEFAULT NULL,
  "sort_code" int DEFAULT NULL,
  "property_json" varchar(256) DEFAULT NULL,
  "status" varchar(20) DEFAULT NULL,
  "remark" varchar(256) DEFAULT NULL,
  "is_deleted" smallint NOT NULL DEFAULT 0,
  "create_at" timestamptz NOT NULL DEFAULT NOW(),
  "create_by" varchar(256) DEFAULT NULL,
  "update_at" timestamptz NULL DEFAULT NULL,
  "update_by" varchar(256) DEFAULT NULL,
  PRIMARY KEY (id)
);
CREATE TABLE "t_file" (
  "id" varchar(64) NOT NULL,
  "original_name" varchar(256) DEFAULT NULL,
  "file_name" varchar(256) DEFAULT NULL,
  "file_path" varchar(512) DEFAULT NULL,
  "thumb_file_path" varchar(512) DEFAULT NULL,
  "storage_type" int DEFAULT NULL,
  "shared" int DEFAULT 0,
  "is_deleted" smallint NOT NULL DEFAULT 0,
  "create_at" timestamptz NOT NULL DEFAULT NOW(),
  "create_by" varchar(256) DEFAULT NULL,
  "update_at" timestamptz NULL DEFAULT NULL,
  "update_by" varchar(256) DEFAULT NULL,
  PRIMARY KEY (id)
);
CREATE TABLE "t_position" (
  "id" varchar(64) NOT NULL,
  "name" varchar(50) NOT NULL,
  "code" varchar(20) DEFAULT NULL,
  "is_virtual" smallint NOT NULL,
  "data_permission_type" varchar(256) DEFAULT NULL,
  "property_json" varchar(20) DEFAULT NULL,
  "is_deleted" smallint NOT NULL DEFAULT 0,
  "create_at" timestamptz NOT NULL DEFAULT NOW(),
  "create_by" varchar(256) DEFAULT NULL,
  "update_at" timestamptz NULL DEFAULT NULL,
  "update_by" varchar(256) DEFAULT NULL,
  PRIMARY KEY (id)
);
CREATE TABLE "t_project" (
  "id" varchar(64) NOT NULL,
  "parent_id" varchar(64) DEFAULT 0,
  "name" text,
  "survey" text,
  "setting" text,
  "status" int DEFAULT 0,
  "mode" varchar(32) DEFAULT NULL,
  "priority" int DEFAULT 1000,
  "is_deleted" smallint NOT NULL DEFAULT 0,
  "create_at" timestamptz NOT NULL DEFAULT NOW(),
  "create_by" varchar(256) DEFAULT NULL,
  "update_at" timestamptz NULL DEFAULT NULL,
  "update_by" varchar(256) DEFAULT NULL,
  PRIMARY KEY (id)
);
CREATE TABLE "t_project_partner" (
  "id" varchar(64) NOT NULL,
  "uid" varchar(64) DEFAULT NULL,
  "project_id" varchar(64) DEFAULT NULL,
  "type" int DEFAULT NULL,
  "status" int DEFAULT 0,
  "user_id" varchar(64) DEFAULT NULL,
  "user_name" varchar(256) DEFAULT NULL,
  "group_id" varchar(64) DEFAULT NULL,
  "data_permission" text,
  "initial_value" text,
  "create_at" timestamptz NOT NULL DEFAULT NOW(),
  "create_by" varchar(256) DEFAULT NULL,
  "update_at" timestamptz NULL DEFAULT NULL,
  "update_by" varchar(256) DEFAULT NULL,
  PRIMARY KEY (id)
);
CREATE TABLE "t_repo" (
  "id" varchar(64) NOT NULL,
  "name" varchar(64) DEFAULT NULL,
  "description" varchar(512) DEFAULT NULL,
  "category" varchar(64) DEFAULT NULL,
  "mode" varchar(32) DEFAULT NULL,
  "shared" smallint DEFAULT 0,
  "tag" varchar(512) DEFAULT NULL,
  "priority" int DEFAULT NULL,
  "setting" text,
  "create_at" timestamptz NOT NULL DEFAULT NOW(),
  "create_by" varchar(256) DEFAULT NULL,
  "update_at" timestamptz NULL DEFAULT NULL,
  "update_by" varchar(256) DEFAULT NULL,
  "is_practice" smallint DEFAULT NULL,
  PRIMARY KEY (id)
);
CREATE TABLE "t_repo_partner" (
  "id" varchar(64) NOT NULL,
  "repo_id" varchar(64) DEFAULT NULL,
  "user_id" varchar(64) DEFAULT NULL,
  "create_at" timestamptz NOT NULL DEFAULT NOW(),
  "create_by" varchar(256) DEFAULT NULL,
  "update_at" timestamptz NULL DEFAULT NULL,
  "update_by" varchar(256) DEFAULT NULL,
  PRIMARY KEY (id)
);
CREATE TABLE "t_repo_template" (
  "id" varchar(64) NOT NULL,
  "template_id" varchar(64) DEFAULT NULL,
  "repo_id" varchar(64) DEFAULT NULL,
  "create_at" timestamptz NOT NULL DEFAULT NOW(),
  "create_by" varchar(256) DEFAULT NULL,
  PRIMARY KEY (id)
);
CREATE TABLE "t_role" (
  "id" varchar(64) NOT NULL,
  "name" varchar(50) NOT NULL,
  "code" varchar(50) NOT NULL,
  "remark" varchar(100) DEFAULT NULL,
  "authority" varchar(3000) DEFAULT NULL,
  "status" smallint DEFAULT 1,
  "is_deleted" smallint NOT NULL DEFAULT 0,
  "create_at" timestamptz NOT NULL DEFAULT NOW(),
  "create_by" varchar(256) DEFAULT NULL,
  "update_at" timestamptz NULL DEFAULT NULL,
  "update_by" varchar(256) DEFAULT NULL,
  PRIMARY KEY (id)
);
CREATE TABLE "t_sys_info" (
  "id" varchar(64) NOT NULL,
  "name" varchar(64) DEFAULT NULL,
  "description" varchar(128) DEFAULT NULL,
  "avatar" varchar(64) DEFAULT NULL,
  "locale" varchar(64) DEFAULT NULL,
  "version" varchar(64) DEFAULT NULL,
  "setting" varchar(1024) DEFAULT NULL,
  "ai_setting" varchar(1024) DEFAULT NULL,
  "register_info" varchar(1024) DEFAULT NULL,
  "is_default" smallint DEFAULT NULL,
  "create_at" timestamptz NOT NULL DEFAULT NOW(),
  "create_by" varchar(256) DEFAULT NULL,
  "update_at" timestamptz NULL DEFAULT NULL,
  "update_by" varchar(256) DEFAULT NULL,
  PRIMARY KEY (id)
);
CREATE TABLE "t_tag" (
  "id" varchar(64) NOT NULL,
  "entity_id" varchar(64) DEFAULT NULL,
  "name" varchar(128) DEFAULT NULL,
  "category" varchar(256) DEFAULT NULL,
  "create_at" timestamptz NOT NULL DEFAULT NOW(),
  "create_by" varchar(256) DEFAULT NULL,
  PRIMARY KEY (id)
);
CREATE TABLE "t_template" (
  "id" varchar(64) NOT NULL,
  "repo_id" varchar(64) DEFAULT NULL,
  "serial_no" varchar(256) DEFAULT NULL,
  "name" varchar(1024) DEFAULT NULL,
  "question_type" varchar(64) DEFAULT NULL,
  "template" text,
  "mode" varchar(32) DEFAULT NULL,
  "category" varchar(256) DEFAULT NULL,
  "tag" varchar(512) DEFAULT NULL,
  "priority" int DEFAULT NULL,
  "preview_url" varchar(512) DEFAULT NULL,
  "shared" smallint DEFAULT 0,
  "is_deleted" smallint NOT NULL DEFAULT 0,
  "create_at" timestamptz NOT NULL DEFAULT NOW(),
  "create_by" varchar(256) DEFAULT NULL,
  "update_at" timestamptz NULL DEFAULT NULL,
  "update_by" varchar(256) DEFAULT NULL,
  PRIMARY KEY (id)
);
CREATE TABLE "t_user" (
  "id" varchar(64) NOT NULL,
  "name" varchar(50) NOT NULL,
  "dept_id" varchar(20) DEFAULT NULL,
  "gender" varchar(10) DEFAULT NULL,
  "birthday" date DEFAULT NULL,
  "phone" varchar(20) DEFAULT NULL,
  "email" varchar(50) DEFAULT NULL,
  "avatar" varchar(200) DEFAULT NULL,
  "status" smallint NOT NULL DEFAULT 1,
  "is_deleted" smallint NOT NULL DEFAULT 0,
  "create_at" timestamptz NOT NULL DEFAULT NOW(),
  "create_by" varchar(256) DEFAULT NULL,
  "update_at" timestamptz NULL DEFAULT NULL,
  "update_by" varchar(256) DEFAULT NULL,
  "profile" varchar(255) DEFAULT NULL,
  "correct_times" int DEFAULT NULL,
  PRIMARY KEY (id)
);
CREATE TABLE "t_user_book" (
  "id" varchar(64) NOT NULL,
  "name" varchar(2048) DEFAULT NULL,
  "template_id" varchar(64) DEFAULT NULL,
  "wrong_times" int DEFAULT NULL,
  "correct_times" int DEFAULT NULL,
  "note" text,
  "status" int DEFAULT NULL,
  "type" int DEFAULT NULL,
  "create_at" timestamptz NOT NULL DEFAULT NOW(),
  "create_by" varchar(256) DEFAULT NULL,
  "update_at" timestamptz NULL DEFAULT NULL,
  "update_by" varchar(256) DEFAULT NULL,
  "repo_id" varchar(256) DEFAULT NULL,
  "is_marked" smallint DEFAULT NULL,
  PRIMARY KEY (id)
);
CREATE TABLE "t_user_position" (
  "id" varchar(64) NOT NULL,
  "user_id" varchar(64) NOT NULL,
  "dept_id" varchar(64) DEFAULT NULL,
  "position_id" varchar(64) DEFAULT NULL,
  "is_primary_position" smallint DEFAULT NULL,
  "propertyJson" varchar(256) DEFAULT NULL,
  "create_at" timestamptz NOT NULL DEFAULT NOW(),
  "create_by" varchar(256) DEFAULT NULL,
  "update_at" timestamptz NULL DEFAULT NULL,
  "update_by" varchar(256) DEFAULT NULL,
  PRIMARY KEY (id)
);
CREATE TABLE "t_user_role" (
  "id" varchar(64) NOT NULL,
  "user_type" varchar(100) NOT NULL DEFAULT 'SysUser',
  "user_id" varchar(64) NOT NULL,
  "role_id" varchar(64) NOT NULL,
  "create_at" timestamptz NOT NULL DEFAULT NOW(),
  "create_by" varchar(256) DEFAULT NULL,
  "update_at" timestamptz NULL DEFAULT NULL,
  "update_by" varchar(256) DEFAULT NULL,
  PRIMARY KEY (id)
);

-- Indexes (extracted from MySQL inline KEY/UNIQUE KEY clauses)
CREATE INDEX "key_answer_pid" ON "t_answer" (project_id);
CREATE INDEX "idx_t_user_role" ON "t_user_role" (user_type,user_id);

-- ON UPDATE CURRENT_TIMESTAMP replacement: a single trigger function,
-- attached to every table that needs auto-bumped update_at.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION qooim_set_update_at() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  NEW.update_at = NOW();
  RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER "t_account_set_update_at" BEFORE UPDATE ON "t_account"
  FOR EACH ROW EXECUTE FUNCTION qooim_set_update_at();
CREATE TRIGGER "t_answer_set_update_at" BEFORE UPDATE ON "t_answer"
  FOR EACH ROW EXECUTE FUNCTION qooim_set_update_at();
CREATE TRIGGER "t_dashboard_set_update_at" BEFORE UPDATE ON "t_dashboard"
  FOR EACH ROW EXECUTE FUNCTION qooim_set_update_at();
CREATE TRIGGER "t_dept_set_update_at" BEFORE UPDATE ON "t_dept"
  FOR EACH ROW EXECUTE FUNCTION qooim_set_update_at();
CREATE TRIGGER "t_file_set_update_at" BEFORE UPDATE ON "t_file"
  FOR EACH ROW EXECUTE FUNCTION qooim_set_update_at();
CREATE TRIGGER "t_position_set_update_at" BEFORE UPDATE ON "t_position"
  FOR EACH ROW EXECUTE FUNCTION qooim_set_update_at();
CREATE TRIGGER "t_project_set_update_at" BEFORE UPDATE ON "t_project"
  FOR EACH ROW EXECUTE FUNCTION qooim_set_update_at();
CREATE TRIGGER "t_project_partner_set_update_at" BEFORE UPDATE ON "t_project_partner"
  FOR EACH ROW EXECUTE FUNCTION qooim_set_update_at();
CREATE TRIGGER "t_repo_set_update_at" BEFORE UPDATE ON "t_repo"
  FOR EACH ROW EXECUTE FUNCTION qooim_set_update_at();
CREATE TRIGGER "t_repo_partner_set_update_at" BEFORE UPDATE ON "t_repo_partner"
  FOR EACH ROW EXECUTE FUNCTION qooim_set_update_at();
CREATE TRIGGER "t_role_set_update_at" BEFORE UPDATE ON "t_role"
  FOR EACH ROW EXECUTE FUNCTION qooim_set_update_at();
CREATE TRIGGER "t_sys_info_set_update_at" BEFORE UPDATE ON "t_sys_info"
  FOR EACH ROW EXECUTE FUNCTION qooim_set_update_at();
CREATE TRIGGER "t_template_set_update_at" BEFORE UPDATE ON "t_template"
  FOR EACH ROW EXECUTE FUNCTION qooim_set_update_at();
CREATE TRIGGER "t_user_set_update_at" BEFORE UPDATE ON "t_user"
  FOR EACH ROW EXECUTE FUNCTION qooim_set_update_at();
CREATE TRIGGER "t_user_book_set_update_at" BEFORE UPDATE ON "t_user_book"
  FOR EACH ROW EXECUTE FUNCTION qooim_set_update_at();
CREATE TRIGGER "t_user_position_set_update_at" BEFORE UPDATE ON "t_user_position"
  FOR EACH ROW EXECUTE FUNCTION qooim_set_update_at();
CREATE TRIGGER "t_user_role_set_update_at" BEFORE UPDATE ON "t_user_role"
  FOR EACH ROW EXECUTE FUNCTION qooim_set_update_at();

-- +goose Down
DROP TABLE IF EXISTS "t_user_role" CASCADE;
DROP TABLE IF EXISTS "t_user_position" CASCADE;
DROP TABLE IF EXISTS "t_user_book" CASCADE;
DROP TABLE IF EXISTS "t_user" CASCADE;
DROP TABLE IF EXISTS "t_template" CASCADE;
DROP TABLE IF EXISTS "t_tag" CASCADE;
DROP TABLE IF EXISTS "t_sys_info" CASCADE;
DROP TABLE IF EXISTS "t_role" CASCADE;
DROP TABLE IF EXISTS "t_repo_template" CASCADE;
DROP TABLE IF EXISTS "t_repo_partner" CASCADE;
DROP TABLE IF EXISTS "t_repo" CASCADE;
DROP TABLE IF EXISTS "t_project_partner" CASCADE;
DROP TABLE IF EXISTS "t_project" CASCADE;
DROP TABLE IF EXISTS "t_position" CASCADE;
DROP TABLE IF EXISTS "t_file" CASCADE;
DROP TABLE IF EXISTS "t_dept" CASCADE;
DROP TABLE IF EXISTS "t_dashboard" CASCADE;
DROP TABLE IF EXISTS "t_comm_dict_item" CASCADE;
DROP TABLE IF EXISTS "t_comm_dict" CASCADE;
DROP TABLE IF EXISTS "t_answer" CASCADE;
DROP TABLE IF EXISTS "t_account" CASCADE;
DROP FUNCTION IF EXISTS qooim_set_update_at();
