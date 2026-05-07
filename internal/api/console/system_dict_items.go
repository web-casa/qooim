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

type dictItemRow struct {
	ID              string
	DictCode        string
	ItemName        string
	ItemValue       string
	ItemOrder       int32
	ParentItemValue string
}

type dictItemForm struct {
	ID              string
	DictCode        string
	ItemName        string
	ItemValue       string
	ItemOrder       int32
	ParentItemValue string
}

// dictItems are scoped to a parent dict — the URL is
// /console/system/dicts/:dictId/items so the active dict is part
// of the page state.
func (s *Server) getDictItems(c *gin.Context) {
	v, ok := s.buildDictItemListView(c, false)
	if !ok {
		c.Redirect(http.StatusFound, "/console/system/dicts")
		return
	}
	s.render(c, "system/dictItems/list.html", v)
}

func (s *Server) getDictItemsTable(c *gin.Context) {
	v, ok := s.buildDictItemListView(c, false)
	if !ok {
		c.String(http.StatusNotFound, "dict not found")
		return
	}
	s.renderPartial(c, "dictItems-table", v)
}

func (s *Server) buildDictItemListView(c *gin.Context, _ bool) (View, bool) {
	dictID := c.Param("id")
	d, err := s.q.GetDictByID(c.Request.Context(), dictID)
	if err != nil {
		return View{}, false
	}
	dr := &dictRow{ID: d.ID}
	if d.Name.Valid {
		dr.Name = d.Name.String
	}
	if d.Code.Valid {
		dr.Code = d.Code.String
	}
	q := strings.TrimSpace(c.Query("q"))
	page, _ := strconv.Atoi(c.Query("page"))
	if page < 1 {
		page = 1
	}
	limit := defaultPageSize
	offset := (page - 1) * limit
	dictCode := sql.NullString{}
	if dr.Code != "" {
		dictCode = sql.NullString{String: dr.Code, Valid: true}
	}
	nameFilter := sql.NullString{}
	if q != "" {
		nameFilter = sql.NullString{String: q, Valid: true}
	}
	rows, err := s.q.ListDictItems(c.Request.Context(), db.ListDictItemsParams{
		DictCode: dictCode,
		ItemName: nameFilter,
		Off:      int32(offset),
		Lim:      int32(limit),
	})
	if err != nil {
		return View{Title: "字典项", Error: "load items failed", CurrentDict: dr}, true
	}
	out := make([]dictItemRow, 0, len(rows))
	for _, r := range rows {
		row := dictItemRow{ID: r.ID, ItemValue: r.ItemValue}
		if r.DictCode.Valid {
			row.DictCode = r.DictCode.String
		}
		if r.ItemName.Valid {
			row.ItemName = r.ItemName.String
		}
		if r.ItemOrder.Valid {
			row.ItemOrder = r.ItemOrder.Int32
		}
		if r.ParentItemValue.Valid {
			row.ParentItemValue = r.ParentItemValue.String
		}
		out = append(out, row)
	}
	total, _ := s.q.CountDictItems(c.Request.Context(), db.CountDictItemsParams{
		DictCode: dictCode,
		ItemName: nameFilter,
	})
	totalPages := int(total) / limit
	if int(total)%limit != 0 {
		totalPages++
	}
	if totalPages == 0 {
		totalPages = 1
	}
	return View{
		Title:        "字典项 / " + dr.Name,
		Active:       "system/dicts",
		Crumb:        "系统设置 / 字典管理 / " + dr.Name,
		Q:            q,
		Page:         page,
		TotalPages:   totalPages,
		Total:        int(total),
		DictItemRows: out,
		CurrentDict:  dr,
	}, true
}

func (s *Server) getDictItemForm(c *gin.Context) {
	dictID := c.Param("id")
	id := c.Param("itemId")
	d, err := s.q.GetDictByID(c.Request.Context(), dictID)
	if err != nil {
		s.renderPartial(c, "dictItem-form", View{Error: "dict not found"})
		return
	}
	v := View{Title: "字典项表单"}
	v.CurrentDict = &dictRow{ID: d.ID}
	if d.Name.Valid {
		v.CurrentDict.Name = d.Name.String
	}
	if d.Code.Valid {
		v.CurrentDict.Code = d.Code.String
	}
	if id != "" {
		// We don't have GetDictItemByID; fetch via list-by-id filter.
		// Simpler: use rawDB for the rare per-edit lookup.
		row, err := s.lookupDictItem(c, id)
		if err != nil {
			v.Error = asError(err)
			s.renderPartial(c, "dictItem-form", v)
			return
		}
		v.DictItemForm = row
	} else {
		v.DictItemForm.DictCode = v.CurrentDict.Code
	}
	s.renderPartial(c, "dictItem-form", v)
}

func (s *Server) postDictItem(c *gin.Context) {
	dictID := c.Param("id")
	f := dictItemFormFromCtx(c)
	if f.ItemValue == "" {
		s.renderDictItemFormError(c, dictID, "", f, errorf("itemValue required"))
		return
	}
	by := principalOf(c).UserID
	p := db.CreateDictItemParams{
		ID:        idgen.New(),
		ItemValue: f.ItemValue,
		CreateBy:  sql.NullString{String: by, Valid: true},
	}
	if f.DictCode != "" {
		p.DictCode = sql.NullString{String: f.DictCode, Valid: true}
	}
	if f.ItemName != "" {
		p.ItemName = sql.NullString{String: f.ItemName, Valid: true}
	}
	if f.ParentItemValue != "" {
		p.ParentItemValue = sql.NullString{String: f.ParentItemValue, Valid: true}
	}
	p.ItemOrder = sql.NullInt32{Int32: f.ItemOrder, Valid: true}
	if err := s.q.CreateDictItem(c.Request.Context(), p); err != nil {
		s.renderDictItemFormError(c, dictID, "", f, err)
		return
	}
	s.dictItemListWithFlash(c, "字典项已创建", flashKindOK)
}

func (s *Server) putDictItem(c *gin.Context) {
	dictID := c.Param("id")
	id := c.Param("itemId")
	f := dictItemFormFromCtx(c)
	by := principalOf(c).UserID
	p := db.UpdateDictItemParams{
		ID:       id,
		UpdateBy: sql.NullString{String: by, Valid: true},
	}
	if f.DictCode != "" {
		p.DictCode = sql.NullString{String: f.DictCode, Valid: true}
	}
	if f.ItemName != "" {
		p.ItemName = sql.NullString{String: f.ItemName, Valid: true}
	}
	if f.ItemValue != "" {
		p.ItemValue = sql.NullString{String: f.ItemValue, Valid: true}
	}
	if f.ParentItemValue != "" {
		p.ParentItemValue = sql.NullString{String: f.ParentItemValue, Valid: true}
	}
	p.ItemOrder = sql.NullInt32{Int32: f.ItemOrder, Valid: true}
	if err := s.q.UpdateDictItem(c.Request.Context(), p); err != nil {
		s.renderDictItemFormError(c, dictID, id, f, err)
		return
	}
	s.dictItemListWithFlash(c, "字典项已更新", flashKindOK)
}

func (s *Server) deleteDictItem(c *gin.Context) {
	id := c.Param("itemId")
	// dict_item rows have no soft-delete column in the schema; the
	// hard delete is what SK uses too.
	if err := s.q.DeleteDictItem(c.Request.Context(), id); err != nil {
		c.String(http.StatusBadRequest, asError(err))
		return
	}
	c.Status(http.StatusOK)
}

func dictItemFormFromCtx(c *gin.Context) dictItemForm {
	f := dictItemForm{
		DictCode:        strings.TrimSpace(c.PostForm("dictCode")),
		ItemName:        strings.TrimSpace(c.PostForm("itemName")),
		ItemValue:       strings.TrimSpace(c.PostForm("itemValue")),
		ParentItemValue: strings.TrimSpace(c.PostForm("parentItemValue")),
	}
	if v := c.PostForm("itemOrder"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			f.ItemOrder = int32(n)
		}
	}
	return f
}

func (s *Server) dictItemListWithFlash(c *gin.Context, msg, kind string) {
	v, ok := s.buildDictItemListView(c, false)
	if !ok {
		c.Status(http.StatusOK)
		return
	}
	v.Flash = &Flash{Kind: kind, Message: msg}
	s.renderPartial(c, "dictItems-refresh", v)
}

func (s *Server) renderDictItemFormError(c *gin.Context, dictID, id string, f dictItemForm, err error) {
	v := View{Title: "字典项表单", Error: asError(err), DictItemForm: f}
	v.DictItemForm.ID = id
	d, derr := s.q.GetDictByID(c.Request.Context(), dictID)
	if derr == nil {
		v.CurrentDict = &dictRow{ID: d.ID}
		if d.Name.Valid {
			v.CurrentDict.Name = d.Name.String
		}
		if d.Code.Valid {
			v.CurrentDict.Code = d.Code.String
		}
	}
	s.renderPartial(c, "dictItem-form", v)
}

// lookupDictItem reaches into rawDB for a single item — there's no
// `GetDictItemByID` query in sqlc yet, and adding one for the edit
// path alone is not worth a migration round.
func (s *Server) lookupDictItem(c *gin.Context, id string) (dictItemForm, error) {
	if s.rawDB == nil {
		return dictItemForm{}, errorf("db unavailable")
	}
	var f dictItemForm
	var dictCode, itemName, parent sql.NullString
	var itemOrder sql.NullInt32
	row := s.rawDB.QueryRowContext(c.Request.Context(),
		`SELECT id, dict_code, item_name, item_value, item_order, parent_item_value
		   FROM t_comm_dict_item
		  WHERE id=$1 AND is_deleted=0`, id)
	if err := row.Scan(&f.ID, &dictCode, &itemName, &f.ItemValue, &itemOrder, &parent); err != nil {
		return dictItemForm{}, err
	}
	if dictCode.Valid {
		f.DictCode = dictCode.String
	}
	if itemName.Valid {
		f.ItemName = itemName.String
	}
	if parent.Valid {
		f.ParentItemValue = parent.String
	}
	if itemOrder.Valid {
		f.ItemOrder = itemOrder.Int32
	}
	return f, nil
}
