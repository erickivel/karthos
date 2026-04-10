package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/erickivel/oksg/pkg/backend/persistence"
	"github.com/erickivel/oksg/pkg/core/entities"
)

func seedEntities(t *testing.T) *persistence.InMemoryEntityRegistry {
	t.Helper()
	reg := persistence.NewInMemoryEntityRegistry()
	ctx := context.Background()

	require.NoError(t, reg.UpsertEntity(ctx, entities.NewPod("cluster", "default", "pod-a", map[string]string{
		"cluster": "cluster", "namespace": "default", "name": "pod-a",
	})))
	require.NoError(t, reg.UpsertEntity(ctx, entities.NewPod("cluster", "kube-system", "pod-b", map[string]string{
		"cluster": "cluster", "namespace": "kube-system", "name": "pod-b",
	})))
	require.NoError(t, reg.UpsertEntity(ctx, entities.NewService("cluster", "default", "svc-a", nil, map[string]string{
		"cluster": "cluster", "namespace": "default", "name": "svc-a",
	})))
	return reg
}

func TestListEntitiesAll(t *testing.T) {
	reg := seedEntities(t)
	srv := newTestServer(t, Config{EntityRegistry: reg})
	defer srv.cancel()

	resp := doGet(t, srv, "/api/v1/entities")
	defer func() { require.NoError(t, resp.Body.Close()) }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	data := parsePaginatedData(t, resp)
	require.EqualValues(t, 3, data["total"])
}

func TestListEntitiesFilterByKind(t *testing.T) {
	reg := seedEntities(t)
	srv := newTestServer(t, Config{EntityRegistry: reg})
	defer srv.cancel()

	resp := doGet(t, srv, "/api/v1/entities?kind=pod")
	defer func() { require.NoError(t, resp.Body.Close()) }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	data := parsePaginatedData(t, resp)
	require.EqualValues(t, 2, data["total"])
}

func TestListEntitiesFilterByNamespace(t *testing.T) {
	reg := seedEntities(t)
	srv := newTestServer(t, Config{EntityRegistry: reg})
	defer srv.cancel()

	resp := doGet(t, srv, "/api/v1/entities?namespace=default")
	defer func() { require.NoError(t, resp.Body.Close()) }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	data := parsePaginatedData(t, resp)
	require.EqualValues(t, 2, data["total"])
}

func TestListEntitiesPagination(t *testing.T) {
	reg := seedEntities(t)
	srv := newTestServer(t, Config{EntityRegistry: reg})
	defer srv.cancel()

	resp := doGet(t, srv, "/api/v1/entities?page_size=2")
	defer func() { require.NoError(t, resp.Body.Close()) }()

	data, nextToken := parsePaginatedDataWithToken(t, resp)
	require.EqualValues(t, 3, data["total"])
	ents := data["entities"].([]any)
	require.Len(t, ents, 2)
	require.NotEmpty(t, nextToken)

	resp2 := doGet(t, srv, "/api/v1/entities?page_size=2&next_token="+nextToken)
	defer func() { require.NoError(t, resp2.Body.Close()) }()
	data2, nextToken2 := parsePaginatedDataWithToken(t, resp2)
	ents2 := data2["entities"].([]any)
	require.Len(t, ents2, 1)
	require.Empty(t, nextToken2)
}

func TestGetEntityByID(t *testing.T) {
	reg := seedEntities(t)
	srv := newTestServer(t, Config{EntityRegistry: reg})
	defer srv.cancel()

	podA := entities.NewPod("cluster", "default", "pod-a", nil)
	resp := doGet(t, srv, "/api/v1/entities/"+string(podA.ID()))
	defer func() { require.NoError(t, resp.Body.Close()) }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	rawBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var envelope Response
	require.NoError(t, json.Unmarshal(rawBody, &envelope))

	data := envelope.Data.(map[string]any)
	require.Equal(t, string(podA.ID()), data["id"])
}

func TestGetEntityByIDReturns404(t *testing.T) {
	reg := seedEntities(t)
	srv := newTestServer(t, Config{EntityRegistry: reg})
	defer srv.cancel()

	resp := doGet(t, srv, "/api/v1/entities/nonexistent-id")
	defer func() { require.NoError(t, resp.Body.Close()) }()
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}
