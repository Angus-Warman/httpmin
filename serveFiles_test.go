package httpmin

import (
	"embed"
	"net/http/httptest"
	"testing"
)

//go:embed all:testdata/public
var publicFiles embed.FS

func TestCleanURLHandling(t *testing.T) {
	handler := serveEmbeddedFiles(publicFiles)

	r := httptest.NewRequest("GET", "/public/test", nil)
	r.Header.Add("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, r)

	res := w.Result()

	if res.StatusCode != 200 {
		t.Fatal(res.StatusCode, "should be 200")
	}

	contentEncoding := res.Header.Get("Content-Encoding")

	if contentEncoding != "" {
		t.Fatal("tiny files shouldn't be compressed")
	}
}

func TestLargeFilesCompressed(t *testing.T) {
	handler := serveEmbeddedFiles(publicFiles)

	r := httptest.NewRequest("GET", "/public/data.txt", nil)
	r.Header.Add("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, r)

	res := w.Result()

	if res.StatusCode != 200 {
		t.Fatal(res.StatusCode, "should be 200")
	}

	contentEncoding := res.Header.Get("Content-Encoding")

	if contentEncoding != "gzip" {
		t.Fatal("large files should be compressed")
	}
}
