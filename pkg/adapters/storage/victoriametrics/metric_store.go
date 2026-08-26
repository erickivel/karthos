// Package victoriametrics implements the MetricStore interface using
// VictoriaMetrics' Prometheus-compatible HTTP API.
package victoriametrics

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/erickivel/oksg/pkg/backend/persistence"
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

// Query evaluates a provider-neutral MetricQuery through VictoriaMetrics. Public
// resolved queries use a precise range request plus, when needed, one instant
// request at a non-grid final evaluation point.
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
	step := queryStep(q)
	rangeEnd := to
	needsFinalEvaluation := q.Step > 0 && !to.Equal(from.Add(to.Sub(from)/step*step))
	if needsFinalEvaluation {
		rangeEnd = from.Add(to.Sub(from) / step * step)
	}

	samples, err := s.queryRange(ctx, promql, from, rangeEnd, step)
	if err != nil {
		return nil, err
	}
	if needsFinalEvaluation {
		final, err := s.queryInstant(ctx, promql, to, step)
		if err != nil {
			return nil, err
		}
		samples = mergeSamples(samples, final)
	}

	for i := range samples {
		samples[i].MetricName = q.MetricName
	}

	return samples, nil
}

func queryStep(q query.MetricQuery) time.Duration {
	if q.Step > 0 {
		return q.Step
	}
	return query.DefaultMetricStep
}

func (s *VMMetricStore) queryRange(ctx context.Context, promql string, from, to time.Time, step time.Duration) ([]metrics.MetricSample, error) {
	params := url.Values{
		"query":   {promql},
		"start":   {formatQueryTimestamp(from)},
		"end":     {formatQueryTimestamp(to)},
		"step":    {step.String()},
		"nocache": {"1"},
	}
	resp, err := s.doQuery(ctx, "/api/v1/query_range", params)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close() //nolint:errcheck
	return parseQueryRangeResponse(resp.Body)
}

func (s *VMMetricStore) queryInstant(ctx context.Context, promql string, at time.Time, step time.Duration) ([]metrics.MetricSample, error) {
	params := url.Values{
		"query":   {promql},
		"time":    {formatQueryTimestamp(at)},
		"step":    {step.String()},
		"nocache": {"1"},
	}
	resp, err := s.doQuery(ctx, "/api/v1/query", params)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close() //nolint:errcheck
	return parseInstantQueryResponse(resp.Body)
}

func (s *VMMetricStore) doQuery(ctx context.Context, path string, params url.Values) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.baseURL+path+"?"+params.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("vm query: %w", err)
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("vm query: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close() //nolint:errcheck
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("vm query: unexpected status %d: %s", resp.StatusCode, string(respBody))
	}
	return resp, nil
}

func formatQueryTimestamp(timestamp time.Time) string {
	return timestamp.UTC().Format(time.RFC3339Nano)
}

func mergeSamples(primary, additional []metrics.MetricSample) []metrics.MetricSample {
	merged := make([]metrics.MetricSample, 0, len(primary)+len(additional))
	seen := make(map[string]struct{}, len(primary)+len(additional))
	for _, sample := range append(primary, additional...) {
		key := metricSampleKey(sample)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		merged = append(merged, sample)
	}
	return merged
}

func metricSampleKey(sample metrics.MetricSample) string {
	labels, _ := json.Marshal(sample.Labels)
	return sample.MetricName + "|" + string(sample.EntityID) + "|" + string(sample.Unit) + "|" + sample.Timestamp.UTC().Format(time.RFC3339Nano) + "|" + string(labels)
}

// QueryCatalogStats issues two instant PromQL queries against VictoriaMetrics
// to aggregate per-(canonical metric, source) sample counts and last-seen
// timestamps over the lookback window. Uses VictoriaMetrics' `tlast_over_time`
// rollup for last-seen — this is VM-specific (not stock Prometheus) and
// matches our deployment target.
func (s *VMMetricStore) QueryCatalogStats(ctx context.Context, lookback time.Duration) ([]persistence.CatalogStat, error) {
	regex := catalogPromNameRegex()
	if regex == "" {
		return nil, nil
	}
	lookbackExpr := strconv.FormatInt(int64(lookback.Seconds()), 10) + "s"

	// Rollups (count_over_time / tlast_over_time) strip __name__ in
	// VictoriaMetrics / Prometheus, which would erase the metric dimension we
	// aggregate by. VM-specific `keep_metric_names` preserves __name__ across
	// the rollup so the `sum by (__name__, source)` aggregation can group on it.
	countQL := fmt.Sprintf(
		`sum by (__name__, source) (count_over_time({__name__=~"%s"}[%s]) keep_metric_names)`,
		regex, lookbackExpr,
	)
	lastSeenQL := fmt.Sprintf(
		`max by (__name__, source) (tlast_over_time({__name__=~"%s"}[%s]) keep_metric_names)`,
		regex, lookbackExpr,
	)

	countSeries, err := s.instantQuery(ctx, countQL)
	if err != nil {
		return nil, fmt.Errorf("vm catalog-stats count: %w", err)
	}
	lastSeenSeries, err := s.instantQuery(ctx, lastSeenQL)
	if err != nil {
		return nil, fmt.Errorf("vm catalog-stats last_seen: %w", err)
	}

	type key struct {
		metric string
		source string
	}
	type agg struct {
		count    int64
		lastSeen time.Time
	}
	buckets := map[key]*agg{}
	promNameToCanonical := catalogPromNameLookup()
	keyOf := func(promName, source string) key {
		canonical, ok := promNameToCanonical[promName]
		if !ok {
			canonical = semconvMetricName(promName)
		}
		return key{metric: canonical, source: source}
	}

	for _, v := range countSeries {
		k := keyOf(v.PromName, v.Source)
		b, ok := buckets[k]
		if !ok {
			b = &agg{}
			buckets[k] = b
		}
		b.count = max(b.count, int64(v.Value))
	}
	for _, v := range lastSeenSeries {
		k := keyOf(v.PromName, v.Source)
		b, ok := buckets[k]
		if !ok {
			b = &agg{}
			buckets[k] = b
		}
		lastSeen := time.Unix(int64(v.Value), 0).UTC()
		if lastSeen.After(b.lastSeen) {
			b.lastSeen = lastSeen
		}
	}

	grouped := map[string][]persistence.SourceStat{}
	for k, b := range buckets {
		grouped[k.metric] = append(grouped[k.metric], persistence.SourceStat{
			SourceID:    k.source,
			SampleCount: b.count,
			LastSeenAt:  b.lastSeen,
		})
	}
	result := make([]persistence.CatalogStat, 0, len(grouped))
	for name, sources := range grouped {
		result = append(result, persistence.CatalogStat{MetricName: name, BySource: sources})
	}
	return result, nil
}

// instantVectorEntry is one (metric, source, value) row from an instant query
// result. Returned to QueryCatalogStats which composes counts and last-seen.
type instantVectorEntry struct {
	PromName string
	Source   string
	Value    float64
}

// instantQuery issues a single PromQL instant query (`/api/v1/query`) and
// extracts the __name__ + source labels alongside the scalar value.
func (s *VMMetricStore) instantQuery(ctx context.Context, promql string) ([]instantVectorEntry, error) {
	params := url.Values{"query": {promql}}
	reqURL := s.baseURL + "/api/v1/query?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("vm instant: %w", err)
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("vm instant: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("vm instant: unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var parsed struct {
		Status string `json:"status"`
		Data   struct {
			ResultType string `json:"resultType"`
			Result     []struct {
				Metric map[string]string `json:"metric"`
				Value  []interface{}     `json:"value"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("vm instant: parse: %w", err)
	}
	if parsed.Status != "success" {
		return nil, fmt.Errorf("vm instant: response status %q", parsed.Status)
	}

	out := make([]instantVectorEntry, 0, len(parsed.Data.Result))
	for _, r := range parsed.Data.Result {
		if len(r.Value) != 2 {
			continue
		}
		valStr, ok := r.Value[1].(string)
		if !ok {
			continue
		}
		val, err := strconv.ParseFloat(valStr, 64)
		if err != nil {
			continue
		}
		out = append(out, instantVectorEntry{
			PromName: r.Metric["__name__"],
			Source:   r.Metric["source"],
			Value:    val,
		})
	}
	return out, nil
}

// catalogPromNameRegex returns a `|`-joined regex of every canonical metric's
// Prometheus-form name (dots → underscores). Computed once per call; the
// catalog is static so this allocation is cheap relative to the HTTP cost.
func catalogPromNameRegex() string {
	catalog := metrics.DefaultCatalog()
	if len(catalog) == 0 {
		return ""
	}
	names := make([]string, 0, len(catalog))
	for _, m := range catalog {
		names = append(names, promMetricName(m.Name))
		if metrics.IsWindowedMeanMetric(m.Name) {
			names = append(names, promMetricName(m.Name+"_sum"), promMetricName(m.Name+"_count"))
		}
	}
	return strings.Join(names, "|")
}

func catalogPromNameLookup() map[string]string {
	catalog := metrics.DefaultCatalog()
	lookup := make(map[string]string, len(catalog))
	for _, m := range catalog {
		lookup[promMetricName(m.Name)] = m.Name
		if metrics.IsWindowedMeanMetric(m.Name) {
			lookup[promMetricName(m.Name+"_sum")] = m.Name
			lookup[promMetricName(m.Name+"_count")] = m.Name
		}
	}
	return lookup
}

// samplesToPrometheusText converts MetricSamples to Prometheus text exposition
// format suitable for /api/v1/import/prometheus.
//
// Format: metric_name{label1="val1",label2="val2"} value timestamp_ms
const storedUnitLabel = "oksg_unit"

func samplesToPrometheusText(samples []metrics.MetricSample) []byte {
	var buf bytes.Buffer
	for _, s := range samples {
		buf.WriteString(promMetricName(s.MetricName))
		buf.WriteByte('{')
		buf.WriteString(`entity_id="`)
		buf.WriteString(string(s.EntityID))
		buf.WriteByte('"')
		if metrics.IsValidUnit(s.Unit) {
			buf.WriteString(`,` + storedUnitLabel + `="`)
			buf.WriteString(string(s.Unit))
			buf.WriteByte('"')
		}
		for k, v := range s.Labels {
			if k == storedUnitLabel {
				continue
			}
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

// buildPromQL constructs a PromQL selector from a MetricQuery.
func buildPromQL(q query.MetricQuery) string {
	if q.MetricName == "" {
		return ""
	}

	q = resolveQueryDefaults(q)
	if metrics.IsWindowedMeanMetric(q.MetricName) {
		sumIncrease := exactWindowIncrease(withMetricName(q, q.MetricName+"_sum"))
		countIncrease := exactWindowIncrease(withMetricName(q, q.MetricName+"_count"))
		return withFreshness(q, fmt.Sprintf("%s / ignoring(%s) (%s > 0)", sumIncrease, storedUnitLabel, countIncrease))
	}

	selector := metricSelector(q)
	if mapping, ok := metrics.OTLPSemconvMetrics[q.MetricName]; ok && mapping.Kind == metrics.MetricKindCounter {
		switch q.Aggregation {
		case metrics.AggregationLatest:
			return withFreshness(q, selector)
		case metrics.AggregationIncrease:
			return withFreshness(q, exactWindowIncrease(q))
		case metrics.AggregationWindowRate:
			return withFreshness(q, fmt.Sprintf("(%s / %d)", exactWindowIncrease(q), int64(q.Window.Seconds())))
		case metrics.AggregationActiveRate:
			return withFreshness(q, activeCounterRate(q))
		}
	}
	return withFreshness(q, selector)
}

func withFreshness(q query.MetricQuery, expression string) string {
	if q.MaxSampleAge <= 0 {
		return expression
	}
	matching := ""
	if metrics.IsWindowedMeanMetric(q.MetricName) {
		matching = fmt.Sprintf(" ignoring(%s)", storedUnitLabel)
	}
	return fmt.Sprintf("(%s) and%s last_over_time(%s[%s])", expression, matching, freshnessSelector(q), durationExpr(q.MaxSampleAge))
}

func freshnessSelector(q query.MetricQuery) string {
	if metrics.IsWindowedMeanMetric(q.MetricName) {
		return metricSelector(withMetricName(q, q.MetricName+"_count"))
	}
	return metricSelector(q)
}

func metricSelector(q query.MetricQuery) string {
	return metricSelectorWithMatchers(q)
}

func metricSelectorWithMatchers(q query.MetricQuery, extraMatchers ...string) string {
	var matchers []string
	if len(q.EntityIDs) > 0 {
		ids := make([]string, 0, len(q.EntityIDs))
		for _, id := range q.EntityIDs {
			ids = append(ids, regexp.QuoteMeta(string(id)))
		}
		matchers = append(matchers, fmt.Sprintf(`entity_id=~"^(?:%s)$"`, strings.Join(ids, "|")))
	}
	if q.EdgeSourceID != "" {
		matchers = append(matchers, fmt.Sprintf(`edge_client_id="%s"`, escapeLabelValue(string(q.EdgeSourceID))))
	}
	if q.EdgeTargetID != "" {
		matchers = append(matchers, fmt.Sprintf(`edge_server_id="%s"`, escapeLabelValue(string(q.EdgeTargetID))))
	}
	matchers = append(matchers, extraMatchers...)
	if len(matchers) == 0 {
		return promMetricName(q.MetricName)
	}
	return fmt.Sprintf(`%s{%s}`, promMetricName(q.MetricName), strings.Join(matchers, ","))
}

func resolveQueryDefaults(q query.MetricQuery) query.MetricQuery {
	if q.Aggregation != "" {
		return q
	}
	metric, ok := metrics.CatalogByName()[q.MetricName]
	if !ok {
		return q
	}
	resolved, err := metrics.ResolveAggregation(metric, "", "")
	if err == nil {
		q.Aggregation, q.Window, q.Unit = resolved.Mode, resolved.Window, resolved.Unit
	}
	return q
}

func exactWindowIncrease(q query.MetricQuery) string {
	window := durationExpr(q.Window)
	historyWindow := durationExpr(2 * q.Window)
	cumulative := metricSelectorWithMatchers(q, `temporality!="delta"`)
	delta := metricSelectorWithMatchers(q, `temporality="delta"`)
	gate := fmt.Sprintf("(count_over_time(%s[%s]) >= 2) and ((tlast_over_time(%s[%s]) - tfirst_over_time(%s[%s])) >= %d)", cumulative, window, cumulative, historyWindow, cumulative, historyWindow, int64(q.Window.Seconds()))
	return fmt.Sprintf("((increase(%s[%s]) and %s) or sum_over_time(%s[%s]))", cumulative, window, gate, delta, window)
}

func activeCounterRate(q query.MetricQuery) string {
	window := durationExpr(q.Window)
	cumulative := metricSelectorWithMatchers(q, `temporality!="delta"`)
	return fmt.Sprintf("(rate(%s[%s]) and (count_over_time(%s[%s]) >= 2))", cumulative, window, cumulative, window)
}

func durationExpr(window time.Duration) string {
	return strconv.FormatInt(int64(window.Seconds()), 10) + "s"
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
type querySeries struct {
	Metric map[string]string `json:"metric"`
	Values [][]interface{}   `json:"values"`
}

type queryRangeResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string        `json:"resultType"`
		Result     []querySeries `json:"result"`
	} `json:"data"`
}

type instantQueryResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Metric map[string]string `json:"metric"`
			Value  []interface{}     `json:"value"`
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

	return parseQuerySeries(resp.Data.Result), nil
}

// parseInstantQueryResponse parses a Prometheus /api/v1/query vector response
// into MetricSamples.
func parseInstantQueryResponse(body io.Reader) ([]metrics.MetricSample, error) {
	var resp instantQueryResponse
	if err := json.NewDecoder(body).Decode(&resp); err != nil {
		return nil, fmt.Errorf("vm parse response: %w", err)
	}
	if resp.Status != "success" {
		return nil, fmt.Errorf("vm query: response status %q", resp.Status)
	}
	series := make([]querySeries, 0, len(resp.Data.Result))
	for _, result := range resp.Data.Result {
		series = append(series, querySeries{Metric: result.Metric, Values: [][]interface{}{result.Value}})
	}
	return parseQuerySeries(series), nil
}

func parseQuerySeries(seriesList []querySeries) []metrics.MetricSample {
	var samples []metrics.MetricSample
	for _, series := range seriesList {
		metricName := semconvMetricName(series.Metric["__name__"])
		entityID := entities.EntityID(series.Metric["entity_id"])
		unit := metrics.Unit(series.Metric[storedUnitLabel])
		if !metrics.IsValidUnit(unit) {
			unit = ""
		}

		// Build labels excluding __name__ and entity_id.
		labels := make(map[string]string)
		for k, v := range series.Metric {
			if k == "__name__" || k == "entity_id" || k == storedUnitLabel {
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
			if math.IsInf(val, 0) || math.IsNaN(val) {
				continue
			}

			samples = append(samples, metrics.MetricSample{
				MetricName: metricName,
				EntityID:   entityID,
				Value:      val,
				Timestamp:  timestampFromUnixSeconds(ts),
				Labels:     labels,
				Unit:       unit,
			})
		}
	}

	return samples
}

func timestampFromUnixSeconds(seconds float64) time.Time {
	whole, fraction := math.Modf(seconds)
	nanos := int64(math.Round(fraction * float64(time.Second)))
	if nanos == int64(time.Second) {
		whole++
		nanos = 0
	}
	return time.Unix(int64(whole), nanos).UTC()
}
