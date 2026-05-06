// SystemService backs /api/system/* — admin CRUD on departments,
// roles, users (sysuser), positions, dictionaries, and the singleton
// sys_info row. Each method is a thin shim over the sqlc-generated
// repo: it normalises NULL handling, generates IDs, and (for users)
// orchestrates the multi-table create/update flows.
package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/web-casa/qooim/internal/auth"
	"github.com/web-casa/qooim/internal/idgen"
	"github.com/web-casa/qooim/internal/repo/db"
)

type SystemService struct {
	q     db.Querier
	rawDB *sql.DB // for transactional flows (user create/update with role assignments)
}

func NewSystemService(q db.Querier, rawDB *sql.DB) *SystemService {
	return &SystemService{q: q, rawDB: rawDB}
}

// =========================================================================
// Departments
// =========================================================================

type CreateDeptInput struct {
	ParentID     string  `json:"parentId,omitempty"`
	Name         string  `json:"name" binding:"required"`
	ShortName    string  `json:"shortName" binding:"required"`
	Code         *string `json:"code,omitempty"`
	ManagerID    *string `json:"managerId,omitempty"`
	SortCode     *int32  `json:"sortCode,omitempty"`
	PropertyJSON *string `json:"propertyJson,omitempty"`
	Status       *string `json:"status,omitempty"`
	Remark       *string `json:"remark,omitempty"`
}

func (s *SystemService) CreateDept(ctx context.Context, in CreateDeptInput, by string) (string, error) {
	id := idgen.New()
	parentID := in.ParentID
	if parentID == "" {
		parentID = "0"
	}
	params := db.CreateDeptParams{
		ID:        id,
		ParentID:  parentID,
		Name:      sql.NullString{String: in.Name, Valid: true},
		ShortName: in.ShortName,
		CreateBy:  sql.NullString{String: by, Valid: true},
	}
	if in.Code != nil {
		params.Code = sql.NullString{String: *in.Code, Valid: true}
	}
	if in.ManagerID != nil {
		params.ManagerID = sql.NullString{String: *in.ManagerID, Valid: true}
	}
	if in.SortCode != nil {
		params.SortCode = sql.NullInt32{Int32: *in.SortCode, Valid: true}
	}
	if in.PropertyJSON != nil {
		params.PropertyJson = sql.NullString{String: *in.PropertyJSON, Valid: true}
	}
	if in.Status != nil {
		params.Status = sql.NullString{String: *in.Status, Valid: true}
	}
	if in.Remark != nil {
		params.Remark = sql.NullString{String: *in.Remark, Valid: true}
	}
	if err := s.q.CreateDept(ctx, params); err != nil {
		return "", fmt.Errorf("create dept: %w", err)
	}
	return id, nil
}

type UpdateDeptInput struct {
	ParentID     *string `json:"parentId,omitempty"`
	Name         *string `json:"name,omitempty"`
	ShortName    *string `json:"shortName,omitempty"`
	Code         *string `json:"code,omitempty"`
	ManagerID    *string `json:"managerId,omitempty"`
	SortCode     *int32  `json:"sortCode,omitempty"`
	PropertyJSON *string `json:"propertyJson,omitempty"`
	Status       *string `json:"status,omitempty"`
	Remark       *string `json:"remark,omitempty"`
}

func (s *SystemService) UpdateDept(ctx context.Context, id string, in UpdateDeptInput, by string) error {
	if _, err := s.q.GetDeptByID(ctx, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	p := db.UpdateDeptParams{
		ID:       id,
		UpdateBy: sql.NullString{String: by, Valid: true},
	}
	if in.ParentID != nil {
		p.ParentID = sql.NullString{String: *in.ParentID, Valid: true}
	}
	if in.Name != nil {
		p.Name = sql.NullString{String: *in.Name, Valid: true}
	}
	if in.ShortName != nil {
		p.ShortName = sql.NullString{String: *in.ShortName, Valid: true}
	}
	if in.Code != nil {
		p.Code = sql.NullString{String: *in.Code, Valid: true}
	}
	if in.ManagerID != nil {
		p.ManagerID = sql.NullString{String: *in.ManagerID, Valid: true}
	}
	if in.SortCode != nil {
		p.SortCode = sql.NullInt32{Int32: *in.SortCode, Valid: true}
	}
	if in.PropertyJSON != nil {
		p.PropertyJson = sql.NullString{String: *in.PropertyJSON, Valid: true}
	}
	if in.Status != nil {
		p.Status = sql.NullString{String: *in.Status, Valid: true}
	}
	if in.Remark != nil {
		p.Remark = sql.NullString{String: *in.Remark, Valid: true}
	}
	return s.q.UpdateDept(ctx, p)
}

func (s *SystemService) DeleteDept(ctx context.Context, id, by string) error {
	if _, err := s.q.GetDeptByID(ctx, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	return s.q.SoftDeleteDept(ctx, db.SoftDeleteDeptParams{
		UpdateBy: sql.NullString{String: by, Valid: true},
		ID:       id,
	})
}

// =========================================================================
// Roles
// =========================================================================

type CreateRoleInput struct {
	Name      string  `json:"name" binding:"required"`
	Code      string  `json:"code" binding:"required"`
	Remark    *string `json:"remark,omitempty"`
	Authority *string `json:"authority,omitempty"`
	Status    *int16  `json:"status,omitempty"`
}

func (s *SystemService) CreateRole(ctx context.Context, in CreateRoleInput, by string) (string, error) {
	id := idgen.New()
	p := db.CreateRoleParams{
		ID:       id,
		Name:     in.Name,
		Code:     in.Code,
		CreateBy: sql.NullString{String: by, Valid: true},
	}
	if in.Remark != nil {
		p.Remark = sql.NullString{String: *in.Remark, Valid: true}
	}
	if in.Authority != nil {
		p.Authority = sql.NullString{String: *in.Authority, Valid: true}
	}
	if in.Status != nil {
		p.Status = sql.NullInt16{Int16: *in.Status, Valid: true}
	} else {
		p.Status = sql.NullInt16{Int16: 1, Valid: true}
	}
	if err := s.q.CreateRole(ctx, p); err != nil {
		return "", fmt.Errorf("create role: %w", err)
	}
	return id, nil
}

type UpdateRoleInput struct {
	Name      *string `json:"name,omitempty"`
	Code      *string `json:"code,omitempty"`
	Remark    *string `json:"remark,omitempty"`
	Authority *string `json:"authority,omitempty"`
	Status    *int16  `json:"status,omitempty"`
}

func (s *SystemService) UpdateRole(ctx context.Context, id string, in UpdateRoleInput, by string) error {
	if _, err := s.q.GetRoleByID(ctx, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	p := db.UpdateRoleParams{
		ID:       id,
		UpdateBy: sql.NullString{String: by, Valid: true},
	}
	if in.Name != nil {
		p.Name = sql.NullString{String: *in.Name, Valid: true}
	}
	if in.Code != nil {
		p.Code = sql.NullString{String: *in.Code, Valid: true}
	}
	if in.Remark != nil {
		p.Remark = sql.NullString{String: *in.Remark, Valid: true}
	}
	if in.Authority != nil {
		p.Authority = sql.NullString{String: *in.Authority, Valid: true}
	}
	if in.Status != nil {
		p.Status = sql.NullInt16{Int16: *in.Status, Valid: true}
	}
	return s.q.UpdateRole(ctx, p)
}

func (s *SystemService) DeleteRole(ctx context.Context, id, by string) error {
	if _, err := s.q.GetRoleByID(ctx, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	return s.q.SoftDeleteRole(ctx, db.SoftDeleteRoleParams{
		UpdateBy: sql.NullString{String: by, Valid: true},
		ID:       id,
	})
}

// =========================================================================
// Sysusers
// =========================================================================

type CreateUserInput struct {
	Username string   `json:"username" binding:"required"`
	Password string   `json:"password" binding:"required"`
	Name     string   `json:"name"`
	DeptID   *string  `json:"deptId,omitempty"`
	Gender   *string  `json:"gender,omitempty"`
	Phone    *string  `json:"phone,omitempty"`
	Email    *string  `json:"email,omitempty"`
	Avatar   *string  `json:"avatar,omitempty"`
	Profile  *string  `json:"profile,omitempty"`
	Status   *int16   `json:"status,omitempty"`
	RoleIDs  []string `json:"roleIds,omitempty"`
}

// CreateUser performs a 3-step transactional insert: t_user, t_account
// (with bcrypt secret), and the user_role bindings. Failure of any step
// rolls everything back so we never end up with an account that can't
// log in or a user without an account.
func (s *SystemService) CreateUser(ctx context.Context, in CreateUserInput, by string) (string, error) {
	if in.Username == "" || in.Password == "" {
		return "", fmt.Errorf("username and password are required")
	}
	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	uid := idgen.New()
	aid := idgen.New()
	display := in.Name
	if display == "" {
		display = in.Username
	}

	tx, err := s.rawDB.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	qtx := db.New(tx)

	uparams := db.CreateUserParams{
		ID:       uid,
		Name:     display,
		CreateBy: sql.NullString{String: by, Valid: true},
	}
	if in.DeptID != nil {
		uparams.DeptID = sql.NullString{String: *in.DeptID, Valid: true}
	}
	if in.Gender != nil {
		uparams.Gender = sql.NullString{String: *in.Gender, Valid: true}
	}
	if in.Phone != nil {
		uparams.Phone = sql.NullString{String: *in.Phone, Valid: true}
	}
	if in.Email != nil {
		uparams.Email = sql.NullString{String: *in.Email, Valid: true}
	}
	if in.Avatar != nil {
		uparams.Avatar = sql.NullString{String: *in.Avatar, Valid: true}
	}
	if in.Profile != nil {
		uparams.Profile = sql.NullString{String: *in.Profile, Valid: true}
	}
	if in.Status != nil {
		uparams.Status = *in.Status
	} else {
		uparams.Status = 1
	}
	if err := qtx.CreateUser(ctx, uparams); err != nil {
		return "", fmt.Errorf("create user: %w", err)
	}

	if err := qtx.CreateAccount(ctx, db.CreateAccountParams{
		ID:          aid,
		UserID:      uid,
		AuthAccount: in.Username,
		AuthSecret:  sql.NullString{String: hash, Valid: true},
		CreateBy:    sql.NullString{String: by, Valid: true},
	}); err != nil {
		return "", fmt.Errorf("create account: %w", err)
	}

	for _, rid := range in.RoleIDs {
		if err := qtx.AddUserRole(ctx, db.AddUserRoleParams{
			ID:       idgen.New(),
			UserID:   uid,
			RoleID:   rid,
			CreateBy: sql.NullString{String: by, Valid: true},
		}); err != nil {
			return "", fmt.Errorf("add user role %s: %w", rid, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("commit: %w", err)
	}
	return uid, nil
}

type UpdateUserInput struct {
	Name       *string  `json:"name,omitempty"`
	DeptID     *string  `json:"deptId,omitempty"`
	Gender     *string  `json:"gender,omitempty"`
	Phone      *string  `json:"phone,omitempty"`
	Email      *string  `json:"email,omitempty"`
	Avatar     *string  `json:"avatar,omitempty"`
	Profile    *string  `json:"profile,omitempty"`
	Status     *int16   `json:"status,omitempty"`
	Password   *string  `json:"password,omitempty"` // optional reset
	RoleIDs    []string `json:"roleIds,omitempty"`  // nil = leave bindings alone, empty slice = clear
	ResetRoles bool     `json:"resetRoles,omitempty"`
}

func (s *SystemService) UpdateUser(ctx context.Context, id string, in UpdateUserInput, by string) error {
	if _, err := s.q.GetUserByID(ctx, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	tx, err := s.rawDB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	qtx := db.New(tx)

	p := db.UpdateUserParams{
		ID:       id,
		UpdateBy: sql.NullString{String: by, Valid: true},
	}
	if in.Name != nil {
		p.Name = sql.NullString{String: *in.Name, Valid: true}
	}
	if in.DeptID != nil {
		p.DeptID = sql.NullString{String: *in.DeptID, Valid: true}
	}
	if in.Gender != nil {
		p.Gender = sql.NullString{String: *in.Gender, Valid: true}
	}
	if in.Phone != nil {
		p.Phone = sql.NullString{String: *in.Phone, Valid: true}
	}
	if in.Email != nil {
		p.Email = sql.NullString{String: *in.Email, Valid: true}
	}
	if in.Avatar != nil {
		p.Avatar = sql.NullString{String: *in.Avatar, Valid: true}
	}
	if in.Profile != nil {
		p.Profile = sql.NullString{String: *in.Profile, Valid: true}
	}
	if in.Status != nil {
		p.Status = sql.NullInt16{Int16: *in.Status, Valid: true}
	}
	if err := qtx.UpdateUser(ctx, p); err != nil {
		return fmt.Errorf("update user: %w", err)
	}
	if in.Password != nil && *in.Password != "" {
		hash, err := auth.HashPassword(*in.Password)
		if err != nil {
			return fmt.Errorf("hash: %w", err)
		}
		if err := qtx.UpdateAccountSecret(ctx, db.UpdateAccountSecretParams{
			AuthSecret: sql.NullString{String: hash, Valid: true},
			UpdateBy:   sql.NullString{String: by, Valid: true},
			UserID:     id,
		}); err != nil {
			return fmt.Errorf("reset password: %w", err)
		}
	}
	if in.ResetRoles || in.RoleIDs != nil {
		if err := qtx.ReplaceUserRoles(ctx, id); err != nil {
			return fmt.Errorf("clear roles: %w", err)
		}
		for _, rid := range in.RoleIDs {
			if err := qtx.AddUserRole(ctx, db.AddUserRoleParams{
				ID:       idgen.New(),
				UserID:   id,
				RoleID:   rid,
				CreateBy: sql.NullString{String: by, Valid: true},
			}); err != nil {
				return fmt.Errorf("add role: %w", err)
			}
		}
	}
	return tx.Commit()
}

func (s *SystemService) DeleteUser(ctx context.Context, id, by string) error {
	if _, err := s.q.GetUserByID(ctx, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	tx, err := s.rawDB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	qtx := db.New(tx)
	if err := qtx.SoftDeleteUser(ctx, db.SoftDeleteUserParams{
		UpdateBy: sql.NullString{String: by, Valid: true},
		ID:       id,
	}); err != nil {
		return err
	}
	if err := qtx.SoftDeleteAccountForUser(ctx, db.SoftDeleteAccountForUserParams{
		UpdateBy: sql.NullString{String: by, Valid: true},
		UserID:   id,
	}); err != nil {
		return err
	}
	return tx.Commit()
}

// =========================================================================
// Positions, dicts, dict items
// =========================================================================

// (Position/DictItem create+update+delete are direct sqlc calls from
// the handler — no orchestration needed; the SK adapter lives in
// internal/api/sk_system.go.)

// DeleteDictWithItems removes a t_comm_dict row and its dependent
// t_comm_dict_item rows under a single tx so an interrupted delete
// can't leave orphan items behind.
func (s *SystemService) DeleteDictWithItems(ctx context.Context, dictID string) error {
	tx, err := s.rawDB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	qtx := db.New(tx)

	row, err := qtx.GetDictByID(ctx, dictID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("load dict: %w", err)
	}
	if row.Code.Valid {
		if err := qtx.DeleteDictItemsByCode(ctx, row.Code); err != nil {
			return fmt.Errorf("cascade items: %w", err)
		}
	}
	if err := qtx.DeleteDict(ctx, dictID); err != nil {
		return fmt.Errorf("delete dict: %w", err)
	}
	return tx.Commit()
}
