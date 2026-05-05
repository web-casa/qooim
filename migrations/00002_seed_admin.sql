-- +goose Up
-- Minimal bootstrap data for a fresh Qoo.IM instance:
--   * one super-admin role with the full SK permission list (kept verbatim
--     so P1+ RBAC behaviour matches SK)
--   * one admin user + matching login account
--   * one default sys_info row branded for Qoo.IM
--
-- The bcrypt hash matches SurveyKing's stock default password ("123456").
-- CHANGE IT ON FIRST RUN.
--
-- SK demo content (28 templates, 4 projects, 1026 dict items, etc.) is
-- intentionally NOT seeded — Qoo.IM targets fresh deployments.

INSERT INTO "t_role" ("id", "name", "code", "remark", "authority", "status", "is_deleted", "create_at", "create_by", "update_at", "update_by") VALUES
('1457995481928998914', '超级管理员', 'admin', '系统初始化角色',
 'answer,answer:list,answer:detail,answer:create,answer:update,answer:delete,answer:export,file,file:detail,file:list,file:import,file:delete,project,project:list,project:detail,project:create,project:update,project:delete,project:report,system,system:role,system:role:list,system:user,system:user:list,system:role:create,system:role:update,system:role:delete,system:user:create,system:user:update,system:user:updatePosition,system:user:delete,position,position:list,position:create,system:position,system:position:update,system:position:delete,system:org,system:org:list,system:org:create,system:org:update,system:org:delete,template,template:list,template:create,template:update,template:delete,system:position:list,system:position:create,system:dept,system:dept:list,system:dept:create,system:dept:update,system:dept:delete,repo,repo:list,repo:detail,repo:create,repo:update,repo:delete,user,user:update,answer:upload,system:dict,system:dict:update,system:dict:delete,system:dictItem,system:dictItem:list,system:dictItem:create,system:dictItem:import,system:dictItem:delete,system:dict:list,system:dict:create,exercise,exercise:list,repo:book,system:dictItem:update,home',
 1, 0, NOW(), NULL, NULL, NULL);

INSERT INTO "t_user" ("id", "name", "dept_id", "gender", "birthday", "phone", "email", "avatar", "status", "is_deleted", "create_at", "create_by", "update_at", "update_by", "profile", "correct_times") VALUES
('1457995481966747649', 'Admin', NULL, NULL, NULL, NULL, NULL, NULL, 1, 0, NOW(), NULL, NULL, '1457995481966747649', NULL, NULL);

INSERT INTO "t_account" ("id", "user_type", "user_id", "auth_type", "auth_account", "auth_secret", "secret_salt", "status", "is_deleted", "create_at", "create_by", "update_at", "update_by") VALUES
('1', 'SysUser', '1457995481966747649', 'PWD', 'admin', '$2a$10$vZk9P3XtbD2KrdLbQYPvBuPAkkUda0OlkDg7io1Q6VEtfFPig/tqO', NULL, 1, 0, NOW(), NULL, NULL, '1457995481966747649');

INSERT INTO "t_user_role" ("id", "user_type", "user_id", "role_id", "create_at", "create_by", "update_at", "update_by") VALUES
('1488542015867121666', 'SysUser', '1457995481966747649', '1457995481928998914', NOW(), '1457995481966747649', NULL, NULL);

INSERT INTO "t_sys_info" ("id", "name", "description", "avatar", "locale", "version", "setting", "ai_setting", "register_info", "is_default", "create_at", "create_by", "update_at", "update_by") VALUES
('1', 'Qoo.IM', 'Surveys, exams, and beyond.', NULL, 'zh-CN', '{}', NULL, NULL, NULL, 1, NOW(), NULL, NULL, NULL);

-- +goose Down
DELETE FROM "t_user_role"  WHERE "id" = '1488542015867121666';
DELETE FROM "t_account"    WHERE "id" = '1';
DELETE FROM "t_user"       WHERE "id" = '1457995481966747649';
DELETE FROM "t_role"       WHERE "id" = '1457995481928998914';
DELETE FROM "t_sys_info"   WHERE "id" = '1';
