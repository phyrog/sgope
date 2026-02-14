// SPDX-License-Identitfier: Apache-2.0

package main

import (
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
)

func main() {
	jsonMode := flag.Bool("json", false, "Output JSON to stdout instead of serving visualization")
	port := flag.String("port", "8080", "Port for visualization")
	flag.Parse()

	args := flag.Args()

	var jsonData []byte
	var err error

	// If no args provided and not in JSON mode, read from stdin
	if len(args) == 0 && !*jsonMode {
		fmt.Fprintln(os.Stderr, "Reading graph data from stdin...")
		jsonData, err = io.ReadAll(os.Stdin)
		if err != nil {
			log.Fatalf("Failed to read JSON from stdin: %v", err)
		}
	} else if len(args) == 0 {
		fmt.Println("Usage: sgope [-json] [-port 8080] <package-path> [<package-path>...] ")
		fmt.Println("  Use '...' suffix for recursive package discovery (e.g., ./pkg/...)")
		fmt.Println("  Omit package paths to read graph data from stdin and serve visualization")
		os.Exit(1)
	} else {
		graph, err := analyzePackages(args...)
		if err != nil {
			log.Fatal(err)
		}

		if *jsonMode {
			jsonData, err = json.MarshalIndent(graph, "", "  ")
		} else {
			jsonData, err = json.Marshal(graph)
		}
		if err != nil {
			log.Fatalf("JSON marshaling error: %v", err)
		}
	}

	if *jsonMode {
		fmt.Println(string(jsonData))
	} else {
		html := generateHTML(string(jsonData))

		http.HandleFunc("/d3.js", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
			w.Header().Set("Cache-Control", "max-age=604800")
			w.Write([]byte(d3))
		})

		http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
			w.Header().Set("Cross-Origin-Embedder-Policy", "require-corp")
			w.Write([]byte(html))
		})

		fmt.Fprintf(os.Stderr, "Serving visualization at http://localhost:%s\n", *port)
		log.Fatal(http.ListenAndServe(":"+*port, nil))
	}
}

//go:embed d3.v7.min.js
var d3 string

//go:embed viz.html
var html string

// generateHTML takes JSON data as a string and embeds it in the HTML
func generateHTML(jsonData string) string {
	return strings.Replace(html, "DATA_PLACEHOLDER", jsonData, 1)
}
