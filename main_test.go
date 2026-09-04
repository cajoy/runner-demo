package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestIndexRendersThepage(t *testing.T) {
	mux, err := handler()
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d", recorder.Code)
	}
	body := recorder.Body.String()
	for _, want := range []string{content().Heading, content().Tagline, "<!doctype html>"} {
		if !strings.Contains(body, want) {
			t.Fatalf("page is missing %q:\n%s", want, body)
		}
	}
}

// The template is embedded, so a syntax error in index.html is a build-time
// and test-time failure rather than a blank page in a browser.
func TestTemplateParses(t *testing.T) {
	if _, err := handler(); err != nil {
		t.Fatalf("index.html does not parse: %v", err)
	}
}

func TestHealthz(t *testing.T) {
	mux, err := handler()
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if recorder.Code != http.StatusOK || strings.TrimSpace(recorder.Body.String()) != "ok" {
		t.Fatalf("healthz = %d %q", recorder.Code, recorder.Body.String())
	}
}

func TestUnknownPathIsNotFound(t *testing.T) {
	mux, err := handler()
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/nope", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", recorder.Code)
	}
}
