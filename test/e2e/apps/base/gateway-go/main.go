package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"
)

type downstreamResponse struct {
	Service string `json:"service"`
	Status  int    `json:"status"`
	Body    string `json:"body"`
	Error   string `json:"error,omitempty"`
}

func main() {
	serviceAURL := envOrDefault("SERVICE_A_URL", "http://service-a-python.oksg-demo.svc.cluster.local:8081/process")
	serviceBURL := envOrDefault("SERVICE_B_URL", "http://service-b-node.oksg-demo.svc.cluster.local:8082/compute")

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		results := []downstreamResponse{
			callDownstream("service-a-python", serviceAURL),
			callDownstream("service-b-node", serviceBURL),
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"service": "gateway-go",
			"path":    r.URL.Path,
			"results": results,
		})
	})

	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{
			"service": "gateway-go",
			"status":  "ok",
		})
	})

	server := &http.Server{
		Addr:              ":8080",
		Handler:           loggingMiddleware(mux),
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("gateway-go listening on %s", server.Addr)
	log.Fatal(server.ListenAndServe())
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func callDownstream(serviceName, url string) downstreamResponse {
	resp, err := http.Get(url) //nolint:gosec // test application request path
	if err != nil {
		return downstreamResponse{Service: serviceName, Error: err.Error()}
	}

	var payload map[string]any
	decodeErr := json.NewDecoder(resp.Body).Decode(&payload)
	closeErr := resp.Body.Close()
	if decodeErr != nil {
		if closeErr != nil {
			return downstreamResponse{
				Service: serviceName,
				Status:  resp.StatusCode,
				Error:   decodeErr.Error() + "; close body: " + closeErr.Error(),
			}
		}
		return downstreamResponse{Service: serviceName, Status: resp.StatusCode, Error: decodeErr.Error()}
	}
	if closeErr != nil {
		return downstreamResponse{Service: serviceName, Status: resp.StatusCode, Error: closeErr.Error()}
	}

	body, _ := json.Marshal(payload)
	return downstreamResponse{Service: serviceName, Status: resp.StatusCode, Body: string(body)}
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s duration=%s", r.Method, r.URL.Path, time.Since(start))
	})
}
