// Package httpapi содержит утилиты записи JSON-ответов для HTTP-хендлеров.
package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

const maxBodyBytes = 1 << 20 // 1 MiB

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string, err error) {
	if err != nil {
		slog.Error(msg, "error", err)
	}
	writeJSON(w, status, map[string]string{"error": msg})
}
