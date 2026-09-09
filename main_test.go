package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStaticPageRendersTemplate(t *testing.T) {
	var output bytes.Buffer
	if err := writeStaticPage(&output); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{content().Heading, content().Tagline, "<!doctype html>"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("static page is missing %q", want)
		}
	}
	if strings.Contains(output.String(), "{{") {
		t.Fatal("static page contains an unresolved template action")
	}
}

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

// The slide is the demo's argument, so its numbers are part of the contract the
// receipt covers. A silent template rename would otherwise ship a blank chart.
func TestPageRendersTheRatioAndTheChart(t *testing.T) {
	var output bytes.Buffer
	if err := writeStaticPage(&output); err != nil {
		t.Fatal(err)
	}
	body := output.String()
	page := content()
	for _, want := range []string{page.Ratio, page.RatioNote, page.Punchline, page.Chart, page.Note} {
		if !strings.Contains(body, want) {
			t.Fatalf("page is missing %q", want)
		}
	}
	for _, bar := range page.Bars {
		if !strings.Contains(body, bar.Label) || !strings.Contains(body, bar.Value) || !strings.Contains(body, bar.Time) {
			t.Fatalf("page is missing bar %q", bar.Label)
		}
	}
	for _, beat := range page.Beats {
		for _, want := range append([]string{beat.Verb, beat.Foot}, beat.Lines...) {
			if !strings.Contains(body, want) {
				t.Fatalf("page is missing beat text %q", want)
			}
		}
	}
	for _, metric := range page.Metrics {
		if !strings.Contains(body, metric.Value) || !strings.Contains(body, metric.Label) {
			t.Fatalf("page is missing metric %q", metric.Label)
		}
	}
	// A filled cell is what makes the comparison visible at all.
	if !strings.Contains(body, `<i class="on">`) {
		t.Fatal("chart rendered no filled cells")
	}
}

// The bar is a comparison: the reused row must be visibly shorter, or the page
// states a saving it does not show.
func TestChartShowsLessTimeWithAReceipt(t *testing.T) {
	bars := content().Bars
	if len(bars) != 2 {
		t.Fatalf("bars = %d, want 2", len(bars))
	}
	filled := func(bar Bar) int {
		count := 0
		for _, cell := range bar.Cells {
			if cell {
				count++
			}
		}
		return count
	}
	if filled(bars[1]) >= filled(bars[0]) {
		t.Fatalf("with-receipt bar is not shorter: %d vs %d cells", filled(bars[1]), filled(bars[0]))
	}
}

func TestBarCellsHandleAZeroMaximum(t *testing.T) {
	for _, cell := range barCells(3, 0) {
		if cell {
			t.Fatal("a zero maximum filled the track")
		}
	}
}
