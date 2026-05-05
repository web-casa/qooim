package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/web-casa/qooim/internal/idgen"
	"github.com/web-casa/qooim/internal/repo/db"
)

type TemplateService struct{ q db.Querier }

func NewTemplateService(q db.Querier) *TemplateService { return &TemplateService{q: q} }

type CreateTemplateInput struct {
	RepoID       *string `json:"repo_id,omitempty"`
	SerialNo     *string `json:"serial_no,omitempty"`
	Name         string  `json:"name" binding:"required"`
	QuestionType *string `json:"question_type,omitempty"`
	Template     *string `json:"template,omitempty"`
	Mode         *string `json:"mode,omitempty"`
	Category     *string `json:"category,omitempty"`
	Tag          *string `json:"tag,omitempty"`
	Priority     *int32  `json:"priority,omitempty"`
	PreviewURL   *string `json:"preview_url,omitempty"`
	Shared       *int16  `json:"shared,omitempty"`
}

func (s *TemplateService) Create(ctx context.Context, in CreateTemplateInput, createdBy string) (string, error) {
	id := idgen.New()
	p := db.CreateTemplateParams{
		ID:       id,
		Name:     sql.NullString{String: in.Name, Valid: true},
		CreateBy: sql.NullString{String: createdBy, Valid: true},
	}
	if in.RepoID != nil {
		p.RepoID = sql.NullString{String: *in.RepoID, Valid: true}
	}
	if in.SerialNo != nil {
		p.SerialNo = sql.NullString{String: *in.SerialNo, Valid: true}
	}
	if in.QuestionType != nil {
		p.QuestionType = sql.NullString{String: *in.QuestionType, Valid: true}
	}
	if in.Template != nil {
		p.Template = sql.NullString{String: *in.Template, Valid: true}
	}
	if in.Mode != nil {
		p.Mode = sql.NullString{String: *in.Mode, Valid: true}
	}
	if in.Category != nil {
		p.Category = sql.NullString{String: *in.Category, Valid: true}
	}
	if in.Tag != nil {
		p.Tag = sql.NullString{String: *in.Tag, Valid: true}
	}
	if in.Priority != nil {
		p.Priority = sql.NullInt32{Int32: *in.Priority, Valid: true}
	}
	if in.PreviewURL != nil {
		p.PreviewUrl = sql.NullString{String: *in.PreviewURL, Valid: true}
	}
	if in.Shared != nil {
		p.Shared = sql.NullInt16{Int16: *in.Shared, Valid: true}
	}
	if err := s.q.CreateTemplate(ctx, p); err != nil {
		return "", fmt.Errorf("create template: %w", err)
	}
	return id, nil
}

type UpdateTemplateInput struct {
	RepoID       *string `json:"repo_id,omitempty"`
	SerialNo     *string `json:"serial_no,omitempty"`
	Name         *string `json:"name,omitempty"`
	QuestionType *string `json:"question_type,omitempty"`
	Template     *string `json:"template,omitempty"`
	Mode         *string `json:"mode,omitempty"`
	Category     *string `json:"category,omitempty"`
	Tag          *string `json:"tag,omitempty"`
	Priority     *int32  `json:"priority,omitempty"`
	PreviewURL   *string `json:"preview_url,omitempty"`
	Shared       *int16  `json:"shared,omitempty"`
}

func (s *TemplateService) Update(ctx context.Context, id string, in UpdateTemplateInput, updatedBy string) error {
	if _, err := s.q.GetTemplateByID(ctx, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("load: %w", err)
	}
	p := db.UpdateTemplateParams{
		ID:       id,
		UpdateBy: sql.NullString{String: updatedBy, Valid: true},
	}
	if in.RepoID != nil {
		p.RepoID = sql.NullString{String: *in.RepoID, Valid: true}
	}
	if in.SerialNo != nil {
		p.SerialNo = sql.NullString{String: *in.SerialNo, Valid: true}
	}
	if in.Name != nil {
		p.Name = sql.NullString{String: *in.Name, Valid: true}
	}
	if in.QuestionType != nil {
		p.QuestionType = sql.NullString{String: *in.QuestionType, Valid: true}
	}
	if in.Template != nil {
		p.Template = sql.NullString{String: *in.Template, Valid: true}
	}
	if in.Mode != nil {
		p.Mode = sql.NullString{String: *in.Mode, Valid: true}
	}
	if in.Category != nil {
		p.Category = sql.NullString{String: *in.Category, Valid: true}
	}
	if in.Tag != nil {
		p.Tag = sql.NullString{String: *in.Tag, Valid: true}
	}
	if in.Priority != nil {
		p.Priority = sql.NullInt32{Int32: *in.Priority, Valid: true}
	}
	if in.PreviewURL != nil {
		p.PreviewUrl = sql.NullString{String: *in.PreviewURL, Valid: true}
	}
	if in.Shared != nil {
		p.Shared = sql.NullInt16{Int16: *in.Shared, Valid: true}
	}
	return s.q.UpdateTemplate(ctx, p)
}

func (s *TemplateService) Get(ctx context.Context, id string) (db.GetTemplateByIDRow, error) {
	row, err := s.q.GetTemplateByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return row, ErrNotFound
		}
		return row, fmt.Errorf("get template: %w", err)
	}
	return row, nil
}

func (s *TemplateService) SoftDelete(ctx context.Context, id, deletedBy string) error {
	if _, err := s.q.GetTemplateByID(ctx, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("load: %w", err)
	}
	return s.q.SoftDeleteTemplate(ctx, db.SoftDeleteTemplateParams{
		UpdateBy: sql.NullString{String: deletedBy, Valid: true},
		ID:       id,
	})
}
