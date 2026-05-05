package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/web-casa/qooim/internal/repo/db"
)

// SurveyService backs the unauthenticated survey-rendering and answer-
// submission flow (the part of the SK answer service that doesn't need
// admin permissions).
//
// Shelved for follow-up:
//   - Random sampling (SK's RandomSurveyProcessor — currently we return
//     the survey JSON verbatim).
//   - Captcha (anji-plus slider) — accepted but not validated in P3.
//   - Partner permission gate beyond uid lookup — P3 only resolves the
//     partner record; rejecting answers based on group/data_permission
//     waits for admin tooling.
type SurveyService struct {
	q db.Querier
}

func NewSurveyService(q db.Querier) *SurveyService { return &SurveyService{q: q} }

// PublicSurvey is the projection returned by GET /api/survey/:projectId.
// Survey content is the SK survey JSON kept as text — clients render it.
type PublicSurvey struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Mode    string `json:"mode,omitempty"`
	Survey  string `json:"survey,omitempty"`
	Setting string `json:"setting,omitempty"`
}

// GetPublic returns the survey if and only if it is published and not
// soft-deleted. Drafts are reported as ErrNotFound to avoid leaking that
// they exist at all.
func (s *SurveyService) GetPublic(ctx context.Context, projectID string) (*PublicSurvey, error) {
	row, err := s.q.GetPublishedSurvey(ctx, projectID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get survey: %w", err)
	}
	out := &PublicSurvey{ID: row.ID}
	if row.Name.Valid {
		out.Name = row.Name.String
	}
	if row.Mode.Valid {
		out.Mode = row.Mode.String
	}
	if row.Survey.Valid {
		out.Survey = row.Survey.String
	}
	if row.Setting.Valid {
		out.Setting = row.Setting.String
	}
	return out, nil
}

// PartnerInfo is what the partner-token middleware reads when ?t=<uid>
// is present on a public route.
type PartnerInfo struct {
	ID        string
	ProjectID string
	UserID    string
	UserName  string
}

// LookupPartner resolves a partner short uid to a (potentially populated)
// PartnerInfo. Missing rows return (nil, ErrNotFound) so the caller can
// decide whether the route requires a partner identity.
func (s *SurveyService) LookupPartner(ctx context.Context, uid string) (*PartnerInfo, error) {
	row, err := s.q.GetPartnerByUID(ctx, sql.NullString{String: uid, Valid: true})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get partner: %w", err)
	}
	out := &PartnerInfo{ID: row.ID}
	if row.ProjectID.Valid {
		out.ProjectID = row.ProjectID.String
	}
	if row.UserID.Valid {
		out.UserID = row.UserID.String
	}
	if row.UserName.Valid {
		out.UserName = row.UserName.String
	}
	return out, nil
}
