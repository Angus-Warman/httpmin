package handler

import (
	"bytes"
	"compress/gzip"
	"net/http"
	"strconv"
	"strings"
)

func gzipBytes(data []byte) ([]byte, error) {
	var zippedData bytes.Buffer

	zw := gzip.NewWriter(&zippedData)

	_, err := zw.Write(data)

	if err != nil {
		zw.Close()
		return nil, err
	}

	err = zw.Close() // Flushes

	if err != nil {
		return nil, err
	}

	return zippedData.Bytes(), nil
}

func mustGzipBytes(data []byte) []byte {
	zipped, err := gzipBytes(data)

	if err != nil {
		panic(err)
	}

	return zipped
}

func gzipAccepted(r *http.Request) bool {
	acceptEncoding := r.Header.Get("Accept-Encoding")
	return strings.Contains(acceptEncoding, "gzip") && !strings.Contains(acceptEncoding, "gzip;q=0")
}

func serveCorrectEncoding(w http.ResponseWriter, r *http.Request, contentType string, rawData, gzipData []byte) {
	isEffective := len(gzipData)*9/10 < len(rawData) // Unless gzip reduces by more than 10%, don't bother

	w.Header().Set("Content-Type", contentType)

	if isEffective && gzipAccepted(r) {
		w.Header().Set("Content-Length", strconv.Itoa(len(gzipData)))
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Add("Vary", "Accept-Encoding")
		w.Write(gzipData)
		return
	}

	w.Header().Set("Content-Length", strconv.Itoa(len(rawData)))
	w.Write(rawData)
}
