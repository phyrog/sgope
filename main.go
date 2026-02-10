// SPDX-License-Identitfier: Apache-2.0

package main

import (
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
)

// --- Main CLI Wrapper ---

func main() {
	vizMode := flag.Bool("viz", false, "Serve web visualization instead of raw text")
	port := flag.String("port", "8080", "Port for visualization")
	flag.Parse()

	args := flag.Args()
	if len(args) < 1 {
		fmt.Println("Usage: sgope [-viz] [-port 8080] <package-path> [<package-path>...] ")
		fmt.Println("  Use '...' suffix for recursive package discovery (e.g., ./pkg/...)")
		os.Exit(1)
	}

	if len(args) == 0 {
		fmt.Println("No valid packages found")
		os.Exit(1)
	}

	// 1. Run Analysis
	graph, err := analyzePackages(args...)
	if err != nil {
		log.Fatal(err)
	}

	// 2. Decide output format
	if !*vizMode {
		// Text mode: output JSON
		jsonData, err := json.MarshalIndent(graph, "", "  ")
		if err != nil {
			log.Fatalf("JSON marshaling error: %v", err)
		}
		fmt.Println(string(jsonData))
	} else {
		// Viz mode: generate HTML with embedded JSON
		jsonData, err := json.Marshal(graph)
		if err != nil {
			log.Fatalf("JSON marshaling error: %v", err)
		}
		html := generateHTML(string(jsonData))

		http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write([]byte(html))
		})

		fmt.Fprintf(os.Stderr, "Serving visualization at http://localhost:%s\n", *port)
		log.Fatal(http.ListenAndServe(":"+*port, nil))
	}
}

//go:embed viz.html
var html string

// generateHTML takes JSON data as a string and embeds it in the HTML
func generateHTML(jsonData string) string {
	return strings.Replace(html, "DATA_PLACEHOLDER", jsonData, 1)
}
