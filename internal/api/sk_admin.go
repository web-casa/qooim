// SK-compat admin CRUD (templates, repos, files).
//
// All routes here are mounted under the SK-shape JWT middleware so 401s
// look right to the bundle. Pagination follows the same {current,
// pageSize} convention C1 established for projects.
package api

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/web-casa/qooim/internal/auth"
	"github.com/web-casa/qooim/internal/repo/db"
	"github.com/web-casa/qooim/internal/service"
	"github.com/web-casa/qooim/internal/storage"
)

// ============================================================================
// Templates
// ============================================================================

type skTemplateListItem struct {
	ID           string          `json:"id"`
	RepoID       string          `json:"repoId,omitempty"`
	SerialNo     string          `json:"serialNo,omitempty"`
	Name         string          `json:"name"`
	QuestionType string          `json:"questionType,omitempty"`
	Mode         string          `json:"mode,omitempty"`
	Category     string          `json:"category,omitempty"`
	Tag          string          `json:"tag,omitempty"`
	Priority     int32           `json:"priority"`
	PreviewURL   string          `json:"previewUrl,omitempty"`
	Shared       int16           `json:"shared"`
	Template     json.RawMessage `json:"template,omitempty"`
	CreateAt     time.Time       `json:"createAt"`
	UpdateAt     *time.Time      `json:"updateAt,omitempty"`
	CreateBy     string          `json:"createBy,omitempty"`
}

func skTemplateFromListRow(r db.ListTemplatesRow) skTemplateListItem {
	out := skTemplateListItem{
		ID:           r.ID,
		Name:         valueOr(r.Name),
		RepoID:       valueOr(r.RepoID),
		SerialNo:     valueOr(r.SerialNo),
		QuestionType: valueOr(r.QuestionType),
		Mode:         valueOr(r.Mode),
		Category:     valueOr(r.Category),
		Tag:          valueOr(r.Tag),
		PreviewURL:   valueOr(r.PreviewUrl),
		CreateAt:     r.CreateAt,
		CreateBy:     valueOr(r.CreateBy),
	}
	if r.Priority.Valid {
		out.Priority = r.Priority.Int32
	}
	if r.Shared.Valid {
		out.Shared = r.Shared.Int16
	}
	if r.UpdateAt.Valid {
		t := r.UpdateAt.Time
		out.UpdateAt = &t
	}
	return out
}

func skTemplateFromGet(r db.GetTemplateByIDRow) skTemplateListItem {
	out := skTemplateListItem{
		ID:           r.ID,
		Name:         valueOr(r.Name),
		RepoID:       valueOr(r.RepoID),
		SerialNo:     valueOr(r.SerialNo),
		QuestionType: valueOr(r.QuestionType),
		Mode:         valueOr(r.Mode),
		Category:     valueOr(r.Category),
		Tag:          valueOr(r.Tag),
		PreviewURL:   valueOr(r.PreviewUrl),
		Template:     decodeJSONColumn(valueOr(r.Template)),
		CreateAt:     r.CreateAt,
		CreateBy:     valueOr(r.CreateBy),
	}
	if r.Priority.Valid {
		out.Priority = r.Priority.Int32
	}
	if r.Shared.Valid {
		out.Shared = r.Shared.Int16
	}
	if r.UpdateAt.Valid {
		t := r.UpdateAt.Time
		out.UpdateAt = &t
	}
	return out
}

func (s *Server) handleSKTemplateList(c *gin.Context) {
	page, _ := strconv.Atoi(c.Query("current"))
	pageSize, _ := strconv.Atoi(c.Query("pageSize"))
	res, err := s.listing.Templates(c.Request.Context(),
		service.Page{Page: page, PageSize: pageSize})
	if err != nil {
		s.logger.Error("sk.template.list", "err", err)
		skErr(c, http.StatusInternalServerError, "list templates")
		return
	}
	// Listing service returns the domain DTO; we have to re-fetch the
	// raw row via a separate path to populate the SK shape, but since
	// they're the same fields just renamed, map directly.
	items := make([]skTemplateListItem, len(res.Items))
	for i, dto := range res.Items {
		items[i] = skTemplateListItem{
			ID:           dto.ID,
			RepoID:       dto.RepoID,
			SerialNo:     dto.SerialNo,
			Name:         dto.Name,
			QuestionType: dto.QuestionType,
			Mode:         dto.Mode,
			Category:     dto.Category,
			Tag:          dto.Tag,
			Priority:     dto.Priority,
			PreviewURL:   dto.PreviewURL,
			Shared:       dto.Shared,
			CreateAt:     dto.CreateAt,
			UpdateAt:     dto.UpdateAt,
			CreateBy:     dto.CreateBy,
		}
	}
	skList(c, items, res.Total)
}

func (s *Server) handleSKTemplateGet(c *gin.Context) {
	id := c.Query("id")
	if id == "" {
		// SK also passes id in JSON body for some flows.
		var b struct {
			ID string `json:"id"`
		}
		_ = c.ShouldBindJSON(&b)
		id = b.ID
	}
	if id == "" {
		skErr(c, http.StatusBadRequest, "id is required")
		return
	}
	row, err := s.templates.Get(c.Request.Context(), id)
	if errors.Is(err, service.ErrNotFound) {
		skErr(c, http.StatusNotFound, "template not found")
		return
	}
	if err != nil {
		s.logger.Error("sk.template.get", "err", err)
		skErr(c, http.StatusInternalServerError, "load template")
		return
	}
	skOK(c, skTemplateFromGet(row))
}

type skTemplateMutateReq struct {
	ID           string          `json:"id,omitempty"`
	RepoID       *string         `json:"repoId,omitempty"`
	SerialNo     *string         `json:"serialNo,omitempty"`
	Name         *string         `json:"name,omitempty"`
	QuestionType *string         `json:"questionType,omitempty"`
	Template     json.RawMessage `json:"template,omitempty"`
	Mode         *string         `json:"mode,omitempty"`
	Category     *string         `json:"category,omitempty"`
	Tag          *string         `json:"tag,omitempty"`
	Priority     *int32          `json:"priority,omitempty"`
	PreviewURL   *string         `json:"previewUrl,omitempty"`
	Shared       *int16          `json:"shared,omitempty"`
}

func (r skTemplateMutateReq) toCreate() service.CreateTemplateInput {
	in := service.CreateTemplateInput{}
	in.RepoID = r.RepoID
	in.SerialNo = r.SerialNo
	if r.Name != nil {
		in.Name = *r.Name
	}
	in.QuestionType = r.QuestionType
	if r.Template != nil {
		s := string(r.Template)
		in.Template = &s
	}
	in.Mode = r.Mode
	in.Category = r.Category
	in.Tag = r.Tag
	in.Priority = r.Priority
	in.PreviewURL = r.PreviewURL
	in.Shared = r.Shared
	return in
}

func (r skTemplateMutateReq) toUpdate() service.UpdateTemplateInput {
	in := service.UpdateTemplateInput{
		RepoID:       r.RepoID,
		SerialNo:     r.SerialNo,
		Name:         r.Name,
		QuestionType: r.QuestionType,
		Mode:         r.Mode,
		Category:     r.Category,
		Tag:          r.Tag,
		Priority:     r.Priority,
		PreviewURL:   r.PreviewURL,
		Shared:       r.Shared,
	}
	if r.Template != nil {
		s := string(r.Template)
		in.Template = &s
	}
	return in
}

func (s *Server) handleSKTemplateCreate(c *gin.Context) {
	var req skTemplateMutateReq
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == nil || *req.Name == "" {
		skErr(c, http.StatusBadRequest, "name is required")
		return
	}
	id, err := s.templates.Create(c.Request.Context(), req.toCreate(), principalID(c))
	if err != nil {
		s.logger.Error("sk.template.create", "err", err)
		skErr(c, http.StatusInternalServerError, "create template")
		return
	}
	skOK(c, gin.H{"id": id})
}

func (s *Server) handleSKTemplateUpdate(c *gin.Context) {
	var req skTemplateMutateReq
	if err := c.ShouldBindJSON(&req); err != nil || req.ID == "" {
		skErr(c, http.StatusBadRequest, "id is required")
		return
	}
	if err := s.templates.Update(c.Request.Context(), req.ID, req.toUpdate(), principalID(c)); err != nil {
		if errors.Is(err, service.ErrNotFound) {
			skErr(c, http.StatusNotFound, "template not found")
			return
		}
		s.logger.Error("sk.template.update", "err", err)
		skErr(c, http.StatusInternalServerError, "update template")
		return
	}
	skOK(c, gin.H{"id": req.ID})
}

func (s *Server) handleSKTemplateDelete(c *gin.Context) {
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
		if err := s.templates.SoftDelete(c.Request.Context(), id, principalID(c)); err != nil {
			if errors.Is(err, service.ErrNotFound) {
				results = append(results, gin.H{"id": id, "ok": false, "reason": "not_found"})
				continue
			}
			s.logger.Error("sk.template.delete", "err", err, "id", id)
			results = append(results, gin.H{"id": id, "ok": false, "reason": "error"})
			skList(c, results, len(ids))
			return
		}
		results = append(results, gin.H{"id": id, "ok": true})
	}
	skOK(c, gin.H{"deleted": countOK(results), "results": results})
}

// /api/template/listCategory — distinct, full-table category values.
// Uses a dedicated SQL query so we don't miss values past the first
// page (the earlier scan-200-rows shortcut hid late entries).
func (s *Server) handleSKTemplateListCategory(c *gin.Context) {
	rows, err := s.q.DistinctTemplateCategories(c.Request.Context())
	if err != nil {
		s.logger.Error("sk.template.listCategory", "err", err)
		skErr(c, http.StatusInternalServerError, "list categories")
		return
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		if r.Valid {
			out = append(out, r.String)
		}
	}
	skOK(c, out)
}

// /api/template/listTag — distinct values across the comma-separated
// tag column for every template. We split + dedupe in Go because the
// SQL gymnastics (string_to_array + DISTINCT) buy almost nothing for a
// table that's typically thousands of rows at most.
func (s *Server) handleSKTemplateListTag(c *gin.Context) {
	blobs, err := s.q.DistinctTemplateTagBlobs(c.Request.Context())
	if err != nil {
		s.logger.Error("sk.template.listTag", "err", err)
		skErr(c, http.StatusInternalServerError, "list tags")
		return
	}
	seen := map[string]struct{}{}
	out := make([]string, 0)
	for _, blob := range blobs {
		if !blob.Valid {
			continue
		}
		for _, tag := range strings.Split(blob.String, ",") {
			tag = strings.TrimSpace(tag)
			if tag == "" {
				continue
			}
			if _, ok := seen[tag]; !ok {
				seen[tag] = struct{}{}
				out = append(out, tag)
			}
		}
	}
	skOK(c, out)
}

// ============================================================================
// Repos
// ============================================================================

type skRepoItem struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Category    string          `json:"category,omitempty"`
	Mode        string          `json:"mode,omitempty"`
	Shared      int16           `json:"shared"`
	Tag         string          `json:"tag,omitempty"`
	Priority    int32           `json:"priority"`
	IsPractice  int16           `json:"isPractice"`
	Setting     json.RawMessage `json:"setting,omitempty"`
	CreateAt    time.Time       `json:"createAt"`
	UpdateAt    *time.Time      `json:"updateAt,omitempty"`
	CreateBy    string          `json:"createBy,omitempty"`
}

func skRepoFromGet(r db.GetRepoByIDRow) skRepoItem {
	out := skRepoItem{
		ID:          r.ID,
		Name:        valueOr(r.Name),
		Description: valueOr(r.Description),
		Category:    valueOr(r.Category),
		Mode:        valueOr(r.Mode),
		Tag:         valueOr(r.Tag),
		Setting:     decodeJSONColumn(valueOr(r.Setting)),
		CreateAt:    r.CreateAt,
		CreateBy:    valueOr(r.CreateBy),
	}
	if r.Shared.Valid {
		out.Shared = r.Shared.Int16
	}
	if r.Priority.Valid {
		out.Priority = r.Priority.Int32
	}
	if r.IsPractice.Valid {
		out.IsPractice = r.IsPractice.Int16
	}
	if r.UpdateAt.Valid {
		t := r.UpdateAt.Time
		out.UpdateAt = &t
	}
	return out
}

func (s *Server) handleSKRepoList(c *gin.Context) {
	page, _ := strconv.Atoi(c.Query("current"))
	pageSize, _ := strconv.Atoi(c.Query("pageSize"))
	res, err := s.listing.Repos(c.Request.Context(),
		service.Page{Page: page, PageSize: pageSize})
	if err != nil {
		s.logger.Error("sk.repo.list", "err", err)
		skErr(c, http.StatusInternalServerError, "list repos")
		return
	}
	items := make([]skRepoItem, len(res.Items))
	for i, dto := range res.Items {
		items[i] = skRepoItem{
			ID:          dto.ID,
			Name:        dto.Name,
			Description: dto.Description,
			Category:    dto.Category,
			Mode:        dto.Mode,
			Shared:      dto.Shared,
			Tag:         dto.Tag,
			Priority:    dto.Priority,
			IsPractice:  dto.IsPractice,
			CreateAt:    dto.CreateAt,
			UpdateAt:    dto.UpdateAt,
			CreateBy:    dto.CreateBy,
		}
	}
	skList(c, items, res.Total)
}

func (s *Server) handleSKRepoGet(c *gin.Context) {
	id := c.Query("id")
	if id == "" {
		var b struct {
			ID string `json:"id"`
		}
		_ = c.ShouldBindJSON(&b)
		id = b.ID
	}
	if id == "" {
		skErr(c, http.StatusBadRequest, "id is required")
		return
	}
	row, err := s.repos.Get(c.Request.Context(), id)
	if errors.Is(err, service.ErrNotFound) {
		skErr(c, http.StatusNotFound, "repo not found")
		return
	}
	if err != nil {
		s.logger.Error("sk.repo.get", "err", err)
		skErr(c, http.StatusInternalServerError, "load repo")
		return
	}
	skOK(c, skRepoFromGet(row))
}

type skRepoMutateReq struct {
	ID          string          `json:"id,omitempty"`
	Name        *string         `json:"name,omitempty"`
	Description *string         `json:"description,omitempty"`
	Category    *string         `json:"category,omitempty"`
	Mode        *string         `json:"mode,omitempty"`
	Shared      *int16          `json:"shared,omitempty"`
	Tag         *string         `json:"tag,omitempty"`
	Priority    *int32          `json:"priority,omitempty"`
	Setting     json.RawMessage `json:"setting,omitempty"`
	IsPractice  *int16          `json:"isPractice,omitempty"`
}

func (r skRepoMutateReq) toCreate() service.CreateRepoInput {
	in := service.CreateRepoInput{}
	if r.Name != nil {
		in.Name = *r.Name
	}
	in.Description = r.Description
	in.Category = r.Category
	in.Mode = r.Mode
	in.Shared = r.Shared
	in.Tag = r.Tag
	in.Priority = r.Priority
	if r.Setting != nil {
		s := string(r.Setting)
		in.Setting = &s
	}
	in.IsPractice = r.IsPractice
	return in
}

func (r skRepoMutateReq) toUpdate() service.UpdateRepoInput {
	in := service.UpdateRepoInput{
		Name:        r.Name,
		Description: r.Description,
		Category:    r.Category,
		Mode:        r.Mode,
		Shared:      r.Shared,
		Tag:         r.Tag,
		Priority:    r.Priority,
		IsPractice:  r.IsPractice,
	}
	if r.Setting != nil {
		s := string(r.Setting)
		in.Setting = &s
	}
	return in
}

func (s *Server) handleSKRepoCreate(c *gin.Context) {
	var req skRepoMutateReq
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == nil || *req.Name == "" {
		skErr(c, http.StatusBadRequest, "name is required")
		return
	}
	id, err := s.repos.Create(c.Request.Context(), req.toCreate(), principalID(c))
	if err != nil {
		s.logger.Error("sk.repo.create", "err", err)
		skErr(c, http.StatusInternalServerError, "create repo")
		return
	}
	skOK(c, gin.H{"id": id})
}

func (s *Server) handleSKRepoUpdate(c *gin.Context) {
	var req skRepoMutateReq
	if err := c.ShouldBindJSON(&req); err != nil || req.ID == "" {
		skErr(c, http.StatusBadRequest, "id is required")
		return
	}
	if err := s.repos.Update(c.Request.Context(), req.ID, req.toUpdate(), principalID(c)); err != nil {
		if errors.Is(err, service.ErrNotFound) {
			skErr(c, http.StatusNotFound, "repo not found")
			return
		}
		s.logger.Error("sk.repo.update", "err", err)
		skErr(c, http.StatusInternalServerError, "update repo")
		return
	}
	skOK(c, gin.H{"id": req.ID})
}

func (s *Server) handleSKRepoDelete(c *gin.Context) {
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
		if err := s.repos.Delete(c.Request.Context(), id); err != nil {
			if errors.Is(err, service.ErrNotFound) {
				results = append(results, gin.H{"id": id, "ok": false, "reason": "not_found"})
				continue
			}
			s.logger.Error("sk.repo.delete", "err", err, "id", id)
			results = append(results, gin.H{"id": id, "ok": false, "reason": "error"})
			skList(c, results, len(ids))
			return
		}
		results = append(results, gin.H{"id": id, "ok": true})
	}
	skOK(c, gin.H{"deleted": countOK(results), "results": results})
}

// ============================================================================
// Files
// ============================================================================

type skFileItem struct {
	ID            string     `json:"id"`
	OriginalName  string     `json:"originalName,omitempty"`
	FileName      string     `json:"fileName,omitempty"`
	FilePath      string     `json:"filePath,omitempty"`
	ThumbFilePath string     `json:"thumbFilePath,omitempty"`
	StorageType   int32      `json:"storageType"`
	Shared        int32      `json:"shared"`
	URL           string     `json:"url,omitempty"`
	CreateAt      time.Time  `json:"createAt"`
	UpdateAt      *time.Time `json:"updateAt,omitempty"`
	CreateBy      string     `json:"createBy,omitempty"`
}

func (s *Server) handleSKFileCreate(c *gin.Context) {
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
	res, err := s.files.Upload(c.Request.Context(), service.UploadInput{
		OriginalName: fh.Filename,
		Content:      f,
	}, principalID(c))
	if err != nil {
		s.logger.Error("sk.file.create", "err", err)
		skErr(c, http.StatusInternalServerError, "save file")
		return
	}
	skOK(c, skFileItem{
		ID:           res.ID,
		OriginalName: res.OriginalName,
		FileName:     res.FileName,
		FilePath:     res.FilePath,
		URL:          "/api/file?id=" + res.ID,
	})
}

// handleSKFileGet — GET /api/file?id=… serves the stored object so SK
// <img> / <a download> tags can hit it directly. The handler is mounted
// outside JWT (image previews can't add an Authorization header) so the
// only access control today is "knows the id".
//
// Content-Type is detected from the original filename's extension, with
// a 512-byte sniff fallback so an unknown extension still gets a usable
// type (image/jpeg, application/pdf, etc.) instead of octet-stream.
func (s *Server) handleSKFileGet(c *gin.Context) {
	id := c.Query("id")
	if id == "" {
		skErr(c, http.StatusBadRequest, "id is required")
		return
	}
	s.serveFileByID(c, id)
}

// handleSKPublicPreview — GET /api/public/preview/:id mirrors handleSKFileGet
// but reads the id from the path. The bundle uses this for avatar /
// header-image renders; an optional "@thumbnail" suffix is currently
// ignored — full image is served instead.
func (s *Server) handleSKPublicPreview(c *gin.Context) {
	id := c.Param("id")
	if i := strings.IndexByte(id, '@'); i > 0 {
		id = id[:i]
	}
	if id == "" {
		skErr(c, http.StatusBadRequest, "id is required")
		return
	}
	s.serveFileByID(c, id)
}

// optionalPrincipal pulls the JWT principal off the gin context if a
// valid Bearer token was sent, returning (nil, false) otherwise.
// Used by routes registered outside the JWT middleware that still
// want to honour an optional bearer (e.g. file read).
func (s *Server) optionalPrincipal(c *gin.Context) (*auth.Principal, bool) {
	if p, ok := auth.FromContext(c); ok {
		return p, true
	}
	raw := c.GetHeader("Authorization")
	if raw == "" || s.jwt == nil {
		return nil, false
	}
	parts := strings.SplitN(raw, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return nil, false
	}
	p, err := s.jwt.Parse(parts[1])
	if err != nil {
		return nil, false
	}
	return p, true
}

// canReadFile gates /api/file?id= and /api/public/preview/:id:
//
//	- shared = 1     → public, anyone (incl. anonymous) may read
//	- owner          → the principal who uploaded it
//	- admin role     → can read any file
//	- otherwise      → 403
//
// `principal` is nil for an unauthenticated request.
func (s *Server) canReadFile(row db.GetFileByIDRow, principal *auth.Principal) bool {
	if row.Shared.Valid && row.Shared.Int32 == 1 {
		return true
	}
	if principal == nil {
		return false
	}
	if row.CreateBy.Valid && row.CreateBy.String == principal.UserID {
		return true
	}
	for _, r := range principal.Roles {
		if r == "admin" {
			return true
		}
	}
	return false
}

func (s *Server) serveFileByID(c *gin.Context, id string) {
	// Resolve principal once. The /api/file?id= route is registered
	// without JWT middleware (so <img> tags can hit it without
	// Authorization headers), but if the client DID send a valid
	// Bearer token we still want to honour it for owner / admin reads.
	principal, _ := s.optionalPrincipal(c)

	row, err := s.files.Get(c.Request.Context(), id)
	if errors.Is(err, service.ErrNotFound) {
		skErr(c, http.StatusNotFound, "file not found")
		return
	}
	if err != nil {
		s.logger.Error("sk.file.get.lookup", "err", err)
		skErr(c, http.StatusInternalServerError, "open file")
		return
	}
	if !s.canReadFile(row, principal) {
		skErr(c, http.StatusForbidden, "file is private")
		return
	}

	rc, _, err := s.files.Open(c.Request.Context(), id)
	if errors.Is(err, service.ErrNotFound) || errors.Is(err, storage.ErrNotFound) {
		skErr(c, http.StatusNotFound, "file not found")
		return
	}
	if err != nil {
		s.logger.Error("sk.file.get", "err", err)
		skErr(c, http.StatusInternalServerError, "open file")
		return
	}
	defer rc.Close()

	// Buffer enough to sniff the type before we commit to the response.
	const sniff = 512
	head := make([]byte, sniff)
	n, _ := io.ReadFull(rc, head)
	head = head[:n]

	ctype := ""
	if row.OriginalName.Valid {
		if ext := filenameExt(row.OriginalName.String); ext != "" {
			ctype = mime.TypeByExtension(ext)
		}
	}
	if ctype == "" {
		ctype = http.DetectContentType(head)
	}
	c.Header("Content-Type", ctype)

	if row.OriginalName.Valid && c.Query("download") != "" {
		c.Header("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{
			"filename": row.OriginalName.String,
		}))
	}
	c.Status(http.StatusOK)
	if _, err := c.Writer.Write(head); err != nil {
		return
	}
	if _, err := io.Copy(c.Writer, rc); err != nil {
		s.logger.Warn("sk.file.get.copy", "id", id, "err", err)
	}
}

// filenameExt returns the lowercased extension including the dot, or
// "" if the name doesn't carry one.
func filenameExt(name string) string {
	if i := strings.LastIndex(name, "."); i >= 0 && i < len(name)-1 {
		return strings.ToLower(name[i:])
	}
	return ""
}

// SK doesn't currently have a paged /api/file/list query in our schema
// (no list query in queries/file.sql). Return an empty success so the
// frontend's file panel renders without error; populating it is
// follow-up work.
func (s *Server) handleSKFileList(c *gin.Context) {
	skList(c, []skFileItem{}, 0)
}

func (s *Server) handleSKFileDelete(c *gin.Context) {
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
		if err := s.files.SoftDelete(c.Request.Context(), id, principalID(c)); err != nil {
			if errors.Is(err, service.ErrNotFound) {
				results = append(results, gin.H{"id": id, "ok": false, "reason": "not_found"})
				continue
			}
			s.logger.Error("sk.file.delete", "err", err, "id", id)
			results = append(results, gin.H{"id": id, "ok": false, "reason": "error"})
			skList(c, results, len(ids))
			return
		}
		results = append(results, gin.H{"id": id, "ok": true})
	}
	skOK(c, gin.H{"deleted": countOK(results), "results": results})
}
