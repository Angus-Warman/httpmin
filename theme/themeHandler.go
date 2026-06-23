package theme

import (
	"embed"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"time"
)

//go:embed all:css
var cssFolder embed.FS

type ThemeName string

const (
	Default ThemeName = "default"
	Minimal ThemeName = "minimal"
	Modern  ThemeName = "modern"
	Console ThemeName = "console"
	Paper   ThemeName = "paper"
)

var serverStartTime = time.Now().UTC().Round(time.Second)
var serverStartTimeString = serverStartTime.Format(http.TimeFormat)

func ThemeHandler(themeName ThemeName) http.Handler {
	cssPath := filepath.Join("themes", string(themeName)+".css")

	cssBytes, err := cssFolder.ReadFile(cssPath)

	if err != nil {
		err := fmt.Errorf("theme %q not found: %w", themeName)
		panic(err)
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		w.Header().Set("Content-Length", strconv.Itoa(len(cssBytes)))
		w.Header().Set("Last-Modified", serverStartTimeString)
		w.WriteHeader(http.StatusOK)

		if r.Method == http.MethodHead {
			return
		}

		w.Write(cssBytes)
	})
}
