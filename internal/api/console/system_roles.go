package console

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/web-casa/qooim/internal/repo/db"
	"github.com/web-casa/qooim/internal/service"
)

type roleRow struct {
	ID        string
	Name      string
	Code      string
	Authority string
	Remark    string
	Status    int16
}

type roleForm struct {
	ID        string
	Name      string
	Code      string
	Authority string
	Remark    string
	Status    int16
}

// ---- list ------------------------------------------------------------------

func (s *Server) getRoles(c *gin.Context) {
	s.render(c, "system/roles/list.html", s.buildRoleListView(c))
}

func (s *Server) getRolesTable(c *gin.Context) {
	s.renderPartial(c, "roles-table", s.buildRoleListView(c))
}

func (s *Server) buildRoleListView(c *gin.Context) View {
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

	rows, err := s.q.ListRoles(c.Request.Context(), db.ListRolesParams{
		Name: nameFilter,
		Off:  int32(offset),
		Lim:  int32(limit),
	})
	if err != nil {
		return View{Title: "角色管理", Error: "load roles failed"}
	}
	out := make([]roleRow, 0, len(rows))
	for _, r := range rows {
		row := roleRow{ID: r.ID, Name: r.Name, Code: r.Code}
		if r.Authority.Valid {
			row.Authority = r.Authority.String
		}
		if r.Remark.Valid {
			row.Remark = r.Remark.String
		}
		if r.Status.Valid {
			row.Status = r.Status.Int16
		}
		out = append(out, row)
	}

	total, _ := s.q.CountRoles(c.Request.Context(), db.CountRolesParams{Name: nameFilter})
	totalPages := int(total) / limit
	if int(total)%limit != 0 {
		totalPages++
	}
	if totalPages == 0 {
		totalPages = 1
	}
	return View{
		Title:      "角色管理",
		Active:     "system/roles",
		Crumb:      "系统设置 / 角色管理",
		Q:          q,
		Page:       page,
		TotalPages: totalPages,
		Total:      int(total),
		RoleRows:   out,
	}
}

// ---- form ------------------------------------------------------------------

func (s *Server) getRoleForm(c *gin.Context) {
	id := c.Param("id")
	v := View{Title: "角色表单"}
	if id != "" {
		r, err := s.q.GetRoleByID(c.Request.Context(), id)
		if err != nil {
			v.Error = asError(err)
			s.renderPartial(c, "role-form", v)
			return
		}
		v.RoleForm = roleForm{ID: r.ID, Name: r.Name, Code: r.Code}
		if r.Authority.Valid {
			v.RoleForm.Authority = r.Authority.String
		}
		if r.Remark.Valid {
			v.RoleForm.Remark = r.Remark.String
		}
		if r.Status.Valid {
			v.RoleForm.Status = r.Status.Int16
		}
	} else {
		v.RoleForm.Status = 1
	}
	s.renderPartial(c, "role-form", v)
}

// ---- create ----------------------------------------------------------------

func (s *Server) postRole(c *gin.Context) {
	in := service.CreateRoleInput{
		Name: strings.TrimSpace(c.PostForm("name")),
		Code: strings.TrimSpace(c.PostForm("code")),
	}
	if v := strings.TrimSpace(c.PostForm("authority")); v != "" {
		in.Authority = &v
	}
	if v := strings.TrimSpace(c.PostForm("remark")); v != "" {
		in.Remark = &v
	}
	if v := c.PostForm("status"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			n16 := int16(n)
			in.Status = &n16
		}
	}
	by := principalOf(c).UserID
	if _, err := s.sysSvc.CreateRole(c.Request.Context(), in, by); err != nil {
		s.renderRoleFormError(c, in, "", err)
		return
	}
	s.roleListWithFlash(c, "角色已创建", flashKindOK)
}

// ---- update ----------------------------------------------------------------

func (s *Server) putRole(c *gin.Context) {
	id := c.Param("id")
	in := service.UpdateRoleInput{}
	if v := strings.TrimSpace(c.PostForm("name")); v != "" {
		in.Name = &v
	}
	if v := strings.TrimSpace(c.PostForm("code")); v != "" {
		in.Code = &v
	}
	if v := strings.TrimSpace(c.PostForm("authority")); v != "" {
		in.Authority = &v
	}
	if v := strings.TrimSpace(c.PostForm("remark")); v != "" {
		in.Remark = &v
	}
	if v := c.PostForm("status"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			n16 := int16(n)
			in.Status = &n16
		}
	}
	by := principalOf(c).UserID
	if err := s.sysSvc.UpdateRole(c.Request.Context(), id, in, by); err != nil {
		s.renderRoleFormError(c, service.CreateRoleInput{
			Name:      deref(in.Name),
			Code:      deref(in.Code),
			Authority: in.Authority,
			Remark:    in.Remark,
			Status:    in.Status,
		}, id, err)
		return
	}
	s.roleListWithFlash(c, "角色已更新", flashKindOK)
}

// ---- delete ----------------------------------------------------------------

func (s *Server) deleteRole(c *gin.Context) {
	id := c.Param("id")
	by := principalOf(c).UserID
	if err := s.sysSvc.DeleteRole(c.Request.Context(), id, by); err != nil {
		c.String(http.StatusBadRequest, asError(err))
		return
	}
	c.Status(http.StatusOK)
}

// ---- helpers ---------------------------------------------------------------

func (s *Server) roleListWithFlash(c *gin.Context, msg, kind string) {
	v := s.buildRoleListView(c)
	v.Flash = &Flash{Kind: kind, Message: msg}
	s.renderPartial(c, "roles-refresh", v)
}

func (s *Server) renderRoleFormError(c *gin.Context, in service.CreateRoleInput, id string, err error) {
	v := View{Title: "角色表单", Error: asError(err)}
	v.RoleForm = roleForm{
		ID:     id,
		Name:   in.Name,
		Code:   in.Code,
		Status: 1,
	}
	if in.Authority != nil {
		v.RoleForm.Authority = *in.Authority
	}
	if in.Remark != nil {
		v.RoleForm.Remark = *in.Remark
	}
	if in.Status != nil {
		v.RoleForm.Status = *in.Status
	}
	s.renderPartial(c, "role-form", v)
}
