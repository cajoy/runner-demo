// A small web server, so the demo has something you can actually look at and
// something worth changing. `runner run` verifies it on your laptop; CI trusts
// that receipt instead of repeating the work.
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
	Chart     string
	Bars      []Bar
	Note      string
	Built     string
}

// Bar is one row of the chart. Its cells are rendered as markup rather than an
// inline width, so the template never interpolates into a style attribute.
type Bar struct {
	Label string
	Value string
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
	// CI runs lint, unit, build and deploy. A receipt that covers the first
	// three leaves only deploy. These are stated here, not measured here: the
	// receipt is what proves them.
	const withoutReceipts, withReceipts = 4, 1
	return Page{
		Title:     "Runner demo",
		Heading:   "Runner demo",
		Tagline:   "Local verification, task receipts, and a shareable page.",
		Ratio:     "4 : 1",
		RatioNote: "CPUs bought per GPU for agentic work — Intel, 2026",
		Punchline: "The fourth one is your laptop.",
		Chart:     "What CI ran for a commit like this one",
		Bars: []Bar{
			{
				Label: "without a receipt",
				Value: "4 jobs",
				Cells: barCells(withoutReceipts, withoutReceipts),
			},
			{
				Label: "with a receipt",
				Value: "1 job",
				Cells: barCells(withReceipts, withoutReceipts),
			},
		},
		Note: "Across 51 public repositories, a median 42.7% of check runs re-proved " +
			"already-proven content.",
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
