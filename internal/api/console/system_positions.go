package console

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/web-casa/qooim/internal/idgen"
	"github.com/web-casa/qooim/internal/repo/db"
)

type positionRow struct {
	ID        string
	Name      string
	Code      string
	IsVirtual int16
}

type positionForm struct {
	ID        string
	Name      string
	Code      string
	IsVirtual int16
}

func (s *Server) getPositions(c *gin.Context) {
	s.render(c, "system/positions/list.html", s.buildPositionListView(c))
}

func (s *Server) getPositionsTable(c *gin.Context) {
	s.renderPartial(c, "positions-table", s.buildPositionListView(c))
}

func (s *Server) buildPositionListView(c *gin.Context) View {
	q := strings.TrimSpace(c.Query("q"))
	page, _ := strconv.Atoi(c.Query("page"))
	if page < 1 {
		page = 1
	}
	limit := defaultPageSize
	offset := (page - 1) * limit
	nameFilter := sql.NullString{}
	if q != "" {
		nameFilter = sql.NullString{String: q, Valid: true}
	}
	rows, err := s.q.ListPositions(c.Request.Context(), db.ListPositionsParams{
		Name: nameFilter,
		Off:  int32(offset),
		Lim:  int32(limit),
	})
	if err != nil {
		return View{Title: "岗位管理", Error: "load positions failed"}
	}
	out := make([]positionRow, 0, len(rows))
	for _, r := range rows {
		row := positionRow{ID: r.ID, Name: r.Name, IsVirtual: r.IsVirtual}
		if r.Code.Valid {
			row.Code = r.Code.String
		}
		out = append(out, row)
	}
	total, _ := s.q.CountPositions(c.Request.Context(), nameFilter)
	totalPages := int(total) / limit
	if int(total)%limit != 0 {
		totalPages++
	}
	if totalPages == 0 {
		totalPages = 1
	}
	return View{
		Title:        "岗位管理",
		Active:       "system/positions",
		Crumb:        "系统设置 / 岗位管理",
		Q:            q,
		Page:         page,
		TotalPages:   totalPages,
		Total:        int(total),
		PositionRows: out,
	}
}

func (s *Server) getPositionForm(c *gin.Context) {
	id := c.Param("id")
	v := View{Title: "岗位表单"}
	if id != "" {
		r, err := s.q.GetPositionByID(c.Request.Context(), id)
		if err != nil {
			v.Error = asError(err)
			s.renderPartial(c, "position-form", v)
			return
		}
		v.PositionForm = positionForm{ID: r.ID, Name: r.Name, IsVirtual: r.IsVirtual}
		if r.Code.Valid {
			v.PositionForm.Code = r.Code.String
		}
	}
	s.renderPartial(c, "position-form", v)
}

func (s *Server) postPosition(c *gin.Context) {
	name := strings.TrimSpace(c.PostForm("name"))
	code := strings.TrimSpace(c.PostForm("code"))
	isVirtual := int16(0)
	if v := c.PostForm("isVirtual"); v == "1" {
		isVirtual = 1
	}
	if name == "" {
		s.renderPositionFormError(c, "", positionForm{Name: name, Code: code, IsVirtual: isVirtual}, errorf("name is required"))
		return
	}
	by := principalOf(c).UserID
	p := db.CreatePositionParams{
		ID:        idgen.New(),
		Name:      name,
		IsVirtual: isVirtual,
		CreateBy:  sql.NullString{String: by, Valid: true},
	}
	if code != "" {
		p.Code = sql.NullString{String: code, Valid: true}
	}
	if err := s.q.CreatePosition(c.Request.Context(), p); err != nil {
		s.flagError("position.create", c, err)
		s.renderPositionFormError(c, "", positionForm{Name: name, Code: code, IsVirtual: isVirtual}, err)
		return
	}
	s.positionListWithFlash(c, "岗位已创建", flashKindOK)
}

func (s *Server) putPosition(c *gin.Context) {
	id := c.Param("id")
	name := strings.TrimSpace(c.PostForm("name"))
	code := strings.TrimSpace(c.PostForm("code"))
	isVirtual := int16(0)
	if v := c.PostForm("isVirtual"); v == "1" {
		isVirtual = 1
	}
	if _, err := s.q.GetPositionByID(c.Request.Context(), id); err != nil {
		c.String(http.StatusNotFound, asError(err))
		return
	}
	by := principalOf(c).UserID
	p := db.UpdatePositionParams{
		ID:        id,
		IsVirtual: sql.NullInt16{Int16: isVirtual, Valid: true},
		UpdateBy:  sql.NullString{String: by, Valid: true},
	}
	if name != "" {
		p.Name = sql.NullString{String: name, Valid: true}
	}
	if code != "" {
		p.Code = sql.NullString{String: code, Valid: true}
	}
	if err := s.q.UpdatePosition(c.Request.Context(), p); err != nil {
		s.flagError("position.update", c, err)
		s.renderPositionFormError(c, id, positionForm{ID: id, Name: name, Code: code, IsVirtual: isVirtual}, err)
		return
	}
	s.positionListWithFlash(c, "岗位已更新", flashKindOK)
}

func (s *Server) deletePosition(c *gin.Context) {
	id := c.Param("id")
	by := principalOf(c).UserID
	if err := s.q.SoftDeletePosition(c.Request.Context(), db.SoftDeletePositionParams{
		ID:       id,
		UpdateBy: sql.NullString{String: by, Valid: true},
	}); err != nil {
		s.flagError("position.delete", c, err)
		c.String(http.StatusBadRequest, asError(err))
		return
	}
	c.Status(http.StatusOK)
}

func (s *Server) positionListWithFlash(c *gin.Context, msg, kind string) {
	v := s.buildPositionListView(c)
	v.Flash = &Flash{Kind: kind, Message: msg}
	s.renderPartial(c, "positions-refresh", v)
}

func (s *Server) renderPositionFormError(c *gin.Context, id string, f positionForm, err error) {
	v := View{Title: "岗位表单", Error: asError(err), PositionForm: f}
	v.PositionForm.ID = id
	s.renderPartial(c, "position-form", v)
}

// errorf is a tiny shim so handlers can produce a typed error from a
// literal string without dragging fmt in for each callsite.
func errorf(msg string) error { return errMsg(msg) }

type errMsg string

func (e errMsg) Error() string { return string(e) }
