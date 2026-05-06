// SK-compat public flow.
//
// /api/public/{loadProject,validateProject,saveAnswer,upload} are the
// endpoints the SK frontend hits during the actual answer-taking
// experience. Authentication for participants is the partner URL token
// (`?token=<partner uid>`); admin JWTs aren't required here. C2 keeps
// permission gating loose (any reachable project ID is OK as long as
// status=1) — partner-scoped checks remain SHELVED until C3+.
package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/web-casa/qooim/internal/repo/db"
	"github.com/web-casa/qooim/internal/service"
)

// ---- POST /api/public/loadProject ------------------------------------

type skLoadProjectReq struct {
	ID                string `json:"id"`
	AnswerID          string `json:"answerId,omitempty"`
	RepoID            string `json:"repoId,omitempty"`
	ExamExerciseType  string `json:"examExerciseType,omitempty"`
}

func (s *Server) handleSKLoadProject(c *gin.Context) {
	var req skLoadProjectReq
	_ = c.ShouldBindJSON(&req)
	if req.ID == "" {
		req.ID = c.Query("id")
	}
	if req.ID == "" {
		skErr(c, http.StatusBadRequest, "id is required")
		return
	}
	survey, err := s.surveys.GetPublic(c.Request.Context(), req.ID)
	if errors.Is(err, service.ErrNotFound) {
		skErr(c, http.StatusNotFound, "survey not found or not published")
		return
	}
	if err != nil {
		s.logger.Error("sk.public.loadProject", "err", err)
		skErr(c, http.StatusInternalServerError, "load survey")
		return
	}
	// SK's frontend reads `setting.status` etc. directly, so an explicit
	// empty object is safer than null when the column is unset.
	setting := decodeJSONColumn(survey.Setting)
	if setting == nil {
		setting = json.RawMessage("{}")
	}
	skOK(c, gin.H{
		"id":      survey.ID,
		"name":    survey.Name,
		"mode":    survey.Mode,
		"survey":  decodeJSONColumn(survey.Survey),
		"setting": setting,
		// `answerId` is echoed back to the frontend; subsequent
		// saveAnswer calls send it back so we can update the same row
		// instead of creating a new one. Empty here means "start fresh".
		"answerId": "",
	})
}

// ---- POST /api/public/validateProject --------------------------------

// validateProject is SK's pre-submit check (e.g., quota / time window /
// captcha). C2 returns success unless the project itself is gone.
func (s *Server) handleSKValidateProject(c *gin.Context) {
	var req struct {
		ID       string `json:"id"`
		AnswerID string `json:"answerId,omitempty"`
	}
	_ = c.ShouldBindJSON(&req)
	if req.ID == "" {
		skErr(c, http.StatusBadRequest, "id is required")
		return
	}
	if _, err := s.surveys.GetPublic(c.Request.Context(), req.ID); err != nil {
		if errors.Is(err, service.ErrNotFound) {
			skErr(c, http.StatusNotFound, "survey not found or not published")
			return
		}
		s.logger.Error("sk.public.validateProject", "err", err)
		skErr(c, http.StatusInternalServerError, "validate")
		return
	}
	// SK reads `data.answerId` to seed the next saveAnswer call.
	skOK(c, gin.H{"answerId": ""})
}

// ---- POST /api/public/saveAnswer -------------------------------------

type skSaveAnswerReq struct {
	ProjectID    string         `json:"projectId"`
	ID           string         `json:"id,omitempty"`       // SK sometimes uses id for project; treat as alias
	AnswerID     string         `json:"answerId,omitempty"` // resume token from a previous save
	Answer       map[string]any `json:"answer"`
	Attachment   string         `json:"attachment,omitempty"`
	TempSave     int32          `json:"tempSave,omitempty"`
	ExamScore    *float32       `json:"examScore,omitempty"`
	CaptchaToken string         `json:"captchaToken,omitempty"`
}

func (s *Server) handleSKSaveAnswer(c *gin.Context) {
	var req skSaveAnswerReq
	if err := c.ShouldBindJSON(&req); err != nil {
		skErr(c, http.StatusBadRequest, "answer body is required")
		return
	}
	pid := req.ProjectID
	if pid == "" {
		pid = req.ID
	}
	if pid == "" {
		skErr(c, http.StatusBadRequest, "projectId is required")
		return
	}
	// Materialise the answer JSON once so the underlying service gets a
	// json.RawMessage — the public-side service expects opaque JSON.
	answerBytes, err := json.Marshal(req.Answer)
	if err != nil {
		answerBytes = []byte("null")
	}

	meta := service.SubmitMeta{
		IP:        c.ClientIP(),
		UserAgent: c.Request.UserAgent(),
	}
	if t := c.Query("token"); t != "" {
		// Strict mode: a token that's malformed or doesn't match the
		// requested project is a hard reject — SK's old behaviour.
		p, err := s.surveys.LookupPartner(c.Request.Context(), t)
		if err != nil {
			skErr(c, http.StatusUnauthorized, "invalid partner token")
			return
		}
		if p.ProjectID != "" && p.ProjectID != pid {
			skErr(c, http.StatusForbidden, "partner token does not belong to this project")
			return
		}
		meta.Partner = p
	}

	id, err := s.answers.Submit(c.Request.Context(), pid, service.SubmitInput{
		ResumeID:     req.AnswerID,
		Answer:       answerBytes,
		Attachment:   req.Attachment,
		TempSave:     req.TempSave,
		ExamScore:    req.ExamScore,
		CaptchaToken: req.CaptchaToken,
	}, meta)
	if errors.Is(err, service.ErrNotFound) {
		skErr(c, http.StatusNotFound, "survey not found or not published")
		return
	}
	if err != nil {
		s.logger.Error("sk.public.saveAnswer", "err", err)
		skErr(c, http.StatusInternalServerError, "save answer")
		return
	}
	// SK reads `data.answerId`.
	skOK(c, gin.H{"answerId": id})
}

// ---- POST /api/public/upload -----------------------------------------

// Public upload during answer-taking. Same FileService as the admin
// upload route; difference is no JWT requirement.
func (s *Server) handleSKPublicUpload(c *gin.Context) {
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
	createdBy := "guest"
	if t := c.Query("token"); t != "" {
		// Strict on the upload path too: a wrong/expired token is a
		// reject, not silent-anonymous.
		p, err := s.surveys.LookupPartner(c.Request.Context(), t)
		if err != nil {
			skErr(c, http.StatusUnauthorized, "invalid partner token")
			return
		}
		if p.UserID != "" {
			createdBy = p.UserID
		} else {
			createdBy = p.ID
		}
	}
	res, err := s.files.Upload(c.Request.Context(), service.UploadInput{
		OriginalName: fh.Filename,
		Content:      f,
	}, createdBy)
	if err != nil {
		s.logger.Error("sk.public.upload", "err", err)
		skErr(c, http.StatusInternalServerError, "save file")
		return
	}
	// SK reads `data.id`, `data.url` (download URL), `data.originalName`.
	skOK(c, gin.H{
		"id":           res.ID,
		"originalName": res.OriginalName,
		"fileName":     res.FileName,
		"filePath":     res.FilePath,
		"url":          "/api/file?id=" + res.ID,
	})
}


// ---- /api/public/load* stubs --------------------------------------------

// handleSKPublicLoadDict — POST /api/public/loadDict
// Body shape: {dictCode}. SK uses this in the answer page and the
// question editor to populate dropdowns (province/city, industry, etc.)
// without needing an admin login. We forward to ListDictItems with the
// dictCode filter and return the items as a flat array.
func (s *Server) handleSKPublicLoadDict(c *gin.Context) {
	var req struct {
		DictCode string `json:"dictCode"`
		Code     string `json:"code"`
	}
	_ = c.ShouldBindJSON(&req)
	code := req.DictCode
	if code == "" {
		code = req.Code
	}
	if code == "" {
		skOK(c, []any{})
		return
	}
	rows, err := s.q.ListDictItems(c.Request.Context(), db.ListDictItemsParams{
		DictCode: sql.NullString{String: code, Valid: true},
		Lim:      1024,
		Off:      0,
	})
	if err != nil {
		s.logger.Warn("sk.loadDict", "err", err, "code", code)
		skOK(c, []any{})
		return
	}
	items := make([]gin.H, 0, len(rows))
	for _, r := range rows {
		items = append(items, gin.H{
			"id":              r.ID,
			"dictCode":        valueOr(r.DictCode),
			"itemName":        valueOr(r.ItemName),
			"itemValue":       r.ItemValue,
			"itemOrder":       nullInt32Ptr(r.ItemOrder),
			"itemLevel":       nullInt32Ptr(r.ItemLevel),
			"parentItemValue": valueOr(r.ParentItemValue),
		})
	}
	skOK(c, items)
}

// handleSKPublicEmpty / handleSKPublicEmptyList — minimal 200 stubs for
// SK helpers we don't (yet) need to back with real data. The bundle's
// generic-error toast fires on any non-2xx response, so an empty 200
// is the right way to make the page silent without committing to a
// behaviour we'll have to undo later.
func (s *Server) handleSKPublicEmpty(c *gin.Context) {
	skOK(c, gin.H{})
}

func (s *Server) handleSKPublicEmptyList(c *gin.Context) {
	skOK(c, []any{})
}

func nullInt32Ptr(n sql.NullInt32) *int32 {
	if !n.Valid {
		return nil
	}
	v := n.Int32
	return &v
}
