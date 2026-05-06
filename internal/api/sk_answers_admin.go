// SK-compat admin answer flows: /api/answer/{list,delete,destroy,
// trash,restore,update,download,upload}. The C2 file already covers
// /api/public/saveAnswer (participant-side); this file is the operator
// side that sees every submission.
package api

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/web-casa/qooim/internal/repo/db"
	"github.com/web-casa/qooim/internal/service"
)

type skAnswerListItem struct {
	ID               string     `json:"id"`
	ProjectID        string     `json:"projectId"`
	TempSave         int32      `json:"tempSave"`
	ExamScore        float32    `json:"examScore,omitempty"`
	ExamExerciseType string     `json:"examExerciseType,omitempty"`
	CreateAt         time.Time  `json:"createAt"`
	UpdateAt         *time.Time `json:"updateAt,omitempty"`
	CreateBy         string     `json:"createBy,omitempty"`
}

func skAnswerFromList(r db.ListAnswersByProjectRow) skAnswerListItem {
	out := skAnswerListItem{
		ID:        r.ID,
		ProjectID: r.ProjectID,
		CreateAt:  r.CreateAt,
		UpdateAt:  nullTime(r.UpdateAt),
		CreateBy:  valueOr(r.CreateBy),
	}
	if r.TempSave.Valid {
		out.TempSave = r.TempSave.Int32
	}
	if r.ExamScore.Valid {
		out.ExamScore = float32(r.ExamScore.Float64)
	}
	if r.ExamExerciseType.Valid {
		out.ExamExerciseType = r.ExamExerciseType.String
	}
	return out
}

func skAnswerFromTrashed(r db.ListTrashedAnswersRow) skAnswerListItem {
	out := skAnswerListItem{
		ID:        r.ID,
		ProjectID: r.ProjectID,
		CreateAt:  r.CreateAt,
		UpdateAt:  nullTime(r.UpdateAt),
		CreateBy:  valueOr(r.CreateBy),
	}
	if r.TempSave.Valid {
		out.TempSave = r.TempSave.Int32
	}
	if r.ExamScore.Valid {
		out.ExamScore = float32(r.ExamScore.Float64)
	}
	if r.ExamExerciseType.Valid {
		out.ExamExerciseType = r.ExamExerciseType.String
	}
	return out
}

// /api/answer/list — admin sees every undeleted answer for a project
// (`projectId` query param).
func (s *Server) handleSKAnswerList(c *gin.Context) {
	pid := c.Query("projectId")
	if pid == "" {
		skErr(c, http.StatusBadRequest, "projectId is required")
		return
	}
	page, _ := strconv.Atoi(c.Query("current"))
	size, _ := strconv.Atoi(c.Query("pageSize"))
	res, err := s.answers.ListByProject(c.Request.Context(), pid,
		service.Page{Page: page, PageSize: size})
	if err != nil {
		s.logger.Error("sk.answer.list", "err", err)
		skErr(c, http.StatusInternalServerError, "list answers")
		return
	}
	// res.Items is already a service-level DTO with snake_case JSON
	// tags; reshape for the SK frontend.
	items := make([]skAnswerListItem, len(res.Items))
	for i, a := range res.Items {
		items[i] = skAnswerListItem{
			ID:               a.ID,
			ProjectID:        a.ProjectID,
			TempSave:         a.TempSave,
			ExamScore:        a.ExamScore,
			ExamExerciseType: a.ExamExerciseType,
			CreateAt:         a.CreateAt,
			UpdateAt:         a.UpdateAt,
			CreateBy:         a.CreateBy,
		}
	}
	skList(c, items, res.Total)
}

// /api/answer/trash — admin lists soft-deleted answers (recovery UI).
func (s *Server) handleSKAnswerTrash(c *gin.Context) {
	page, _ := strconv.Atoi(c.Query("current"))
	size, _ := strconv.Atoi(c.Query("pageSize"))
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 20
	}
	if size > 200 {
		size = 200
	}
	params := db.ListTrashedAnswersParams{Lim: int32(size), Off: int32((page - 1) * size)}
	if v := c.Query("projectId"); v != "" {
		params.ProjectID = sql.NullString{String: v, Valid: true}
	}
	rows, err := s.q.ListTrashedAnswers(c.Request.Context(), params)
	if err != nil {
		s.logger.Error("sk.answer.trash", "err", err)
		skErr(c, http.StatusInternalServerError, "list trashed answers")
		return
	}
	total, _ := s.q.CountTrashedAnswers(c.Request.Context(), params.ProjectID)
	items := make([]skAnswerListItem, len(rows))
	for i, r := range rows {
		items[i] = skAnswerFromTrashed(r)
	}
	skList(c, items, int(total))
}

// /api/answer/delete — soft delete (single id or ids). Lenient on stale
// ids per the SK convention.
func (s *Server) handleSKAnswerDelete(c *gin.Context) {
	ids := readIDOrIDs(c)
	if len(ids) == 0 {
		skErr(c, http.StatusBadRequest, "id or ids is required")
		return
	}
	results := make([]gin.H, 0, len(ids))
	for _, id := range ids {
		if err := s.answers.SoftDelete(c.Request.Context(), id, principalID(c)); err != nil {
			if errors.Is(err, service.ErrNotFound) {
				results = append(results, gin.H{"id": id, "ok": false, "reason": "not_found"})
				continue
			}
			s.logger.Error("sk.answer.delete", "err", err, "id", id)
			results = append(results, gin.H{"id": id, "ok": false, "reason": "error"})
			skList(c, results, len(ids))
			return
		}
		results = append(results, gin.H{"id": id, "ok": true})
	}
	skOK(c, gin.H{"deleted": countOK(results), "results": results})
}

// /api/answer/restore — un-soft-delete previously trashed rows.
func (s *Server) handleSKAnswerRestore(c *gin.Context) {
	ids := readIDOrIDs(c)
	if len(ids) == 0 {
		skErr(c, http.StatusBadRequest, "id or ids is required")
		return
	}
	by := principalID(c)
	results := make([]gin.H, 0, len(ids))
	for _, id := range ids {
		if err := s.q.RestoreAnswer(c.Request.Context(), db.RestoreAnswerParams{
			UpdateBy: sql.NullString{String: by, Valid: true},
			ID:       id,
		}); err != nil {
			s.logger.Error("sk.answer.restore", "err", err, "id", id)
			results = append(results, gin.H{"id": id, "ok": false, "reason": "error"})
			skList(c, results, len(ids))
			return
		}
		results = append(results, gin.H{"id": id, "ok": true})
	}
	skOK(c, gin.H{"restored": countOK(results), "results": results})
}

// /api/answer/destroy — hard delete (skips soft-delete and physically
// removes the row). Used by the trash bin's "purge" action.
func (s *Server) handleSKAnswerDestroy(c *gin.Context) {
	ids := readIDOrIDs(c)
	if len(ids) == 0 {
		skErr(c, http.StatusBadRequest, "id or ids is required")
		return
	}
	results := make([]gin.H, 0, len(ids))
	for _, id := range ids {
		if err := s.q.HardDeleteAnswer(c.Request.Context(), id); err != nil {
			s.logger.Error("sk.answer.destroy", "err", err, "id", id)
			results = append(results, gin.H{"id": id, "ok": false, "reason": "error"})
			skList(c, results, len(ids))
			return
		}
		results = append(results, gin.H{"id": id, "ok": true})
	}
	skOK(c, gin.H{"destroyed": countOK(results), "results": results})
}

// /api/answer/update — admin edits an existing answer row. Doesn't
// touch the project's published-state check (admin should be able to
// fix typos in finalized exams or unpublished surveys), and doesn't
// re-snapshot the survey JSON — only the patchable columns flow
// through.
func (s *Server) handleSKAnswerUpdate(c *gin.Context) {
	var req struct {
		ID         string         `json:"id"`
		ProjectID  string         `json:"projectId,omitempty"`
		Answer     map[string]any `json:"answer"`
		Attachment *string        `json:"attachment,omitempty"`
		TempSave   *int32         `json:"tempSave,omitempty"`
		ExamScore  *float32       `json:"examScore,omitempty"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.ID == "" {
		skErr(c, http.StatusBadRequest, "id is required")
		return
	}
	in := service.AdminUpdateInput{
		Attachment: req.Attachment,
		TempSave:   req.TempSave,
		ExamScore:  req.ExamScore,
	}
	if req.Answer != nil {
		b, err := json.Marshal(req.Answer)
		if err == nil {
			in.Answer = b
		}
	}
	if err := s.answers.AdminUpdate(c.Request.Context(), req.ID, in, principalID(c)); err != nil {
		if errors.Is(err, service.ErrNotFound) {
			skErr(c, http.StatusNotFound, "answer not found")
			return
		}
		s.logger.Error("sk.answer.update", "err", err)
		skErr(c, http.StatusInternalServerError, "update answer")
		return
	}
	skOK(c, gin.H{"id": req.ID})
}

// /api/answer/download — paginated xlsx export of every undeleted
// answer for a project. Wraps the existing ReportService streamer; the
// only SK-compat-shaped thing here is the route + filename header.
func (s *Server) handleSKAnswerDownload(c *gin.Context) {
	pid := c.Query("projectId")
	if pid == "" {
		skErr(c, http.StatusBadRequest, "projectId is required")
		return
	}
	var buf bytes.Buffer
	if err := s.reports.ExportProjectAnswers(c.Request.Context(), pid, &buf); err != nil {
		s.logger.Error("sk.answer.download", "err", err)
		skErr(c, http.StatusInternalServerError, "export")
		return
	}
	c.Header("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	c.Header("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{
		"filename": "answers-" + pid + ".xlsx",
	}))
	_, _ = c.Writer.Write(buf.Bytes())
}

// /api/answer/upload — admin uploads an xlsx of pre-populated answers.
// SK's old behaviour was very tolerant — we mirror by accepting any
// xlsx with a header row whose first column is "answer" and treating
// each row as a one-shot saveAnswer call against the supplied
// projectId. Failures abort with a 400 rather than partially apply.
func (s *Server) handleSKAnswerUpload(c *gin.Context) {
	pid := c.Query("projectId")
	if pid == "" {
		skErr(c, http.StatusBadRequest, "projectId is required")
		return
	}
	if s.cfg.Storage.MaxUploadBytes > 0 {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, s.cfg.Storage.MaxUploadBytes)
	}
	fh, err := c.FormFile("file")
	if err != nil {
		skErr(c, http.StatusBadRequest, "multipart 'file' is required")
		return
	}
	f, err := fh.Open()
	if err != nil {
		skErr(c, http.StatusBadRequest, "open upload: "+err.Error())
		return
	}
	defer f.Close()

	created, err := s.reports.ImportAnswersXLSX(c.Request.Context(), pid, f, principalID(c))
	if err != nil {
		s.logger.Error("sk.answer.upload", "err", err)
		skErr(c, http.StatusBadRequest, err.Error())
		return
	}
	skOK(c, gin.H{"created": created})
}

// readBody is a fallback used when JSON binding fails; some SK callers
// send compact form-encoded bodies for trash/restore/destroy.
func readBody(c *gin.Context) []byte {
	b, _ := io.ReadAll(c.Request.Body)
	return b
}
