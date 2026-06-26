package main

import (
	"strings"

	"github.com/jim-ww/shiraberu/providers"
)

func formatResults(results []providers.SearchResult, emitFields, fieldsSep, entriesSep string) string {
	if len(results) == 0 {
		return ""
	}

	var output strings.Builder

	for i, result := range results {
		var parts []string

		if strings.Contains(emitFields, "title") && result.Title != "" {
			parts = append(parts, result.Title)
		}

		if strings.Contains(emitFields, "desc") && result.Description != "" {
			parts = append(parts, result.Description)
		}

		if strings.Contains(emitFields, "url") && result.URL != "" {
			parts = append(parts, result.URL)
		}

		if len(parts) > 0 {
			output.WriteString(strings.Join(parts, fieldsSep))
		}

		if i < len(results)-1 {
			output.WriteString(entriesSep)
		}
	}

	return output.String()
}
