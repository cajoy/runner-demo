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

// Page is what index.html renders. Change the copy here or in the template and
// the unit test, the receipt, and the deployed page all move together.
type Page struct {
	Title   string
	Heading string
	Tagline string
	Built   string
}

func content() Page {
	return Page{
		Title:   "Runner demo",
		Heading: "Runner demo",
		Tagline: "Local verification, task receipts, and a shareable page.",
		Built:   time.Now().UTC().Format(time.RFC3339),
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
