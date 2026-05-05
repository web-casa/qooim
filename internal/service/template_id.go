package service

import "github.com/web-casa/qooim/internal/idgen"

// newTemplateID is a hop the import flow uses so the report service
// doesn't have to import idgen directly (avoids tight coupling between
// import-from-xlsx logic and the ULID choice).
func newTemplateID() string { return idgen.New() }
