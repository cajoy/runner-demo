// A small web server, so the demo has something you can actually look at and
// something worth changing. Runner verifies original signed evidence before
// reusing checks whose declared inputs and runtime still match.
package main

import (
	_ "embed"
	"flag"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

//go:embed index.html
var page string

// Page is what index.html renders. Change the copy or the numbers here and the
// unit test, the receipt, and the deployed page all move together. That is the
// demo: editing this struct is a code change, so it has to earn the same proof
// as any other one.
type Page struct {
	Title     string
	Heading   string
	Tagline   string
	Ratio     string
	RatioNote string
	Punchline string
	Beats     []Beat
	Chart     string
	Bars      []Bar
	Metrics   []Metric
	Note      string
	Built     string
}

// Beat is one of the three moves the demo argues for: run it, prove it, skip
// what is proven. Copy and figures live here for the same reason the bars do.
type Beat struct {
	Verb  string
	Lines []string
	Foot  string
}

// Metric is a rounded illustration from the dated rehearsal.
type Metric struct {
	Value string
	Label string
}

// Bar is one row of the chart. Its cells are rendered as markup rather than an
// inline width, so the template never interpolates into a style attribute.
type Bar struct {
	Label string
	Value string
	// Time sits beside the count because the count alone understates the case:
	// skipping three checks is only interesting if running them costs something.
	Time  string
	Cells []bool
}

// barCells fills a fixed-width track proportionally. A zero value still renders
// an empty track, so a row never silently disappears.
func barCells(value, max int) []bool {
	const width = 20
	cells := make([]bool, width)
	if max <= 0 {
		return cells
	}
	filled := min(value*width/max, width)
	for i := range filled {
		cells[i] = true
	}
	return cells
}

func content() Page {
	// The demonstrated all-valid case: lint, unit, and build already passed.
	// CI still verifies signatures, policy, content, and the runtime.
	const withoutReceipts, withReceipts = 3, 0
	return Page{
		Title:     "Runner demo",
		Heading:   "Runner demo",
		Tagline:   "Verify once. Sign automatically. Carry proof with git push.",
		Ratio:     "3 → 0",
		RatioNote: "Checks CI repeats when the original proof still verifies",
		Punchline: "Run the checks. Keep their proof.",
		Beats: []Beat{
			{Verb: "Run.", Lines: []string{
				"9 MB binary. CLI, MCP server and dashboard over one local state.",
				"A DAG, so independent checks start together.",
				"An agent runs the full workflow, or a selected step, over MCP.",
			}, Foot: "laptop \u00b7 cloud CI \u00b7 edge \u00b7 enterprise"},
			{Verb: "Prove.", Lines: []string{
				"Ed25519 over the receipt bytes, bound to commit, tree and config digest.",
				"A policy file on main names which keys are trusted.",
			}, Foot: "a dirty tree signs nothing"},
			{Verb: "Skip.", Lines: []string{
				"CI runs nothing the proof covers. Not faster checks, absent ones.",
				"The example below shows a fresh run and a run reusing its proof.",
			}, Foot: "0 hosted minutes"},
		},
		Chart: "Verification commands in the all-valid demo case",
		Bars: []Bar{
			{Label: "without a receipt", Value: "3 checks", Time: "about 11 s", Cells: barCells(withoutReceipts, withoutReceipts)},
			{Label: "with a receipt", Value: "0 checks", Time: "about 1 s", Cells: barCells(withReceipts, withoutReceipts)},
		},
		// Only figures the chart does not already carry, all in seconds so the
		// page never asks a reader to convert units to compare two numbers.
		Metrics: []Metric{
			{Value: "under 0.1 s", Label: "from run accepted to all three checks running"},
			{Value: "about 7 s", Label: "in the pinned container, before the first command runs"},
		},
		Note:  "Illustrative timings, rounded from the September 9, 2026 rehearsal on one machine. These are not current benchmarks or performance guarantees. Runtime startup and cache restoration vary by host.",
		Built: time.Now().UTC().Format(time.RFC3339),
	}
}

func handler() (http.Handler, error) {
	parsed, err := template.New("index").Parse(page)
	if err != nil {
		return nil, err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprintln(w, "ok")
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := parsed.Execute(w, content()); err != nil {
			http.Error(w, "template failed", http.StatusInternalServerError)
		}
	})
	return mux, nil
}

func writeStaticPage(w io.Writer) error {
	parsed, err := template.New("index").Parse(page)
	if err != nil {
		return err
	}
	return parsed.Execute(w, content())
}

func main() {
	export := flag.Bool("export", false, "render index.html to stdout for static hosting")
	flag.Parse()
	if *export {
		if err := writeStaticPage(os.Stdout); err != nil {
			log.Fatal(err)
		}
		return
	}
	address := os.Getenv("ADDR")
	if address == "" {
		address = "127.0.0.1:8080"
	}
	mux, err := handler()
	if err != nil {
		log.Fatal(err)
	}
	server := &http.Server{
		Addr: address, Handler: mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Printf("listening on http://%s/", address)
	log.Fatal(server.ListenAndServe())
}
