package handler_test

import (
	"embed"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Angus-Warman/httpmin/handler"
)

//go:embed testdata/templates
var templatesFS embed.FS

func TestTemplateHelloWorld(t *testing.T) {
	dataFn := func(w http.ResponseWriter, r *http.Request) (any, error) {
		return map[string]string{"Name": "World"}, nil
	}

	h := handler.Template(templatesFS, "hello.tmpl", dataFn)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()

	if !strings.Contains(body, "Hello, World!") {
		t.Errorf("expected body to contain %q, got: %s", "Hello, World!", body)
	}
}
