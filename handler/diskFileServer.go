package handler

import (
	"net/http"
	"path"
	"path/filepath"
	"strings"
)

// Serves "clean" URLs, /page -> /page.html
func DiskFileServer(rootPath string) http.Handler {
	dir := http.Dir(rootPath)
	fileServer := http.FileServer(dir)

	fn := func(w http.ResponseWriter, r *http.Request) {
		cleanURL := path.Clean("/" + r.URL.Path)

		if !strings.HasSuffix(cleanURL, "/") && filepath.Ext(cleanURL) == "" {
			// Could be a clean path, check if it exists
			htmlPath := cleanURL + ".html"

			f, err := dir.Open(htmlPath)

			exists := err == nil

			if exists {
				f.Close()
				r2 := r.Clone(r.Context())
				r2.URL.Path = htmlPath
				fileServer.ServeHTTP(w, r2)
				return
			}
		}

		// Otherwise, serve normally
		fileServer.ServeHTTP(w, r)
	}

	return http.HandlerFunc(fn)
}
