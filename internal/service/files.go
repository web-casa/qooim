package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/web-casa/qooim/internal/idgen"
	"github.com/web-casa/qooim/internal/repo/db"
	"github.com/web-casa/qooim/internal/storage"
)

type FileService struct {
	q  db.Querier
	st storage.Storage
}

func NewFileService(q db.Querier, st storage.Storage) *FileService {
	return &FileService{q: q, st: st}
}

// dangerousExtensions are file extensions we hard-refuse to store. The
// list is intentionally short and focused on executables / server-side
// scripts because the same files get served back at /api/file?id=…;
// we don't want Qoo.IM to be a free static host for malware.
//
// HTML-shaped types (.html/.htm/.svg/.xml/.xsl/.xslt) are blocked too
// because the same /api/file route serves on the qooim.* origin, and
// any uploaded HTML/SVG would render same-origin and get cookie scope
// — that's a classic stored-XSS path. Polyglot files that happen to
// hit a benign extension are still possible (.pdf with embedded JS,
// .docx with macros) and rely on the user's reader settings.
var dangerousExtensions = map[string]bool{
	".exe": true, ".bat": true, ".cmd": true, ".com": true, ".msi": true,
	".sh": true, ".bash": true, ".zsh": true,
	".ps1": true, ".psm1": true,
	".php": true, ".phtml": true, ".php3": true, ".php4": true, ".php5": true, ".php7": true,
	".asp": true, ".aspx": true, ".jsp": true, ".jspx": true,
	".pl": true, ".cgi": true,
	".scr": true, ".vbs": true, ".wsf": true,
	".dll": true, ".so": true, ".dylib": true,
	".jar": true, ".war": true,
	".elf": true, ".app": true,
	// HTML/SVG/XML — same-origin XSS vectors when served from /api/file.
	".html": true, ".htm": true, ".xhtml": true,
	".svg": true, ".svgz": true,
	".xml": true, ".xsl": true, ".xslt": true,
	".swf": true, ".class": true,
}

// ErrDangerousFileType is returned when a caller tries to upload a
// file whose extension or sniffed content type is on the deny list.
var ErrDangerousFileType = errors.New("dangerous file type")

type UploadInput struct {
	OriginalName string
	Content      io.Reader
	Shared       *int32
}

type UploadResult struct {
	ID           string `json:"id"`
	OriginalName string `json:"original_name"`
	FileName     string `json:"file_name"`
	FilePath     string `json:"file_path"`
	StorageKind  string `json:"storage_kind"`
}

// storageTypeLocal mirrors SK's t_file.storage_type integer code.
// SK uses 1=local, 2=oss, etc.; we lock 1 to local for P2.
const storageTypeLocal = 1

func (s *FileService) Upload(ctx context.Context, in UploadInput, createdBy string) (*UploadResult, error) {
	if in.OriginalName == "" {
		return nil, fmt.Errorf("original name is required")
	}
	id := idgen.New()
	ext := strings.ToLower(filepath.Ext(in.OriginalName))
	if dangerousExtensions[ext] {
		return nil, ErrDangerousFileType
	}
	stored := id
	if ext != "" && len(ext) <= 16 {
		stored = id + ext
	}
	// Bucket by id prefix to keep dirs small.
	key := filepath.Join("upload", id[:2], stored)
	canonKey, err := s.st.Save(ctx, key, in.Content)
	if err != nil {
		return nil, fmt.Errorf("save object: %w", err)
	}
	shared := sql.NullInt32{Int32: 0, Valid: true}
	if in.Shared != nil {
		shared = sql.NullInt32{Int32: *in.Shared, Valid: true}
	}
	if err := s.q.CreateFile(ctx, db.CreateFileParams{
		ID:           id,
		OriginalName: sql.NullString{String: in.OriginalName, Valid: true},
		FileName:     sql.NullString{String: stored, Valid: true},
		FilePath:     sql.NullString{String: canonKey, Valid: true},
		StorageType:  sql.NullInt32{Int32: int32(storageTypeLocal), Valid: true},
		Shared:       shared,
		CreateBy:     sql.NullString{String: createdBy, Valid: true},
	}); err != nil {
		// Best effort cleanup of the just-written object.
		_ = s.st.Delete(ctx, canonKey)
		return nil, fmt.Errorf("record file: %w", err)
	}
	return &UploadResult{
		ID:           id,
		OriginalName: in.OriginalName,
		FileName:     stored,
		FilePath:     canonKey,
		StorageKind:  s.st.Kind(),
	}, nil
}

func (s *FileService) Get(ctx context.Context, id string) (db.GetFileByIDRow, error) {
	row, err := s.q.GetFileByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return row, ErrNotFound
		}
		return row, fmt.Errorf("get file: %w", err)
	}
	return row, nil
}

func (s *FileService) Open(ctx context.Context, id string) (io.ReadCloser, db.GetFileByIDRow, error) {
	row, err := s.Get(ctx, id)
	if err != nil {
		return nil, row, err
	}
	if !row.FilePath.Valid {
		return nil, row, fmt.Errorf("file %s has no file_path", id)
	}
	rc, err := s.st.Open(ctx, row.FilePath.String)
	return rc, row, err
}

func (s *FileService) SoftDelete(ctx context.Context, id, deletedBy string) error {
	if _, err := s.q.GetFileByID(ctx, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("load: %w", err)
	}
	return s.q.SoftDeleteFile(ctx, db.SoftDeleteFileParams{
		UpdateBy: sql.NullString{String: deletedBy, Valid: true},
		ID:       id,
	})
}
