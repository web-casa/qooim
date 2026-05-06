-- +goose Up
-- t_user.dept_id was declared varchar(20) in SK because their snowflake
-- IDs are 19 chars; Qoo.IM uses 26-char ULIDs for any new dept created
-- through /api/system/dept/create, so the FK reference column has to
-- widen to match t_dept.id (varchar(64)). Existing rows are unaffected
-- because Postgres simply changes the column metadata.
ALTER TABLE "t_user" ALTER COLUMN "dept_id" TYPE varchar(64);

-- +goose Down
ALTER TABLE "t_user" ALTER COLUMN "dept_id" TYPE varchar(20);
