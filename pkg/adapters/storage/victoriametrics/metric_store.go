// Package victoriametrics implements the MetricStore interface using
// VictoriaMetrics' Prometheus-compatible HTTP API.
package victoriametrics

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/erickivel/oksg/pkg/backend/query"
	"github.com/erickivel/oksg/pkg/core/entities"
	"github.com/erickivel/oksg/pkg/core/metrics"
)

// VMMetricStore implements persistence.MetricStore using VictoriaMetrics HTTP API.
type VMMetricStore struct {
	baseURL    string
	httpClient *http.Client
}

// NewVMMetricStore creates a new VMMetricStore pointed at the given VictoriaMetrics URL.
func NewVMMetricStore(baseURL string) *VMMetricStore {
	return &VMMetricStore{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Ping checks that VictoriaMetrics is reachable.
func (s *VMMetricStore) Ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.baseURL+"/health", nil)
	if err != nil {
		return fmt.Errorf("vm ping: %w", err)
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("vm ping: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("vm ping: unexpected status %d", resp.StatusCode)
	}
	return nil
}

// Close closes idle HTTP connections.
func (s *VMMetricStore) Close() error {
	s.httpClient.CloseIdleConnections()
	return nil
}

// Write converts MetricSamples to Prometheus text format and POSTs them to
// VictoriaMetrics' /api/v1/import/prometheus endpoint.
func (s *VMMetricStore) Write(ctx context.Context, samples []metrics.MetricSample) error {
	if len(samples) == 0 {
		return nil
	}

	body := samplesToPrometheusText(samples)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL+"/api/v1/import/prometheus", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("vm write: %w", err)
	}
	req.Header.Set("Content-Type", "text/plain")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("vm write: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("vm write: unexpected status %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// Query builds a PromQL query from MetricQuery and calls VictoriaMetrics'
// /api/v1/query_range endpoint. Returns parsed MetricSamples.
func (s *VMMetricStore) Query(ctx context.Context, q query.MetricQuery) ([]metrics.MetricSample, error) {
	promql := buildPromQL(q)
	if promql == "" {
		return nil, nil
	}

	from := q.From
	to := q.To
	if from.IsZero() {
		from = time.Now().Add(-1 * time.Hour)
	}
	if to.IsZero() {
		to = time.Now()
	}

	params := url.Values{
		"query": {promql},
		"start": {strconv.FormatInt(from.Unix(), 10)},
		"end":   {strconv.FormatInt(to.Unix(), 10)},
		"step":  {"15s"},
	}

	reqURL := s.baseURL + "/api/v1/query_range?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("vm query: %w", err)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("vm query: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("vm query: unexpected status %d: %s", resp.StatusCode, string(respBody))
	}

	samples, err := parseQueryRangeResponse(resp.Body)
	if err != nil {
		return nil, err
	}

	for i := range samples {
		samples[i].MetricName = q.MetricName
	}

	return samples, nil
}

// samplesToPrometheusText converts MetricSamples to Prometheus text exposition
// format suitable for /api/v1/import/prometheus.
//
// Format: metric_name{label1="val1",label2="val2"} value timestamp_ms
func samplesToPrometheusText(samples []metrics.MetricSample) []byte {
	var buf bytes.Buffer
	for _, s := range samples {
		buf.WriteString(promMetricName(s.MetricName))
		buf.WriteByte('{')
		buf.WriteString(`entity_id="`)
		buf.WriteString(string(s.EntityID))
		buf.WriteByte('"')
		for k, v := range s.Labels {
			buf.WriteByte(',')
			buf.WriteString(k)
			buf.WriteString(`="`)
			buf.WriteString(escapeLabelValue(v))
			buf.WriteByte('"')
		}
		buf.WriteString("} ")
		buf.WriteString(strconv.FormatFloat(s.Value, 'f', -1, 64))
		buf.WriteByte(' ')
		buf.WriteString(strconv.FormatInt(s.Timestamp.UnixMilli(), 10))
		buf.WriteByte('\n')
	}
	return buf.Bytes()
}

// escapeLabelValue escapes backslash, double-quote, and newline for Prometheus labels.
func escapeLabelValue(v string) string {
	v = strings.ReplaceAll(v, `\`, `\\`)
	v = strings.ReplaceAll(v, `"`, `\"`)
	v = strings.ReplaceAll(v, "\n", `\n`)
	return v
}

// histogramDerivedMeanMetrics maps a canonical metric name to the pair of cumulative
// counters emitted by the normalizer (see pkg/backend/ingest/normalizer.go histogram
// path). A query for the base name is served as rate(sum)/rate(count), yielding a
// rate-weighted mean in the metric's native units.
var histogramDerivedMeanMetrics = map[string]struct{}{
	"http.server.request.duration":    {},
	"rpc.server.duration":             {},
	"servicegraph.request.duration":   {},
	"servicegraph.request.body.size":  {},
	"servicegraph.response.body.size": {},
}

// buildPromQL constructs a PromQL selector from a MetricQuery.
func buildPromQL(q query.MetricQuery) string {
	if q.MetricName == "" {
		return ""
	}

	if _, ok := histogramDerivedMeanMetrics[q.MetricName]; ok {
		sumSelector := metricSelector(withMetricName(q, q.MetricName+"_sum"))
		countSelector := metricSelector(withMetricName(q, q.MetricName+"_count"))
		return fmt.Sprintf("rate(%s[1m]) / clamp_min(rate(%s[1m]), 1)", sumSelector, countSelector)
	}

	selector := metricSelector(q)
	switch q.MetricName {
	case "http.server.request.count",
		"rpc.server.request.count",
		"servicegraph.request.count",
		"servicegraph.request.error.count",
		"servicegraph.grpc.request.message.count",
		"servicegraph.grpc.response.message.count",
		"servicegraph.tcp.connection.open.count",
		"servicegraph.tcp.connection.close.count":
		return fmt.Sprintf("rate(%s[1m])", selector)
	case "servicegraph.request.error.ratio":
		errorRate := fmt.Sprintf("rate(%s[1m])", metricSelector(withMetricName(q, "servicegraph.request.error.count")))
		requestRate := fmt.Sprintf("rate(%s[1m])", metricSelector(withMetricName(q, "servicegraph.request.count")))
		return fmt.Sprintf("%s / clamp_min(%s, 1e-9)", errorRate, requestRate)
	case "oksg.container.health.status":
		// Per-container composite. Priority order: restarting > not_ready > healthy.
		// Each clause uses the `(predicate) * 0 + N` idiom so a clause emits
		// a sample with value N only when its predicate matches, and emits
		// no sample otherwise. PromQL `or` then keeps the first matching
		// clause per series (clause ordering = priority). The `bool`
		// modifier would have collapsed false predicates to 0-valued samples
		// instead of dropping them, breaking the priority cascade.
		// Samples are keyed by the same entity_id (container ID), so no
		// cross-entity join is needed.
		ready := metricSelector(withMetricName(q, "k8s.container.ready"))
		restarts := metricSelector(withMetricName(q, "k8s.container.restarts"))
		return fmt.Sprintf(
			"(rate(%s[5m]) > 0) * 0 + 3 or (%s == 0) * 0 + 2 or (%s == 1) * 0 + 1",
			restarts, ready, ready,
		)
	case "oksg.pod.health.status":
		// Per-pod composite, derived from k8s.pod.phase only (1=Pending,
		// 2=Running, 3=Succeeded, 4=Failed, 5=Unknown). Public enum:
		// 1=healthy, 4=pending, 5=succeeded, 6=failed, 7=unknown. The
		// running_not_ready (2) and restarting (3) states are observable at
		// container scope; rolling them up to pod scope requires a pod_id
		// label on container metrics — see plan.
		phase := metricSelector(withMetricName(q, "k8s.pod.phase"))
		return fmt.Sprintf(
			"(%s == 2) * 0 + 1 or (%s == 1) * 0 + 4 or (%s == 3) * 0 + 5 or (%s == 4) * 0 + 6 or (%s == 5) * 0 + 7",
			phase, phase, phase, phase, phase,
		)
	default:
		return selector
	}
}

func metricSelector(q query.MetricQuery) string {
	var matchers []string
	if len(q.EntityIDs) > 0 {
		ids := make([]string, 0, len(q.EntityIDs))
		for _, id := range q.EntityIDs {
			ids = append(ids, string(id))
		}
		matchers = append(matchers, fmt.Sprintf(`entity_id=~"%s"`, strings.Join(ids, "|")))
	}
	if q.EdgeSourceID != "" {
		matchers = append(matchers, fmt.Sprintf(`edge_client_id="%s"`, q.EdgeSourceID))
	}
	if q.EdgeTargetID != "" {
		matchers = append(matchers, fmt.Sprintf(`edge_server_id="%s"`, q.EdgeTargetID))
	}
	if len(matchers) == 0 {
		return promMetricName(q.MetricName)
	}
	return fmt.Sprintf(`%s{%s}`, promMetricName(q.MetricName), strings.Join(matchers, ","))
}

func withMetricName(q query.MetricQuery, metricName string) query.MetricQuery {
	q.MetricName = metricName
	return q
}

func promMetricName(metricName string) string {
	return strings.ReplaceAll(metricName, ".", "_")
}

func semconvMetricName(metricName string) string {
	return strings.ReplaceAll(metricName, "_", ".")
}

// queryRangeResponse represents the Prometheus query_range API response.
type queryRangeResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Metric map[string]string `json:"metric"`
			Values [][]interface{}   `json:"values"`
		} `json:"result"`
	} `json:"data"`
}

// parseQueryRangeResponse parses a Prometheus /api/v1/query_range JSON response
// into MetricSamples.
func parseQueryRangeResponse(body io.Reader) ([]metrics.MetricSample, error) {
	var resp queryRangeResponse
	if err := json.NewDecoder(body).Decode(&resp); err != nil {
		return nil, fmt.Errorf("vm parse response: %w", err)
	}

	if resp.Status != "success" {
		return nil, fmt.Errorf("vm query: response status %q", resp.Status)
	}

	var samples []metrics.MetricSample
	for _, series := range resp.Data.Result {
		metricName := semconvMetricName(series.Metric["__name__"])
		entityID := entities.EntityID(series.Metric["entity_id"])

		// Build labels excluding __name__ and entity_id.
		labels := make(map[string]string)
		for k, v := range series.Metric {
			if k == "__name__" || k == "entity_id" {
				continue
			}
			labels[k] = v
		}

		for _, valuePair := range series.Values {
			if len(valuePair) != 2 {
				continue
			}
			ts, ok := valuePair[0].(float64)
			if !ok {
				continue
			}
			valStr, ok := valuePair[1].(string)
			if !ok {
				continue
			}
			val, err := strconv.ParseFloat(valStr, 64)
			if err != nil {
				continue
			}

			samples = append(samples, metrics.MetricSample{
				MetricName: metricName,
				EntityID:   entityID,
				Value:      val,
				Timestamp:  time.Unix(int64(ts), 0),
				Labels:     labels,
			})
		}
	}

	return samples, nil
}
