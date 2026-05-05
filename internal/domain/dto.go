// Package domain holds JSON-friendly DTOs that flatten sqlc's
// sql.Null* wrappers so HTTP clients see plain strings/ints/timestamps.
//
// We chose to keep sqlc-generated types (with their Null wrappers) as-is
// at the persistence layer — they tell the truth about the schema. The
// service / API layer maps them through these DTOs.
package domain

import (
	"database/sql"
	"time"

	"github.com/web-casa/qooim/internal/repo/db"
)

func nsString(n sql.NullString) string {
	if n.Valid {
		return n.String
	}
	return ""
}

func nsInt32(n sql.NullInt32) int32 {
	if n.Valid {
		return n.Int32
	}
	return 0
}

func nsInt16(n sql.NullInt16) int16 {
	if n.Valid {
		return n.Int16
	}
	return 0
}

func nsTime(n sql.NullTime) *time.Time {
	if n.Valid {
		t := n.Time
		return &t
	}
	return nil
}

// Project ----------------------------------------------------------------

type ProjectDTO struct {
	ID       string     `json:"id"`
	ParentID string     `json:"parent_id,omitempty"`
	Name     string     `json:"name"`
	Survey   string     `json:"survey,omitempty"`
	Setting  string     `json:"setting,omitempty"`
	Status   int32      `json:"status"`
	Mode     string     `json:"mode,omitempty"`
	Priority int32      `json:"priority"`
	CreateAt time.Time  `json:"create_at"`
	CreateBy string     `json:"create_by,omitempty"`
	UpdateAt *time.Time `json:"update_at,omitempty"`
	UpdateBy string     `json:"update_by,omitempty"`
}

type ProjectListItem struct {
	ID       string     `json:"id"`
	ParentID string     `json:"parent_id,omitempty"`
	Name     string     `json:"name"`
	Status   int32      `json:"status"`
	Mode     string     `json:"mode,omitempty"`
	Priority int32      `json:"priority"`
	CreateAt time.Time  `json:"create_at"`
	UpdateAt *time.Time `json:"update_at,omitempty"`
	CreateBy string     `json:"create_by,omitempty"`
}

func ProjectFromGet(r db.GetProjectByIDRow) ProjectDTO {
	return ProjectDTO{
		ID:       r.ID,
		ParentID: nsString(r.ParentID),
		Name:     nsString(r.Name),
		Survey:   nsString(r.Survey),
		Setting:  nsString(r.Setting),
		Status:   nsInt32(r.Status),
		Mode:     nsString(r.Mode),
		Priority: nsInt32(r.Priority),
		CreateAt: r.CreateAt,
		CreateBy: nsString(r.CreateBy),
		UpdateAt: nsTime(r.UpdateAt),
		UpdateBy: nsString(r.UpdateBy),
	}
}

func ProjectFromListRow(r db.ListProjectsRow) ProjectListItem {
	return ProjectListItem{
		ID:       r.ID,
		ParentID: nsString(r.ParentID),
		Name:     nsString(r.Name),
		Status:   nsInt32(r.Status),
		Mode:     nsString(r.Mode),
		Priority: nsInt32(r.Priority),
		CreateAt: r.CreateAt,
		UpdateAt: nsTime(r.UpdateAt),
		CreateBy: nsString(r.CreateBy),
	}
}

func ProjectsFromListRows(rs []db.ListProjectsRow) []ProjectListItem {
	out := make([]ProjectListItem, len(rs))
	for i, r := range rs {
		out[i] = ProjectFromListRow(r)
	}
	return out
}

// Repo -------------------------------------------------------------------

type RepoDTO struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description,omitempty"`
	Category    string     `json:"category,omitempty"`
	Mode        string     `json:"mode,omitempty"`
	Shared      int16      `json:"shared"`
	Tag         string     `json:"tag,omitempty"`
	Priority    int32      `json:"priority"`
	Setting     string     `json:"setting,omitempty"`
	IsPractice  int16      `json:"is_practice"`
	CreateAt    time.Time  `json:"create_at"`
	CreateBy    string     `json:"create_by,omitempty"`
	UpdateAt    *time.Time `json:"update_at,omitempty"`
	UpdateBy    string     `json:"update_by,omitempty"`
}

type RepoListItem struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description,omitempty"`
	Category    string     `json:"category,omitempty"`
	Mode        string     `json:"mode,omitempty"`
	Shared      int16      `json:"shared"`
	Tag         string     `json:"tag,omitempty"`
	Priority    int32      `json:"priority"`
	IsPractice  int16      `json:"is_practice"`
	CreateAt    time.Time  `json:"create_at"`
	UpdateAt    *time.Time `json:"update_at,omitempty"`
	CreateBy    string     `json:"create_by,omitempty"`
}

func RepoFromGet(r db.GetRepoByIDRow) RepoDTO {
	return RepoDTO{
		ID:          r.ID,
		Name:        nsString(r.Name),
		Description: nsString(r.Description),
		Category:    nsString(r.Category),
		Mode:        nsString(r.Mode),
		Shared:      nsInt16(r.Shared),
		Tag:         nsString(r.Tag),
		Priority:    nsInt32(r.Priority),
		Setting:     nsString(r.Setting),
		IsPractice:  nsInt16(r.IsPractice),
		CreateAt:    r.CreateAt,
		CreateBy:    nsString(r.CreateBy),
		UpdateAt:    nsTime(r.UpdateAt),
		UpdateBy:    nsString(r.UpdateBy),
	}
}

func RepoFromListRow(r db.ListReposRow) RepoListItem {
	return RepoListItem{
		ID:          r.ID,
		Name:        nsString(r.Name),
		Description: nsString(r.Description),
		Category:    nsString(r.Category),
		Mode:        nsString(r.Mode),
		Shared:      nsInt16(r.Shared),
		Tag:         nsString(r.Tag),
		Priority:    nsInt32(r.Priority),
		IsPractice:  nsInt16(r.IsPractice),
		CreateAt:    r.CreateAt,
		UpdateAt:    nsTime(r.UpdateAt),
		CreateBy:    nsString(r.CreateBy),
	}
}

func ReposFromListRows(rs []db.ListReposRow) []RepoListItem {
	out := make([]RepoListItem, len(rs))
	for i, r := range rs {
		out[i] = RepoFromListRow(r)
	}
	return out
}

// Template ---------------------------------------------------------------

type TemplateDTO struct {
	ID           string     `json:"id"`
	RepoID       string     `json:"repo_id,omitempty"`
	SerialNo     string     `json:"serial_no,omitempty"`
	Name         string     `json:"name"`
	QuestionType string     `json:"question_type,omitempty"`
	Template     string     `json:"template,omitempty"`
	Mode         string     `json:"mode,omitempty"`
	Category     string     `json:"category,omitempty"`
	Tag          string     `json:"tag,omitempty"`
	Priority     int32      `json:"priority"`
	PreviewURL   string     `json:"preview_url,omitempty"`
	Shared       int16      `json:"shared"`
	CreateAt     time.Time  `json:"create_at"`
	CreateBy     string     `json:"create_by,omitempty"`
	UpdateAt     *time.Time `json:"update_at,omitempty"`
	UpdateBy     string     `json:"update_by,omitempty"`
}

type TemplateListItem struct {
	ID           string     `json:"id"`
	RepoID       string     `json:"repo_id,omitempty"`
	SerialNo     string     `json:"serial_no,omitempty"`
	Name         string     `json:"name"`
	QuestionType string     `json:"question_type,omitempty"`
	Mode         string     `json:"mode,omitempty"`
	Category     string     `json:"category,omitempty"`
	Tag          string     `json:"tag,omitempty"`
	Priority     int32      `json:"priority"`
	PreviewURL   string     `json:"preview_url,omitempty"`
	Shared       int16      `json:"shared"`
	CreateAt     time.Time  `json:"create_at"`
	UpdateAt     *time.Time `json:"update_at,omitempty"`
	CreateBy     string     `json:"create_by,omitempty"`
}

func TemplateFromGet(r db.GetTemplateByIDRow) TemplateDTO {
	return TemplateDTO{
		ID:           r.ID,
		RepoID:       nsString(r.RepoID),
		SerialNo:     nsString(r.SerialNo),
		Name:         nsString(r.Name),
		QuestionType: nsString(r.QuestionType),
		Template:     nsString(r.Template),
		Mode:         nsString(r.Mode),
		Category:     nsString(r.Category),
		Tag:          nsString(r.Tag),
		Priority:     nsInt32(r.Priority),
		PreviewURL:   nsString(r.PreviewUrl),
		Shared:       nsInt16(r.Shared),
		CreateAt:     r.CreateAt,
		CreateBy:     nsString(r.CreateBy),
		UpdateAt:     nsTime(r.UpdateAt),
		UpdateBy:     nsString(r.UpdateBy),
	}
}

func TemplateFromListRow(r db.ListTemplatesRow) TemplateListItem {
	return TemplateListItem{
		ID:           r.ID,
		RepoID:       nsString(r.RepoID),
		SerialNo:     nsString(r.SerialNo),
		Name:         nsString(r.Name),
		QuestionType: nsString(r.QuestionType),
		Mode:         nsString(r.Mode),
		Category:     nsString(r.Category),
		Tag:          nsString(r.Tag),
		Priority:     nsInt32(r.Priority),
		PreviewURL:   nsString(r.PreviewUrl),
		Shared:       nsInt16(r.Shared),
		CreateAt:     r.CreateAt,
		UpdateAt:     nsTime(r.UpdateAt),
		CreateBy:     nsString(r.CreateBy),
	}
}

func TemplatesFromListRows(rs []db.ListTemplatesRow) []TemplateListItem {
	out := make([]TemplateListItem, len(rs))
	for i, r := range rs {
		out[i] = TemplateFromListRow(r)
	}
	return out
}

// Dashboard --------------------------------------------------------------

type DashboardListItem struct {
	ID        string     `json:"id"`
	Key       string     `json:"key"`
	Type      int32      `json:"type"`
	ProjectID string     `json:"project_id,omitempty"`
	Setting   string     `json:"setting,omitempty"`
	CreateAt  time.Time  `json:"create_at"`
	UpdateAt  *time.Time `json:"update_at,omitempty"`
	CreateBy  string     `json:"create_by,omitempty"`
}

func DashboardFromListRow(r db.ListDashboardsRow) DashboardListItem {
	return DashboardListItem{
		ID:        r.ID,
		Key:       r.Key,
		Type:      nsInt32(r.Type),
		ProjectID: nsString(r.ProjectID),
		Setting:   nsString(r.Setting),
		CreateAt:  r.CreateAt,
		UpdateAt:  nsTime(r.UpdateAt),
		CreateBy:  nsString(r.CreateBy),
	}
}

func DashboardsFromListRows(rs []db.ListDashboardsRow) []DashboardListItem {
	out := make([]DashboardListItem, len(rs))
	for i, r := range rs {
		out[i] = DashboardFromListRow(r)
	}
	return out
}

// File -------------------------------------------------------------------

type FileDTO struct {
	ID            string     `json:"id"`
	OriginalName  string     `json:"original_name,omitempty"`
	FileName      string     `json:"file_name,omitempty"`
	FilePath      string     `json:"file_path,omitempty"`
	ThumbFilePath string     `json:"thumb_file_path,omitempty"`
	StorageType   int32      `json:"storage_type"`
	Shared        int32      `json:"shared"`
	CreateAt      time.Time  `json:"create_at"`
	CreateBy      string     `json:"create_by,omitempty"`
	UpdateAt      *time.Time `json:"update_at,omitempty"`
	UpdateBy      string     `json:"update_by,omitempty"`
}

func FileFromGet(r db.GetFileByIDRow) FileDTO {
	return FileDTO{
		ID:            r.ID,
		OriginalName:  nsString(r.OriginalName),
		FileName:      nsString(r.FileName),
		FilePath:      nsString(r.FilePath),
		ThumbFilePath: nsString(r.ThumbFilePath),
		StorageType:   nsInt32(r.StorageType),
		Shared:        nsInt32(r.Shared),
		CreateAt:      r.CreateAt,
		CreateBy:      nsString(r.CreateBy),
		UpdateAt:      nsTime(r.UpdateAt),
		UpdateBy:      nsString(r.UpdateBy),
	}
}
