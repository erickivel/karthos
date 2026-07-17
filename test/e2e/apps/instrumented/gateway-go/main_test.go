package main

import (
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
