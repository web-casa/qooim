package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/web-casa/qooim/internal/excel"
	"github.com/web-casa/qooim/internal/repo/db"
)

type ReportService struct{ q db.Querier }

func NewReportService(q db.Querier) *ReportService { return &ReportService{q: q} }

// ProjectReport is the JSON returned by GET /api/projects/:id/report.
type ProjectReport struct {
	ProjectID string  `json:"project_id"`
	Total     int64   `json:"total"`
	Finished  int64   `json:"finished"`
	Draft     int64   `json:"draft"`
	AvgScore  float64 `json:"avg_score,omitempty"`
}

func (s *ReportService) Project(ctx context.Context, projectID string) (*ProjectReport, error) {
	row, err := s.q.ProjectAnswerStats(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("project report: %w", err)
	}
	return &ProjectReport{
		ProjectID: projectID,
		Total:     row.Total,
		Finished:  row.Finished,
		Draft:     row.Draft,
		AvgScore:  row.AvgScore,
	}, nil
}

// exportPageSize bounds the rows fetched per DB round-trip during a
// streaming xlsx export. Tuned so peak Go heap and PG round-trip cost
// stay sane on a project with tens of thousands of answers.
const exportPageSize = 1000

// ExportProjectAnswers streams an xlsx of every undeleted answer for a
// project to dst. We page through the answer table at the SQL level
// (LIMIT/OFFSET) so the worst case is one page in memory plus
// excelize's internal streaming buffer.
func (s *ReportService) ExportProjectAnswers(ctx context.Context, projectID string, dst io.Writer) error {
	w, err := excel.NewWriter([]string{
		"id", "project_id", "create_at", "create_by", "temp_save",
		"exam_score", "exam_exercise_type", "answer", "meta_info",
	})
	if err != nil {
		return err
	}
	flushed := false
	defer func() {
		// Close the workbook on every error path; Flush() already does
		// it on success but not on early return.
		if !flushed {
			_ = w.Close()
		}
	}()
	offset := int32(0)
	for {
		rows, err := s.q.AnswersForExportPage(ctx, db.AnswersForExportPageParams{
			ProjectID: projectID,
			Limit:     exportPageSize,
			Offset:    offset,
		})
		if err != nil {
			return fmt.Errorf("load answers (offset=%d): %w", offset, err)
		}
		if len(rows) == 0 {
			break
		}
		for _, r := range rows {
			if err := w.AppendRow([]any{
				r.ID,
				r.ProjectID,
				r.CreateAt,
				nullableString(r.CreateBy),
				nullableInt32(r.TempSave),
				nullableFloat(r.ExamScore),
				nullableString(r.ExamExerciseType),
				nullableString(r.Answer),
				nullableString(r.MetaInfo),
			}); err != nil {
				return fmt.Errorf("append row: %w", err)
			}
		}
		if len(rows) < exportPageSize {
			break
		}
		offset += exportPageSize
	}
	if err := w.Flush(dst); err != nil {
		return err
	}
	flushed = true
	return nil
}

// ExerciseProject is one row of the exercise/exam overview.
type ExerciseProject struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	Mode          string  `json:"mode"`
	AnswerCount   int64   `json:"answer_count"`
	FinishedCount int64   `json:"finished_count"`
	AvgScore      float64 `json:"avg_score,omitempty"`
}

func (s *ReportService) Exercises(ctx context.Context) ([]ExerciseProject, error) {
	rows, err := s.q.ListExerciseProjects(ctx)
	if err != nil {
		return nil, fmt.Errorf("list exercises: %w", err)
	}
	out := make([]ExerciseProject, len(rows))
	for i, r := range rows {
		ep := ExerciseProject{
			ID:            r.ID,
			AnswerCount:   r.AnswerCount,
			FinishedCount: r.FinishedCount,
			AvgScore:      r.AvgScore,
		}
		if r.Name.Valid {
			ep.Name = r.Name.String
		}
		if r.Mode.Valid {
			ep.Mode = r.Mode.String
		}
		out[i] = ep
	}
	return out, nil
}

// ImportTemplatesXLSX reads an xlsx with header row [serial_no, name,
// question_type, template, mode, category, tag] and creates one t_template
// row per data row. Returns the number of created rows. Empty rows are
// skipped. The whole import runs in a single transaction is NOT
// implemented in P4 — the service uses individual sqlc calls; recovery
// for partial imports is therefore caller's responsibility.
func (s *ReportService) ImportTemplatesXLSX(ctx context.Context, repoID string, r io.Reader, createdBy string, q db.Querier) (int, error) {
	rows, err := excel.ReadAllRows(r)
	if err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, errors.New("xlsx is empty")
	}
	header := rows[0]
	colIdx := map[string]int{}
	for i, h := range header {
		colIdx[h] = i
	}
	if _, ok := colIdx["name"]; !ok {
		return 0, errors.New("missing required column 'name'")
	}

	created := 0
	for i := 1; i < len(rows); i++ {
		r := rows[i]
		if rowIsEmpty(r) {
			continue
		}
		name := strings.TrimSpace(getCell(r, colIdx, "name"))
		if name == "" {
			return created, fmt.Errorf("row %d: 'name' is required (other cells are populated)", i+1)
		}
		params := db.CreateTemplateParams{
			Name:     sql.NullString{String: name, Valid: true},
			RepoID:   sql.NullString{String: repoID, Valid: true},
			CreateBy: sql.NullString{String: createdBy, Valid: true},
		}
		// We allocate the ID inside the helper because idgen lives in the
		// callee's package, but we want this service to remain pure-DB.
		params.ID = newTemplateID()

		if v := getCell(r, colIdx, "serial_no"); v != "" {
			params.SerialNo = sql.NullString{String: v, Valid: true}
		}
		if v := getCell(r, colIdx, "question_type"); v != "" {
			params.QuestionType = sql.NullString{String: v, Valid: true}
		}
		if v := getCell(r, colIdx, "template"); v != "" {
			params.Template = sql.NullString{String: v, Valid: true}
		}
		if v := getCell(r, colIdx, "mode"); v != "" {
			params.Mode = sql.NullString{String: v, Valid: true}
		}
		if v := getCell(r, colIdx, "category"); v != "" {
			params.Category = sql.NullString{String: v, Valid: true}
		}
		if v := getCell(r, colIdx, "tag"); v != "" {
			params.Tag = sql.NullString{String: v, Valid: true}
		}
		if err := q.CreateTemplate(ctx, params); err != nil {
			return created, fmt.Errorf("row %d: %w", i+1, err)
		}
		created++
	}
	return created, nil
}

// ImportAnswersXLSX accepts an xlsx whose header row may include any
// of {answer, attachment, tempSave, examScore}. Each non-empty data
// row becomes one t_answer insert against the supplied project. The
// service stays single-row-per-insert (consistent with the existing
// AnswerService.Submit path); a per-call tx would localise partial
// failures but isn't needed for C4 import volumes.
func (s *ReportService) ImportAnswersXLSX(ctx context.Context, projectID string, r io.Reader, createdBy string) (int, error) {
	rows, err := excel.ReadAllRows(r)
	if err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, errors.New("xlsx is empty")
	}
	header := rows[0]
	colIdx := map[string]int{}
	for i, h := range header {
		colIdx[strings.TrimSpace(strings.ToLower(h))] = i
	}
	if _, ok := colIdx["answer"]; !ok {
		return 0, errors.New("missing required column 'answer'")
	}

	created := 0
	for i := 1; i < len(rows); i++ {
		row := rows[i]
		if rowIsEmpty(row) {
			continue
		}
		ans := getCell(row, colIdx, "answer")
		if strings.TrimSpace(ans) == "" {
			return created, fmt.Errorf("row %d: 'answer' is required (other cells are populated)", i+1)
		}
		params := db.CreateAnswerParams{
			ID:        newAnswerID(),
			ProjectID: projectID,
			Answer:    sql.NullString{String: ans, Valid: true},
			TempSave:  sql.NullInt32{Int32: 1, Valid: true},
			CreateBy:  sql.NullString{String: createdBy, Valid: true},
		}
		if v := getCell(row, colIdx, "attachment"); v != "" {
			params.Attachment = sql.NullString{String: v, Valid: true}
		}
		if v := getCell(row, colIdx, "tempsave"); v != "" {
			if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
				params.TempSave = sql.NullInt32{Int32: int32(n), Valid: true}
			}
		}
		if v := getCell(row, colIdx, "examscore"); v != "" {
			if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
				params.ExamScore = sql.NullFloat64{Float64: f, Valid: true}
			}
		}
		if err := s.q.CreateAnswer(ctx, params); err != nil {
			return created, fmt.Errorf("row %d: %w", i+1, err)
		}
		created++
	}
	return created, nil
}

// newAnswerID is the same indirection trick we used for templates so
// report.go doesn't need a direct idgen import.
var newAnswerID = newTemplateID

// getCell safely reads cells when a row is shorter than the header.
func getCell(row []string, colIdx map[string]int, name string) string {
	i, ok := colIdx[name]
	if !ok {
		return ""
	}
	if i >= len(row) {
		return ""
	}
	return row[i]
}

// rowIsEmpty reports whether every cell in row is whitespace-only.
func rowIsEmpty(row []string) bool {
	for _, c := range row {
		if strings.TrimSpace(c) != "" {
			return false
		}
	}
	return true
}

func nullableString(n sql.NullString) string {
	if n.Valid {
		return n.String
	}
	return ""
}

func nullableInt32(n sql.NullInt32) any {
	if n.Valid {
		return n.Int32
	}
	return ""
}

func nullableFloat(n sql.NullFloat64) any {
	if n.Valid {
		return n.Float64
	}
	return ""
}
