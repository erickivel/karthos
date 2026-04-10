package api

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// Response is the standard JSON envelope for successful API responses.
type Response struct {
	Data any `json:"data"`
}

// ErrorResponse is the standard JSON envelope for error API responses.
type ErrorResponse struct {
	Error string `json:"error"`
	Code  int    `json:"code"`
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(Response{Data: data})
}

func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(ErrorResponse{Error: message, Code: status})
}

const (
	defaultPageSize = 100
	maxPageSize     = 1000
)

// PaginationParams holds parsed cursor-based pagination parameters.
type PaginationParams struct {
	PageSize int
	Offset   int
}

// PaginatedResponse wraps paginated list responses.
type PaginatedResponse struct {
	Data      any    `json:"data"`
	NextToken string `json:"next_token,omitempty"`
}

func parsePaginationParams(r *http.Request) PaginationParams {
	pageSize := defaultPageSize
	if raw := r.URL.Query().Get("page_size"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			pageSize = n
		}
	}
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}

	var offset int
	if tok := r.URL.Query().Get("next_token"); tok != "" {
		if decoded, err := base64.StdEncoding.DecodeString(tok); err == nil {
			if n, err := strconv.Atoi(string(decoded)); err == nil && n > 0 {
				offset = n
			}
		}
	}
	return PaginationParams{PageSize: pageSize, Offset: offset}
}

func encodePaginationToken(offset int) string {
	return base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%d", offset)))
}

func applyPagination[T any](items []T, params PaginationParams) (page []T, nextToken string) {
	if params.Offset >= len(items) {
		return []T{}, ""
	}
	end := params.Offset + params.PageSize
	if end >= len(items) {
		return items[params.Offset:], ""
	}
	return items[params.Offset:end], encodePaginationToken(end)
}

func writeJSONPaginated(w http.ResponseWriter, status int, data any, nextToken string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(PaginatedResponse{Data: data, NextToken: nextToken})
}

func parseTimeParam(r *http.Request, name string) (time.Time, error) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339, raw)
}
