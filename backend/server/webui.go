package server

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed webui
var webUI embed.FS

// webUIFiles exposes the embedded web assets as an fs.FS rooted at the webui
// directory so path lookups are relative to it.
var webUIFiles, _ = fs.Sub(webUI, "webui")

// serveWebUI serves the mobile SPA. Unknown non-file paths fall back to
// index.html so deep-linking/hash routing never 404s.
func serveWebUI(w http.ResponseWriter, r *http.Request) {
	p := r.URL.Path
	if p == "/" {
		serveFileFromFS(w, r, "index.html")
		return
	}
	clean := strings.TrimPrefix(path.Clean(p), "/")
	if clean == "" {
		serveFileFromFS(w, r, "index.html")
		return
	}
	// Serve the requested asset only if it exists; otherwise index.html.
	data, err := fs.ReadFile(webUIFiles, clean)
	if err != nil {
		// Not a real file → SPA route → index.html.
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}
		serveFileFromFS(w, r, "index.html")
		return
	}
	ct := contentTypeByExt(clean)
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func serveFileFromFS(w http.ResponseWriter, r *http.Request, name string) {
	data, err := fs.ReadFile(webUIFiles, name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", contentTypeByExt(name))
	// index.html must not be cached or mobile static assets go stale.
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func contentTypeByExt(name string) string {
	switch strings.ToLower(path.Ext(name)) {
	case ".html":
		return "text/html; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	case ".js":
		return "application/javascript; charset=utf-8"
	case ".json":
		return "application/json; charset=utf-8"
	case ".svg":
		return "image/svg+xml"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".woff2":
		return "font/woff2"
	default:
		return "application/octet-stream"
	}
}
