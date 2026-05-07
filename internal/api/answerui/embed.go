// Package answerui serves the public answer-taking UI at /answer/:projectId.
// It is a Gate-4 spike: two question types (radio + upload), temp-save,
// scoring (server-side), mobile responsive. The package is intentionally
// independent from internal/api/console — different audience (participants
// vs admins), different auth model (none vs cookie session), different
// CSP/cache rules.
package answerui

import "embed"

//go:embed all:templates all:static
var FS embed.FS
