// SK-compat extras for C4: project/repo partner CRUD, repo/book
// (per-user wrong-question / favourites bag), /api/exercise/list (SK
// shape), /api/userOverview, /api/workflow/* placeholder stubs (SK's
// Flowable was dropped in P0), and /api/ai/chat/{create-conversation,
// stream,close-conversation,models} on top of the existing P5
// AIService.
package api

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/web-casa/qooim/internal/ai"
	"github.com/web-casa/qooim/internal/idgen"
	"github.com/web-casa/qooim/internal/repo/db"
)

// ============================================================================
// /api/userOverview — dashboard stats
// ============================================================================

func (s *Server) handleSKUserOverview(c *gin.Context) {
	row, err := s.q.UserOverviewCounts(c.Request.Context())
	if err != nil {
		s.logger.Error("sk.userOverview", "err", err)
		skErr(c, http.StatusInternalServerError, "load overview")
		return
	}
	skOK(c, gin.H{
		"projectTotal":     row.ProjectTotal,
		"projectPublished": row.ProjectPublished,
		"templateTotal":    row.TemplateTotal,
		"repoTotal":        row.RepoTotal,
		"answerTotal":      row.AnswerTotal,
		"answerFinished":   row.AnswerFinished,
		"userTotal":        row.UserTotal,
	})
}

// ============================================================================
// /api/exercise/list — published exam/exercise projects with stats.
// SK shape wraps the same query already used for the REST endpoint.
// ============================================================================

func (s *Server) handleSKExerciseList(c *gin.Context) {
	out, err := s.reports.Exercises(c.Request.Context())
	if err != nil {
		s.logger.Error("sk.exercise.list", "err", err)
		skErr(c, http.StatusInternalServerError, "list exercises")
		return
	}
	// SK frontend reads `data.list[*]` even though there's no
	// pagination here — feed the same envelope skList builds.
	skList(c, out, len(out))
}

// ============================================================================
// /api/project/partner/*  +  /api/repo/partner/*
// ============================================================================

type skProjectPartnerItem struct {
	ID             string          `json:"id"`
	UID            string          `json:"uid,omitempty"`
	ProjectID      string          `json:"projectId,omitempty"`
	Type           int32           `json:"type"`
	Status         int32           `json:"status"`
	UserID         string          `json:"userId,omitempty"`
	UserName       string          `json:"userName,omitempty"`
	GroupID        string          `json:"groupId,omitempty"`
	DataPermission json.RawMessage `json:"dataPermission,omitempty"`
	InitialValue   json.RawMessage `json:"initialValue,omitempty"`
	CreateAt       time.Time       `json:"createAt"`
	UpdateAt       *time.Time      `json:"updateAt,omitempty"`
	CreateBy       string          `json:"createBy,omitempty"`
}

func (s *Server) handleSKProjectPartnerList(c *gin.Context) {
	page, size := pageSize(c)
	params := db.ListProjectPartnersParams{Lim: int32(size), Off: int32((page - 1) * size)}
	if v := c.Query("projectId"); v != "" {
		params.ProjectID = sql.NullString{String: v, Valid: true}
	}
	if v := c.Query("userName"); v != "" {
		params.UserName = sql.NullString{String: v, Valid: true}
	}
	rows, err := s.q.ListProjectPartners(c.Request.Context(), params)
	if err != nil {
		s.logger.Error("sk.projectPartner.list", "err", err)
		skErr(c, http.StatusInternalServerError, "list project partners")
		return
	}
	total, _ := s.q.CountProjectPartners(c.Request.Context(), db.CountProjectPartnersParams{
		ProjectID: params.ProjectID, UserName: params.UserName,
	})
	items := make([]skProjectPartnerItem, len(rows))
	for i, r := range rows {
		item := skProjectPartnerItem{
			ID:             r.ID,
			UID:            valueOr(r.Uid),
			ProjectID:      valueOr(r.ProjectID),
			UserID:         valueOr(r.UserID),
			UserName:       valueOr(r.UserName),
			GroupID:        valueOr(r.GroupID),
			DataPermission: decodeJSONColumn(valueOr(r.DataPermission)),
			InitialValue:   decodeJSONColumn(valueOr(r.InitialValue)),
			CreateAt:       r.CreateAt,
			UpdateAt:       nullTime(r.UpdateAt),
			CreateBy:       valueOr(r.CreateBy),
		}
		if r.Type.Valid {
			item.Type = r.Type.Int32
		}
		if r.Status.Valid {
			item.Status = r.Status.Int32
		}
		items[i] = item
	}
	skList(c, items, int(total))
}

type skProjectPartnerCreateReq struct {
	UID            *string         `json:"uid,omitempty"`
	ProjectID      string          `json:"projectId" binding:"required"`
	Type           *int32          `json:"type,omitempty"`
	Status         *int32          `json:"status,omitempty"`
	UserID         *string         `json:"userId,omitempty"`
	UserName       *string         `json:"userName,omitempty"`
	GroupID        *string         `json:"groupId,omitempty"`
	DataPermission json.RawMessage `json:"dataPermission,omitempty"`
	InitialValue   json.RawMessage `json:"initialValue,omitempty"`
}

func (s *Server) handleSKProjectPartnerCreate(c *gin.Context) {
	var req skProjectPartnerCreateReq
	if err := c.ShouldBindJSON(&req); err != nil || req.ProjectID == "" {
		skErr(c, http.StatusBadRequest, "projectId is required")
		return
	}
	id := idgen.New()
	uid := idgen.New() // short partner uid that SK uses as the URL token
	if req.UID != nil && *req.UID != "" {
		uid = *req.UID
	}
	p := db.CreateProjectPartnerParams{
		ID:        id,
		Uid:       sql.NullString{String: uid, Valid: true},
		ProjectID: sql.NullString{String: req.ProjectID, Valid: true},
		CreateBy:  sql.NullString{String: principalID(c), Valid: true},
	}
	if req.Type != nil {
		p.Type = sql.NullInt32{Int32: *req.Type, Valid: true}
	}
	if req.Status != nil {
		p.Status = sql.NullInt32{Int32: *req.Status, Valid: true}
	}
	if req.UserID != nil {
		p.UserID = sql.NullString{String: *req.UserID, Valid: true}
	}
	if req.UserName != nil {
		p.UserName = sql.NullString{String: *req.UserName, Valid: true}
	}
	if req.GroupID != nil {
		p.GroupID = sql.NullString{String: *req.GroupID, Valid: true}
	}
	if req.DataPermission != nil {
		p.DataPermission = sql.NullString{String: string(req.DataPermission), Valid: true}
	}
	if req.InitialValue != nil {
		p.InitialValue = sql.NullString{String: string(req.InitialValue), Valid: true}
	}
	if err := s.q.CreateProjectPartner(c.Request.Context(), p); err != nil {
		s.logger.Error("sk.projectPartner.create", "err", err)
		skErr(c, http.StatusInternalServerError, "create project partner")
		return
	}
	skOK(c, gin.H{"id": id, "uid": uid})
}

func (s *Server) handleSKProjectPartnerDelete(c *gin.Context) {
	ids := readIDOrIDs(c)
	if len(ids) == 0 {
		skErr(c, http.StatusBadRequest, "id or ids is required")
		return
	}
	for _, id := range ids {
		_ = s.q.DeleteProjectPartner(c.Request.Context(), id)
	}
	skOK(c, gin.H{"deleted": len(ids)})
}

// SK has /api/project/partner/{download,import} for bulk operations
// against an xlsx of name+id rows. C4 returns 200 with empty data so
// the UI's button doesn't crash; full implementation is follow-up.
func (s *Server) handleSKProjectPartnerImport(c *gin.Context) {
	skOK(c, gin.H{"created": 0, "skipped": "implementation pending"})
}

func (s *Server) handleSKProjectPartnerDownload(c *gin.Context) {
	skOK(c, gin.H{"items": []any{}, "skipped": "implementation pending"})
}

// /api/repo/partner/* — same shape, different table.
type skRepoPartnerItem struct {
	ID       string     `json:"id"`
	RepoID   string     `json:"repoId,omitempty"`
	UserID   string     `json:"userId,omitempty"`
	CreateAt time.Time  `json:"createAt"`
	UpdateAt *time.Time `json:"updateAt,omitempty"`
	CreateBy string     `json:"createBy,omitempty"`
}

func (s *Server) handleSKRepoPartnerList(c *gin.Context) {
	page, size := pageSize(c)
	params := db.ListRepoPartnersParams{Lim: int32(size), Off: int32((page - 1) * size)}
	if v := c.Query("repoId"); v != "" {
		params.RepoID = sql.NullString{String: v, Valid: true}
	}
	rows, err := s.q.ListRepoPartners(c.Request.Context(), params)
	if err != nil {
		s.logger.Error("sk.repoPartner.list", "err", err)
		skErr(c, http.StatusInternalServerError, "list repo partners")
		return
	}
	total, _ := s.q.CountRepoPartners(c.Request.Context(), params.RepoID)
	items := make([]skRepoPartnerItem, len(rows))
	for i, r := range rows {
		items[i] = skRepoPartnerItem{
			ID:       r.ID,
			RepoID:   valueOr(r.RepoID),
			UserID:   valueOr(r.UserID),
			CreateAt: r.CreateAt,
			UpdateAt: nullTime(r.UpdateAt),
			CreateBy: valueOr(r.CreateBy),
		}
	}
	skList(c, items, int(total))
}

func (s *Server) handleSKRepoPartnerCreate(c *gin.Context) {
	var req struct {
		RepoID string `json:"repoId" binding:"required"`
		UserID string `json:"userId" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.RepoID == "" || req.UserID == "" {
		skErr(c, http.StatusBadRequest, "repoId and userId are required")
		return
	}
	id := idgen.New()
	if err := s.q.CreateRepoPartner(c.Request.Context(), db.CreateRepoPartnerParams{
		ID:       id,
		RepoID:   sql.NullString{String: req.RepoID, Valid: true},
		UserID:   sql.NullString{String: req.UserID, Valid: true},
		CreateBy: sql.NullString{String: principalID(c), Valid: true},
	}); err != nil {
		s.logger.Error("sk.repoPartner.create", "err", err)
		skErr(c, http.StatusInternalServerError, "create repo partner")
		return
	}
	skOK(c, gin.H{"id": id})
}

func (s *Server) handleSKRepoPartnerDelete(c *gin.Context) {
	ids := readIDOrIDs(c)
	if len(ids) == 0 {
		skErr(c, http.StatusBadRequest, "id or ids is required")
		return
	}
	for _, id := range ids {
		_ = s.q.DeleteRepoPartner(c.Request.Context(), id)
	}
	skOK(c, gin.H{"deleted": len(ids)})
}

// ============================================================================
// /api/repo/book/* — per-user wrong-question / favourites bag
// ============================================================================

type skUserBookItem struct {
	ID           string     `json:"id"`
	Name         string     `json:"name,omitempty"`
	TemplateID   string     `json:"templateId,omitempty"`
	WrongTimes   int32      `json:"wrongTimes,omitempty"`
	CorrectTimes int32      `json:"correctTimes,omitempty"`
	Note         string     `json:"note,omitempty"`
	Status       int32      `json:"status"`
	Type         int32      `json:"type"`
	RepoID       string     `json:"repoId,omitempty"`
	IsMarked     int16      `json:"isMarked"`
	CreateAt     time.Time  `json:"createAt"`
	UpdateAt     *time.Time `json:"updateAt,omitempty"`
	CreateBy     string     `json:"createBy,omitempty"`
}

func (s *Server) handleSKUserBookList(c *gin.Context) {
	page, size := pageSize(c)
	params := db.ListUserBooksParams{Lim: int32(size), Off: int32((page - 1) * size)}
	// Default to the requesting user — book is "per user".
	if pid := principalID(c); pid != "" {
		params.CreateBy = sql.NullString{String: pid, Valid: true}
	}
	if v := c.Query("repoId"); v != "" {
		params.RepoID = sql.NullString{String: v, Valid: true}
	}
	if v := c.Query("type"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			params.Type = sql.NullInt32{Int32: int32(n), Valid: true}
		}
	}
	rows, err := s.q.ListUserBooks(c.Request.Context(), params)
	if err != nil {
		s.logger.Error("sk.userBook.list", "err", err)
		skErr(c, http.StatusInternalServerError, "list user books")
		return
	}
	total, _ := s.q.CountUserBooks(c.Request.Context(), db.CountUserBooksParams{
		CreateBy: params.CreateBy, RepoID: params.RepoID, Type: params.Type,
	})
	items := make([]skUserBookItem, len(rows))
	for i, r := range rows {
		item := skUserBookItem{
			ID:         r.ID,
			Name:       valueOr(r.Name),
			TemplateID: valueOr(r.TemplateID),
			Note:       valueOr(r.Note),
			RepoID:     valueOr(r.RepoID),
			CreateAt:   r.CreateAt,
			UpdateAt:   nullTime(r.UpdateAt),
			CreateBy:   valueOr(r.CreateBy),
		}
		if r.WrongTimes.Valid {
			item.WrongTimes = r.WrongTimes.Int32
		}
		if r.CorrectTimes.Valid {
			item.CorrectTimes = r.CorrectTimes.Int32
		}
		if r.Status.Valid {
			item.Status = r.Status.Int32
		}
		if r.Type.Valid {
			item.Type = r.Type.Int32
		}
		if r.IsMarked.Valid {
			item.IsMarked = r.IsMarked.Int16
		}
		items[i] = item
	}
	skList(c, items, int(total))
}

type skUserBookMutateReq struct {
	ID           string  `json:"id,omitempty"`
	Name         *string `json:"name,omitempty"`
	TemplateID   *string `json:"templateId,omitempty"`
	WrongTimes   *int32  `json:"wrongTimes,omitempty"`
	CorrectTimes *int32  `json:"correctTimes,omitempty"`
	Note         *string `json:"note,omitempty"`
	Status       *int32  `json:"status,omitempty"`
	Type         *int32  `json:"type,omitempty"`
	RepoID       *string `json:"repoId,omitempty"`
	IsMarked     *int16  `json:"isMarked,omitempty"`
}

func (s *Server) handleSKUserBookCreate(c *gin.Context) {
	var req skUserBookMutateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		skErr(c, http.StatusBadRequest, "invalid body")
		return
	}
	id := idgen.New()
	p := db.CreateUserBookParams{ID: id, CreateBy: sql.NullString{String: principalID(c), Valid: true}}
	if req.Name != nil {
		p.Name = sql.NullString{String: *req.Name, Valid: true}
	}
	if req.TemplateID != nil {
		p.TemplateID = sql.NullString{String: *req.TemplateID, Valid: true}
	}
	if req.WrongTimes != nil {
		p.WrongTimes = sql.NullInt32{Int32: *req.WrongTimes, Valid: true}
	}
	if req.CorrectTimes != nil {
		p.CorrectTimes = sql.NullInt32{Int32: *req.CorrectTimes, Valid: true}
	}
	if req.Note != nil {
		p.Note = sql.NullString{String: *req.Note, Valid: true}
	}
	if req.Status != nil {
		p.Status = sql.NullInt32{Int32: *req.Status, Valid: true}
	}
	if req.Type != nil {
		p.Type = sql.NullInt32{Int32: *req.Type, Valid: true}
	}
	if req.RepoID != nil {
		p.RepoID = sql.NullString{String: *req.RepoID, Valid: true}
	}
	if req.IsMarked != nil {
		p.IsMarked = sql.NullInt16{Int16: *req.IsMarked, Valid: true}
	}
	if err := s.q.CreateUserBook(c.Request.Context(), p); err != nil {
		s.logger.Error("sk.userBook.create", "err", err)
		skErr(c, http.StatusInternalServerError, "create user book")
		return
	}
	skOK(c, gin.H{"id": id})
}

func (s *Server) handleSKUserBookUpdate(c *gin.Context) {
	var req skUserBookMutateReq
	if err := c.ShouldBindJSON(&req); err != nil || req.ID == "" {
		skErr(c, http.StatusBadRequest, "id is required")
		return
	}
	p := db.UpdateUserBookParams{ID: req.ID, UpdateBy: sql.NullString{String: principalID(c), Valid: true}}
	if req.Name != nil {
		p.Name = sql.NullString{String: *req.Name, Valid: true}
	}
	if req.TemplateID != nil {
		p.TemplateID = sql.NullString{String: *req.TemplateID, Valid: true}
	}
	if req.WrongTimes != nil {
		p.WrongTimes = sql.NullInt32{Int32: *req.WrongTimes, Valid: true}
	}
	if req.CorrectTimes != nil {
		p.CorrectTimes = sql.NullInt32{Int32: *req.CorrectTimes, Valid: true}
	}
	if req.Note != nil {
		p.Note = sql.NullString{String: *req.Note, Valid: true}
	}
	if req.Status != nil {
		p.Status = sql.NullInt32{Int32: *req.Status, Valid: true}
	}
	if req.Type != nil {
		p.Type = sql.NullInt32{Int32: *req.Type, Valid: true}
	}
	if req.RepoID != nil {
		p.RepoID = sql.NullString{String: *req.RepoID, Valid: true}
	}
	if req.IsMarked != nil {
		p.IsMarked = sql.NullInt16{Int16: *req.IsMarked, Valid: true}
	}
	if err := s.q.UpdateUserBook(c.Request.Context(), p); err != nil {
		s.logger.Error("sk.userBook.update", "err", err)
		skErr(c, http.StatusInternalServerError, "update user book")
		return
	}
	skOK(c, gin.H{"id": req.ID})
}

func (s *Server) handleSKUserBookDelete(c *gin.Context) {
	ids := readIDOrIDs(c)
	if len(ids) == 0 {
		skErr(c, http.StatusBadRequest, "id or ids is required")
		return
	}
	for _, id := range ids {
		_ = s.q.DeleteUserBook(c.Request.Context(), id)
	}
	skOK(c, gin.H{"deleted": len(ids)})
}

// ============================================================================
// /api/workflow/* — Flowable was dropped in P0. SK frontend probes
// these on the project edit screen; returning empty 200s keeps the
// UI from crashing.
// ============================================================================

func (s *Server) handleSKWorkflowEmptyObject(c *gin.Context) { skOK(c, gin.H{}) }
func (s *Server) handleSKWorkflowEmptyList(c *gin.Context)   { skList(c, []any{}, 0) }

// ============================================================================
// /api/ai/chat/* — SK shape on top of the existing P5 AIService
// ============================================================================

// SK uses a stateful conversation model (createConversation → stream
// with conversationId → closeConversation). Our AIService is
// stateless; we mint a new conversationId per createConversation and
// otherwise ignore it. Messages are still sent verbatim each call.

type aiConvCreateReq struct {
	Model string `json:"model,omitempty"`
}

func (s *Server) handleSKAIChatCreateConversation(c *gin.Context) {
	id := idgen.New()
	skOK(c, gin.H{"conversationId": id})
}

func (s *Server) handleSKAIChatCloseConversation(c *gin.Context) {
	skOK(c, gin.H{"closed": true, "conversationId": c.Query("conversationId")})
}

func (s *Server) handleSKAIChatModels(c *gin.Context) {
	if !s.cfg.AI.Enabled {
		skOK(c, []any{})
		return
	}
	skOK(c, []gin.H{
		{
			"value":       s.cfg.AI.Model,
			"displayName": s.cfg.AI.Model,
			"provider":    s.cfg.AI.Provider,
		},
	})
}

// /api/ai/chat/stream?conversationId=… — SSE stream.
// Body is the same {messages, model, temperature} shape as our REST
// /api/ai/chat. We forward the deltas with one extra concession to
// SK's UI: it expects each SSE frame to be the assistant content
// directly, not the {role,content,done,err} wrapper.
func (s *Server) handleSKAIChatStream(c *gin.Context) {
	if s.aiSvc == nil {
		skErr(c, http.StatusNotFound, "endpoint not available")
		return
	}
	var req struct {
		Model       string       `json:"model,omitempty"`
		Messages    []ai.Message `json:"messages" binding:"required"`
		Temperature float32      `json:"temperature,omitempty"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Messages) == 0 {
		skErr(c, http.StatusBadRequest, "messages is required")
		return
	}

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.WriteHeader(http.StatusOK)
	w := bufio.NewWriter(c.Writer)
	flush := func() {
		_ = w.Flush()
		if f, ok := c.Writer.(http.Flusher); ok {
			f.Flush()
		}
	}

	send := func(d ai.Delta) error {
		select {
		case <-c.Request.Context().Done():
			return c.Request.Context().Err()
		default:
		}
		if d.Done {
			_, err := w.WriteString("data: [DONE]\n\n")
			flush()
			return err
		}
		// SK frontend reads the raw content per frame.
		body := gin.H{"content": d.Content, "role": d.Role}
		b, _ := json.Marshal(body)
		if _, err := w.WriteString("data: "); err != nil {
			return err
		}
		if _, err := w.Write(b); err != nil {
			return err
		}
		if _, err := w.WriteString("\n\n"); err != nil {
			return err
		}
		flush()
		return nil
	}

	if err := s.aiSvc.Chat(c.Request.Context(), ai.ChatRequest{
		Model:       req.Model,
		Messages:    req.Messages,
		Temperature: req.Temperature,
	}, send); err != nil {
		s.logger.Error("sk.ai.stream", "err", err)
		_ = send(ai.Delta{Err: err.Error(), Done: true})
		return
	}
	_, _ = w.WriteString("data: [DONE]\n\n")
	flush()
}

// /api/ai/chat/answer-analysis/* — SK's "analyse this answer" feature.
// We re-use the chat flow; the upstream AI service is the same.
func (s *Server) handleSKAIAnswerAnalysisCreate(c *gin.Context)  { s.handleSKAIChatCreateConversation(c) }
func (s *Server) handleSKAIAnswerAnalysisStream(c *gin.Context)  { s.handleSKAIChatStream(c) }
func (s *Server) handleSKAIAnswerAnalysisClose(c *gin.Context)   { s.handleSKAIChatCloseConversation(c) }

// ============================================================================
// /api/file/downloadTemplate?name=… — empty xlsx so the download button
// doesn't 404. Real templates would live under web/dist/templates.
// ============================================================================

func (s *Server) handleSKFileDownloadTemplate(c *gin.Context) {
	name := c.Query("name")
	if name == "" {
		skErr(c, http.StatusBadRequest, "name is required")
		return
	}
	c.Header("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	c.Header("Content-Disposition", `attachment; filename="`+strings.ReplaceAll(name, `"`, "")+`.xlsx"`)
	// Tiny zip-empty xlsx so the browser still saves something. Real
	// templates need the proper [Content_Types].xml etc; SK shipped
	// canned files in /scripts/template — porting them is follow-up.
	c.Status(http.StatusOK)
	_, _ = c.Writer.Write([]byte("PK\x05\x06\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00"))
}

// ============================================================================
// helpers
// ============================================================================

// pageSize parses SK's standard query params with sane defaults +
// clamp. Pulled out of the repeated copy-paste flagged by Codex C3.
func pageSize(c *gin.Context) (int, int) {
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
	return page, size
}

// silence "imported but unused" if a future trim removes the only consumer.
var _ context.Context = nil
