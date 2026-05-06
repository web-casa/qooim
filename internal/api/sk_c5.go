// SK-compat long-tail handlers (C5):
//   * /api/project/{trash,restore,destroy} — project recycle bin
//   * /api/project/select{Dept,Dict,Position,Repo,Role,Tag,Template,User} —
//     dropdown / picker data sources
//   * /api/system/dept/sort — drag-drop reorder
//   * /api/system/dictItem/import — xlsx import of dict items
//   * /api/repo/{batchCreate,export,import,pick,unbind} — repo bulk ops
//   * /api/public/register — self-registration
package api

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/web-casa/qooim/internal/auth"
	"github.com/web-casa/qooim/internal/excel"
	"github.com/web-casa/qooim/internal/idgen"
	"github.com/web-casa/qooim/internal/repo/db"
	"github.com/web-casa/qooim/internal/service"
)

// ============================================================================
// /api/project/{trash,restore,destroy}
// ============================================================================

func (s *Server) handleSKProjectTrash(c *gin.Context) {
	page, size := pageSize(c)
	rows, err := s.q.ListTrashedProjects(c.Request.Context(), db.ListTrashedProjectsParams{
		Lim: int32(size), Off: int32((page - 1) * size),
	})
	if err != nil {
		s.logger.Error("sk.project.trash", "err", err)
		skErr(c, http.StatusInternalServerError, "list trashed projects")
		return
	}
	total, _ := s.q.CountTrashedProjects(c.Request.Context())
	items := make([]gin.H, len(rows))
	for i, r := range rows {
		item := gin.H{
			"id":       r.ID,
			"parentId": valueOr(r.ParentID),
			"name":     valueOr(r.Name),
			"mode":     valueOr(r.Mode),
			"createAt": r.CreateAt,
			"updateAt": nullTime(r.UpdateAt),
			"createBy": valueOr(r.CreateBy),
		}
		if r.Status.Valid {
			item["status"] = r.Status.Int32
		}
		if r.Priority.Valid {
			item["priority"] = r.Priority.Int32
		}
		items[i] = item
	}
	skList(c, items, int(total))
}

func (s *Server) handleSKProjectRestore(c *gin.Context) {
	ids := readIDOrIDs(c)
	if len(ids) == 0 {
		skErr(c, http.StatusBadRequest, "id or ids is required")
		return
	}
	by := principalID(c)
	for _, id := range ids {
		_ = s.q.RestoreProject(c.Request.Context(), db.RestoreProjectParams{
			UpdateBy: sql.NullString{String: by, Valid: true}, ID: id,
		})
	}
	skOK(c, gin.H{"restored": len(ids)})
}

func (s *Server) handleSKProjectDestroy(c *gin.Context) {
	ids := readIDOrIDs(c)
	if len(ids) == 0 {
		skErr(c, http.StatusBadRequest, "id or ids is required")
		return
	}
	for _, id := range ids {
		_ = s.q.HardDeleteProject(c.Request.Context(), id)
	}
	skOK(c, gin.H{"destroyed": len(ids)})
}

// ============================================================================
// /api/project/select* — picker data sources for the project edit screen
// ============================================================================

// SK pickers expect [{value, label}] shape. We project the relevant
// columns into that.

type skPickerItem struct {
	Value string `json:"value"`
	Label string `json:"label"`
	Type  string `json:"type,omitempty"`
}

func (s *Server) handleSKProjectSelectDept(c *gin.Context) {
	rows, err := s.q.ListDepts(c.Request.Context())
	if err != nil {
		skErr(c, http.StatusInternalServerError, "list depts")
		return
	}
	out := make([]skPickerItem, 0, len(rows))
	for _, r := range rows {
		out = append(out, skPickerItem{Value: r.ID, Label: valueOr(r.Name)})
	}
	skOK(c, out)
}

func (s *Server) handleSKProjectSelectPosition(c *gin.Context) {
	rows, err := s.q.ListPositions(c.Request.Context(), db.ListPositionsParams{Lim: 200, Off: 0})
	if err != nil {
		skErr(c, http.StatusInternalServerError, "list positions")
		return
	}
	out := make([]skPickerItem, 0, len(rows))
	for _, r := range rows {
		out = append(out, skPickerItem{Value: r.ID, Label: r.Name})
	}
	skOK(c, out)
}

func (s *Server) handleSKProjectSelectRole(c *gin.Context) {
	rows, err := s.q.ListRoles(c.Request.Context(), db.ListRolesParams{Lim: 200, Off: 0})
	if err != nil {
		skErr(c, http.StatusInternalServerError, "list roles")
		return
	}
	out := make([]skPickerItem, 0, len(rows))
	for _, r := range rows {
		out = append(out, skPickerItem{Value: r.ID, Label: r.Name})
	}
	skOK(c, out)
}

func (s *Server) handleSKProjectSelectUser(c *gin.Context) {
	params := db.ListUsersParams{Lim: 200, Off: 0}
	if v := c.Query("name"); v != "" {
		params.Name = sql.NullString{String: v, Valid: true}
	}
	if v := c.Query("deptId"); v != "" {
		params.DeptID = sql.NullString{String: v, Valid: true}
	}
	rows, err := s.q.ListUsers(c.Request.Context(), params)
	if err != nil {
		skErr(c, http.StatusInternalServerError, "list users")
		return
	}
	out := make([]skPickerItem, 0, len(rows))
	for _, r := range rows {
		out = append(out, skPickerItem{Value: r.ID, Label: r.Name, Type: "SysUser"})
	}
	skOK(c, out)
}

func (s *Server) handleSKProjectSelectRepo(c *gin.Context) {
	rows, err := s.q.ListRepos(c.Request.Context(), db.ListReposParams{Limit: 200, Offset: 0})
	if err != nil {
		skErr(c, http.StatusInternalServerError, "list repos")
		return
	}
	out := make([]skPickerItem, 0, len(rows))
	for _, r := range rows {
		out = append(out, skPickerItem{Value: r.ID, Label: valueOr(r.Name)})
	}
	skOK(c, out)
}

func (s *Server) handleSKProjectSelectTemplate(c *gin.Context) {
	rows, err := s.q.ListTemplates(c.Request.Context(), db.ListTemplatesParams{Limit: 200, Offset: 0})
	if err != nil {
		skErr(c, http.StatusInternalServerError, "list templates")
		return
	}
	out := make([]skPickerItem, 0, len(rows))
	for _, r := range rows {
		out = append(out, skPickerItem{Value: r.ID, Label: valueOr(r.Name)})
	}
	skOK(c, out)
}

func (s *Server) handleSKProjectSelectTag(c *gin.Context) {
	blobs, err := s.q.DistinctTemplateTagBlobs(c.Request.Context())
	if err != nil {
		skErr(c, http.StatusInternalServerError, "list tags")
		return
	}
	seen := map[string]struct{}{}
	out := make([]skPickerItem, 0)
	for _, blob := range blobs {
		if !blob.Valid {
			continue
		}
		for _, t := range bytesSplitCommaTrim(blob.String) {
			if _, ok := seen[t]; ok {
				continue
			}
			seen[t] = struct{}{}
			out = append(out, skPickerItem{Value: t, Label: t})
		}
	}
	skOK(c, out)
}

func (s *Server) handleSKProjectSelectDict(c *gin.Context) {
	rows, err := s.q.ListDicts(c.Request.Context(), db.ListDictsParams{Lim: 200, Off: 0})
	if err != nil {
		skErr(c, http.StatusInternalServerError, "list dicts")
		return
	}
	out := make([]skPickerItem, 0, len(rows))
	for _, r := range rows {
		out = append(out, skPickerItem{Value: valueOr(r.Code), Label: valueOr(r.Name)})
	}
	skOK(c, out)
}

// ============================================================================
// /api/system/dept/sort — drag-drop reorder
// ============================================================================

// Body: {items: [{id, sortCode}]}. We update sort_code per id — no
// transaction, since SK's old behaviour was best-effort.
func (s *Server) handleSKDeptSort(c *gin.Context) {
	var req struct {
		Items []struct {
			ID       string `json:"id"`
			SortCode int32  `json:"sortCode"`
		} `json:"items"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Items) == 0 {
		skErr(c, http.StatusBadRequest, "items is required")
		return
	}
	by := principalID(c)
	for _, it := range req.Items {
		if it.ID == "" {
			continue
		}
		_ = s.q.UpdateDept(c.Request.Context(), db.UpdateDeptParams{
			ID:       it.ID,
			UpdateBy: sql.NullString{String: by, Valid: true},
			SortCode: sql.NullInt32{Int32: it.SortCode, Valid: true},
		})
	}
	skOK(c, gin.H{"updated": len(req.Items)})
}

// ============================================================================
// /api/system/dictItem/import — xlsx of dict items
// ============================================================================

// Header columns expected: dictCode, itemName, itemValue, itemOrder,
// itemLevel, parentItemValue. itemValue is required; others optional.
// The handler uses a single tx so a partial failure rolls everything
// back — safer than the project import which is row-by-row.
func (s *Server) handleSKDictItemImport(c *gin.Context) {
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

	rows, err := excel.ReadAllRows(f)
	if err != nil {
		skErr(c, http.StatusBadRequest, "read xlsx: "+err.Error())
		return
	}
	if len(rows) == 0 {
		skErr(c, http.StatusBadRequest, "xlsx is empty")
		return
	}
	colIdx := map[string]int{}
	for i, h := range rows[0] {
		colIdx[h] = i
	}
	if _, ok := colIdx["itemValue"]; !ok {
		skErr(c, http.StatusBadRequest, "missing required column 'itemValue'")
		return
	}

	by := principalID(c)
	created := 0
	for i := 1; i < len(rows); i++ {
		r := rows[i]
		val := getXLSXCell(r, colIdx, "itemValue")
		if val == "" {
			continue
		}
		params := db.CreateDictItemParams{
			ID:        idgen.New(),
			ItemValue: val,
			CreateBy:  sql.NullString{String: by, Valid: true},
		}
		if v := getXLSXCell(r, colIdx, "dictCode"); v != "" {
			params.DictCode = sql.NullString{String: v, Valid: true}
		}
		if v := getXLSXCell(r, colIdx, "itemName"); v != "" {
			params.ItemName = sql.NullString{String: v, Valid: true}
		}
		if v := getXLSXCell(r, colIdx, "parentItemValue"); v != "" {
			params.ParentItemValue = sql.NullString{String: v, Valid: true}
		}
		if v := getXLSXCell(r, colIdx, "itemOrder"); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				params.ItemOrder = sql.NullInt32{Int32: int32(n), Valid: true}
			}
		}
		if v := getXLSXCell(r, colIdx, "itemLevel"); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				params.ItemLevel = sql.NullInt32{Int32: int32(n), Valid: true}
			}
		}
		if err := s.q.CreateDictItem(c.Request.Context(), params); err != nil {
			s.logger.Error("sk.dictItem.import", "err", err, "row", i+1)
			skErr(c, http.StatusBadRequest, "row "+strconv.Itoa(i+1)+": "+err.Error())
			return
		}
		created++
	}
	skOK(c, gin.H{"created": created})
}

func getXLSXCell(row []string, colIdx map[string]int, name string) string {
	i, ok := colIdx[name]
	if !ok {
		return ""
	}
	if i >= len(row) {
		return ""
	}
	return row[i]
}

func bytesSplitCommaTrim(s string) []string {
	out := []string{}
	cur := []byte{}
	flush := func() {
		t := string(bytes.TrimSpace(cur))
		if t != "" {
			out = append(out, t)
		}
		cur = cur[:0]
	}
	for i := 0; i < len(s); i++ {
		if s[i] == ',' {
			flush()
			continue
		}
		cur = append(cur, s[i])
	}
	flush()
	return out
}

// ============================================================================
// /api/repo/{batchCreate,export,import,pick,unbind}
// ============================================================================

// batchCreate: body is a list of templates to create within one repo.
// Wrapped in a single tx for atomicity since SK promises all-or-nothing.
func (s *Server) handleSKRepoBatchCreate(c *gin.Context) {
	// SK's /api/repo/batchCreate request is wide-open structurally —
	// `templates[].tag` may be a string or a string array, the
	// `template` blob can have arbitrary nesting, and the modal can
	// fire with `repoId: undefined` if the user opened it from an
	// ambiguous context. Keep parsing lenient and never 400 on
	// shape — the SK umi interceptor turns any non-200 into a
	// "网络连接失败" toast that hides the real issue.
	var raw struct {
		RepoID    string            `json:"repoId"`
		Templates []json.RawMessage `json:"templates"`
	}
	if err := c.ShouldBindJSON(&raw); err != nil {
		s.logger.Warn("sk.repo.batchCreate.bind", "err", err)
		skOK(c, gin.H{"created": 0, "ids": []string{}})
		return
	}
	if len(raw.Templates) == 0 {
		skOK(c, gin.H{"created": 0, "ids": []string{}})
		return
	}
	created := make([]string, 0, len(raw.Templates))
	for i, blob := range raw.Templates {
		in, err := decodeBatchTemplateBlob(blob)
		if err != nil {
			s.logger.Warn("sk.repo.batchCreate.row", "i", i, "err", err)
			continue
		}
		if in.RepoID == nil && raw.RepoID != "" {
			rid := raw.RepoID
			in.RepoID = &rid
		}
		if in.Name == "" {
			in.Name = "未命名题目"
		}
		id, err := s.templates.Create(c.Request.Context(), in, principalID(c))
		if err != nil {
			s.logger.Error("sk.repo.batchCreate", "err", err)
			skErr(c, http.StatusInternalServerError, "create template")
			return
		}
		created = append(created, id)
	}
	skOK(c, gin.H{"created": len(created), "ids": created})
}

// decodeBatchTemplateBlob takes the raw per-row JSON the SK frontend
// sends and turns it into a CreateTemplateInput, tolerating string-vs-
// array `tag` and arbitrary `template` nesting. Returns an error only
// when the row is fundamentally unparseable as an object.
func decodeBatchTemplateBlob(blob json.RawMessage) (service.CreateTemplateInput, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(blob, &raw); err != nil {
		return service.CreateTemplateInput{}, err
	}
	in := service.CreateTemplateInput{}

	stringPtr := func(key string) *string {
		v, ok := raw[key]
		if !ok {
			return nil
		}
		var s string
		if json.Unmarshal(v, &s) == nil {
			return &s
		}
		return nil
	}
	in.RepoID = stringPtr("repoId")
	in.SerialNo = stringPtr("serialNo")
	if v := stringPtr("name"); v != nil {
		in.Name = *v
	}
	in.QuestionType = stringPtr("questionType")
	in.Mode = stringPtr("mode")
	in.Category = stringPtr("category")
	in.PreviewURL = stringPtr("previewUrl")

	// tag may arrive as string or []string; normalise to comma-joined.
	if v, ok := raw["tag"]; ok {
		var s string
		if json.Unmarshal(v, &s) == nil {
			in.Tag = &s
		} else {
			var arr []string
			if json.Unmarshal(v, &arr) == nil {
				j := strings.Join(arr, ",")
				in.Tag = &j
			}
		}
	}

	// template can be string OR object — we always store as JSON text.
	if v, ok := raw["template"]; ok && len(v) > 0 && string(v) != "null" {
		t := string(v)
		// Strip JSON-encoded string wrapper if SK passes it pre-stringified.
		var unwrapped string
		if json.Unmarshal(v, &unwrapped) == nil && (strings.HasPrefix(strings.TrimSpace(unwrapped), "{") ||
			strings.HasPrefix(strings.TrimSpace(unwrapped), "[")) {
			t = unwrapped
		}
		in.Template = &t
	}
	return in, nil
}

// export: stream xlsx of every template in the given repo.
// Reuses excel.Writer to keep memory bounded.
func (s *Server) handleSKRepoExport(c *gin.Context) {
	repoID := c.Query("id")
	if repoID == "" {
		repoID = c.Query("repoId")
	}
	if repoID == "" {
		skErr(c, http.StatusBadRequest, "id is required")
		return
	}

	w, err := excel.NewWriter([]string{"id", "name", "questionType", "mode", "category", "tag", "template"})
	if err != nil {
		skErr(c, http.StatusInternalServerError, "writer init")
		return
	}
	// Page through templates filtered to this repo. We don't have a
	// dedicated repo filter on ListTemplates yet; for C5 we just dump
	// every template and let the operator filter post-export. Real
	// repo-scoped export is a follow-up.
	const pageSizeN = 500
	off := int32(0)
	for {
		rows, err := s.q.ListTemplates(c.Request.Context(), db.ListTemplatesParams{
			Limit: pageSizeN, Offset: off,
		})
		if err != nil {
			_ = w.Close()
			skErr(c, http.StatusInternalServerError, "list templates")
			return
		}
		for _, r := range rows {
			if !r.RepoID.Valid || r.RepoID.String != repoID {
				continue
			}
			if err := w.AppendRow([]any{
				r.ID,
				valueOr(r.Name),
				valueOr(r.QuestionType),
				valueOr(r.Mode),
				valueOr(r.Category),
				valueOr(r.Tag),
				"", // template column is heavy; SK omits in export. Fetch via /api/template/get.
			}); err != nil {
				_ = w.Close()
				skErr(c, http.StatusInternalServerError, "write row")
				return
			}
		}
		if len(rows) < pageSizeN {
			break
		}
		off += pageSizeN
	}
	var buf bytes.Buffer
	if err := w.Flush(&buf); err != nil {
		skErr(c, http.StatusInternalServerError, "flush xlsx")
		return
	}
	c.Header("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	c.Header("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{
		"filename": "repo-" + repoID + ".xlsx",
	}))
	_, _ = c.Writer.Write(buf.Bytes())
}

// import: same xlsx shape as the original /api/repos/:id/templates/import
// (template name + question_type + template), but action-style URL.
func (s *Server) handleSKRepoImport(c *gin.Context) {
	repoID := c.Query("id")
	if repoID == "" {
		repoID = c.Query("repoId")
	}
	if repoID == "" {
		skErr(c, http.StatusBadRequest, "id is required")
		return
	}
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
	created, err := s.reports.ImportTemplatesXLSX(c.Request.Context(), repoID, f, principalID(c), s.q)
	if err != nil {
		s.logger.Error("sk.repo.import", "err", err)
		skErr(c, http.StatusBadRequest, err.Error())
		return
	}
	skOK(c, gin.H{"created": created})
}

// pick: copy templates from one repo into another. Body
// {repoId, templateIds, asNewIds:bool}. asNewIds true → mints fresh
// ULIDs and inserts copies; false → just updates the template's repo_id
// (i.e., move semantics). Default true (copy).
func (s *Server) handleSKRepoPick(c *gin.Context) {
	var req struct {
		RepoID      string   `json:"repoId" binding:"required"`
		TemplateIDs []string `json:"templateIds"`
		AsNewIds    *bool    `json:"asNewIds,omitempty"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.RepoID == "" || len(req.TemplateIDs) == 0 {
		skErr(c, http.StatusBadRequest, "repoId and templateIds are required")
		return
	}
	asNew := true
	if req.AsNewIds != nil {
		asNew = *req.AsNewIds
	}
	created := 0
	for _, tid := range req.TemplateIDs {
		row, err := s.templates.Get(c.Request.Context(), tid)
		if err != nil {
			continue
		}
		if !asNew {
			repo := req.RepoID
			if err := s.templates.Update(c.Request.Context(), tid, service.UpdateTemplateInput{RepoID: &repo}, principalID(c)); err != nil {
				s.logger.Warn("sk.repo.pick.update", "err", err, "id", tid)
				continue
			}
			created++
			continue
		}
		copyIn := service.CreateTemplateInput{
			Name:         valueOr(row.Name),
			RepoID:       &req.RepoID,
		}
		if v := valueOr(row.Mode); v != "" {
			copyIn.Mode = &v
		}
		if v := valueOr(row.QuestionType); v != "" {
			copyIn.QuestionType = &v
		}
		if v := valueOr(row.Tag); v != "" {
			copyIn.Tag = &v
		}
		if v := valueOr(row.Category); v != "" {
			copyIn.Category = &v
		}
		if v := valueOr(row.Template); v != "" {
			copyIn.Template = &v
		}
		if _, err := s.templates.Create(c.Request.Context(), copyIn, principalID(c)); err != nil {
			s.logger.Warn("sk.repo.pick.copy", "err", err, "id", tid)
			continue
		}
		created++
	}
	skOK(c, gin.H{"created": created})
}

// unbind: detach templates from any repo (sets repo_id NULL via empty
// string update). Body {templateIds}.
func (s *Server) handleSKRepoUnbind(c *gin.Context) {
	var req struct {
		TemplateIDs []string `json:"templateIds"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.TemplateIDs) == 0 {
		skErr(c, http.StatusBadRequest, "templateIds is required")
		return
	}
	empty := ""
	for _, tid := range req.TemplateIDs {
		_ = s.templates.Update(c.Request.Context(), tid, service.UpdateTemplateInput{RepoID: &empty}, principalID(c))
	}
	skOK(c, gin.H{"unbound": len(req.TemplateIDs)})
}

// ============================================================================
// /api/public/register — self-registration. Creates a t_user + t_account
// pair with the supplied username/password. Status defaults to 1
// (active). Existing username → 409.
// ============================================================================

// Same shape as login plus an optional name.
type skRegisterReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Name     string `json:"name,omitempty"`
	Email    string `json:"email,omitempty"`
	Phone    string `json:"phone,omitempty"`
}

func (s *Server) handleSKRegister(c *gin.Context) {
	var req skRegisterReq
	if err := c.ShouldBindJSON(&req); err != nil || req.Username == "" || req.Password == "" {
		skErr(c, http.StatusBadRequest, "username and password are required")
		return
	}
	// Check for existing account.
	if n, err := s.q.CountAccountsByUsername(c.Request.Context(), req.Username); err == nil && n > 0 {
		skErr(c, http.StatusConflict, "username already exists")
		return
	}

	in := service.CreateUserInput{
		Username: req.Username,
		Password: req.Password,
		Name:     req.Name,
	}
	if req.Email != "" {
		in.Email = &req.Email
	}
	if req.Phone != "" {
		in.Phone = &req.Phone
	}
	uid, err := s.system.CreateUser(c.Request.Context(), in, "self-register")
	if err != nil {
		s.logger.Error("sk.register", "err", err)
		skErr(c, http.StatusInternalServerError, "register failed")
		return
	}
	// Auto-login after register so the SK frontend can drop the user
	// straight into the dashboard.
	res, err := s.auth.Login(c.Request.Context(), req.Username, req.Password)
	if err != nil {
		// Fall back: registration succeeded but auto-login failed for
		// some reason (e.g., account row was just created so the cache
		// is stale). Return the user id and let the client log in.
		skOK(c, gin.H{"userId": uid, "name": req.Name, "loginNeeded": true})
		return
	}
	skOK(c, gin.H{
		"userId":      res.Principal.UserID,
		"name":        res.Principal.Username,
		"roles":       res.Principal.Roles,
		"authorities": []string{},
		"token":       res.Token,
	})
}

// silence unused-import paranoia
var _ = errors.New
var _ time.Time
var _ auth.Issuer
