package handler

import (
	"embed"
	"net/http/httptest"
	"testing"
)

//go:embed all:testdata
var publicFiles embed.FS

func TestIndexServed(t *testing.T) {
	handler, err := EmbeddedFileServer(publicFiles)

	if err != nil {
		t.Fatal(err)
	}

	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Add("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, r)

	res := w.Result()

	if res.StatusCode != 200 {
		t.Fatal(res.StatusCode, "should be 200")
	}
}

func TestCleanURLHandling(t *testing.T) {
	handler, err := EmbeddedFileServer(publicFiles)

	if err != nil {
		t.Fatal(err)
	}

	r := httptest.NewRequest("GET", "/test", nil)
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
	handler, err := EmbeddedFileServer(publicFiles)

	if err != nil {
		t.Fatal(err)
	}

	r := httptest.NewRequest("GET", "/data.txt", nil)
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
