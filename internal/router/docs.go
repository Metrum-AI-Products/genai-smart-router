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
)

//go:embed all:docsdist
var embeddedDocs embed.FS

func docsHandler() http.Handler {
	sub, err := fs.Sub(embeddedDocs, "docsdist")
	if err != nil {
		return http.NotFoundHandler()
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		setPublicDocsSecurityHeaders(w)
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
			if serveEmbeddedDocStatus(w, r, sub, "404.html", http.StatusNotFound) {
				return
			}
			if name == "index.html" {
				writeFallbackDocs(w)
				return
			}
			if docsFallbackRouteAllowed(name) {
				writeFallbackDocs(w)
				return
			}
			if !strings.Contains(path.Base(name), ".") {
				writeFallbackDocsStatus(w, http.StatusNotFound)
				return
			}
		}
		http.NotFound(w, r)
	})
}

func docsFallbackRouteAllowed(name string) bool {
	switch strings.Trim(strings.TrimSuffix(name, "/"), "/") {
	case "solution-brief", "privacy", "dpa", "subprocessors", "transfer-schedule",
		"legal/dpa", "legal/subprocessors", "legal/transfer-schedule":
		return true
	default:
		return false
	}
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
	return serveEmbeddedDocStatus(w, r, root, name, http.StatusOK)
}

func serveEmbeddedDocStatus(w http.ResponseWriter, r *http.Request, root fs.FS, name string, status int) bool {
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
			if status == http.StatusOK {
				http.ServeContent(w, r, name, stat.ModTime(), bytes.NewReader(data))
			} else {
				w.WriteHeader(status)
				if r.Method != http.MethodHead {
					_, _ = w.Write(data)
				}
			}
			return true
		}
	}
	if !strings.HasSuffix(name, "/index.html") {
		dirIndex := strings.TrimSuffix(name, "/") + "/index.html"
		return serveEmbeddedDocStatus(w, r, root, dirIndex, status)
	}
	return false
}

func writeFallbackDocs(w http.ResponseWriter) {
	writeFallbackDocsStatus(w, http.StatusOK)
}

func writeFallbackDocsStatus(w http.ResponseWriter, status int) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
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

func setPublicDocsSecurityHeaders(w http.ResponseWriter) {
	w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; font-src 'self' data:; connect-src 'self' https://api.elevenlabs.io wss://api.elevenlabs.io wss://livekit.rtc.elevenlabs.io; media-src 'self' blob:; worker-src 'self' blob:")
	// The consented ConvAI voice control needs same-origin microphone access.
	w.Header().Set("Permissions-Policy", "camera=(), geolocation=(), payment=(), usb=(), browsing-topics=(), microphone=(self)")
	w.Header().Del("Server")
	w.Header().Del("X-Smart-LLMRouter-Version")
	w.Header().Del("X-Smart-LLMRouter-Build-Date")
}

func securityTextHandler(w http.ResponseWriter, r *http.Request) {
	setPublicDocsSecurityHeaders(w)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = io.WriteString(w, "Contact: mailto:contact@metrum.ai\nExpires: 2027-08-19T00:00:00.000Z\nPreferred-Languages: en\nPolicy: mailto:contact@metrum.ai\n")
}
