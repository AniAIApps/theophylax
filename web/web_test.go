package web

import (
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestHandlerServesPlaceholderWithoutConsole checks that a binary built
// without `make ui` serves a page explaining how to build the console,
// for the root and for client-side routes, while API paths stay 404.
func TestHandlerServesPlaceholderWithoutConsole(t *testing.T) {
	_, err := fs.Stat(dist, "dist/index.html")
	built := err == nil
	h := Handler()
	for _, p := range []string{"/", "/projects/abc"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, p, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s: status %d", p, rec.Code)
		}
		if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
			t.Fatalf("GET %s: content type %q", p, ct)
		}
		body, _ := io.ReadAll(rec.Body)
		if !built && !strings.Contains(string(body), "make ui") {
			t.Fatalf("GET %s: want the placeholder, got %q", p, body)
		}
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/nope", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("GET /api/nope: status %d", rec.Code)
	}
}
