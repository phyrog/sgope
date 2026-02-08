package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
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

// expandRecursive finds all Go package directories under baseDir
func expandRecursive(baseDir string) []string {
	var packages []string

	filepath.Walk(baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip directories with errors
		}

		if !info.IsDir() {
			return nil
		}

		// Check if directory contains .go files
		entries, err := os.ReadDir(path)
		if err != nil {
			return nil
		}

		hasGoFiles := false
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".go") {
				hasGoFiles = true
				break
			}
		}

		if hasGoFiles {
			packages = append(packages, path)
		}

		return nil
	})

	return packages
}
