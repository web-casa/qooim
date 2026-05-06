// SK-compat adapter layer.
//
// The legacy SurveyKing UmiJS frontend (we ship a copy under web/dist)
// speaks action-style routes — POST /api/project/list, GET /api/project?id=…,
// POST /api/public/login, etc. — and expects every successful response in
// a {success: true, code: 200, data, total?} envelope. The clean REST API
// we built in P0..P5 (POST /api/auth/login, GET /api/projects) doesn't
// match.
//
// Rather than fork the frontend or rebuild every service, we keep the
// REST routes for new clients and add this thin adapter for the SK UI.
// The adapter calls the same services and reshapes input/output.
//
// Coverage: C1 ships login + currentUser + project list/get/CRUD only.
// C2/C3/C4 will add survey/answer/file/system/* as user requests.
package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/web-casa/qooim/internal/auth"
	"github.com/web-casa/qooim/internal/repo/db"
	"github.com/web-casa/qooim/internal/service"
)

// ---- envelope helpers --------------------------------------------------

// skOK wraps a successful response. The bundle's umi-request interceptor
// rewrites anything without `code === 200` to `success: false`, so the
// numeric code is mandatory.
func skOK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"code":    200,
		"data":    data,
	})
}

// skList is the shape SK list pages expect. They read both
// `data.list[*]` and the top-level `total`, plus `data` for some
// table-toolbar paths — we expose the same items at both keys to be safe.
func skList(c *gin.Context, items any, total int) {
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"code":    200,
		"data": gin.H{
			"list":  items,
			"total": total,
		},
		"list":  items,
		"total": total,
	})
}

// skErr returns an unsuccessful response with both `errorMessage` and
// `message` keys (the SK bundle reads either depending on the endpoint).
// Status 401 specifically triggers SK's auto-logout flow, so we keep the
// numeric code aligned with the HTTP status.
func skErr(c *gin.Context, status int, message string) {
	c.AbortWithStatusJSON(status, gin.H{
		"success":      false,
		"code":         status,
		"message":      message,
		"errorMessage": message,
	})
}

// skJWTMiddleware verifies the Bearer token and emits skErr on failure
// (the standard auth.Issuer.Middleware uses our REST error shape, which
// the SK bundle doesn't recognise).
func (s *Server) skJWTMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		raw := c.GetHeader("Authorization")
		if raw == "" {
			skErr(c, http.StatusUnauthorized, "missing Authorization header")
			return
		}
		parts := strings.SplitN(raw, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			skErr(c, http.StatusUnauthorized, "invalid Authorization header")
			return
		}
		p, err := s.jwt.Parse(parts[1])
		if err != nil {
			skErr(c, http.StatusUnauthorized, "invalid or expired token")
			return
		}
		c.Set(auth.ContextKey, p)
		c.Next()
	}
}

// ---- DTOs (camelCase + parsed JSON columns) ----------------------------

type skProjectListItem struct {
	ID       string          `json:"id"`
	ParentID string          `json:"parentId,omitempty"`
	Name     string          `json:"name"`
	Mode     string          `json:"mode,omitempty"`
	Status   int32           `json:"status"`
	Priority int32           `json:"priority"`
	Survey   json.RawMessage `json:"survey,omitempty"`
	Setting  json.RawMessage `json:"setting,omitempty"`
	CreateAt time.Time       `json:"createAt"`
	UpdateAt *time.Time      `json:"updateAt,omitempty"`
	CreateBy string           `json:"createBy,omitempty"`
}

type skProjectDetail struct {
	skProjectListItem
	UpdateBy string `json:"updateBy,omitempty"`
}

func decodeJSONColumn(s string) json.RawMessage {
	t := strings.TrimSpace(s)
	if t == "" {
		return nil
	}
	if !json.Valid([]byte(t)) {
		// Fall back to string-encoded so the bundle doesn't choke; SK's
		// UI is forgiving when these columns are pre-stringified.
		b, _ := json.Marshal(s)
		return json.RawMessage(b)
	}
	return json.RawMessage(t)
}

func skProjectFromListRow(r db.ListProjectsRow) skProjectListItem {
	out := skProjectListItem{
		ID:       r.ID,
		Name:     valueOr(r.Name),
		Mode:     valueOr(r.Mode),
		Survey:   decodeJSONColumn(valueOr(r.Survey)),
		Setting:  decodeJSONColumn(valueOr(r.Setting)),
		CreateAt: r.CreateAt,
		CreateBy: valueOr(r.CreateBy),
	}
	if r.ParentID.Valid {
		out.ParentID = r.ParentID.String
	}
	if r.Status.Valid {
		out.Status = r.Status.Int32
	}
	if r.Priority.Valid {
		out.Priority = r.Priority.Int32
	}
	if r.UpdateAt.Valid {
		t := r.UpdateAt.Time
		out.UpdateAt = &t
	}
	return out
}

func skProjectFromGet(r db.GetProjectByIDRow) skProjectDetail {
	out := skProjectDetail{
		skProjectListItem: skProjectListItem{
			ID:       r.ID,
			Name:     valueOr(r.Name),
			Mode:     valueOr(r.Mode),
			Survey:   decodeJSONColumn(valueOr(r.Survey)),
			Setting:  decodeJSONColumn(valueOr(r.Setting)),
			CreateAt: r.CreateAt,
			CreateBy: valueOr(r.CreateBy),
		},
	}
	if r.ParentID.Valid {
		out.ParentID = r.ParentID.String
	}
	if r.Status.Valid {
		out.Status = r.Status.Int32
	}
	if r.Priority.Valid {
		out.Priority = r.Priority.Int32
	}
	if r.UpdateAt.Valid {
		t := r.UpdateAt.Time
		out.UpdateAt = &t
	}
	if r.UpdateBy.Valid {
		out.UpdateBy = r.UpdateBy.String
	}
	return out
}

func valueOr(n sql.NullString) string {
	if n.Valid {
		return n.String
	}
	return ""
}

// ---- /api/public/login -------------------------------------------------

type skLoginReq struct {
	Username string `json:"username"`
	Account  string `json:"account"`
	Password string `json:"password"`
}

func (s *Server) handleSKLogin(c *gin.Context) {
	var req skLoginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		// SK's old backend tolerates form-encoded bodies on login; fall
		// back to PostForm if JSON binding fails.
		req.Username = c.PostForm("username")
		req.Account = c.PostForm("account")
		req.Password = c.PostForm("password")
	}
	acct := req.Username
	if acct == "" {
		acct = req.Account
	}
	if acct == "" || req.Password == "" {
		skErr(c, http.StatusBadRequest, "username and password are required")
		return
	}
	res, err := s.auth.Login(c.Request.Context(), acct, req.Password)
	if err != nil {
		if service.IsBadCredentials(err) {
			skErr(c, http.StatusUnauthorized, "invalid credentials")
			return
		}
		s.logger.Error("sk.login", "err", err)
		skErr(c, http.StatusInternalServerError, "login failed")
		return
	}
	skOK(c, gin.H{
		"token":       res.Token,
		"name":        res.Principal.Username,
		"userId":      res.Principal.UserID,
		"roles":       res.Principal.Roles,
		"authorities": []string{},
	})
}

func (s *Server) handleSKLogout(c *gin.Context) { skOK(c, gin.H{"ok": true}) }

func (s *Server) handleSKCurrentUser(c *gin.Context) {
	p, ok := auth.FromContext(c)
	if !ok {
		skErr(c, http.StatusUnauthorized, "not authenticated")
		return
	}
	res, err := s.auth.Me(c.Request.Context(), *p)
	if err != nil {
		s.logger.Error("sk.currentUser", "err", err)
		skErr(c, http.StatusInternalServerError, "load user")
		return
	}
	skOK(c, gin.H{
		"userId":      res.Principal.UserID,
		"name":        res.Principal.Username,
		"roles":       res.Principal.Roles,
		"authorities": []string{},
		"email":       res.Email,
		"avatar":      res.Avatar,
		"profile":     res.Profile,
	})
}

// ---- /api/project/list -------------------------------------------------

// skProjectListReq is the body SK posts. It's lenient: any of the
// fields can be missing, and pagination knobs are also accepted via
// query string.
type skProjectListReq struct {
	Current  int     `json:"current"`
	PageSize int     `json:"pageSize"`
	ParentID *string `json:"parentId,omitempty"`
	Mode     *string `json:"mode,omitempty"`
	Name     *string `json:"name,omitempty"`
}

func (s *Server) handleSKProjectList(c *gin.Context) {
	var req skProjectListReq
	_ = c.ShouldBindJSON(&req)
	if req.Current == 0 {
		req.Current, _ = strconv.Atoi(c.Query("current"))
	}
	if req.PageSize == 0 {
		req.PageSize, _ = strconv.Atoi(c.Query("pageSize"))
	}
	if req.ParentID == nil {
		if v := c.Query("parentId"); v != "" {
			req.ParentID = &v
		}
	}
	if req.Mode == nil {
		if v := c.Query("mode"); v != "" {
			req.Mode = &v
		}
	}
	if req.Name == nil {
		if v := c.Query("name"); v != "" {
			req.Name = &v
		}
	}
	res, err := s.listing.Projects(c.Request.Context(),
		service.Page{Page: req.Current, PageSize: req.PageSize},
		service.ProjectFilters{ParentID: req.ParentID, Mode: req.Mode, Name: req.Name})
	if err != nil {
		s.logger.Error("sk.project.list", "err", err)
		skErr(c, http.StatusInternalServerError, "list projects")
		return
	}
	items := make([]skProjectListItem, len(res.Items))
	for i, r := range res.Items {
		items[i] = skProjectFromListRow(r)
	}
	skList(c, items, res.Total)
}

func (s *Server) handleSKProjectGet(c *gin.Context) {
	id := c.Query("id")
	if id == "" {
		skErr(c, http.StatusBadRequest, "id is required")
		return
	}
	row, err := s.projects.Get(c.Request.Context(), id)
	if errors.Is(err, service.ErrNotFound) {
		skErr(c, http.StatusNotFound, "project not found")
		return
	}
	if err != nil {
		s.logger.Error("sk.project.get", "err", err)
		skErr(c, http.StatusInternalServerError, "load project")
		return
	}
	skOK(c, skProjectFromGet(row))
}

// skProjectMutateReq is the camelCase write payload SK sends for both
// create and update. We keep it explicit (not embedding the snake_case
// service input) so JSON tag drift can't sneak past us.
type skProjectMutateReq struct {
	ID       string          `json:"id,omitempty"`
	ParentID *string         `json:"parentId,omitempty"`
	Name     *string         `json:"name,omitempty"`
	Survey   json.RawMessage `json:"survey,omitempty"`
	Setting  json.RawMessage `json:"setting,omitempty"`
	Status   *int32          `json:"status,omitempty"`
	Mode     *string         `json:"mode,omitempty"`
	Priority *int32          `json:"priority,omitempty"`
}

func (r skProjectMutateReq) toCreate() service.CreateProjectInput {
	in := service.CreateProjectInput{}
	if r.ParentID != nil {
		in.ParentID = r.ParentID
	}
	if r.Name != nil {
		in.Name = *r.Name
	}
	if r.Survey != nil {
		s := string(r.Survey)
		in.Survey = &s
	}
	if r.Setting != nil {
		s := string(r.Setting)
		in.Setting = &s
	}
	in.Status = r.Status
	in.Mode = r.Mode
	in.Priority = r.Priority
	return in
}

func (r skProjectMutateReq) toUpdate() service.UpdateProjectInput {
	in := service.UpdateProjectInput{
		ParentID: r.ParentID,
		Name:     r.Name,
		Status:   r.Status,
		Mode:     r.Mode,
		Priority: r.Priority,
	}
	if r.Survey != nil {
		s := string(r.Survey)
		in.Survey = &s
	}
	if r.Setting != nil {
		s := string(r.Setting)
		in.Setting = &s
	}
	return in
}

func (s *Server) handleSKProjectCreate(c *gin.Context) {
	var req skProjectMutateReq
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == nil || *req.Name == "" {
		skErr(c, http.StatusBadRequest, "name is required")
		return
	}
	id, err := s.projects.Create(c.Request.Context(), req.toCreate(), principalID(c))
	if err != nil {
		s.logger.Error("sk.project.create", "err", err)
		skErr(c, http.StatusInternalServerError, "create project")
		return
	}
	skOK(c, gin.H{"id": id})
}

func (s *Server) handleSKProjectUpdate(c *gin.Context) {
	var req skProjectMutateReq
	if err := c.ShouldBindJSON(&req); err != nil || req.ID == "" {
		skErr(c, http.StatusBadRequest, "id is required")
		return
	}
	if err := s.projects.Update(c.Request.Context(), req.ID, req.toUpdate(), principalID(c)); err != nil {
		if errors.Is(err, service.ErrNotFound) {
			skErr(c, http.StatusNotFound, "project not found")
			return
		}
		s.logger.Error("sk.project.update", "err", err)
		skErr(c, http.StatusInternalServerError, "update project")
		return
	}
	skOK(c, gin.H{"id": req.ID})
}

type skIDOnly struct {
	ID  string   `json:"id,omitempty"`
	IDs []string `json:"ids,omitempty"`
}

// handleSKProjectDelete returns per-id results so callers see which
// items in a bulk request actually applied. Errors that aren't
// ErrNotFound abort the loop with a 500 (the row is left in whatever
// partial state the loop reached — same trade-off SK had).
func (s *Server) handleSKProjectDelete(c *gin.Context) {
	var in skIDOnly
	_ = c.ShouldBindJSON(&in)
	ids := in.IDs
	if in.ID != "" {
		ids = append(ids, in.ID)
	}
	if len(ids) == 0 {
		skErr(c, http.StatusBadRequest, "id or ids is required")
		return
	}
	results := make([]gin.H, 0, len(ids))
	for _, id := range ids {
		if err := s.projects.SoftDelete(c.Request.Context(), id, principalID(c)); err != nil {
			if errors.Is(err, service.ErrNotFound) {
				results = append(results, gin.H{"id": id, "ok": false, "reason": "not_found"})
				continue
			}
			s.logger.Error("sk.project.delete", "err", err, "id", id)
			results = append(results, gin.H{"id": id, "ok": false, "reason": "error"})
			skList(c, results, len(ids))
			return
		}
		results = append(results, gin.H{"id": id, "ok": true})
	}
	skOK(c, gin.H{"deleted": countOK(results), "results": results})
}

func countOK(results []gin.H) int {
	n := 0
	for _, r := range results {
		if v, ok := r["ok"].(bool); ok && v {
			n++
		}
	}
	return n
}

