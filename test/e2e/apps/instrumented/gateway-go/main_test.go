package main

import (
	"bytes"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestErrorURLAddsForceErrorQuery(t *testing.T) {
	got := errorURL("http://service-a-python:8081/process?source=gateway")
	want := "http://service-a-python:8081/process?force_error=true&source=gateway"
	if got != want {
		t.Fatalf("errorURL() = %q, want %q", got, want)
	}
}

func TestStatusForResultsReportsGatewayError(t *testing.T) {
	results := []downstreamResponse{
		{Service: "service-a-python", Status: 200},
		{Service: "service-b-node", Status: 500},
	}
	if got := statusForResults(results); got != 502 {
		t.Fatalf("statusForResults() = %d, want 502", got)
	}
}

func TestRouteTemplateUsesBoundedKnownRoutes(t *testing.T) {
	tests := map[string]string{
		"/":              "/",
		"/error":         "/error",
		"/health":        "/health",
		"/tenant/abc123": "/{path}",
	}

	for path, want := range tests {
		if got := routeTemplate(path); got != want {
			t.Errorf("routeTemplate(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestLoggingMiddlewareDoesNotLogRequestInput(t *testing.T) {
	var output bytes.Buffer
	previousOutput := log.Writer()
	previousFlags := log.Flags()
	log.SetOutput(&output)
	log.SetFlags(0)
	t.Cleanup(func() {
		log.SetOutput(previousOutput)
		log.SetFlags(previousFlags)
	})

	handler := loggingMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "http://example.test/%0Aforged", nil))

	if got := output.String(); !strings.HasPrefix(got, "request completed duration=") || strings.Contains(got, "forged") {
		t.Fatalf("loggingMiddleware() log = %q, want request-independent output", got)
	}
}
