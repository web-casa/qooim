// SK-compat adapter layer.
//
// The legacy SurveyKing UmiJS frontend (we ship a copy under web/dist)
// speaks to action-style routes — POST /api/project/list, GET /api/project?id=…,
// POST /api/public/login, etc. — and expects every successful response in
// a {success: true, data|result, total?} envelope. The clean REST API we
// built in P0..P5 (POST /api/auth/login, GET /api/projects) doesn't match.
//
// Rather than fork the frontend or rebuild every service, we keep the
// REST routes for new clients and add this thin adapter for the SK UI.
// The adapter calls the same services and reshapes input/output.
//
// Coverage: C1 ships login + currentUser + project list/get/CRUD only.
// C2/C3/C4 will add survey/answer/file/system/* as user requests.
package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/web-casa/qooim/internal/auth"
	"github.com/web-casa/qooim/internal/domain"
	"github.com/web-casa/qooim/internal/service"
)

// skOK wraps a successful response.
func skOK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{"success": true, "data": data})
}

// skList wraps a paginated response. SK's frontend looks for {data, total}.
func skList(c *gin.Context, items any, total int) {
	c.JSON(http.StatusOK, gin.H{"success": true, "data": items, "total": total})
}

// skErr returns an unsuccessful response. The frontend checks `success`
// before reading `data`; status code is included so callers can still
// branch on it.
func skErr(c *gin.Context, status int, message string) {
	c.AbortWithStatusJSON(status, gin.H{"success": false, "errorMessage": message})
}

// ---------- Auth ----------

type skLoginReq struct {
	// SK sends `username`; some flows also send `account`. Accept both.
	Username string `json:"username"`
	Account  string `json:"account"`
	Password string `json:"password" binding:"required"`
}

func (s *Server) handleSKLogin(c *gin.Context) {
	var req skLoginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		skErr(c, http.StatusBadRequest, "username and password are required")
		return
	}
	acct := req.Username
	if acct == "" {
		acct = req.Account
	}
	if acct == "" {
		skErr(c, http.StatusBadRequest, "username is required")
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
	// SK frontend stores token under data.token and reads roles from
	// data.authorities (a permission code list, not just role codes).
	skOK(c, gin.H{
		"token":       res.Token,
		"name":        res.Principal.Username,
		"userId":      res.Principal.UserID,
		"roles":       res.Principal.Roles,
		"authorities": []string{},
	})
}

func (s *Server) handleSKLogout(c *gin.Context) {
	// Stateless JWT: logout is purely a client-side token-clear.
	skOK(c, gin.H{"ok": true})
}

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

// ---------- Projects ----------

// skListReq is SK's standard list payload. SK uses `current` (1-based)
// and `pageSize`; many endpoints also accept `query` filters.
type skListReq struct {
	Current  int `json:"current"`
	PageSize int `json:"pageSize"`
	// Filter fields – we accept what we know about and ignore the rest.
	Mode   string `json:"mode,omitempty"`
	Status *int32 `json:"status,omitempty"`
	Name   string `json:"name,omitempty"`
}

func (r skListReq) page() service.Page {
	return service.Page{Page: r.Current, PageSize: r.PageSize}
}

func (s *Server) handleSKProjectList(c *gin.Context) {
	var req skListReq
	_ = c.ShouldBindJSON(&req) // SK sometimes sends GET-style query params; ignore parse errors.
	if req.Current == 0 {
		req.Current, _ = strconv.Atoi(c.Query("current"))
	}
	if req.PageSize == 0 {
		req.PageSize, _ = strconv.Atoi(c.Query("pageSize"))
	}
	res, err := s.listing.Projects(c.Request.Context(), req.page())
	if err != nil {
		s.logger.Error("sk.project.list", "err", err)
		skErr(c, http.StatusInternalServerError, "list projects")
		return
	}
	skList(c, res.Items, res.Total)
}

// handleSKProjectGet handles GET /api/project?id=xxx
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
	skOK(c, domain.ProjectFromGet(row))
}

func (s *Server) handleSKProjectCreate(c *gin.Context) {
	var in service.CreateProjectInput
	if err := c.ShouldBindJSON(&in); err != nil || in.Name == "" {
		skErr(c, http.StatusBadRequest, "name is required")
		return
	}
	id, err := s.projects.Create(c.Request.Context(), in, principalID(c))
	if err != nil {
		s.logger.Error("sk.project.create", "err", err)
		skErr(c, http.StatusInternalServerError, "create project")
		return
	}
	// SK callers expect the new id (or full row) on data.
	skOK(c, gin.H{"id": id})
}

// skUpdateProjectReq embeds the id alongside the update fields, since
// SK's POST /api/project/update sends {id, ...patch} in one body.
type skUpdateProjectReq struct {
	ID string `json:"id" binding:"required"`
	service.UpdateProjectInput
}

func (s *Server) handleSKProjectUpdate(c *gin.Context) {
	var in skUpdateProjectReq
	if err := c.ShouldBindJSON(&in); err != nil {
		skErr(c, http.StatusBadRequest, "id is required")
		return
	}
	if err := s.projects.Update(c.Request.Context(), in.ID, in.UpdateProjectInput, principalID(c)); err != nil {
		if errors.Is(err, service.ErrNotFound) {
			skErr(c, http.StatusNotFound, "project not found")
			return
		}
		s.logger.Error("sk.project.update", "err", err)
		skErr(c, http.StatusInternalServerError, "update project")
		return
	}
	skOK(c, gin.H{"id": in.ID})
}

type skIDOnly struct {
	ID  string   `json:"id,omitempty"`
	IDs []string `json:"ids,omitempty"`
}

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
	for _, id := range ids {
		if err := s.projects.SoftDelete(c.Request.Context(), id, principalID(c)); err != nil {
			if errors.Is(err, service.ErrNotFound) {
				continue // be lenient — bulk deletes shouldn't fail on stale ids.
			}
			s.logger.Error("sk.project.delete", "err", err, "id", id)
			skErr(c, http.StatusInternalServerError, "delete project")
			return
		}
	}
	skOK(c, gin.H{"deleted": len(ids)})
}
