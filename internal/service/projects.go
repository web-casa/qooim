package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/web-casa/qooim/internal/idgen"
	"github.com/web-casa/qooim/internal/repo/db"
)

// ErrNotFound is returned by Get/Update/Delete when the row doesn't exist
// (or was already soft-deleted).
var ErrNotFound = errors.New("not found")

// ProjectService handles t_project CRUD.
type ProjectService struct{ q db.Querier }

func NewProjectService(q db.Querier) *ProjectService { return &ProjectService{q: q} }

type CreateProjectInput struct {
	ParentID *string `json:"parent_id,omitempty"`
	Name     string  `json:"name"  binding:"required"`
	Survey   *string `json:"survey,omitempty"`
	Setting  *string `json:"setting,omitempty"`
	Status   *int32  `json:"status,omitempty"`
	Mode     *string `json:"mode,omitempty"`
	Priority *int32  `json:"priority,omitempty"`
}

func (s *ProjectService) Create(ctx context.Context, in CreateProjectInput, createdBy string) (string, error) {
	id := idgen.New()
	parentID := sql.NullString{}
	if in.ParentID != nil {
		parentID = sql.NullString{String: *in.ParentID, Valid: true}
	} else {
		parentID = sql.NullString{String: "0", Valid: true}
	}
	survey := sql.NullString{}
	if in.Survey != nil {
		survey = sql.NullString{String: *in.Survey, Valid: true}
	}
	setting := sql.NullString{}
	if in.Setting != nil {
		setting = sql.NullString{String: *in.Setting, Valid: true}
	}
	status := sql.NullInt32{Int32: 0, Valid: true}
	if in.Status != nil {
		status = sql.NullInt32{Int32: *in.Status, Valid: true}
	}
	mode := sql.NullString{}
	if in.Mode != nil {
		mode = sql.NullString{String: *in.Mode, Valid: true}
	}
	priority := sql.NullInt32{Int32: 1000, Valid: true}
	if in.Priority != nil {
		priority = sql.NullInt32{Int32: *in.Priority, Valid: true}
	}
	name := sql.NullString{String: in.Name, Valid: true}
	createBy := sql.NullString{String: createdBy, Valid: true}
	if err := s.q.CreateProject(ctx, db.CreateProjectParams{
		ID:       id,
		ParentID: parentID,
		Name:     name,
		Survey:   survey,
		Setting:  setting,
		Status:   status,
		Mode:     mode,
		Priority: priority,
		CreateBy: createBy,
	}); err != nil {
		return "", fmt.Errorf("create project: %w", err)
	}
	return id, nil
}

type UpdateProjectInput struct {
	ParentID *string `json:"parent_id,omitempty"`
	Name     *string `json:"name,omitempty"`
	Survey   *string `json:"survey,omitempty"`
	Setting  *string `json:"setting,omitempty"`
	Status   *int32  `json:"status,omitempty"`
	Mode     *string `json:"mode,omitempty"`
	Priority *int32  `json:"priority,omitempty"`
}

func (s *ProjectService) Update(ctx context.Context, id string, in UpdateProjectInput, updatedBy string) error {
	if _, err := s.q.GetProjectByID(ctx, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("load: %w", err)
	}
	params := db.UpdateProjectParams{
		ID:       id,
		UpdateBy: sql.NullString{String: updatedBy, Valid: true},
	}
	if in.ParentID != nil {
		params.ParentID = sql.NullString{String: *in.ParentID, Valid: true}
	}
	if in.Name != nil {
		params.Name = sql.NullString{String: *in.Name, Valid: true}
	}
	if in.Survey != nil {
		params.Survey = sql.NullString{String: *in.Survey, Valid: true}
	}
	if in.Setting != nil {
		params.Setting = sql.NullString{String: *in.Setting, Valid: true}
	}
	if in.Status != nil {
		params.Status = sql.NullInt32{Int32: *in.Status, Valid: true}
	}
	if in.Mode != nil {
		params.Mode = sql.NullString{String: *in.Mode, Valid: true}
	}
	if in.Priority != nil {
		params.Priority = sql.NullInt32{Int32: *in.Priority, Valid: true}
	}
	if err := s.q.UpdateProject(ctx, params); err != nil {
		return fmt.Errorf("update project: %w", err)
	}
	return nil
}

func (s *ProjectService) Get(ctx context.Context, id string) (db.GetProjectByIDRow, error) {
	row, err := s.q.GetProjectByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return row, ErrNotFound
		}
		return row, fmt.Errorf("get project: %w", err)
	}
	return row, nil
}

func (s *ProjectService) SoftDelete(ctx context.Context, id, deletedBy string) error {
	if _, err := s.q.GetProjectByID(ctx, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("load: %w", err)
	}
	if err := s.q.SoftDeleteProject(ctx, db.SoftDeleteProjectParams{
		UpdateBy: sql.NullString{String: deletedBy, Valid: true},
		ID:       id,
	}); err != nil {
		return fmt.Errorf("soft-delete project: %w", err)
	}
	return nil
}
