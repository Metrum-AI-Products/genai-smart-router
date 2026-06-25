package router

import (
	"bytes"
	"embed"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"

	"smart-llmrouter/internal/buildinfo"
)

//go:embed all:docsdist
var embeddedDocs embed.FS

func docsHandler() http.Handler {
	sub, err := fs.Sub(embeddedDocs, "docsdist")
	if err != nil {
		return http.NotFoundHandler()
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.NotFound(w, r)
			return
		}
		if isReservedRouterPath(r.URL.Path) {
			http.NotFound(w, r)
			return
		}
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/docs/", http.StatusTemporaryRedirect)
			return
		}
		if r.URL.Path != "/docs" && !strings.HasPrefix(r.URL.Path, "/docs/") {
			http.NotFound(w, r)
			return
		}
		name := strings.TrimPrefix(path.Clean("/"+strings.TrimPrefix(r.URL.Path, "/docs")), "/")
		if name == "." || name == "" {
			name = "index.html"
		}
		if serveEmbeddedDoc(w, r, sub, name) {
			return
		}
		if !strings.Contains(path.Base(name), ".") && serveEmbeddedDoc(w, r, sub, name+".html") {
			return
		}
		if strings.Contains(r.Header.Get("Accept"), "text/html") || !strings.Contains(path.Base(name), ".") {
			if serveEmbeddedDoc(w, r, sub, "index.html") {
				return
			}
			if name == "index.html" || !strings.Contains(path.Base(name), ".") {
				writeFallbackDocs(w)
				return
			}
		}
		http.NotFound(w, r)
	})
}

func isReservedRouterPath(p string) bool {
	return p == "/metrics" ||
		p == "/healthz" ||
		p == "/readyz" ||
		strings.HasPrefix(p, "/v1/") ||
		p == "/v1" ||
		p == "/admin" ||
		strings.HasPrefix(p, "/admin/")
}

func serveEmbeddedDoc(w http.ResponseWriter, r *http.Request, root fs.FS, name string) bool {
	file, err := root.Open(name)
	if err == nil {
		defer file.Close()
		if stat, statErr := file.Stat(); statErr == nil && !stat.IsDir() {
			data, readErr := io.ReadAll(file)
			if readErr != nil {
				http.Error(w, "read embedded docs", http.StatusInternalServerError)
				return true
			}
			if ct := mime.TypeByExtension(path.Ext(name)); ct != "" {
				w.Header().Set("Content-Type", ct)
			}
			setDocsVersionHeaders(w)
			http.ServeContent(w, r, name, stat.ModTime(), bytes.NewReader(data))
			return true
		}
	}
	if !strings.HasSuffix(name, "/index.html") {
		dirIndex := strings.TrimSuffix(name, "/") + "/index.html"
		return serveEmbeddedDoc(w, r, root, dirIndex)
	}
	return false
}

func writeFallbackDocs(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	setDocsVersionHeaders(w)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>GenAI Smart Router Docs</title>
</head>
<body>
  <main>
    <h1>GenAI Smart Router Docs</h1>
    <p>Run <code>make docs-build</code> before release builds to embed the full Docusaurus site.</p>
    <p>Solution Brief</p>
  </main>
</body>
</html>`))
}

func setDocsVersionHeaders(w http.ResponseWriter) {
	info := buildinfo.Current()
	w.Header().Set("X-Smart-LLMRouter-Version", info.Version)
	w.Header().Set("X-Smart-LLMRouter-Build-Date", info.BuildDate)
}
