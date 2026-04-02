package api

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestServerStartsAndServesStatus(t *testing.T) {
	srv := newTestServer(t, Config{})
	defer srv.cancel()

	resp := doGet(t, srv, "/api/v1/status")
	defer func() { require.NoError(t, resp.Body.Close()) }()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "application/json", resp.Header.Get("Content-Type"))
}
