// Package respond provides shared helpers for writing JSON HTTP responses.
package respond

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// JSON writes v as a JSON body with the given status code.
func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("encode json response", "err", err)
	}
}

// Error writes a JSON error body: {"error": msg}.
func Error(w http.ResponseWriter, status int, msg string) {
	JSON(w, status, map[string]string{"error": msg})
}
