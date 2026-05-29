package httpx

import (
	"io"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

func (s *Server) ServeStatic(staticFS fs.FS) {
	handler := staticHandler(staticFS)
	s.router.Handle("/*", handler)
}

func staticHandler(staticFS fs.FS) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}

		filePath := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if filePath == "." || filePath == "" {
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

		f, err := staticFS.Open(filePath)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer f.Close()

		info, err := f.Stat()
		if err != nil {
			http.NotFound(w, r)
			return
		}

		http.ServeContent(w, r, filePath, info.ModTime(), f.(io.ReadSeeker))
	}
}
