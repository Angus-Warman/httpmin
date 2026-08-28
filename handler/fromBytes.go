package handler

import (
	"net/http"
)

func FromBytes(data []byte) http.Handler {
	contentType := http.DetectContentType(data)

	return FromBytesAsType(data, contentType)
}

func FromBytesAsType(data []byte, contentType string) http.Handler {
	gzipData := mustGzipBytes(data)

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serveCorrectEncoding(w, r, contentType, data, gzipData)
	})

	return Immutable(next)
}
