package handler

import (
	"bytes"
	"compress/gzip"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Strips the top-level folder, so /static/styles.css is served at /styles.css
func EmbeddedFileServerAtRoot(folder fs.FS) http.Handler {
	// By default, embedded folder is expecting a path like "/public/index.html"
	// Moving down one level results in normal behaviour
	innerFolder := substituteTopLevelDir(folder)

	return EmbeddedFileServer(innerFolder)
}

// Pre-computes gzip data for compressed responses
//
// Serves "clean" URLs, /page -> /page.html
func EmbeddedFileServer(folder fs.FS) http.Handler {
	fallback := http.FileServerFS(folder)
	fallback = setLastModified(fallback)

	handler := &embeddedFileServer{
		folder:       folder,
		fallback:     fallback,
		pathLookup:   make(map[string]string),
		gzippedFiles: make(map[string][]byte),
	}

	err := handler.build()

	if err != nil {
		panic(err)
	}

	return handler
}

var serverStartTime = time.Now().UTC().Round(time.Second)
var serverStartTimeString = serverStartTime.Format(http.TimeFormat)

type embeddedFileServer struct {
	folder       fs.FS
	fallback     http.Handler
	pathLookup   map[string]string
	gzippedFiles map[string][]byte
}

func (h *embeddedFileServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if isNotModified(r) {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	acceptEncoding := r.Header.Get("Accept-Encoding")

	gzipAccepted := strings.Contains(acceptEncoding, "gzip") && !strings.Contains(acceptEncoding, "gzip;q=0")

	canonical := h.getCanonicalPath(r.URL.Path)

	if gzipAccepted {
		gzippedBytes, ok := h.gzippedFiles[canonical]

		if ok {
			serveGzipped(w, r, gzippedBytes)
			return
		}

		// Fall-through intentional
	}

	// If lookup exists, clone request and continue with correct path
	// This doesn't use canonical, since index.html handling is dealt with by fallback
	realPath, ok := h.pathLookup[r.URL.Path]

	if ok {
		r2 := r.Clone(r.Context())
		r2.URL.Path = realPath
		h.fallback.ServeHTTP(w, r2)
		return
	}

	h.fallback.ServeHTTP(w, r)
}

func isNotModified(r *http.Request) bool {
	ifModifiedSinceStr := r.Header.Get("If-Modified-Since")

	if ifModifiedSinceStr == "" {
		return false
	}

	ifModifiedSince, err := http.ParseTime(ifModifiedSinceStr)

	if err != nil {
		return false
	}

	stale := ifModifiedSince.Before(serverStartTime)

	return !stale
}

func serveGzipped(w http.ResponseWriter, r *http.Request, gzippedBytes []byte) {
	contentType := mime.TypeByExtension(filepath.Ext(r.URL.Path))

	// Standard
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(gzippedBytes)))

	// Zip
	w.Header().Set("Content-Encoding", "gzip")
	w.Header().Add("Vary", "Accept-Encoding")

	// Caching
	// While this isn't technically correct, it is impossible for any files to be modified after the server is started
	// So this has a useful caching effect
	w.Header().Set("Last-Modified", serverStartTimeString)

	w.WriteHeader(http.StatusOK)

	if r.Method == http.MethodHead {
		return
	}

	w.Write(gzippedBytes)
}

func (h *embeddedFileServer) getCanonicalPath(original string) string {
	if original == "" || original == "/" {
		return "/index.html"
	}

	if strings.HasSuffix(original, "/") {
		return original + "index.html"
	}

	realPath, ok := h.pathLookup[original]

	if ok {
		return realPath
	}

	return original
}

func (h *embeddedFileServer) build() error {
	return fs.WalkDir(h.folder, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			return nil
		}

		// Build lookup for HTML files only
		ext := filepath.Ext(path)

		if ext == ".html" {
			lookupPath := "/" + strings.TrimSuffix(path, ".html")
			h.pathLookup[lookupPath] = "/" + path
		}

		// Pre-gzip files
		if shouldZip(ext) {
			file, err := h.folder.Open(path)

			if err != nil {
				return err
			}

			defer file.Close()

			fileBytes, err := io.ReadAll(file)

			if err != nil {
				return err
			}

			key := "/" + path

			gzippedBytes, err := gzipBytes(fileBytes)

			if err != nil {
				return err
			}

			if len(gzippedBytes) >= len(fileBytes) {
				// No point in serving zipped
				return nil
			}

			h.gzippedFiles[key] = gzippedBytes
		}

		return nil
	})
}

func shouldZip(ext string) bool {
	switch ext {
	case ".html", ".htm", ".js", ".mjs", ".cjs", ".css",
		".json", ".jsonld", ".txt", ".md", ".xml", ".rss",
		".svg", ".csv", ".wasm", ".map":
		return true
	default:
		return false
	}
}

func gzipBytes(fileBytes []byte) ([]byte, error) {
	var buf bytes.Buffer

	zw := gzip.NewWriter(&buf)

	_, err := zw.Write(fileBytes)

	if err != nil {
		zw.Close()
		return nil, err
	}

	err = zw.Close() // Flushes

	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func setLastModified(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {

		// While this isn't technically correct, it is impossible for any files to be modified after the server is started
		// So this has a useful caching effect
		w.Header().Set("Last-Modified", serverStartTimeString)

		next.ServeHTTP(w, r)
	}

	return http.HandlerFunc(fn)
}
