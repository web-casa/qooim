package api

import (
	"net/http"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

// installSPA wires the static-asset handler. Hashed asset paths
// (`/*.async.js`, `/*.chunk.css`, `/static/*`, `/favicon.ico`, etc.) are
// served directly from `web/dist/`. Anything else that isn't an /api/
// route falls through to `index.html` so the SPA router takes over.
func (s *Server) installSPA(root string) {
	if root == "" {
		return
	}
	indexPath := filepath.Join(root, "index.html")
	fs := http.Dir(root)
	staticHandler := http.FileServer(fs)

	// Top-level static files: index.html + every hashed asset is at the
	// root of web/dist in SK's UmiJS build, so we register a single
	// catch-all NoRoute that does the routing.
	s.engine.NoRoute(func(c *gin.Context) {
		// API routes that didn't match are real 404s. Return the SK
		// envelope so the umi response interceptor in the bundle treats
		// them consistently with our explicit handler 404s — otherwise
		// `code: "not_found"` (a string) breaks the `code === 200`
		// check on the way in and any downstream success-flag reads.
		if strings.HasPrefix(c.Request.URL.Path, s.cfg.HTTP.APIPrefix) ||
			strings.HasPrefix(c.Request.URL.Path, "/healthz") ||
			strings.HasPrefix(c.Request.URL.Path, "/readyz") {
			c.JSON(http.StatusNotFound, gin.H{
				"success":      false,
				"code":         404,
				"message":      "endpoint not found",
				"errorMessage": "endpoint not found",
			})
			return
		}
		// Try the literal path first; if absent, return index.html so
		// client-side routing can resolve it.
		p := filepath.Clean(c.Request.URL.Path)
		if p == "/" || p == "." {
			c.File(indexPath)
			return
		}
		// Only serve files that actually exist, otherwise SPA fallback.
		f, err := fs.Open(strings.TrimPrefix(p, "/"))
		if err != nil {
			c.File(indexPath)
			return
		}
		_ = f.Close()
		staticHandler.ServeHTTP(c.Writer, c.Request)
	})
}
