//go:build pg

package e2e

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"

	"github.com/web-casa/qooim/tests/testenv"
	"github.com/xuri/excelize/v2"
)

// TestP4Reports walks through:
//  1. Build an exam project with two answers, one finished one draft.
//  2. GET /api/projects/:id/report — totals match.
//  3. GET /api/projects/:id/answers.xlsx — bytes are a valid xlsx with
//     header + 2 data rows.
//  4. POST /api/repos/:id/templates/import — server parses xlsx and
//     creates one template per data row.
//  5. GET /api/exercises — the published exam project shows up.
func TestP4Reports(t *testing.T) {
	db := testenv.Postgres(t)
	s := testenv.NewServer(t, db)
	tok := login(t, s, "admin", "123456")
	bearer := [2]string{"Authorization", "Bearer " + tok}

	// Create a published exam project.
	r := s.POST(t, "/api/projects", "application/json", mustJSON(t, map[string]any{
		"name":   "p4-exam",
		"mode":   "exam",
		"status": 1,
		"survey": `{"title":"q4"}`,
	}), bearer)
	mustStatus(t, r, http.StatusCreated, "create project")
	var proj struct{ ID string }
	r.JSON(t, &proj)

	// Two answers: finished + draft.
	r = s.POST(t, "/api/survey/"+proj.ID+"/answer", "application/json",
		`{"answer":{"q1":"A"},"temp_save":1,"exam_score":80.5}`)
	mustStatus(t, r, http.StatusCreated, "submit final")
	r = s.POST(t, "/api/survey/"+proj.ID+"/answer", "application/json",
		`{"answer":{"q1":"B"},"temp_save":0,"exam_score":40}`)
	mustStatus(t, r, http.StatusCreated, "submit draft")

	// Report counts.
	r = s.GET(t, "/api/projects/"+proj.ID+"/report", bearer)
	mustStatus(t, r, http.StatusOK, "project report")
	var rep struct {
		Total, Finished, Draft int64
		AvgScore               float64 `json:"avg_score"`
	}
	r.JSON(t, &rep)
	if rep.Total != 2 || rep.Finished != 1 || rep.Draft != 1 {
		t.Fatalf("counts off: %+v", rep)
	}
	if rep.AvgScore < 60.2 || rep.AvgScore > 60.4 {
		t.Fatalf("avg score off: %v", rep.AvgScore)
	}

	// Export xlsx.
	r = s.GET(t, "/api/projects/"+proj.ID+"/answers.xlsx", bearer)
	mustStatus(t, r, http.StatusOK, "answers.xlsx")
	if !strings.HasPrefix(string(r.Body), "PK") {
		t.Fatalf("xlsx must start with zip magic 'PK', got %q", r.Body[:4])
	}
	f, err := excelize.OpenReader(bytes.NewReader(r.Body))
	if err != nil {
		t.Fatalf("open xlsx: %v", err)
	}
	defer f.Close()
	rows, err := f.GetRows("Sheet1")
	if err != nil {
		t.Fatalf("get rows: %v", err)
	}
	if len(rows) != 3 { // header + 2 data
		t.Fatalf("xlsx rows = %d, want 3", len(rows))
	}
	if rows[0][0] != "id" {
		t.Fatalf("xlsx header[0] = %q, want id", rows[0][0])
	}

	// Build a small import xlsx and upload it.
	repoR := s.POST(t, "/api/repos", "application/json",
		mustJSON(t, map[string]any{"name": "import-target"}), bearer)
	mustStatus(t, repoR, http.StatusCreated, "create repo")
	var repo struct{ ID string }
	repoR.JSON(t, &repo)

	wb := excelize.NewFile()
	defer wb.Close()
	_ = wb.SetSheetRow("Sheet1", "A1", &[]any{"name", "question_type", "template", "mode"})
	_ = wb.SetSheetRow("Sheet1", "A2", &[]any{"Q-imported-1", "Radio", `{"options":["A","B"]}`, "survey"})
	_ = wb.SetSheetRow("Sheet1", "A3", &[]any{"Q-imported-2", "Checkbox", `{"options":["C","D"]}`, "survey"})
	var xbuf bytes.Buffer
	if err := wb.Write(&xbuf); err != nil {
		t.Fatalf("write workbook: %v", err)
	}

	var mp bytes.Buffer
	mw := multipart.NewWriter(&mp)
	fw, _ := mw.CreateFormFile("file", "templates.xlsx")
	_, _ = fw.Write(xbuf.Bytes())
	_ = mw.Close()

	r = s.Do(t, http.MethodPost, "/api/repos/"+repo.ID+"/templates/import",
		mw.FormDataContentType(), &mp, bearer)
	mustStatus(t, r, http.StatusCreated, "templates import")
	if !bytes.Contains(r.Body, []byte(`"created":2`)) {
		t.Fatalf("expected created:2 in response, got %s", r.Body)
	}

	// Exercises list — our project must be in there.
	r = s.GET(t, "/api/exercises", bearer)
	mustStatus(t, r, http.StatusOK, "exercises")
	if !bytes.Contains(r.Body, []byte(proj.ID)) {
		t.Fatalf("exercises missing project: %s", r.Body)
	}
}
