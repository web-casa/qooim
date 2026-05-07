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

type dictRow struct {
	ID       string
	Name     string
	Code     string
	DictType int32
	Remark   string
}

type dictForm struct {
	ID       string
	Name     string
	Code     string
	DictType int32
	Remark   string
}

func (s *Server) getDicts(c *gin.Context) {
	s.render(c, "system/dicts/list.html", s.buildDictListView(c))
}

func (s *Server) getDictsTable(c *gin.Context) {
	s.renderPartial(c, "dicts-table", s.buildDictListView(c))
}

func (s *Server) buildDictListView(c *gin.Context) View {
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
	rows, err := s.q.ListDicts(c.Request.Context(), db.ListDictsParams{
		Name: nameFilter,
		Off:  int32(offset),
		Lim:  int32(limit),
	})
	if err != nil {
		return View{Title: "字典管理", Error: "load dicts failed"}
	}
	out := make([]dictRow, 0, len(rows))
	for _, r := range rows {
		row := dictRow{ID: r.ID}
		if r.Name.Valid {
			row.Name = r.Name.String
		}
		if r.Code.Valid {
			row.Code = r.Code.String
		}
		if r.DictType.Valid {
			row.DictType = r.DictType.Int32
		}
		if r.Remark.Valid {
			row.Remark = r.Remark.String
		}
		out = append(out, row)
	}
	total, _ := s.q.CountDicts(c.Request.Context(), db.CountDictsParams{Name: nameFilter})
	totalPages := int(total) / limit
	if int(total)%limit != 0 {
		totalPages++
	}
	if totalPages == 0 {
		totalPages = 1
	}
	return View{
		Title:      "字典管理",
		Active:     "system/dicts",
		Crumb:      "系统设置 / 字典管理",
		Q:          q,
		Page:       page,
		TotalPages: totalPages,
		Total:      int(total),
		DictRows:   out,
	}
}

func (s *Server) getDictForm(c *gin.Context) {
	id := c.Param("id")
	v := View{Title: "字典表单"}
	if id != "" {
		r, err := s.q.GetDictByID(c.Request.Context(), id)
		if err != nil {
			v.Error = asError(err)
			s.renderPartial(c, "dict-form", v)
			return
		}
		v.DictForm = dictForm{ID: r.ID}
		if r.Name.Valid {
			v.DictForm.Name = r.Name.String
		}
		if r.Code.Valid {
			v.DictForm.Code = r.Code.String
		}
		if r.DictType.Valid {
			v.DictForm.DictType = r.DictType.Int32
		}
		if r.Remark.Valid {
			v.DictForm.Remark = r.Remark.String
		}
	}
	s.renderPartial(c, "dict-form", v)
}

func (s *Server) postDict(c *gin.Context) {
	f := dictFormFromCtx(c)
	if f.Name == "" || f.Code == "" {
		s.renderDictFormError(c, "", f, errorf("name and code required"))
		return
	}
	by := principalOf(c).UserID
	p := db.CreateDictParams{
		ID:       idgen.New(),
		Name:     sql.NullString{String: f.Name, Valid: true},
		Code:     sql.NullString{String: f.Code, Valid: true},
		DictType: sql.NullInt32{Int32: f.DictType, Valid: true},
		CreateBy: sql.NullString{String: by, Valid: true},
	}
	if f.Remark != "" {
		p.Remark = sql.NullString{String: f.Remark, Valid: true}
	}
	if err := s.q.CreateDict(c.Request.Context(), p); err != nil {
		s.renderDictFormError(c, "", f, err)
		return
	}
	s.dictListWithFlash(c, "字典已创建", flashKindOK)
}

func (s *Server) putDict(c *gin.Context) {
	id := c.Param("id")
	f := dictFormFromCtx(c)
	if _, err := s.q.GetDictByID(c.Request.Context(), id); err != nil {
		c.String(http.StatusNotFound, asError(err))
		return
	}
	by := principalOf(c).UserID
	p := db.UpdateDictParams{
		ID:       id,
		UpdateBy: sql.NullString{String: by, Valid: true},
	}
	if f.Name != "" {
		p.Name = sql.NullString{String: f.Name, Valid: true}
	}
	if f.Code != "" {
		p.Code = sql.NullString{String: f.Code, Valid: true}
	}
	if f.Remark != "" {
		p.Remark = sql.NullString{String: f.Remark, Valid: true}
	}
	p.DictType = sql.NullInt32{Int32: f.DictType, Valid: true}
	if err := s.q.UpdateDict(c.Request.Context(), p); err != nil {
		s.renderDictFormError(c, id, f, err)
		return
	}
	s.dictListWithFlash(c, "字典已更新", flashKindOK)
}

// deleteDict cascades to dictItems via SystemService.DeleteDictWithItems
// — admin can wipe a whole dictionary in one click; the service runs it
// inside a transaction so a partial delete can't leave orphan items.
func (s *Server) deleteDict(c *gin.Context) {
	id := c.Param("id")
	if err := s.sysSvc.DeleteDictWithItems(c.Request.Context(), id); err != nil {
		c.String(http.StatusBadRequest, asError(err))
		return
	}
	c.Status(http.StatusOK)
}

func dictFormFromCtx(c *gin.Context) dictForm {
	f := dictForm{
		Name:   strings.TrimSpace(c.PostForm("name")),
		Code:   strings.TrimSpace(c.PostForm("code")),
		Remark: strings.TrimSpace(c.PostForm("remark")),
	}
	if v := c.PostForm("dictType"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			f.DictType = int32(n)
		}
	}
	return f
}

func (s *Server) dictListWithFlash(c *gin.Context, msg, kind string) {
	v := s.buildDictListView(c)
	v.Flash = &Flash{Kind: kind, Message: msg}
	s.renderPartial(c, "dicts-refresh", v)
}

func (s *Server) renderDictFormError(c *gin.Context, id string, f dictForm, err error) {
	v := View{Title: "字典表单", Error: asError(err), DictForm: f}
	v.DictForm.ID = id
	s.renderPartial(c, "dict-form", v)
}
