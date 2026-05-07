//go:build pg

package e2e

import (
	"net/http"
	"strings"
	"testing"

	"github.com/web-casa/qooim/tests/testenv"
)

// TestAnswerUIDemoMode locks the demo route's contract:
//
//   - Demo content only renders for the explicit pid="demo" + ?demo=1
//     pair. Random project IDs return 404 — no fixture leak.
//   - The demo page references both question types the spike covers
//     (radio + upload).
//
// We can't easily exercise the actual fetch() submit path from a Go
// test (it's a JS XHR), but we cover the contract surface that the
// JS depends on: the rendered HTML's structure.
func TestAnswerUIDemoMode(t *testing.T) {
	db := testenv.Postgres(t)
	// Default test env (env="test") has DemoMode on.
	s := testenv.NewServer(t, db)

	t.Run("demo_route_renders", func(t *testing.T) {
		r := s.GET(t, "/answerui/demo?demo=1")
		mustStatus(t, r, http.StatusOK, "demo page")
		body := string(r.Body)
		for _, want := range []string{
			"Qoo.IM Console 答题器 spike",
			`type="radio"`, // radio question
			`type="file"`,  // upload question
			"/api/public/saveAnswer",
			"/api/public/upload",
		} {
			if !strings.Contains(body, want) {
				t.Errorf("demo body missing %q", want)
			}
		}
	})

	t.Run("random_id_404s", func(t *testing.T) {
		// DemoMode + random pid + ?demo=1 → still 404 (must match pid="demo")
		r := s.GET(t, "/answerui/01abc?demo=1")
		mustStatus(t, r, http.StatusNotFound, "random id with demo flag")
	})

	t.Run("demo_pid_without_flag_404s", func(t *testing.T) {
		// pid="demo" but no ?demo=1 → not found, no leak.
		r := s.GET(t, "/answerui/demo")
		mustStatus(t, r, http.StatusNotFound, "demo pid without flag")
	})
}

// TestAnswerUIDemoOffInProd ensures the prod build (DemoMode=false)
// never serves the fixture, even with the magic pid + flag pair.
func TestAnswerUIDemoOffInProd(t *testing.T) {
	db := testenv.Postgres(t)
	s := testenv.NewServer(t, db, testenv.WithEnv("prod"))

	r := s.GET(t, "/answerui/demo?demo=1")
	mustStatus(t, r, http.StatusNotFound, "demo route with DemoMode=false")
	if strings.Contains(string(r.Body), "答题器 spike") {
		t.Errorf("prod build leaked demo content")
	}
}

// TestAnswerSaveContract exercises the saveAnswer endpoint the way
// the answer UI actually does: a draft save with tempSave=0, then a
// final save with tempSave=1 carrying the answerId from the draft.
// Both saves should land on the SAME answer row.
//
// Codex Gate-4 review #2 caught a regression where the UI was
// reading data.data.id but the service returns data.data.answerId,
// so draft-resume silently broke. This test pins the correct
// contract end-to-end.
func TestAnswerSaveContract(t *testing.T) {
	db := testenv.Postgres(t)
	s := testenv.NewServer(t, db)

	// Admin creates a published project so /api/public/saveAnswer
	// has somewhere to write.
	tok := login(t, s, "admin", "123456")
	bearer := [2]string{"Authorization", "Bearer " + tok}
	body := mustJSON(t, map[string]any{
		"name":   "answer-contract",
		"mode":   "survey",
		"status": 1,
		"survey": `{"questions":[{"id":"q1","type":"radio"}]}`,
	})
	r := s.POST(t, "/api/projects", "application/json", body, bearer)
	mustStatus(t, r, http.StatusCreated, "create project")
	var p struct{ ID string }
	r.JSON(t, &p)
	t.Cleanup(func() {
		_, _ = db.ExecContext(t.Context(), `DELETE FROM t_answer WHERE project_id=$1`, p.ID)
		_, _ = db.ExecContext(t.Context(), `UPDATE t_project SET is_deleted=1 WHERE id=$1`, p.ID)
	})

	// Draft save (tempSave=0) — first hit, no answerId yet.
	draftBody := `{"projectId":"` + p.ID + `","answer":{"q1":"a"},"tempSave":0}`
	r = s.POST(t, "/api/public/saveAnswer", "application/json", draftBody)
	mustStatus(t, r, http.StatusOK, "draft save")
	var draft struct {
		Success bool `json:"success"`
		Data    struct {
			AnswerID string `json:"answerId"`
		} `json:"data"`
	}
	r.JSON(t, &draft)
	if !draft.Success || draft.Data.AnswerID == "" {
		t.Fatalf("draft save: success=%v answerId=%q (full body=%s)", draft.Success, draft.Data.AnswerID, r.Body)
	}

	// Final save (tempSave=1) carrying answerId from the draft —
	// must update the SAME row, not insert a new one.
	finalBody := `{"projectId":"` + p.ID + `","answerId":"` + draft.Data.AnswerID + `","answer":{"q1":"b"},"tempSave":1}`
	r = s.POST(t, "/api/public/saveAnswer", "application/json", finalBody)
	mustStatus(t, r, http.StatusOK, "final save")
	var final struct {
		Success bool `json:"success"`
		Data    struct {
			AnswerID string `json:"answerId"`
		} `json:"data"`
	}
	r.JSON(t, &final)
	if final.Data.AnswerID != draft.Data.AnswerID {
		t.Errorf("answerId not stable: draft=%q final=%q", draft.Data.AnswerID, final.Data.AnswerID)
	}

	// Confirm only ONE row exists for this project.
	var n int
	err := db.QueryRowContext(t.Context(),
		`SELECT count(*) FROM t_answer WHERE project_id=$1 AND is_deleted=0`, p.ID).Scan(&n)
	if err != nil {
		t.Fatalf("count answers: %v", err)
	}
	if n != 1 {
		t.Errorf("expected 1 answer row, got %d", n)
	}

	// And confirm the row's temp_save flipped to 1 (finished). Storage
	// contract is "1=finished, 0=draft" — Gate-4 review #1 inversion
	// regression guard.
	var tempSave int
	err = db.QueryRowContext(t.Context(),
		`SELECT temp_save FROM t_answer WHERE project_id=$1`, p.ID).Scan(&tempSave)
	if err != nil {
		t.Fatalf("read answer: %v", err)
	}
	if tempSave != 1 {
		t.Errorf("temp_save=%d, want 1 (finished)", tempSave)
	}
}
