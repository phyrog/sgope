package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
)

// --- Main CLI Wrapper ---

func main() {
	vizMode := flag.Bool("viz", false, "Serve web visualization instead of raw text")
	port := flag.String("port", "8080", "Port for visualization")
	flag.Parse()

	args := flag.Args()
	if len(args) < 1 {
		fmt.Println("Usage: dep [-viz] [-port 8080] <package-path>")
		os.Exit(1)
	}

	// 1. Run Analysis
	graph := analyze(args[0])
	outputText := graph.toString()

	// 2. Decide output format
	if !*vizMode {
		fmt.Print(outputText)
	} else {
		data := parseDependencies(outputText)
		html := generateHTML(data)

		http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write([]byte(html))
		})

		fmt.Fprintf(os.Stderr, "Serving visualization at http://localhost:%s\n", *port)
		log.Fatal(http.ListenAndServe(":"+*port, nil))
	}
}
