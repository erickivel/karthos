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
