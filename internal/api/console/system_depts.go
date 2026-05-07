package console

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/web-casa/qooim/internal/repo/db"
	"github.com/web-casa/qooim/internal/service"
)

// dept page intentionally renders as a flat list, not a tree. The
// SK frontend's tree view is a Phase-2-or-later concern; flat with a
// parentId column gives the same usability for ~50 depts.
type deptRow struct {
	ID         string
	Name       string
	ShortName  string
	Code       string
	ParentID   string
	ParentName string
	Status     string
	SortCode   int32
}

type deptForm struct {
	ID        string
	Name      string
	ShortName string
	Code      string
	ParentID  string
	Status    string
	SortCode  int32
	Remark    string
}

// ---- list ------------------------------------------------------------------

func (s *Server) getDepts(c *gin.Context) {
	s.render(c, "system/depts/list.html", s.buildDeptListView(c))
}

func (s *Server) getDeptsTable(c *gin.Context) {
	s.renderPartial(c, "depts-table", s.buildDeptListView(c))
}

func (s *Server) buildDeptListView(c *gin.Context) View {
	rows, err := s.q.ListDepts(c.Request.Context())
	if err != nil {
		return View{Title: "部门管理", Error: "load depts failed"}
	}
	// Build id→name once so we can show parent name without N+1.
	nameByID := make(map[string]string, len(rows))
	for _, r := range rows {
		if r.Name.Valid {
			nameByID[r.ID] = r.Name.String
		}
	}
	q := strings.TrimSpace(strings.ToLower(c.Query("q")))
	out := make([]deptRow, 0, len(rows))
	for _, r := range rows {
		row := deptRow{
			ID:        r.ID,
			ShortName: r.ShortName,
			ParentID:  r.ParentID,
		}
		if r.Name.Valid {
			row.Name = r.Name.String
		}
		if r.Code.Valid {
			row.Code = r.Code.String
		}
		if r.Status.Valid {
			row.Status = r.Status.String
		}
		if r.SortCode.Valid {
			row.SortCode = r.SortCode.Int32
		}
		if r.ParentID != "" && r.ParentID != "0" {
			row.ParentName = nameByID[r.ParentID]
		}
		if q != "" {
			hay := strings.ToLower(row.Name + " " + row.ShortName + " " + row.Code)
			if !strings.Contains(hay, q) {
				continue
			}
		}
		out = append(out, row)
	}
	return View{
		Title:    "部门管理",
		Active:   "system/depts",
		Crumb:    "系统设置 / 部门管理",
		Q:        c.Query("q"),
		Total:    len(out),
		DeptRows: out,
		// Reuse Depts (loadDepts shape) for the parent picker. Cheap.
		Depts: deptOptionsFromRows(rows),
	}
}

func deptOptionsFromRows(rows []db.ListDeptsRow) []deptOption {
	out := make([]deptOption, 0, len(rows))
	for _, r := range rows {
		name := ""
		if r.Name.Valid {
			name = r.Name.String
		}
		out = append(out, deptOption{ID: r.ID, Name: name})
	}
	return out
}

// ---- form ------------------------------------------------------------------

func (s *Server) getDeptForm(c *gin.Context) {
	id := c.Param("id")
	v := View{Title: "部门表单"}
	rows, _ := s.q.ListDepts(c.Request.Context())
	v.Depts = deptOptionsFromRows(rows)
	if id != "" {
		r, err := s.q.GetDeptByID(c.Request.Context(), id)
		if err != nil {
			v.Error = asError(err)
			s.renderPartial(c, "dept-form", v)
			return
		}
		v.DeptForm = deptForm{
			ID:        r.ID,
			ShortName: r.ShortName,
			ParentID:  r.ParentID,
		}
		if r.Name.Valid {
			v.DeptForm.Name = r.Name.String
		}
		if r.Code.Valid {
			v.DeptForm.Code = r.Code.String
		}
		if r.Status.Valid {
			v.DeptForm.Status = r.Status.String
		}
		if r.SortCode.Valid {
			v.DeptForm.SortCode = r.SortCode.Int32
		}
		if r.Remark.Valid {
			v.DeptForm.Remark = r.Remark.String
		}
	} else {
		v.DeptForm.Status = "1"
	}
	s.renderPartial(c, "dept-form", v)
}

// ---- create / update / delete ---------------------------------------------

func (s *Server) postDept(c *gin.Context) {
	in := s.deptInputFromForm(c)
	by := principalOf(c).UserID
	create := service.CreateDeptInput{
		ParentID:  in.ParentID,
		Name:      in.Name,
		ShortName: in.ShortName,
		Code:      in.Code,
		SortCode:  in.SortCode,
		Status:    in.Status,
		Remark:    in.Remark,
	}
	if _, err := s.sysSvc.CreateDept(c.Request.Context(), create, by); err != nil {
		s.flagError("dept.create", c, err)
		s.renderDeptFormError(c, "", in, err)
		return
	}
	s.deptListWithFlash(c, "部门已创建", flashKindOK)
}

func (s *Server) putDept(c *gin.Context) {
	id := c.Param("id")
	in := s.deptInputFromForm(c)
	by := principalOf(c).UserID
	upd := service.UpdateDeptInput{
		Name:      strPtr(in.Name),
		ShortName: strPtr(in.ShortName),
		Code:      in.Code,
		SortCode:  in.SortCode,
		Status:    in.Status,
		Remark:    in.Remark,
	}
	if in.ParentID != "" {
		upd.ParentID = &in.ParentID
	}
	if err := s.sysSvc.UpdateDept(c.Request.Context(), id, upd, by); err != nil {
		s.flagError("dept.update", c, err)
		s.renderDeptFormError(c, id, in, err)
		return
	}
	s.deptListWithFlash(c, "部门已更新", flashKindOK)
}

func (s *Server) deleteDept(c *gin.Context) {
	id := c.Param("id")
	by := principalOf(c).UserID
	if err := s.sysSvc.DeleteDept(c.Request.Context(), id, by); err != nil {
		s.flagError("dept.delete", c, err)
		c.String(http.StatusBadRequest, asError(err))
		return
	}
	c.Status(http.StatusOK)
}

// ---- form helpers ----------------------------------------------------------

// deptInputFromForm normalises the modal's form-encoded body into the
// shape both create and update flows want. Empty-string fields are
// treated as "leave alone" for nullable columns.
func (s *Server) deptInputFromForm(c *gin.Context) deptInput {
	in := deptInput{
		Name:      strings.TrimSpace(c.PostForm("name")),
		ShortName: strings.TrimSpace(c.PostForm("shortName")),
		ParentID:  strings.TrimSpace(c.PostForm("parentId")),
	}
	if v := strings.TrimSpace(c.PostForm("code")); v != "" {
		in.Code = &v
	}
	if v := strings.TrimSpace(c.PostForm("status")); v != "" {
		in.Status = &v
	}
	if v := strings.TrimSpace(c.PostForm("remark")); v != "" {
		in.Remark = &v
	}
	if v := c.PostForm("sortCode"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			n32 := int32(n)
			in.SortCode = &n32
		}
	}
	return in
}

type deptInput struct {
	Name      string
	ShortName string
	ParentID  string
	Code      *string
	Status    *string
	Remark    *string
	SortCode  *int32
}

func (s *Server) deptListWithFlash(c *gin.Context, msg, kind string) {
	v := s.buildDeptListView(c)
	v.Flash = &Flash{Kind: kind, Message: msg}
	s.renderPartial(c, "depts-refresh", v)
}

func (s *Server) renderDeptFormError(c *gin.Context, id string, in deptInput, err error) {
	v := View{Title: "部门表单", Error: asError(err)}
	rows, _ := s.q.ListDepts(c.Request.Context())
	v.Depts = deptOptionsFromRows(rows)
	v.DeptForm = deptForm{
		ID:        id,
		Name:      in.Name,
		ShortName: in.ShortName,
		ParentID:  in.ParentID,
	}
	if in.Code != nil {
		v.DeptForm.Code = *in.Code
	}
	if in.Status != nil {
		v.DeptForm.Status = *in.Status
	} else {
		v.DeptForm.Status = "1"
	}
	if in.Remark != nil {
		v.DeptForm.Remark = *in.Remark
	}
	if in.SortCode != nil {
		v.DeptForm.SortCode = *in.SortCode
	}
	s.renderPartial(c, "dept-form", v)
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
