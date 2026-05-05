//go:build pg

package e2e

import (
	"net/http"
	"strings"
	"testing"

	"github.com/web-casa/qooim/tests/testenv"
)

// TestP3Survey covers the public-survey + answer-submit + admin-answer
// view paths end to end.
//
//  1. Admin creates a project (status=0). Public GET should 404 — drafts
//     are invisible.
//  2. Admin publishes (status=1). Public GET returns the survey JSON.
//  3. A guest POSTs an answer (no token). Admin paginates over answers
//     and reads the row back.
//  4. Admin soft-deletes the answer; subsequent GET 404s.
func TestP3Survey(t *testing.T) {
	db := testenv.Postgres(t)
	s := testenv.NewServer(t, db)

	tok := login(t, s, "admin", "123456")
	bearer := [2]string{"Authorization", "Bearer " + tok}

	// 1. Create a draft project with a tiny survey JSON.
	body := mustJSON(t, map[string]any{
		"name":   "p3-survey",
		"mode":   "survey",
		"survey": `{"title":"hello"}`,
	})
	r := s.POST(t, "/api/projects", "application/json", body, bearer)
	mustStatus(t, r, http.StatusCreated, "create project")
	var proj struct{ ID string }
	r.JSON(t, &proj)

	// Draft must be invisible publicly.
	r = s.GET(t, "/api/survey/"+proj.ID)
	mustStatus(t, r, http.StatusNotFound, "draft must 404 publicly")

	// 2. Publish (status=1).
	r = s.Do(t, http.MethodPut, "/api/projects/"+proj.ID, "application/json",
		strings.NewReader(mustJSON(t, map[string]any{"status": 1})),
		bearer)
	mustStatus(t, r, http.StatusNoContent, "publish")

	// Public render now succeeds.
	r = s.GET(t, "/api/survey/"+proj.ID)
	mustStatus(t, r, http.StatusOK, "public survey")
	if !strings.Contains(string(r.Body), `hello`) {
		t.Fatalf("survey body didn't render: %s", r.Body)
	}

	// 3. Guest submits an answer.
	r = s.POST(t, "/api/survey/"+proj.ID+"/answer", "application/json",
		`{"answer":{"q1":"A"},"temp_save":1}`)
	mustStatus(t, r, http.StatusCreated, "submit answer")
	var ans struct{ ID string }
	r.JSON(t, &ans)

	// Admin lists answers for that project.
	r = s.GET(t, "/api/projects/"+proj.ID+"/answers", bearer)
	mustStatus(t, r, http.StatusOK, "list answers")
	if !strings.Contains(string(r.Body), `"`+ans.ID+`"`) {
		t.Fatalf("listed answers missing the new id: %s", r.Body)
	}

	// Admin reads a single answer (must contain the survey snapshot).
	r = s.GET(t, "/api/answers/"+ans.ID, bearer)
	mustStatus(t, r, http.StatusOK, "get answer")
	if !strings.Contains(string(r.Body), `hello`) {
		t.Fatalf("answer should contain survey snapshot: %s", r.Body)
	}

	// 4. Soft-delete; re-read 404s.
	r = s.Do(t, http.MethodDelete, "/api/answers/"+ans.ID, "", nil, bearer)
	mustStatus(t, r, http.StatusNoContent, "delete answer")
	r = s.GET(t, "/api/answers/"+ans.ID, bearer)
	mustStatus(t, r, http.StatusNotFound, "deleted answer 404")
}
