package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/web-casa/qooim/internal/idgen"
	"github.com/web-casa/qooim/internal/repo/db"
)

type AnswerService struct {
	q       db.Querier
	surveys *SurveyService
}

func NewAnswerService(q db.Querier, surveys *SurveyService) *AnswerService {
	return &AnswerService{q: q, surveys: surveys}
}

// SubmitInput is the body of POST /api/survey/:projectId/answer. The
// survey snapshot is filled in by the service from the project row so a
// dishonest client can't claim a different survey shape than what was
// published.
type SubmitInput struct {
	// Answer is opaque JSON (matches SK's t_answer.answer column).
	Answer json.RawMessage `json:"answer" binding:"required"`
	// Attachment is an optional comma-separated list of file IDs.
	Attachment string `json:"attachment,omitempty"`
	// TempSave: 0 = draft, 1 = final.
	TempSave int32 `json:"temp_save,omitempty"`
	// ExamScore (only used in exam mode; SK populates this from grading).
	ExamScore *float32 `json:"exam_score,omitempty"`
	// CaptchaToken is accepted but not validated in P3 — see SHELVED
	// in autonomous-run-log.md for the captcha plan.
	CaptchaToken string `json:"captcha_token,omitempty"`
	// ResumeID, when non-empty and pointing at an existing un-deleted
	// row, makes Submit update that row instead of inserting. Used by
	// the SK draft-save flow so multiple `tempSave: 1` calls don't pile
	// up rows in t_answer.
	ResumeID string `json:"-"`
}

// SubmitMeta is what the handler captures from the request itself
// (IP/UA, partner if present) — kept separate from the user-controllable
// SubmitInput body.
type SubmitMeta struct {
	IP        string
	UserAgent string
	Partner   *PartnerInfo
}

// Submit creates a t_answer row, or updates an existing one when the
// caller supplied a known ResumeID. Partner identity (if any) becomes
// create_by; otherwise create_by is "guest".
func (s *AnswerService) Submit(ctx context.Context, projectID string, in SubmitInput, meta SubmitMeta) (string, error) {
	survey, err := s.surveys.GetPublic(ctx, projectID)
	if err != nil {
		return "", err
	}

	createBy := "guest"
	if meta.Partner != nil {
		if meta.Partner.UserID != "" {
			createBy = meta.Partner.UserID
		} else {
			createBy = meta.Partner.ID
		}
	}
	metaJSON, _ := json.Marshal(map[string]any{
		"ip":         meta.IP,
		"user_agent": meta.UserAgent,
	})

	// Resume path: if the client gave us back a previously-issued
	// answerId we should update that row instead of creating a new one.
	// On a stale id (deleted or unknown) the row count is zero — fall
	// through to insert so the client still gets a valid id back.
	if in.ResumeID != "" {
		// Scope the resume to the same (project, creator) so an
		// attacker who knows or guesses an answerId can't pivot it
		// to another project or clobber a different participant's
		// draft. The COALESCE in the SQL keeps "guest" rows
		// updatable by guests on the same project.
		params := db.UpdateAnswerInPlaceParams{
			UpdateBy:   sql.NullString{String: createBy, Valid: true},
			ID:         in.ResumeID,
			ProjectID:  projectID,
			CreateBy:   sql.NullString{String: createBy, Valid: true},
			Survey:     sql.NullString{String: survey.Survey, Valid: survey.Survey != ""},
			Answer:     sql.NullString{String: string(in.Answer), Valid: len(in.Answer) > 0},
			Attachment: sql.NullString{String: in.Attachment, Valid: in.Attachment != ""},
			MetaInfo:   sql.NullString{String: string(metaJSON), Valid: true},
			TempSave:   sql.NullInt32{Int32: in.TempSave, Valid: true},
		}
		if in.ExamScore != nil {
			params.ExamScore = sql.NullFloat64{Float64: float64(*in.ExamScore), Valid: true}
		}
		n, err := s.q.UpdateAnswerInPlace(ctx, params)
		if err != nil {
			return "", fmt.Errorf("update answer: %w", err)
		}
		if n > 0 {
			return in.ResumeID, nil
		}
	}

	id := idgen.New()
	params := db.CreateAnswerParams{
		ID:        id,
		ProjectID: projectID,
		Survey:    sql.NullString{String: survey.Survey, Valid: survey.Survey != ""},
		Answer:    sql.NullString{String: string(in.Answer), Valid: len(in.Answer) > 0},
		MetaInfo:  sql.NullString{String: string(metaJSON), Valid: true},
		Attachment: sql.NullString{
			String: in.Attachment,
			Valid:  in.Attachment != "",
		},
		TempSave: sql.NullInt32{Int32: in.TempSave, Valid: true},
		CreateBy: sql.NullString{String: createBy, Valid: true},
	}
	if in.ExamScore != nil {
		params.ExamScore = sql.NullFloat64{Float64: float64(*in.ExamScore), Valid: true}
	}
	if survey.Mode == "exam" {
		params.ExamExerciseType = sql.NullString{String: "O", Valid: true}
	}
	if err := s.q.CreateAnswer(ctx, params); err != nil {
		return "", fmt.Errorf("create answer: %w", err)
	}
	return id, nil
}

// AnswerListItem is what /api/projects/:id/answers returns per row.
type AnswerListItem struct {
	ID               string     `json:"id"`
	ProjectID        string     `json:"project_id"`
	TempSave         int32      `json:"temp_save"`
	ExamScore        float32    `json:"exam_score,omitempty"`
	ExamExerciseType string     `json:"exam_exercise_type,omitempty"`
	CreateAt         time.Time  `json:"create_at"`
	UpdateAt         *time.Time `json:"update_at,omitempty"`
	CreateBy         string     `json:"create_by,omitempty"`
}

func (s *AnswerService) ListByProject(ctx context.Context, projectID string, p Page) (ListResponse[AnswerListItem], error) {
	p = p.Normalize()
	rows, err := s.q.ListAnswersByProject(ctx, db.ListAnswersByProjectParams{
		ProjectID: projectID,
		Limit:     int32(p.PageSize),
		Offset:    int32(p.Offset()),
	})
	if err != nil {
		return ListResponse[AnswerListItem]{}, fmt.Errorf("list answers: %w", err)
	}
	total, err := s.q.CountAnswersByProject(ctx, projectID)
	if err != nil {
		return ListResponse[AnswerListItem]{}, fmt.Errorf("count answers: %w", err)
	}
	items := make([]AnswerListItem, len(rows))
	for i, r := range rows {
		ai := AnswerListItem{ID: r.ID, ProjectID: r.ProjectID, CreateAt: r.CreateAt}
		if r.TempSave.Valid {
			ai.TempSave = r.TempSave.Int32
		}
		if r.ExamScore.Valid {
			ai.ExamScore = float32(r.ExamScore.Float64)
		}
		if r.ExamExerciseType.Valid {
			ai.ExamExerciseType = r.ExamExerciseType.String
		}
		if r.UpdateAt.Valid {
			t := r.UpdateAt.Time
			ai.UpdateAt = &t
		}
		if r.CreateBy.Valid {
			ai.CreateBy = r.CreateBy.String
		}
		items[i] = ai
	}
	return newListResponse(items, total, p), nil
}

// AnswerDetail is what GET /api/answers/:id returns to admins.
type AnswerDetail struct {
	ID               string  `json:"id"`
	ProjectID        string  `json:"project_id"`
	Survey           string  `json:"survey,omitempty"`
	Answer           string  `json:"answer,omitempty"`
	Attachment       string  `json:"attachment,omitempty"`
	MetaInfo         string  `json:"meta_info,omitempty"`
	TempSave         int32   `json:"temp_save"`
	ExamScore        float32 `json:"exam_score,omitempty"`
	ExamExerciseType string  `json:"exam_exercise_type,omitempty"`
	CreateBy         string  `json:"create_by,omitempty"`
}

func (s *AnswerService) Get(ctx context.Context, id string) (*AnswerDetail, error) {
	row, err := s.q.GetAnswerByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get answer: %w", err)
	}
	d := &AnswerDetail{ID: row.ID, ProjectID: row.ProjectID}
	if row.Survey.Valid {
		d.Survey = row.Survey.String
	}
	if row.Answer.Valid {
		d.Answer = row.Answer.String
	}
	if row.Attachment.Valid {
		d.Attachment = row.Attachment.String
	}
	if row.MetaInfo.Valid {
		d.MetaInfo = row.MetaInfo.String
	}
	if row.TempSave.Valid {
		d.TempSave = row.TempSave.Int32
	}
	if row.ExamScore.Valid {
		d.ExamScore = float32(row.ExamScore.Float64)
	}
	if row.ExamExerciseType.Valid {
		d.ExamExerciseType = row.ExamExerciseType.String
	}
	if row.CreateBy.Valid {
		d.CreateBy = row.CreateBy.String
	}
	return d, nil
}

func (s *AnswerService) SoftDelete(ctx context.Context, id, by string) error {
	if _, err := s.q.GetAnswerByID(ctx, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("load: %w", err)
	}
	return s.q.SoftDeleteAnswer(ctx, db.SoftDeleteAnswerParams{
		UpdateBy: sql.NullString{String: by, Valid: true},
		ID:       id,
	})
}

// AdminUpdateInput is what handleSKAnswerUpdate sends. Unlike the
// participant-side Submit path, admin edits don't go through the
// "is the project still published?" check and never re-snapshot the
// survey JSON — they only patch the answer columns.
type AdminUpdateInput struct {
	Answer     json.RawMessage
	Attachment *string
	TempSave   *int32
	ExamScore  *float32
}

// AdminUpdate edits an existing t_answer row directly. Returns
// ErrNotFound if the row is gone or already soft-deleted.
func (s *AnswerService) AdminUpdate(ctx context.Context, id string, in AdminUpdateInput, by string) error {
	row, err := s.q.GetAnswerByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("load: %w", err)
	}
	// UpdateAnswerInPlace's WHERE now scopes on (project_id, create_by)
	// to keep public-flow callers from clobbering each other's drafts.
	// Admin edits go through the same query, so we re-emit the row's
	// own (project_id, create_by) to keep the WHERE matching.
	creator := "guest"
	if row.CreateBy.Valid && row.CreateBy.String != "" {
		creator = row.CreateBy.String
	}
	params := db.UpdateAnswerInPlaceParams{
		ID:        id,
		ProjectID: row.ProjectID,
		CreateBy:  sql.NullString{String: creator, Valid: true},
		UpdateBy:  sql.NullString{String: by, Valid: true},
	}
	if len(in.Answer) > 0 {
		params.Answer = sql.NullString{String: string(in.Answer), Valid: true}
	}
	if in.Attachment != nil {
		params.Attachment = sql.NullString{String: *in.Attachment, Valid: true}
	}
	if in.TempSave != nil {
		params.TempSave = sql.NullInt32{Int32: *in.TempSave, Valid: true}
	}
	if in.ExamScore != nil {
		params.ExamScore = sql.NullFloat64{Float64: float64(*in.ExamScore), Valid: true}
	}
	if _, err := s.q.UpdateAnswerInPlace(ctx, params); err != nil {
		return fmt.Errorf("update answer: %w", err)
	}
	return nil
}
