package service

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/web-casa/qooim/internal/domain"
	"github.com/web-casa/qooim/internal/repo/db"
)

const (
	defaultPageSize = 20
	maxPageSize     = 200
)

// ListingService backs the read-only list endpoints.
// Permission gates and partner-scoped filtering land in P3+.
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
	if items == nil {
		items = []T{}
	}
	return ListResponse[T]{
		Items:    items,
		Total:    int(total),
		Page:     page.Page,
		PageSize: page.PageSize,
	}
}

// ProjectFilters mirrors the optional WHERE-clause args of the
// ListProjects query. Pass nil pointers for filters you don't want.
type ProjectFilters struct {
	ParentID *string
	Mode     *string
	Name     *string
}

func nullableStr(p *string) sql.NullString {
	if p == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *p, Valid: true}
}

func (s *ListingService) Projects(ctx context.Context, p Page, f ProjectFilters) (ListResponse[db.ListProjectsRow], error) {
	p = p.Normalize()
	rows, err := s.q.ListProjects(ctx, db.ListProjectsParams{
		ParentID: nullableStr(f.ParentID),
		Mode:     nullableStr(f.Mode),
		Name:     nullableStr(f.Name),
		Lim:      int32(p.PageSize),
		Off:      int32(p.Offset()),
	})
	if err != nil {
		return ListResponse[db.ListProjectsRow]{}, fmt.Errorf("list projects: %w", err)
	}
	total, err := s.q.CountProjects(ctx, db.CountProjectsParams{
		ParentID: nullableStr(f.ParentID),
		Mode:     nullableStr(f.Mode),
		Name:     nullableStr(f.Name),
	})
	if err != nil {
		return ListResponse[db.ListProjectsRow]{}, fmt.Errorf("count projects: %w", err)
	}
	// Both REST and SK handlers map the rows to their own DTO shapes,
	// so the service returns the raw row type.
	return newListResponse(rows, total, p), nil
}

// ProjectsAsDTO is the REST shape — flat camelCase via internal/domain.
func (s *ListingService) ProjectsAsDTO(ctx context.Context, p Page, f ProjectFilters) (ListResponse[domain.ProjectListItem], error) {
	res, err := s.Projects(ctx, p, f)
	if err != nil {
		return ListResponse[domain.ProjectListItem]{}, err
	}
	return newListResponse(domain.ProjectsFromListRows(res.Items), int64(res.Total), p.Normalize()), nil
}

func (s *ListingService) Repos(ctx context.Context, p Page) (ListResponse[domain.RepoListItem], error) {
	p = p.Normalize()
	rows, err := s.q.ListRepos(ctx, db.ListReposParams{
		Limit:  int32(p.PageSize),
		Offset: int32(p.Offset()),
	})
	if err != nil {
		return ListResponse[domain.RepoListItem]{}, fmt.Errorf("list repos: %w", err)
	}
	total, err := s.q.CountRepos(ctx)
	if err != nil {
		return ListResponse[domain.RepoListItem]{}, fmt.Errorf("count repos: %w", err)
	}
	return newListResponse(domain.ReposFromListRows(rows), total, p), nil
}

func (s *ListingService) Templates(ctx context.Context, p Page) (ListResponse[domain.TemplateListItem], error) {
	p = p.Normalize()
	rows, err := s.q.ListTemplates(ctx, db.ListTemplatesParams{
		Limit:  int32(p.PageSize),
		Offset: int32(p.Offset()),
	})
	if err != nil {
		return ListResponse[domain.TemplateListItem]{}, fmt.Errorf("list templates: %w", err)
	}
	total, err := s.q.CountTemplates(ctx)
	if err != nil {
		return ListResponse[domain.TemplateListItem]{}, fmt.Errorf("count templates: %w", err)
	}
	return newListResponse(domain.TemplatesFromListRows(rows), total, p), nil
}

func (s *ListingService) Dashboards(ctx context.Context, p Page) (ListResponse[domain.DashboardListItem], error) {
	p = p.Normalize()
	rows, err := s.q.ListDashboards(ctx, db.ListDashboardsParams{
		Limit:  int32(p.PageSize),
		Offset: int32(p.Offset()),
	})
	if err != nil {
		return ListResponse[domain.DashboardListItem]{}, fmt.Errorf("list dashboards: %w", err)
	}
	total, err := s.q.CountDashboards(ctx)
	if err != nil {
		return ListResponse[domain.DashboardListItem]{}, fmt.Errorf("count dashboards: %w", err)
	}
	return newListResponse(domain.DashboardsFromListRows(rows), total, p), nil
}
