// SK-compat /api/system/* handlers — admin panel for sysuser, role,
// dept, position, dict/dictItem, plus the singleton system info row.
//
// Out of scope for C3 (deferred to C4):
//   - dept/sort
//   - dictItem/import
//   - aiSetting write (read returns the column as-is)
//   - register/check side endpoints used by /api/public/register
package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/web-casa/qooim/internal/idgen"
	"github.com/web-casa/qooim/internal/repo/db"
	"github.com/web-casa/qooim/internal/service"
)

// ============================================================================
// /api/system  — system info
// ============================================================================

func (s *Server) handleSKSystem(c *gin.Context) {
	pubKey := ""
	if s.loginKP != nil {
		pubKey = s.loginKP.PublicKeyB64()
	}
	row, err := s.q.GetDefaultSysInfo(c.Request.Context())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// Even without a sys_info row the SPA still needs a publicKey
			// to RSA-encrypt the login form, so emit the bare minimum.
			skOK(c, gin.H{"publicKey": pubKey})
			return
		}
		s.logger.Error("sk.system.get", "err", err)
		skErr(c, http.StatusInternalServerError, "load system info")
		return
	}
	skOK(c, gin.H{
		"id":           row.ID,
		"name":         valueOr(row.Name),
		"description":  valueOr(row.Description),
		"avatar":       valueOr(row.Avatar),
		"locale":       valueOr(row.Locale),
		"version":      valueOr(row.Version),
		"setting":      jsonOrEmptyObject(row.Setting),
		"aiSetting":    jsonOrEmptyObject(row.AiSetting),
		"registerInfo": jsonOrEmptyObject(row.RegisterInfo),
		"isDefault":    nullBool(row.IsDefault),
		"createAt":     row.CreateAt,
		"updateAt":     nullTime(row.UpdateAt),
		// SK frontend reads `system.publicKey` and RSA-encrypts the
		// login password with it before posting /api/public/login.
		"publicKey": pubKey,
	})
}

// jsonOrEmptyObject decodes a NullString JSON column, defaulting to
// {} so SK frontend code that does `setting.foo` doesn't crash on null.
func jsonOrEmptyObject(n sql.NullString) json.RawMessage {
	if !n.Valid || strings.TrimSpace(n.String) == "" {
		return json.RawMessage("{}")
	}
	if !json.Valid([]byte(n.String)) {
		return json.RawMessage("{}")
	}
	return json.RawMessage(n.String)
}

// PUT-style update via the SK action route POST /api/system/update.
func (s *Server) handleSKSystemUpdate(c *gin.Context) {
	var in struct {
		ID           string          `json:"id,omitempty"`
		Name         *string         `json:"name,omitempty"`
		Description  *string         `json:"description,omitempty"`
		Avatar       *string         `json:"avatar,omitempty"`
		Locale       *string         `json:"locale,omitempty"`
		Version      *string         `json:"version,omitempty"`
		Setting      json.RawMessage `json:"setting,omitempty"`
		AiSetting    json.RawMessage `json:"aiSetting,omitempty"`
		RegisterInfo json.RawMessage `json:"registerInfo,omitempty"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		skErr(c, http.StatusBadRequest, "invalid body")
		return
	}
	id := in.ID
	if id == "" {
		row, err := s.q.GetDefaultSysInfo(c.Request.Context())
		if err != nil {
			skErr(c, http.StatusInternalServerError, "load system info")
			return
		}
		id = row.ID
	}
	p := db.UpdateDefaultSysInfoParams{
		ID:       id,
		UpdateBy: sql.NullString{String: principalID(c), Valid: true},
	}
	if in.Name != nil {
		p.Name = sql.NullString{String: *in.Name, Valid: true}
	}
	if in.Description != nil {
		p.Description = sql.NullString{String: *in.Description, Valid: true}
	}
	if in.Avatar != nil {
		p.Avatar = sql.NullString{String: *in.Avatar, Valid: true}
	}
	if in.Locale != nil {
		p.Locale = sql.NullString{String: *in.Locale, Valid: true}
	}
	if in.Version != nil {
		p.Version = sql.NullString{String: *in.Version, Valid: true}
	}
	if in.Setting != nil {
		p.Setting = sql.NullString{String: string(in.Setting), Valid: true}
	}
	if in.AiSetting != nil {
		p.AiSetting = sql.NullString{String: string(in.AiSetting), Valid: true}
	}
	if in.RegisterInfo != nil {
		p.RegisterInfo = sql.NullString{String: string(in.RegisterInfo), Valid: true}
	}
	if err := s.q.UpdateDefaultSysInfo(c.Request.Context(), p); err != nil {
		s.logger.Error("sk.system.update", "err", err)
		skErr(c, http.StatusInternalServerError, "update system info")
		return
	}
	skOK(c, gin.H{"id": id})
}

// /api/system/aiSetting — read-only alias to system.aiSetting.
func (s *Server) handleSKAiSetting(c *gin.Context) {
	row, err := s.q.GetDefaultSysInfo(c.Request.Context())
	if err != nil {
		// Treat absence as empty rather than error; the panel renders OK.
		skOK(c, gin.H{})
		return
	}
	skOK(c, decodeJSONColumn(valueOr(row.AiSetting)))
}

// ============================================================================
// /api/system/permission/list
// ============================================================================

// SK frontend renders an authority picker for the role-edit screen. We
// return the unique union of every active role's `authority` column,
// flat. This matches what SK's old backend exposed (a simple list, not
// a tree); the UI groups by colon-separated prefixes itself.
//
// We query unpaged via AllRoleAuthorities so deployments with hundreds
// of roles aren't silently truncated.
func (s *Server) handleSKPermissionList(c *gin.Context) {
	blobs, err := s.q.AllRoleAuthorities(c.Request.Context())
	if err != nil {
		s.logger.Error("sk.permission.list", "err", err)
		skErr(c, http.StatusInternalServerError, "load permissions")
		return
	}
	seen := map[string]struct{}{}
	out := make([]string, 0)
	for _, blob := range blobs {
		for _, p := range strings.Split(blob, ",") {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			if _, ok := seen[p]; !ok {
				seen[p] = struct{}{}
				out = append(out, p)
			}
		}
	}
	skOK(c, out)
}

// ============================================================================
// /api/system/dept/* — dept/list returns a flat array (no pagination)
// ============================================================================

type skDeptItem struct {
	ID           string          `json:"id"`
	ParentID     string          `json:"parentId,omitempty"`
	Name         string          `json:"name"`
	ShortName    string          `json:"shortName,omitempty"`
	Code         string          `json:"code,omitempty"`
	ManagerID    string          `json:"managerId,omitempty"`
	SortCode     int32           `json:"sortCode"`
	PropertyJSON json.RawMessage `json:"propertyJson,omitempty"`
	Status       string          `json:"status,omitempty"`
	Remark       string          `json:"remark,omitempty"`
	CreateAt     time.Time       `json:"createAt"`
	UpdateAt     *time.Time      `json:"updateAt,omitempty"`
	CreateBy     string          `json:"createBy,omitempty"`
}

func (s *Server) handleSKDeptList(c *gin.Context) {
	rows, err := s.q.ListDepts(c.Request.Context())
	if err != nil {
		s.logger.Error("sk.dept.list", "err", err)
		skErr(c, http.StatusInternalServerError, "list depts")
		return
	}
	out := make([]skDeptItem, len(rows))
	for i, r := range rows {
		item := skDeptItem{
			ID:           r.ID,
			ParentID:     r.ParentID,
			Name:         valueOr(r.Name),
			ShortName:    r.ShortName,
			Code:         valueOr(r.Code),
			ManagerID:    valueOr(r.ManagerID),
			PropertyJSON: decodeJSONColumn(valueOr(r.PropertyJson)),
			Status:       valueOr(r.Status),
			Remark:       valueOr(r.Remark),
			CreateAt:     r.CreateAt,
			UpdateAt:     nullTime(r.UpdateAt),
			CreateBy:     valueOr(r.CreateBy),
		}
		if r.SortCode.Valid {
			item.SortCode = r.SortCode.Int32
		}
		out[i] = item
	}
	skOK(c, out)
}

func (s *Server) handleSKDeptCreate(c *gin.Context) {
	var in service.CreateDeptInput
	if err := c.ShouldBindJSON(&in); err != nil || in.Name == "" || in.ShortName == "" {
		skErr(c, http.StatusBadRequest, "name and shortName are required")
		return
	}
	id, err := s.system.CreateDept(c.Request.Context(), in, principalID(c))
	if err != nil {
		s.logger.Error("sk.dept.create", "err", err)
		skErr(c, http.StatusInternalServerError, "create dept")
		return
	}
	skOK(c, gin.H{"id": id})
}

func (s *Server) handleSKDeptUpdate(c *gin.Context) {
	var in struct {
		ID string `json:"id"`
		service.UpdateDeptInput
	}
	if err := c.ShouldBindJSON(&in); err != nil || in.ID == "" {
		skErr(c, http.StatusBadRequest, "id is required")
		return
	}
	if err := s.system.UpdateDept(c.Request.Context(), in.ID, in.UpdateDeptInput, principalID(c)); err != nil {
		if errors.Is(err, service.ErrNotFound) {
			skErr(c, http.StatusNotFound, "dept not found")
			return
		}
		s.logger.Error("sk.dept.update", "err", err)
		skErr(c, http.StatusInternalServerError, "update dept")
		return
	}
	skOK(c, gin.H{"id": in.ID})
}

func (s *Server) handleSKDeptDelete(c *gin.Context) {
	ids := readIDOrIDs(c)
	if len(ids) == 0 {
		skErr(c, http.StatusBadRequest, "id or ids is required")
		return
	}
	results := make([]gin.H, 0, len(ids))
	for _, id := range ids {
		if err := s.system.DeleteDept(c.Request.Context(), id, principalID(c)); err != nil {
			if errors.Is(err, service.ErrNotFound) {
				results = append(results, gin.H{"id": id, "ok": false, "reason": "not_found"})
				continue
			}
			s.logger.Error("sk.dept.delete", "err", err, "id", id)
			results = append(results, gin.H{"id": id, "ok": false, "reason": "error"})
			skList(c, results, len(ids))
			return
		}
		results = append(results, gin.H{"id": id, "ok": true})
	}
	skOK(c, gin.H{"deleted": countOK(results), "results": results})
}

// ============================================================================
// /api/system/role/*
// ============================================================================

type skRoleItem struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Code      string     `json:"code"`
	Remark    string     `json:"remark,omitempty"`
	Authority string     `json:"authority,omitempty"`
	Status    int16      `json:"status"`
	CreateAt  time.Time  `json:"createAt"`
	UpdateAt  *time.Time `json:"updateAt,omitempty"`
	CreateBy  string     `json:"createBy,omitempty"`
}

// systemRoles wraps the sqlc list for both the public list endpoint and
// the permission-extraction helper above.
func (s *Server) systemRoles(ctx context.Context, page, pageSize int, name, code string) (struct {
	Items []skRoleItem
	Total int
}, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 200 {
		pageSize = 200
	}
	params := db.ListRolesParams{
		Lim: int32(pageSize),
		Off: int32((page - 1) * pageSize),
	}
	if name != "" {
		params.Name = sql.NullString{String: name, Valid: true}
	}
	if code != "" {
		params.Code = sql.NullString{String: code, Valid: true}
	}
	rows, err := s.q.ListRoles(ctx, params)
	if err != nil {
		return struct {
			Items []skRoleItem
			Total int
		}{}, err
	}
	cparams := db.CountRolesParams{Name: params.Name, Code: params.Code}
	total, err := s.q.CountRoles(ctx, cparams)
	if err != nil {
		return struct {
			Items []skRoleItem
			Total int
		}{}, err
	}
	items := make([]skRoleItem, len(rows))
	for i, r := range rows {
		ri := skRoleItem{
			ID:        r.ID,
			Name:      r.Name,
			Code:      r.Code,
			Remark:    valueOr(r.Remark),
			Authority: valueOr(r.Authority),
			CreateAt:  r.CreateAt,
			UpdateAt:  nullTime(r.UpdateAt),
			CreateBy:  valueOr(r.CreateBy),
		}
		if r.Status.Valid {
			ri.Status = r.Status.Int16
		}
		items[i] = ri
	}
	return struct {
		Items []skRoleItem
		Total int
	}{Items: items, Total: int(total)}, nil
}

func (s *Server) handleSKRoleList(c *gin.Context) {
	page, _ := strconv.Atoi(c.Query("current"))
	size, _ := strconv.Atoi(c.Query("pageSize"))
	res, err := s.systemRoles(c.Request.Context(), page, size, c.Query("name"), c.Query("code"))
	if err != nil {
		s.logger.Error("sk.role.list", "err", err)
		skErr(c, http.StatusInternalServerError, "list roles")
		return
	}
	skList(c, res.Items, res.Total)
}

func (s *Server) handleSKRoleCreate(c *gin.Context) {
	var in service.CreateRoleInput
	if err := c.ShouldBindJSON(&in); err != nil || in.Name == "" || in.Code == "" {
		skErr(c, http.StatusBadRequest, "name and code are required")
		return
	}
	id, err := s.system.CreateRole(c.Request.Context(), in, principalID(c))
	if err != nil {
		s.logger.Error("sk.role.create", "err", err)
		skErr(c, http.StatusInternalServerError, "create role")
		return
	}
	skOK(c, gin.H{"id": id})
}

func (s *Server) handleSKRoleUpdate(c *gin.Context) {
	var in struct {
		ID string `json:"id"`
		service.UpdateRoleInput
	}
	if err := c.ShouldBindJSON(&in); err != nil || in.ID == "" {
		skErr(c, http.StatusBadRequest, "id is required")
		return
	}
	if err := s.system.UpdateRole(c.Request.Context(), in.ID, in.UpdateRoleInput, principalID(c)); err != nil {
		if errors.Is(err, service.ErrNotFound) {
			skErr(c, http.StatusNotFound, "role not found")
			return
		}
		s.logger.Error("sk.role.update", "err", err)
		skErr(c, http.StatusInternalServerError, "update role")
		return
	}
	skOK(c, gin.H{"id": in.ID})
}

func (s *Server) handleSKRoleDelete(c *gin.Context) {
	ids := readIDOrIDs(c)
	if len(ids) == 0 {
		skErr(c, http.StatusBadRequest, "id or ids is required")
		return
	}
	results := make([]gin.H, 0, len(ids))
	for _, id := range ids {
		if err := s.system.DeleteRole(c.Request.Context(), id, principalID(c)); err != nil {
			if errors.Is(err, service.ErrNotFound) {
				results = append(results, gin.H{"id": id, "ok": false, "reason": "not_found"})
				continue
			}
			s.logger.Error("sk.role.delete", "err", err, "id", id)
			results = append(results, gin.H{"id": id, "ok": false, "reason": "error"})
			skList(c, results, len(ids))
			return
		}
		results = append(results, gin.H{"id": id, "ok": true})
	}
	skOK(c, gin.H{"deleted": countOK(results), "results": results})
}

// ============================================================================
// /api/system/user/*
// ============================================================================

type skUserItem struct {
	ID       string     `json:"id"`
	Name     string     `json:"name"`
	DeptID   string     `json:"deptId,omitempty"`
	Gender   string     `json:"gender,omitempty"`
	Phone    string     `json:"phone,omitempty"`
	Email    string     `json:"email,omitempty"`
	Avatar   string     `json:"avatar,omitempty"`
	Status   int16      `json:"status"`
	Profile  string     `json:"profile,omitempty"`
	RoleIDs  []string   `json:"roleIds,omitempty"`
	CreateAt time.Time  `json:"createAt"`
	UpdateAt *time.Time `json:"updateAt,omitempty"`
	CreateBy string     `json:"createBy,omitempty"`
}

func (s *Server) handleSKUserList(c *gin.Context) {
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
	params := db.ListUsersParams{
		Lim: int32(size),
		Off: int32((page - 1) * size),
	}
	if v := c.Query("name"); v != "" {
		params.Name = sql.NullString{String: v, Valid: true}
	}
	if v := c.Query("deptId"); v != "" {
		params.DeptID = sql.NullString{String: v, Valid: true}
	}
	rows, err := s.q.ListUsers(c.Request.Context(), params)
	if err != nil {
		s.logger.Error("sk.user.list", "err", err)
		skErr(c, http.StatusInternalServerError, "list users")
		return
	}
	cparams := db.CountUsersParams{Name: params.Name, DeptID: params.DeptID}
	total, err := s.q.CountUsers(c.Request.Context(), cparams)
	if err != nil {
		s.logger.Error("sk.user.count", "err", err)
		skErr(c, http.StatusInternalServerError, "count users")
		return
	}
	items := make([]skUserItem, len(rows))
	userIDs := make([]string, 0, len(rows))
	for i, r := range rows {
		items[i] = skUserItem{
			ID:       r.ID,
			Name:     r.Name,
			DeptID:   valueOr(r.DeptID),
			Gender:   valueOr(r.Gender),
			Phone:    valueOr(r.Phone),
			Email:    valueOr(r.Email),
			Avatar:   valueOr(r.Avatar),
			Status:   r.Status,
			Profile:  valueOr(r.Profile),
			CreateAt: r.CreateAt,
			UpdateAt: nullTime(r.UpdateAt),
			CreateBy: valueOr(r.CreateBy),
		}
		userIDs = append(userIDs, r.ID)
	}
	// Role bindings in ONE batch query, then group in memory. The
	// previous ListUserRoleIDs-per-row was a 1+N pattern that paged
	// out to ~200 trips at pageSize=200.
	if len(userIDs) > 0 {
		bindings, err := s.q.ListUserRolesByUserIDs(c.Request.Context(), userIDs)
		if err == nil {
			byUser := make(map[string][]string, len(userIDs))
			for _, b := range bindings {
				byUser[b.UserID] = append(byUser[b.UserID], b.RoleID)
			}
			for i := range items {
				items[i].RoleIDs = byUser[items[i].ID]
			}
		}
	}
	skList(c, items, int(total))
}

func (s *Server) handleSKUserCreate(c *gin.Context) {
	var in service.CreateUserInput
	if err := c.ShouldBindJSON(&in); err != nil || in.Username == "" || in.Password == "" {
		skErr(c, http.StatusBadRequest, "username and password are required")
		return
	}
	id, err := s.system.CreateUser(c.Request.Context(), in, principalID(c))
	if err != nil {
		s.logger.Error("sk.user.create", "err", err)
		skErr(c, http.StatusInternalServerError, "create user")
		return
	}
	skOK(c, gin.H{"id": id})
}

func (s *Server) handleSKUserUpdate(c *gin.Context) {
	var in struct {
		ID string `json:"id"`
		service.UpdateUserInput
	}
	if err := c.ShouldBindJSON(&in); err != nil || in.ID == "" {
		skErr(c, http.StatusBadRequest, "id is required")
		return
	}
	if err := s.system.UpdateUser(c.Request.Context(), in.ID, in.UpdateUserInput, principalID(c)); err != nil {
		if errors.Is(err, service.ErrNotFound) {
			skErr(c, http.StatusNotFound, "user not found")
			return
		}
		s.logger.Error("sk.user.update", "err", err)
		skErr(c, http.StatusInternalServerError, "update user")
		return
	}
	skOK(c, gin.H{"id": in.ID})
}

func (s *Server) handleSKUserDelete(c *gin.Context) {
	ids := readIDOrIDs(c)
	if len(ids) == 0 {
		skErr(c, http.StatusBadRequest, "id or ids is required")
		return
	}
	results := make([]gin.H, 0, len(ids))
	for _, id := range ids {
		if err := s.system.DeleteUser(c.Request.Context(), id, principalID(c)); err != nil {
			if errors.Is(err, service.ErrNotFound) {
				results = append(results, gin.H{"id": id, "ok": false, "reason": "not_found"})
				continue
			}
			s.logger.Error("sk.user.delete", "err", err, "id", id)
			results = append(results, gin.H{"id": id, "ok": false, "reason": "error"})
			skList(c, results, len(ids))
			return
		}
		results = append(results, gin.H{"id": id, "ok": true})
	}
	skOK(c, gin.H{"deleted": countOK(results), "results": results})
}

// /api/system/checkUsernameExist?username=… — boolean exists check.
func (s *Server) handleSKCheckUsername(c *gin.Context) {
	username := c.Query("username")
	if username == "" {
		skErr(c, http.StatusBadRequest, "username is required")
		return
	}
	n, err := s.q.CountAccountsByUsername(c.Request.Context(), username)
	if err != nil {
		s.logger.Error("sk.checkUsername", "err", err)
		skErr(c, http.StatusInternalServerError, "check username")
		return
	}
	skOK(c, gin.H{"exists": n > 0, "username": username})
}

// ============================================================================
// /api/system/position/*
// ============================================================================

type skPositionItem struct {
	ID                 string          `json:"id"`
	Name               string          `json:"name"`
	Code               string          `json:"code,omitempty"`
	IsVirtual          int16           `json:"isVirtual"`
	DataPermissionType string          `json:"dataPermissionType,omitempty"`
	PropertyJSON       json.RawMessage `json:"propertyJson,omitempty"`
	CreateAt           time.Time       `json:"createAt"`
	UpdateAt           *time.Time      `json:"updateAt,omitempty"`
	CreateBy           string          `json:"createBy,omitempty"`
}

func (s *Server) handleSKPositionList(c *gin.Context) {
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
	params := db.ListPositionsParams{
		Lim: int32(size),
		Off: int32((page - 1) * size),
	}
	if v := c.Query("name"); v != "" {
		params.Name = sql.NullString{String: v, Valid: true}
	}
	rows, err := s.q.ListPositions(c.Request.Context(), params)
	if err != nil {
		s.logger.Error("sk.position.list", "err", err)
		skErr(c, http.StatusInternalServerError, "list positions")
		return
	}
	total, _ := s.q.CountPositions(c.Request.Context(), params.Name)
	items := make([]skPositionItem, len(rows))
	for i, r := range rows {
		items[i] = skPositionItem{
			ID:                 r.ID,
			Name:               r.Name,
			Code:               valueOr(r.Code),
			IsVirtual:          r.IsVirtual,
			DataPermissionType: valueOr(r.DataPermissionType),
			PropertyJSON:       decodeJSONColumn(valueOr(r.PropertyJson)),
			CreateAt:           r.CreateAt,
			UpdateAt:           nullTime(r.UpdateAt),
			CreateBy:           valueOr(r.CreateBy),
		}
	}
	skList(c, items, int(total))
}

type skPositionMutateReq struct {
	ID                 string  `json:"id,omitempty"`
	Name               *string `json:"name,omitempty"`
	Code               *string `json:"code,omitempty"`
	IsVirtual          *int16  `json:"isVirtual,omitempty"`
	DataPermissionType *string `json:"dataPermissionType,omitempty"`
	PropertyJSON       *string `json:"propertyJson,omitempty"`
}

func (s *Server) handleSKPositionCreate(c *gin.Context) {
	var req skPositionMutateReq
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == nil || *req.Name == "" {
		skErr(c, http.StatusBadRequest, "name is required")
		return
	}
	id := idgen.New()
	p := db.CreatePositionParams{
		ID:        id,
		Name:      *req.Name,
		IsVirtual: 0,
		CreateBy:  sql.NullString{String: principalID(c), Valid: true},
	}
	if req.IsVirtual != nil {
		p.IsVirtual = *req.IsVirtual
	}
	if req.Code != nil {
		p.Code = sql.NullString{String: *req.Code, Valid: true}
	}
	if req.DataPermissionType != nil {
		p.DataPermissionType = sql.NullString{String: *req.DataPermissionType, Valid: true}
	}
	if req.PropertyJSON != nil {
		p.PropertyJson = sql.NullString{String: *req.PropertyJSON, Valid: true}
	}
	if err := s.q.CreatePosition(c.Request.Context(), p); err != nil {
		s.logger.Error("sk.position.create", "err", err)
		skErr(c, http.StatusInternalServerError, "create position")
		return
	}
	skOK(c, gin.H{"id": id})
}

func (s *Server) handleSKPositionUpdate(c *gin.Context) {
	var req skPositionMutateReq
	if err := c.ShouldBindJSON(&req); err != nil || req.ID == "" {
		skErr(c, http.StatusBadRequest, "id is required")
		return
	}
	if _, err := s.q.GetPositionByID(c.Request.Context(), req.ID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			skErr(c, http.StatusNotFound, "position not found")
			return
		}
		skErr(c, http.StatusInternalServerError, "load position")
		return
	}
	p := db.UpdatePositionParams{
		ID:       req.ID,
		UpdateBy: sql.NullString{String: principalID(c), Valid: true},
	}
	if req.Name != nil {
		p.Name = sql.NullString{String: *req.Name, Valid: true}
	}
	if req.Code != nil {
		p.Code = sql.NullString{String: *req.Code, Valid: true}
	}
	if req.IsVirtual != nil {
		p.IsVirtual = sql.NullInt16{Int16: *req.IsVirtual, Valid: true}
	}
	if req.DataPermissionType != nil {
		p.DataPermissionType = sql.NullString{String: *req.DataPermissionType, Valid: true}
	}
	if req.PropertyJSON != nil {
		p.PropertyJson = sql.NullString{String: *req.PropertyJSON, Valid: true}
	}
	if err := s.q.UpdatePosition(c.Request.Context(), p); err != nil {
		s.logger.Error("sk.position.update", "err", err)
		skErr(c, http.StatusInternalServerError, "update position")
		return
	}
	skOK(c, gin.H{"id": req.ID})
}

func (s *Server) handleSKPositionDelete(c *gin.Context) {
	ids := readIDOrIDs(c)
	if len(ids) == 0 {
		skErr(c, http.StatusBadRequest, "id or ids is required")
		return
	}
	results := make([]gin.H, 0, len(ids))
	for _, id := range ids {
		if _, err := s.q.GetPositionByID(c.Request.Context(), id); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				results = append(results, gin.H{"id": id, "ok": false, "reason": "not_found"})
				continue
			}
			s.logger.Error("sk.position.delete", "err", err, "id", id)
			results = append(results, gin.H{"id": id, "ok": false, "reason": "error"})
			skList(c, results, len(ids))
			return
		}
		if err := s.q.SoftDeletePosition(c.Request.Context(), db.SoftDeletePositionParams{
			UpdateBy: sql.NullString{String: principalID(c), Valid: true},
			ID:       id,
		}); err != nil {
			s.logger.Error("sk.position.delete", "err", err, "id", id)
			results = append(results, gin.H{"id": id, "ok": false, "reason": "error"})
			skList(c, results, len(ids))
			return
		}
		results = append(results, gin.H{"id": id, "ok": true})
	}
	skOK(c, gin.H{"deleted": countOK(results), "results": results})
}

// ============================================================================
// /api/system/dict/* and /api/system/dictItem/*
// ============================================================================

type skDictItem struct {
	ID       string     `json:"id"`
	Code     string     `json:"code,omitempty"`
	Name     string     `json:"name,omitempty"`
	Remark   string     `json:"remark,omitempty"`
	DictType int32      `json:"dictType"`
	CreateAt time.Time  `json:"createAt"`
	UpdateAt *time.Time `json:"updateAt,omitempty"`
	CreateBy string     `json:"createBy,omitempty"`
}

type skDictItemRow struct {
	ID              string     `json:"id"`
	DictCode        string     `json:"dictCode,omitempty"`
	ItemName        string     `json:"itemName,omitempty"`
	ItemValue       string     `json:"itemValue"`
	ItemOrder       int32      `json:"itemOrder"`
	ItemLevel       int32      `json:"itemLevel"`
	ParentItemValue string     `json:"parentItemValue,omitempty"`
	CreateAt        time.Time  `json:"createAt"`
	UpdateAt        *time.Time `json:"updateAt,omitempty"`
	CreateBy        string     `json:"createBy,omitempty"`
}

func (s *Server) handleSKDictList(c *gin.Context) {
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
	params := db.ListDictsParams{Lim: int32(size), Off: int32((page - 1) * size)}
	if v := c.Query("name"); v != "" {
		params.Name = sql.NullString{String: v, Valid: true}
	}
	if v := c.Query("code"); v != "" {
		params.Code = sql.NullString{String: v, Valid: true}
	}
	rows, err := s.q.ListDicts(c.Request.Context(), params)
	if err != nil {
		s.logger.Error("sk.dict.list", "err", err)
		skErr(c, http.StatusInternalServerError, "list dicts")
		return
	}
	total, _ := s.q.CountDicts(c.Request.Context(), db.CountDictsParams{Name: params.Name, Code: params.Code})
	items := make([]skDictItem, len(rows))
	for i, r := range rows {
		di := skDictItem{
			ID:       r.ID,
			Code:     valueOr(r.Code),
			Name:     valueOr(r.Name),
			Remark:   valueOr(r.Remark),
			UpdateAt: nullTime(r.UpdateAt),
			CreateBy: valueOr(r.CreateBy),
		}
		if r.CreateAt.Valid {
			di.CreateAt = r.CreateAt.Time
		}
		if r.DictType.Valid {
			di.DictType = r.DictType.Int32
		}
		items[i] = di
	}
	skList(c, items, int(total))
}

type skDictMutateReq struct {
	ID       string  `json:"id,omitempty"`
	Code     *string `json:"code,omitempty"`
	Name     *string `json:"name,omitempty"`
	Remark   *string `json:"remark,omitempty"`
	DictType *int32  `json:"dictType,omitempty"`
}

func (s *Server) handleSKDictCreate(c *gin.Context) {
	var req skDictMutateReq
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == nil || *req.Name == "" {
		skErr(c, http.StatusBadRequest, "name is required")
		return
	}
	id := idgen.New()
	p := db.CreateDictParams{ID: id, CreateBy: sql.NullString{String: principalID(c), Valid: true}}
	if req.Code != nil {
		p.Code = sql.NullString{String: *req.Code, Valid: true}
	}
	if req.Name != nil {
		p.Name = sql.NullString{String: *req.Name, Valid: true}
	}
	if req.Remark != nil {
		p.Remark = sql.NullString{String: *req.Remark, Valid: true}
	}
	if req.DictType != nil {
		p.DictType = sql.NullInt32{Int32: *req.DictType, Valid: true}
	}
	if err := s.q.CreateDict(c.Request.Context(), p); err != nil {
		s.logger.Error("sk.dict.create", "err", err)
		skErr(c, http.StatusInternalServerError, "create dict")
		return
	}
	skOK(c, gin.H{"id": id})
}

func (s *Server) handleSKDictUpdate(c *gin.Context) {
	var req skDictMutateReq
	if err := c.ShouldBindJSON(&req); err != nil || req.ID == "" {
		skErr(c, http.StatusBadRequest, "id is required")
		return
	}
	p := db.UpdateDictParams{ID: req.ID, UpdateBy: sql.NullString{String: principalID(c), Valid: true}}
	if req.Code != nil {
		p.Code = sql.NullString{String: *req.Code, Valid: true}
	}
	if req.Name != nil {
		p.Name = sql.NullString{String: *req.Name, Valid: true}
	}
	if req.Remark != nil {
		p.Remark = sql.NullString{String: *req.Remark, Valid: true}
	}
	if req.DictType != nil {
		p.DictType = sql.NullInt32{Int32: *req.DictType, Valid: true}
	}
	if err := s.q.UpdateDict(c.Request.Context(), p); err != nil {
		s.logger.Error("sk.dict.update", "err", err)
		skErr(c, http.StatusInternalServerError, "update dict")
		return
	}
	skOK(c, gin.H{"id": req.ID})
}

// dict + cascade-items must be atomic so a half-applied delete can't
// leave orphan items pointing at a vanished code. SystemService wraps
// both ops in one tx and surfaces the error.
func (s *Server) handleSKDictDelete(c *gin.Context) {
	ids := readIDOrIDs(c)
	if len(ids) == 0 {
		skErr(c, http.StatusBadRequest, "id or ids is required")
		return
	}
	results := make([]gin.H, 0, len(ids))
	for _, id := range ids {
		err := s.system.DeleteDictWithItems(c.Request.Context(), id)
		switch {
		case errors.Is(err, service.ErrNotFound):
			results = append(results, gin.H{"id": id, "ok": false, "reason": "not_found"})
		case err != nil:
			s.logger.Error("sk.dict.delete", "err", err, "id", id)
			results = append(results, gin.H{"id": id, "ok": false, "reason": "error"})
			skList(c, results, len(ids))
			return
		default:
			results = append(results, gin.H{"id": id, "ok": true})
		}
	}
	skOK(c, gin.H{"deleted": countOK(results), "results": results})
}

func (s *Server) handleSKDictItemList(c *gin.Context) {
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
	params := db.ListDictItemsParams{Lim: int32(size), Off: int32((page - 1) * size)}
	if v := c.Query("dictCode"); v != "" {
		params.DictCode = sql.NullString{String: v, Valid: true}
	}
	if v := c.Query("parentItemValue"); v != "" {
		params.ParentItemValue = sql.NullString{String: v, Valid: true}
	}
	if v := c.Query("itemName"); v != "" {
		params.ItemName = sql.NullString{String: v, Valid: true}
	}
	rows, err := s.q.ListDictItems(c.Request.Context(), params)
	if err != nil {
		s.logger.Error("sk.dictItem.list", "err", err)
		skErr(c, http.StatusInternalServerError, "list dict items")
		return
	}
	total, _ := s.q.CountDictItems(c.Request.Context(), db.CountDictItemsParams{
		DictCode: params.DictCode, ParentItemValue: params.ParentItemValue, ItemName: params.ItemName,
	})
	items := make([]skDictItemRow, len(rows))
	for i, r := range rows {
		di := skDictItemRow{
			ID:              r.ID,
			DictCode:        valueOr(r.DictCode),
			ItemName:        valueOr(r.ItemName),
			ItemValue:       r.ItemValue,
			ParentItemValue: valueOr(r.ParentItemValue),
			UpdateAt:        nullTime(r.UpdateAt),
			CreateBy:        valueOr(r.CreateBy),
		}
		if r.CreateAt.Valid {
			di.CreateAt = r.CreateAt.Time
		}
		if r.ItemOrder.Valid {
			di.ItemOrder = r.ItemOrder.Int32
		}
		if r.ItemLevel.Valid {
			di.ItemLevel = r.ItemLevel.Int32
		}
		items[i] = di
	}
	skList(c, items, int(total))
}

type skDictItemMutateReq struct {
	ID              string  `json:"id,omitempty"`
	DictCode        *string `json:"dictCode,omitempty"`
	ItemName        *string `json:"itemName,omitempty"`
	ItemValue       *string `json:"itemValue,omitempty"`
	ItemOrder       *int32  `json:"itemOrder,omitempty"`
	ItemLevel       *int32  `json:"itemLevel,omitempty"`
	ParentItemValue *string `json:"parentItemValue,omitempty"`
}

func (s *Server) handleSKDictItemCreate(c *gin.Context) {
	var req skDictItemMutateReq
	if err := c.ShouldBindJSON(&req); err != nil || req.ItemValue == nil || *req.ItemValue == "" {
		skErr(c, http.StatusBadRequest, "itemValue is required")
		return
	}
	id := idgen.New()
	p := db.CreateDictItemParams{ID: id, ItemValue: *req.ItemValue, CreateBy: sql.NullString{String: principalID(c), Valid: true}}
	if req.DictCode != nil {
		p.DictCode = sql.NullString{String: *req.DictCode, Valid: true}
	}
	if req.ItemName != nil {
		p.ItemName = sql.NullString{String: *req.ItemName, Valid: true}
	}
	if req.ItemOrder != nil {
		p.ItemOrder = sql.NullInt32{Int32: *req.ItemOrder, Valid: true}
	}
	if req.ItemLevel != nil {
		p.ItemLevel = sql.NullInt32{Int32: *req.ItemLevel, Valid: true}
	}
	if req.ParentItemValue != nil {
		p.ParentItemValue = sql.NullString{String: *req.ParentItemValue, Valid: true}
	}
	if err := s.q.CreateDictItem(c.Request.Context(), p); err != nil {
		s.logger.Error("sk.dictItem.create", "err", err)
		skErr(c, http.StatusInternalServerError, "create dict item")
		return
	}
	skOK(c, gin.H{"id": id})
}

func (s *Server) handleSKDictItemUpdate(c *gin.Context) {
	var req skDictItemMutateReq
	if err := c.ShouldBindJSON(&req); err != nil || req.ID == "" {
		skErr(c, http.StatusBadRequest, "id is required")
		return
	}
	p := db.UpdateDictItemParams{ID: req.ID, UpdateBy: sql.NullString{String: principalID(c), Valid: true}}
	if req.DictCode != nil {
		p.DictCode = sql.NullString{String: *req.DictCode, Valid: true}
	}
	if req.ItemName != nil {
		p.ItemName = sql.NullString{String: *req.ItemName, Valid: true}
	}
	if req.ItemValue != nil {
		p.ItemValue = sql.NullString{String: *req.ItemValue, Valid: true}
	}
	if req.ItemOrder != nil {
		p.ItemOrder = sql.NullInt32{Int32: *req.ItemOrder, Valid: true}
	}
	if req.ItemLevel != nil {
		p.ItemLevel = sql.NullInt32{Int32: *req.ItemLevel, Valid: true}
	}
	if req.ParentItemValue != nil {
		p.ParentItemValue = sql.NullString{String: *req.ParentItemValue, Valid: true}
	}
	if err := s.q.UpdateDictItem(c.Request.Context(), p); err != nil {
		s.logger.Error("sk.dictItem.update", "err", err)
		skErr(c, http.StatusInternalServerError, "update dict item")
		return
	}
	skOK(c, gin.H{"id": req.ID})
}

func (s *Server) handleSKDictItemDelete(c *gin.Context) {
	ids := readIDOrIDs(c)
	if len(ids) == 0 {
		skErr(c, http.StatusBadRequest, "id or ids is required")
		return
	}
	for _, id := range ids {
		_ = s.q.DeleteDictItem(c.Request.Context(), id)
	}
	skOK(c, gin.H{"deleted": len(ids)})
}

// ============================================================================
// helpers
// ============================================================================

func readIDOrIDs(c *gin.Context) []string {
	var in skIDOnly
	_ = c.ShouldBindJSON(&in)
	ids := in.IDs
	if in.ID != "" {
		ids = append(ids, in.ID)
	}
	return ids
}

func nullBool(n sql.NullInt16) bool {
	return n.Valid && n.Int16 != 0
}

func nullTime(n sql.NullTime) *time.Time {
	if n.Valid {
		t := n.Time
		return &t
	}
	return nil
}
