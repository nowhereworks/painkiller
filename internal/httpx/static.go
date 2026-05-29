package httpx

import (
	"io/fs"
	"net/http"
	"net/url"
	"path"
	"strings"
)

func (s *Server) ServeStatic(staticFS fs.FS) {
	handler := staticHandler(staticFS)
	s.router.Handle("/*", handler)
}

func staticHandler(staticFS fs.FS) http.HandlerFunc {
	fileServer := http.FileServer(http.FS(staticFS))

	return func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}

		filePath := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if filePath == "." {
			filePath = "index.html"
		}

		if info, err := fs.Stat(staticFS, filePath); err == nil {
			if info.IsDir() {
				filePath = path.Join(filePath, "index.html")
			}
		} else {
			filePath = "index.html"
		}

		if strings.HasPrefix(filePath, "_next/static/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}

		r2 := new(http.Request)
		*r2 = *r
		r2.URL = cloneURL(r.URL)
		r2.URL.Path = "/" + filePath

		fileServer.ServeHTTP(w, r2)
	}
}

func cloneURL(u *url.URL) *url.URL {
	copy := *u
	return &copy
}
