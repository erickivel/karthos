package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestWriteJSON(t *testing.T) {
	w := httptest.NewRecorder()
	writeJSON(w, http.StatusOK, map[string]string{"key": "value"})

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var resp Response
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	data, ok := resp.Data.(map[string]any)
	require.True(t, ok)
	require.Equal(t, "value", data["key"])
}

func TestWriteError(t *testing.T) {
	w := httptest.NewRecorder()
	writeError(w, http.StatusNotFound, "not found")

	require.Equal(t, http.StatusNotFound, w.Code)
	require.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var resp ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, "not found", resp.Error)
	require.Equal(t, http.StatusNotFound, resp.Code)
}

func TestParsePaginationParams(t *testing.T) {
	t.Run("defaults when no params", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/test", nil)
		p := parsePaginationParams(r)
		require.Equal(t, 100, p.PageSize)
		require.Equal(t, 0, p.Offset)
	})

	t.Run("respects page_size", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/test?page_size=50", nil)
		p := parsePaginationParams(r)
		require.Equal(t, 50, p.PageSize)
	})

	t.Run("clamps page_size to max", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/test?page_size=5000", nil)
		p := parsePaginationParams(r)
		require.Equal(t, 1000, p.PageSize)
	})

	t.Run("ignores invalid page_size", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/test?page_size=abc", nil)
		p := parsePaginationParams(r)
		require.Equal(t, 100, p.PageSize)
	})

	t.Run("decodes valid next_token", func(t *testing.T) {
		token := encodePaginationToken(200)
		r := httptest.NewRequest(http.MethodGet, "/test?next_token="+token, nil)
		p := parsePaginationParams(r)
		require.Equal(t, 200, p.Offset)
	})

	t.Run("ignores invalid next_token", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/test?next_token=garbage", nil)
		p := parsePaginationParams(r)
		require.Equal(t, 0, p.Offset)
	})
}

func TestApplyPagination(t *testing.T) {
	items := make([]int, 250)
	for i := range items {
		items[i] = i
	}

	t.Run("first page", func(t *testing.T) {
		page, token := applyPagination(items, PaginationParams{PageSize: 100, Offset: 0})
		require.Len(t, page, 100)
		require.NotEmpty(t, token)
		require.Equal(t, 0, page[0])
	})

	t.Run("second page", func(t *testing.T) {
		page, token := applyPagination(items, PaginationParams{PageSize: 100, Offset: 100})
		require.Len(t, page, 100)
		require.NotEmpty(t, token)
		require.Equal(t, 100, page[0])
	})

	t.Run("last page", func(t *testing.T) {
		page, token := applyPagination(items, PaginationParams{PageSize: 100, Offset: 200})
		require.Len(t, page, 50)
		require.Empty(t, token)
		require.Equal(t, 200, page[0])
	})

	t.Run("offset beyond length", func(t *testing.T) {
		page, token := applyPagination(items, PaginationParams{PageSize: 100, Offset: 300})
		require.Empty(t, page)
		require.Empty(t, token)
	})
}

func TestWriteJSONPaginated(t *testing.T) {
	w := httptest.NewRecorder()
	writeJSONPaginated(w, http.StatusOK, map[string]int{"total": 5}, "abc123")

	require.Equal(t, http.StatusOK, w.Code)

	var resp PaginatedResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, "abc123", resp.NextToken)

	data, ok := resp.Data.(map[string]any)
	require.True(t, ok)
	require.Equal(t, float64(5), data["total"])
}

func TestWriteJSONPaginatedOmitsEmptyToken(t *testing.T) {
	w := httptest.NewRecorder()
	writeJSONPaginated(w, http.StatusOK, map[string]int{"total": 0}, "")

	raw := w.Body.String()
	require.NotContains(t, raw, "next_token")
}

func TestParseTimeParam(t *testing.T) {
	t.Run("empty returns zero", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/test", nil)
		ts, err := parseTimeParam(r, "from")
		require.NoError(t, err)
		require.True(t, ts.IsZero())
	})

	t.Run("valid RFC3339", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/test?from=2026-03-30T10:00:00Z", nil)
		ts, err := parseTimeParam(r, "from")
		require.NoError(t, err)
		require.Equal(t, time.Date(2026, 3, 30, 10, 0, 0, 0, time.UTC), ts)
	})

	t.Run("invalid format returns error", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/test?from=not-a-time", nil)
		_, err := parseTimeParam(r, "from")
		require.Error(t, err)
	})
}
