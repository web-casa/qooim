package console

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/web-casa/qooim/internal/repo/db"
	"github.com/web-casa/qooim/internal/service"
)

// userRow is the table-row projection. We assemble it from t_user +
// t_account (for username) + t_dept (for the department name) — list
// queries don't join these in the existing sqlc, so we hydrate in
// Go. For Gate-1 spike that's fine; if it shows up in profiles later
// we promote it to a sqlc query.
type userRow struct {
	ID       string
	Name     string
	Username string
	DeptName string
	Email    string
	Status   int16
	CreateAt time.Time
}

type userForm struct {
	ID       string
	Name     string
	Username string
	Email    string
	Phone    string
	DeptID   string
	Status   int16
}

type deptOption struct {
	ID   string
	Name string
}

type roleOption struct {
	ID   string
	Name string
}

// ---- list ------------------------------------------------------------------

func (s *Server) getUsers(c *gin.Context) {
	s.render(c, "system/users/list.html", s.buildUserListView(c))
}

func (s *Server) getUsersTable(c *gin.Context) {
	s.renderPartial(c, "users-table", s.buildUserListView(c))
}

func (s *Server) buildUserListView(c *gin.Context) View {
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

	rows, err := s.q.ListUsersForConsole(c.Request.Context(), db.ListUsersForConsoleParams{
		Name: nameFilter,
		Off:  int32(offset),
		Lim:  int32(limit),
	})
	if err != nil {
		return View{Title: "用户管理", Error: err.Error()}
	}

	out := make([]userRow, 0, len(rows))
	for _, r := range rows {
		row := userRow{
			ID:       r.ID,
			Name:     r.Name,
			Status:   r.Status,
			CreateAt: r.CreateAt,
		}
		if r.Email.Valid {
			row.Email = r.Email.String
		}
		if r.DeptName.Valid {
			row.DeptName = r.DeptName.String
		}
		// Username comes back from the LATERAL sub-query. sqlc types
		// it as a plain string (auth_account is NOT NULL in the
		// table), but a user with zero active PWD accounts will land
		// here as "" — fine for display.
		row.Username = r.Username
		out = append(out, row)
	}

	total, _ := s.q.CountUsers(c.Request.Context(), db.CountUsersParams{Name: nameFilter})
	totalPages := int(total) / limit
	if int(total)%limit != 0 {
		totalPages++
	}
	if totalPages == 0 {
		totalPages = 1
	}

	return View{
		Title:      "用户管理",
		Active:     "system/users",
		Crumb:      "系统设置 / 用户管理",
		Q:          q,
		Page:       page,
		TotalPages: totalPages,
		Total:      int(total),
		Rows:       out,
	}
}

// ---- form ------------------------------------------------------------------

// getUserForm handles both /new and /:id/edit. We tell them apart via
// the route param: empty id → new.
func (s *Server) getUserForm(c *gin.Context) {
	id := c.Param("id")
	v := View{Title: "用户表单"}
	v.Depts = s.loadDepts(c)
	v.Roles = s.loadRoles(c)
	v.UserRoleSet = map[string]bool{}

	if id != "" {
		u, err := s.q.GetUserByID(c.Request.Context(), id)
		if err != nil {
			v.Error = asError(err)
			s.renderPartial(c, "user-form", v)
			return
		}
		v.User.ID = u.ID
		v.User.Name = u.Name
		v.User.Status = u.Status
		if u.Email.Valid {
			v.User.Email = u.Email.String
		}
		if u.DeptID.Valid {
			v.User.DeptID = u.DeptID.String
		}
		// Username comes from t_account; one extra round-trip on the
		// edit path is fine since we land here at most once per click.
		// Phone isn't projected by GetUserByID (sqlc); we skip it on
		// the form for the spike. Promote to a sqlc query if the
		// product team wants it editable.
		v.User.Username = s.lookupUsername(c, u.ID)
		if rids, err := s.q.ListUserRoleIDs(c.Request.Context(), id); err == nil {
			for _, r := range rids {
				v.UserRoleSet[r] = true
			}
		}
	} else {
		v.User.Status = 1
	}
	s.renderPartial(c, "user-form", v)
}

func (s *Server) loadDepts(c *gin.Context) []deptOption {
	rows, err := s.q.ListDepts(c.Request.Context())
	if err != nil {
		return nil
	}
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

func (s *Server) loadRoles(c *gin.Context) []roleOption {
	rows, err := s.q.ListRoles(c.Request.Context(), db.ListRolesParams{
		Off: 0,
		Lim: 200,
	})
	if err != nil {
		return nil
	}
	out := make([]roleOption, 0, len(rows))
	for _, r := range rows {
		name := r.Name
		if name == "" {
			name = r.Code
		}
		out = append(out, roleOption{ID: r.ID, Name: name})
	}
	return out
}

// lookupUsername fetches t_account.auth_account for a user_id with one
// trip to rawDB. We don't have a sqlc query for it yet; bringing this
// inline keeps the spike moving without scope-creeping migrations.
func (s *Server) lookupUsername(c *gin.Context, userID string) string {
	if s.rawDB == nil {
		return ""
	}
	var name sql.NullString
	row := s.rawDB.QueryRowContext(c.Request.Context(),
		`SELECT auth_account FROM t_account
		   WHERE user_id=$1 AND auth_type='PWD' AND is_deleted=0
		   LIMIT 1`, userID)
	_ = row.Scan(&name)
	return name.String
}

// ---- create ----------------------------------------------------------------

func (s *Server) postUser(c *gin.Context) {
	in := service.CreateUserInput{
		Username: strings.TrimSpace(c.PostForm("username")),
		Password: c.PostForm("password"),
		Name:     strings.TrimSpace(c.PostForm("name")),
		RoleIDs:  c.PostFormArray("roleIds"),
	}
	if v := strings.TrimSpace(c.PostForm("email")); v != "" {
		in.Email = &v
	}
	if v := strings.TrimSpace(c.PostForm("phone")); v != "" {
		in.Phone = &v
	}
	if v := strings.TrimSpace(c.PostForm("deptId")); v != "" {
		in.DeptID = &v
	}
	if v := c.PostForm("status"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			n16 := int16(n)
			in.Status = &n16
		}
	}

	by := principalOf(c).UserID
	if _, err := s.sysSvc.CreateUser(c.Request.Context(), in, by); err != nil {
		s.renderUserFormError(c, in, "", err)
		return
	}
	s.userListWithFlash(c, "用户已创建", flashKindOK)
}

// ---- update ----------------------------------------------------------------

func (s *Server) putUser(c *gin.Context) {
	id := c.Param("id")
	in := service.UpdateUserInput{
		ResetRoles: true, // form always sends the full role set
		RoleIDs:    c.PostFormArray("roleIds"),
	}
	if v := strings.TrimSpace(c.PostForm("name")); v != "" {
		in.Name = &v
	}
	if v := strings.TrimSpace(c.PostForm("email")); v != "" {
		in.Email = &v
	}
	if v := strings.TrimSpace(c.PostForm("phone")); v != "" {
		in.Phone = &v
	}
	if v := strings.TrimSpace(c.PostForm("deptId")); v != "" {
		in.DeptID = &v
	}
	if v := c.PostForm("status"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			n16 := int16(n)
			in.Status = &n16
		}
	}

	by := principalOf(c).UserID
	if err := s.sysSvc.UpdateUser(c.Request.Context(), id, in, by); err != nil {
		// Re-render the form with the error visible — losing the user's
		// in-flight edits to a generic 500 page is the kind of thing
		// that erodes trust fast.
		ce := service.CreateUserInput{
			Name:    deref(in.Name),
			RoleIDs: in.RoleIDs,
		}
		if in.Email != nil {
			ce.Email = in.Email
		}
		if in.Phone != nil {
			ce.Phone = in.Phone
		}
		if in.DeptID != nil {
			ce.DeptID = in.DeptID
		}
		if in.Status != nil {
			ce.Status = in.Status
		}
		s.renderUserFormError(c, ce, id, err)
		return
	}
	s.userListWithFlash(c, "用户已更新", flashKindOK)
}

// ---- delete ----------------------------------------------------------------

func (s *Server) deleteUser(c *gin.Context) {
	id := c.Param("id")
	by := principalOf(c).UserID
	if err := s.sysSvc.DeleteUser(c.Request.Context(), id, by); err != nil {
		// HTMX swap target is the table row; a 4xx with text triggers
		// the visible error trail without leaving the row in the DOM.
		c.String(http.StatusBadRequest, asError(err))
		return
	}
	// Empty body → row gets swapped out of existence.
	c.Status(http.StatusOK)
	c.Header("HX-Trigger", `{"flash":{"kind":"","message":"用户已删除"}}`)
}

// ---- helpers ---------------------------------------------------------------

// userListWithFlash renders the post-mutation response: an OOB swap of
// the user-table region (which closes the modal because the form's hx-
// target points at #modal-host and we emit nothing inside it) plus a
// transient flash message. The whole thing is a single partial template
// — no hand-built HTML strings, no `err.Error()` slipping into the
// response unescaped.
func (s *Server) userListWithFlash(c *gin.Context, msg, kind string) {
	v := s.buildUserListView(c)
	v.Flash = &Flash{Kind: kind, Message: msg}
	s.renderPartial(c, "users-refresh", v)
}

func (s *Server) renderUserFormError(c *gin.Context, in service.CreateUserInput, id string, err error) {
	v := View{Title: "用户表单", Error: asError(err)}
	v.Depts = s.loadDepts(c)
	v.Roles = s.loadRoles(c)
	v.UserRoleSet = map[string]bool{}
	for _, r := range in.RoleIDs {
		v.UserRoleSet[r] = true
	}
	v.User = userForm{
		ID:       id,
		Name:     in.Name,
		Username: in.Username,
		Status:   1,
	}
	if in.Email != nil {
		v.User.Email = *in.Email
	}
	if in.Phone != nil {
		v.User.Phone = *in.Phone
	}
	if in.DeptID != nil {
		v.User.DeptID = *in.DeptID
	}
	if in.Status != nil {
		v.User.Status = *in.Status
	}
	s.renderPartial(c, "user-form", v)
}

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// keep the unused-import linter quiet during early scaffolding.
var _ = errors.Is
