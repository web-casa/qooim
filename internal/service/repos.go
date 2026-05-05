package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/web-casa/qooim/internal/idgen"
	"github.com/web-casa/qooim/internal/repo/db"
)

type RepoService struct{ q db.Querier }

func NewRepoService(q db.Querier) *RepoService { return &RepoService{q: q} }

type CreateRepoInput struct {
	Name        string  `json:"name" binding:"required"`
	Description *string `json:"description,omitempty"`
	Category    *string `json:"category,omitempty"`
	Mode        *string `json:"mode,omitempty"`
	Shared      *int16  `json:"shared,omitempty"`
	Tag         *string `json:"tag,omitempty"`
	Priority    *int32  `json:"priority,omitempty"`
	Setting     *string `json:"setting,omitempty"`
	IsPractice  *int16  `json:"is_practice,omitempty"`
}

func (s *RepoService) Create(ctx context.Context, in CreateRepoInput, createdBy string) (string, error) {
	id := idgen.New()
	params := db.CreateRepoParams{
		ID:       id,
		Name:     sql.NullString{String: in.Name, Valid: true},
		CreateBy: sql.NullString{String: createdBy, Valid: true},
	}
	if in.Description != nil {
		params.Description = sql.NullString{String: *in.Description, Valid: true}
	}
	if in.Category != nil {
		params.Category = sql.NullString{String: *in.Category, Valid: true}
	}
	if in.Mode != nil {
		params.Mode = sql.NullString{String: *in.Mode, Valid: true}
	}
	if in.Shared != nil {
		params.Shared = sql.NullInt16{Int16: *in.Shared, Valid: true}
	}
	if in.Tag != nil {
		params.Tag = sql.NullString{String: *in.Tag, Valid: true}
	}
	if in.Priority != nil {
		params.Priority = sql.NullInt32{Int32: *in.Priority, Valid: true}
	}
	if in.Setting != nil {
		params.Setting = sql.NullString{String: *in.Setting, Valid: true}
	}
	if in.IsPractice != nil {
		params.IsPractice = sql.NullInt16{Int16: *in.IsPractice, Valid: true}
	}
	if err := s.q.CreateRepo(ctx, params); err != nil {
		return "", fmt.Errorf("create repo: %w", err)
	}
	return id, nil
}

type UpdateRepoInput struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
	Category    *string `json:"category,omitempty"`
	Mode        *string `json:"mode,omitempty"`
	Shared      *int16  `json:"shared,omitempty"`
	Tag         *string `json:"tag,omitempty"`
	Priority    *int32  `json:"priority,omitempty"`
	Setting     *string `json:"setting,omitempty"`
	IsPractice  *int16  `json:"is_practice,omitempty"`
}

func (s *RepoService) Update(ctx context.Context, id string, in UpdateRepoInput, updatedBy string) error {
	if _, err := s.q.GetRepoByID(ctx, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("load: %w", err)
	}
	params := db.UpdateRepoParams{
		ID:       id,
		UpdateBy: sql.NullString{String: updatedBy, Valid: true},
	}
	if in.Name != nil {
		params.Name = sql.NullString{String: *in.Name, Valid: true}
	}
	if in.Description != nil {
		params.Description = sql.NullString{String: *in.Description, Valid: true}
	}
	if in.Category != nil {
		params.Category = sql.NullString{String: *in.Category, Valid: true}
	}
	if in.Mode != nil {
		params.Mode = sql.NullString{String: *in.Mode, Valid: true}
	}
	if in.Shared != nil {
		params.Shared = sql.NullInt16{Int16: *in.Shared, Valid: true}
	}
	if in.Tag != nil {
		params.Tag = sql.NullString{String: *in.Tag, Valid: true}
	}
	if in.Priority != nil {
		params.Priority = sql.NullInt32{Int32: *in.Priority, Valid: true}
	}
	if in.Setting != nil {
		params.Setting = sql.NullString{String: *in.Setting, Valid: true}
	}
	if in.IsPractice != nil {
		params.IsPractice = sql.NullInt16{Int16: *in.IsPractice, Valid: true}
	}
	return s.q.UpdateRepo(ctx, params)
}

func (s *RepoService) Get(ctx context.Context, id string) (db.GetRepoByIDRow, error) {
	row, err := s.q.GetRepoByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return row, ErrNotFound
		}
		return row, fmt.Errorf("get repo: %w", err)
	}
	return row, nil
}

func (s *RepoService) Delete(ctx context.Context, id string) error {
	if _, err := s.q.GetRepoByID(ctx, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("load: %w", err)
	}
	return s.q.DeleteRepo(ctx, id)
}
