// Package console serves the server-rendered admin UI mounted at /console.
// It is a Gate-1 spike: html/template + HTMX, no JS framework. Lives
// alongside the SK UmiJS bundle at /; the two are independent.
package console

import "embed"

//go:embed all:templates all:static
var FS embed.FS
