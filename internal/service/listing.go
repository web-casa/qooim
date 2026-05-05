package service

import (
	"context"
	"fmt"

	"github.com/web-casa/qooim/internal/repo/db"
)

const (
	defaultPageSize = 20
	maxPageSize     = 200
)

// ListingService backs the read-only list endpoints in P1.
// Permission gates and partner-scoped filtering land in P2+.
type ListingService struct {
	q db.Querier
}

func NewListingService(q db.Querier) *ListingService {
	return &ListingService{q: q}
}

// Page is a normalised paginator. Page is 1-based.
type Page struct {
	Page     int `json:"page"`
	PageSize int `json:"page_size"`
}

// Normalize clamps Page/PageSize to safe values.
func (p Page) Normalize() Page {
	if p.Page < 1 {
		p.Page = 1
	}
	if p.PageSize < 1 {
		p.PageSize = defaultPageSize
	}
	if p.PageSize > maxPageSize {
		p.PageSize = maxPageSize
	}
	return p
}

// Offset is page * size minus a page (i.e. skip).
func (p Page) Offset() int { return (p.Page - 1) * p.PageSize }

// ListResponse[T] is the standard list payload.
type ListResponse[T any] struct {
	Items    []T `json:"items"`
	Total    int `json:"total"`
	Page     int `json:"page"`
	PageSize int `json:"page_size"`
}

func newListResponse[T any](items []T, total int64, page Page) ListResponse[T] {
	return ListResponse[T]{
		Items:    items,
		Total:    int(total),
		Page:     page.Page,
		PageSize: page.PageSize,
	}
}

func (s *ListingService) Projects(ctx context.Context, p Page) (ListResponse[db.ListProjectsRow], error) {
	p = p.Normalize()
	rows, err := s.q.ListProjects(ctx, db.ListProjectsParams{
		Limit:  int32(p.PageSize),
		Offset: int32(p.Offset()),
	})
	if err != nil {
		return ListResponse[db.ListProjectsRow]{}, fmt.Errorf("list projects: %w", err)
	}
	total, err := s.q.CountProjects(ctx)
	if err != nil {
		return ListResponse[db.ListProjectsRow]{}, fmt.Errorf("count projects: %w", err)
	}
	return newListResponse(rows, total, p), nil
}

func (s *ListingService) Repos(ctx context.Context, p Page) (ListResponse[db.ListReposRow], error) {
	p = p.Normalize()
	rows, err := s.q.ListRepos(ctx, db.ListReposParams{
		Limit:  int32(p.PageSize),
		Offset: int32(p.Offset()),
	})
	if err != nil {
		return ListResponse[db.ListReposRow]{}, fmt.Errorf("list repos: %w", err)
	}
	total, err := s.q.CountRepos(ctx)
	if err != nil {
		return ListResponse[db.ListReposRow]{}, fmt.Errorf("count repos: %w", err)
	}
	return newListResponse(rows, total, p), nil
}

func (s *ListingService) Templates(ctx context.Context, p Page) (ListResponse[db.ListTemplatesRow], error) {
	p = p.Normalize()
	rows, err := s.q.ListTemplates(ctx, db.ListTemplatesParams{
		Limit:  int32(p.PageSize),
		Offset: int32(p.Offset()),
	})
	if err != nil {
		return ListResponse[db.ListTemplatesRow]{}, fmt.Errorf("list templates: %w", err)
	}
	total, err := s.q.CountTemplates(ctx)
	if err != nil {
		return ListResponse[db.ListTemplatesRow]{}, fmt.Errorf("count templates: %w", err)
	}
	return newListResponse(rows, total, p), nil
}

func (s *ListingService) Dashboards(ctx context.Context, p Page) (ListResponse[db.ListDashboardsRow], error) {
	p = p.Normalize()
	rows, err := s.q.ListDashboards(ctx, db.ListDashboardsParams{
		Limit:  int32(p.PageSize),
		Offset: int32(p.Offset()),
	})
	if err != nil {
		return ListResponse[db.ListDashboardsRow]{}, fmt.Errorf("list dashboards: %w", err)
	}
	total, err := s.q.CountDashboards(ctx)
	if err != nil {
		return ListResponse[db.ListDashboardsRow]{}, fmt.Errorf("count dashboards: %w", err)
	}
	return newListResponse(rows, total, p), nil
}
