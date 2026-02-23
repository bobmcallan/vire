package server

import (
	"encoding/json"
	"net/http"
	"strings"
)

// ErrorResponse is the standard error format for REST API responses.
type ErrorResponse struct {
	Error string `json:"error"`
	Code  string `json:"code,omitempty"`
}

// WriteJSON writes a JSON response with the given status code.
func WriteJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(data)
}

// WriteError writes a JSON error response.
func WriteError(w http.ResponseWriter, statusCode int, message string) {
	WriteJSON(w, statusCode, ErrorResponse{Error: message})
}

// WriteErrorWithCode writes a JSON error response with an error code.
func WriteErrorWithCode(w http.ResponseWriter, statusCode int, message, code string) {
	WriteJSON(w, statusCode, ErrorResponse{Error: message, Code: code})
}

// RequireMethod validates the HTTP method and returns true if it matches.
// If it doesn't match, it writes a 405 response and returns false.
func RequireMethod(w http.ResponseWriter, r *http.Request, methods ...string) bool {
	for _, m := range methods {
		if r.Method == m {
			return true
		}
	}
	w.Header().Set("Allow", strings.Join(methods, ", "))
	WriteError(w, http.StatusMethodNotAllowed, "Method not allowed")
	return false
}

// DecodeJSON reads and decodes JSON from the request body into v.
// Returns false and writes a 400 error if decoding fails.
func DecodeJSON(w http.ResponseWriter, r *http.Request, v interface{}) bool {
	if r.Body == nil {
		WriteError(w, http.StatusBadRequest, "Request body is required")
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1MB limit
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid JSON: "+err.Error())
		return false
	}
	return true
}

// UnmarshalArrayParam handles MCP array parameters that may contain either native
// JSON objects or string-encoded JSON objects. MCP proxies often send array items
// as strings ("[\"{ ... }\", \"{ ... }\"]") instead of objects ("[{ ... }, { ... }]").
// raw is the JSON-encoded array, dest is a pointer to the target slice (e.g. *[]MyStruct).
func UnmarshalArrayParam(raw json.RawMessage, dest interface{}) error {
	// Try native array of objects first.
	if err := json.Unmarshal(raw, dest); err == nil {
		return nil
	}

	// Fall back to array of string-encoded objects.
	var strings []string
	if err := json.Unmarshal(raw, &strings); err != nil {
		return json.Unmarshal(raw, dest) // return the original error
	}

	// Reconstruct a JSON array from the unwrapped strings and unmarshal.
	parts := make([]json.RawMessage, len(strings))
	for i, s := range strings {
		parts[i] = json.RawMessage(s)
	}
	rebuilt, err := json.Marshal(parts)
	if err != nil {
		return err
	}
	return json.Unmarshal(rebuilt, dest)
}

// PathParam extracts a path parameter from the URL path.
// For a pattern like /api/portfolios/{name}/review, calling PathParam(r, "/api/portfolios/", "/review")
// extracts the {name} part.
func PathParam(r *http.Request, prefix, suffix string) string {
	path := r.URL.Path
	if !strings.HasPrefix(path, prefix) {
		return ""
	}
	rest := path[len(prefix):]
	if suffix != "" {
		idx := strings.Index(rest, suffix)
		if idx < 0 {
			return rest
		}
		return rest[:idx]
	}
	// No suffix — return up to the next /
	if idx := strings.Index(rest, "/"); idx >= 0 {
		return rest[:idx]
	}
	return rest
}
