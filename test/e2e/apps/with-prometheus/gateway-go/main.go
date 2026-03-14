package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	requestCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total HTTP requests handled by gateway-go.",
		},
		[]string{"method", "path", "status"},
	)
	requestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request duration in seconds.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)
)

type downstreamResponse struct {
	Service string `json:"service"`
	Status  int    `json:"status"`
	Body    string `json:"body"`
	Error   string `json:"error,omitempty"`
}

func init() {
	prometheus.MustRegister(requestCounter, requestDuration)
}

func main() {
	serviceAURL := envOrDefault("SERVICE_A_URL", "http://service-a-python.oksg-demo.svc.cluster.local:8081/process")
	serviceBURL := envOrDefault("SERVICE_B_URL", "http://service-b-node.oksg-demo.svc.cluster.local:8082/compute")

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
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
		Handler:           metricsMiddleware(loggingMiddleware(mux)),
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
	defer resp.Body.Close()

	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return downstreamResponse{Service: serviceName, Status: resp.StatusCode, Error: err.Error()}
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

func metricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/metrics" {
			next.ServeHTTP(w, r)
			return
		}

		start := time.Now()
		recorder := &statusRecorder{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(recorder, r)

		status := http.StatusText(recorder.statusCode)
		requestCounter.WithLabelValues(r.Method, r.URL.Path, status).Inc()
		requestDuration.WithLabelValues(r.Method, r.URL.Path).Observe(time.Since(start).Seconds())
	})
}

type statusRecorder struct {
	http.ResponseWriter
	statusCode int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.statusCode = code
	r.ResponseWriter.WriteHeader(code)
}
