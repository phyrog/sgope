package main

import (
	_ "embed"
	"strings"
)

//go:embed viz.html
var html string

// generateHTML takes JSON data as a string and embeds it in the HTML
func generateHTML(jsonData string) string {
	return strings.Replace(html, "DATA_PLACEHOLDER", jsonData, 1)
}
