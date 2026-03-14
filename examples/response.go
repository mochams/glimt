package main

import (
	"encoding/json"
	"net/http"
)

// APIResponse is a generic response envelope.
type APIResponse struct {
	Data  any    `json:"data,omitempty"`
	Error string `json:"error,omitempty"`
	Count int    `json:"count,omitempty"`
}

// writeJSON writes a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// writeError writes a JSON error response.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, APIResponse{Error: msg})
}

// writeData writes a JSON data response.
func writeData(w http.ResponseWriter, status int, data any) {
	writeJSON(w, status, APIResponse{Data: data})
}

// writeList writes a JSON list response with a count.
func writeList(w http.ResponseWriter, status int, data any, count int) {
	writeJSON(w, status, APIResponse{Data: data, Count: count})
}

// writeValidationError writes a JSON response for validation errors.
func writeValidationError(w http.ResponseWriter, errors []FieldError) {
	writeJSON(w, http.StatusBadRequest, APIResponse{
		Error: "validation failed",
		Data:  errors,
	})
}
