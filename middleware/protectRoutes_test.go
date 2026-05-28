package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func setEnv(t *testing.T) {
	t.Helper()
	t.Setenv(ProtectRoutesSettings.JwtSecretEnvKey, "password123")
}

func browser(w *httptest.ResponseRecorder, r *http.Request) (*httptest.ResponseRecorder, *http.Request) {
	nr := httptest.NewRequest(r.Method, r.URL.Path, nil)
	nw := httptest.NewRecorder()

	// Copy cookies to next request
	for _, cookie := range w.Result().Cookies() {
		nr.AddCookie(cookie)
	}

	return nw, nr
}

var endpoint = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
})

func TestProtectRoutes_Deny(t *testing.T) {
	setEnv(t)

	r := httptest.NewRequest("GET", "/protect", nil)
	w := httptest.NewRecorder()

	mw := ProtectRoutes(ProtectRoutesConfig{SecuredRoutes: "/protect"})

	handler := mw(endpoint)

	handler.ServeHTTP(w, r)

	if w.Code != 401 {
		t.Fatal("expected 401, was", w.Code)
	}
}

func TestProtectRoutes_Allow(t *testing.T) {
	setEnv(t)

	r := httptest.NewRequest("GET", "/protect", nil)
	w := httptest.NewRecorder()

	err := Authorize("", w)

	if err != nil {
		t.Fatal(err)
	}

	w, r = browser(w, r)

	mw := ProtectRoutes(ProtectRoutesConfig{SecuredRoutes: "/protect"})
	handler := mw(endpoint)
	handler.ServeHTTP(w, r)

	if w.Code != 200 {
		t.Fatal("expected 200, was", w.Code)
	}
}

func TestProtectRoutes_ExtractSubject(t *testing.T) {
	setEnv(t)

	r := httptest.NewRequest("GET", "/protect", nil)
	w := httptest.NewRecorder()

	userID := "1234"
	err := Authorize(userID, w)

	if err != nil {
		t.Fatal(err)
	}

	w, r = browser(w, r)

	mw := ProtectRoutes(ProtectRoutesConfig{SecuredRoutes: "/protect"})
	handler := mw(endpoint)
	handler.ServeHTTP(w, r)

	if w.Code != 200 {
		t.Fatal("expected 200, was", w.Code)
	}

	requestID := r.Header.Get(ProtectRoutesSettings.UserIDHeader)

	if userID != requestID {
		t.Fatal("JWT sub mismatch, expected", userID, "was", requestID)
	}
}
