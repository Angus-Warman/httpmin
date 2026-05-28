package httpmin

import (
	"io/fs"
	"net/http"
	"path/filepath"
	"strings"
)

type embeddedFileServer struct {
	folder   fs.FS
	fallback http.Handler
	lookup   map[string]string
}

func (h *embeddedFileServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p := r.URL.Path

	// index
	if p == "" || p == "/" {
		h.fallback.ServeHTTP(w, r)
		return
	}

	// real files
	if filepath.Ext(p) != "" {
		h.fallback.ServeHTTP(w, r)
		return
	}

	realPath, ok := h.lookup[p]

	if ok {
		r2 := r.Clone(r.Context())
		r2.URL.Path = realPath
		h.fallback.ServeHTTP(w, r2)
		return
	}

	h.fallback.ServeHTTP(w, r)
}

func (h *embeddedFileServer) buildLookup() {
	fs.WalkDir(h.folder, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}

		if d.IsDir() {
			return nil
		}

		ext := filepath.Ext(path)

		if ext == ".html" {
			lookupPath := "/" + strings.TrimSuffix(path, ".html")
			h.lookup[lookupPath] = "/" + path
		}
		return nil
	})
}

func serveEmbeddedFiles(folder fs.FS) http.Handler {
	fallback := http.FileServerFS(folder)

	handler := &embeddedFileServer{
		folder:   folder,
		fallback: fallback,
		lookup:   make(map[string]string),
	}

	handler.buildLookup()

	return handler
}
