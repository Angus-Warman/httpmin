package theme

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBuildThemes(t *testing.T) {
	// Theme builders panic on failure
	Basic()
	Minimal()
	Modern()
	Paper()
	Console()
}

func TestBuildCustomTheme(t *testing.T) {
	h := CustomTheme("reset.css", "html { font-family: system-ui; }")
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	h.ServeHTTP(w, r)

	theme := w.Body.String()

	if !strings.Contains(theme, "system-ui") {
		t.Fatal("expected theme to contain font")
	}
}
