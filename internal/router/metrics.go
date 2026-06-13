package router

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

type metricsStore struct {
	mu     sync.Mutex
	series map[metricLabels]*metricValues
}

type metricLabels struct {
	CallerID          string
	CallerUser        string
	CallerProject     string
	CallerEnvironment string
	TokenID           string
	ModelGroup        string
	TargetProvider    string
	TargetModel       string
	Status            int
}

type metricValues struct {
	Requests     int64
	Errors       int64
	InputTokens  int64
	OutputTokens int64
	TotalTokens  int64
	LatencyMS    int64
	CacheHits    int64
	CacheMisses  int64
	Attempts     int64
	Fallbacks    int64
}

func newMetricsStore() *metricsStore {
	return &metricsStore{series: map[metricLabels]*metricValues{}}
}

func (m *metricsStore) Observe(rec logRecord) {
	if m == nil || rec.CallerID == "" {
		return
	}
	labels := metricLabels{
		CallerID:          rec.CallerID,
		CallerUser:        rec.CallerUser,
		CallerProject:     rec.CallerProject,
		CallerEnvironment: rec.CallerEnvironment,
		TokenID:           rec.TokenID,
		ModelGroup:        defaultString(rec.ResolvedGroup, rec.RequestedModel),
		TargetProvider:    rec.TargetProvider,
		TargetModel:       rec.TargetModel,
		Status:            rec.Status,
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	values := m.series[labels]
	if values == nil {
		values = &metricValues{}
		m.series[labels] = values
	}
	values.Requests++
	if rec.Status >= 400 {
		values.Errors++
	}
	values.InputTokens += int64(rec.Usage.InputTokens)
	values.OutputTokens += int64(rec.Usage.OutputTokens)
	values.TotalTokens += int64(rec.Usage.TotalTokens)
	values.LatencyMS += rec.LatencyMS
	switch rec.Cache {
	case "hit":
		values.CacheHits++
	case "miss":
		values.CacheMisses++
	}
	values.Attempts += int64(rec.Attempts)
	if rec.FallbackUsed {
		values.Fallbacks++
	}
}

func (m *metricsStore) Prometheus() string {
	if m == nil {
		return ""
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	keys := make([]metricLabels, 0, len(m.series))
	for labels := range m.series {
		keys = append(keys, labels)
	}
	sort.Slice(keys, func(i, j int) bool {
		return labelKey(keys[i]) < labelKey(keys[j])
	})
	var b strings.Builder
	writeHelp(&b, "smart_llmrouter_requests_total", "Total authorized router requests.")
	writeHelp(&b, "smart_llmrouter_errors_total", "Total authorized router requests ending in HTTP error status.")
	writeHelp(&b, "smart_llmrouter_input_tokens_total", "Total input tokens reported or estimated.")
	writeHelp(&b, "smart_llmrouter_output_tokens_total", "Total output tokens reported.")
	writeHelp(&b, "smart_llmrouter_tokens_total", "Total tokens reported.")
	writeHelp(&b, "smart_llmrouter_latency_ms_sum", "Sum of request latency in milliseconds.")
	writeHelp(&b, "smart_llmrouter_cache_hits_total", "Total cache hits.")
	writeHelp(&b, "smart_llmrouter_cache_misses_total", "Total cache misses.")
	writeHelp(&b, "smart_llmrouter_upstream_attempts_total", "Total upstream attempts.")
	writeHelp(&b, "smart_llmrouter_fallbacks_total", "Total requests that used fallback targets.")
	for _, labels := range keys {
		values := m.series[labels]
		labelText := prometheusLabels(labels)
		writeMetric(&b, "smart_llmrouter_requests_total", labelText, values.Requests)
		writeMetric(&b, "smart_llmrouter_errors_total", labelText, values.Errors)
		writeMetric(&b, "smart_llmrouter_input_tokens_total", labelText, values.InputTokens)
		writeMetric(&b, "smart_llmrouter_output_tokens_total", labelText, values.OutputTokens)
		writeMetric(&b, "smart_llmrouter_tokens_total", labelText, values.TotalTokens)
		writeMetric(&b, "smart_llmrouter_latency_ms_sum", labelText, values.LatencyMS)
		writeMetric(&b, "smart_llmrouter_cache_hits_total", labelText, values.CacheHits)
		writeMetric(&b, "smart_llmrouter_cache_misses_total", labelText, values.CacheMisses)
		writeMetric(&b, "smart_llmrouter_upstream_attempts_total", labelText, values.Attempts)
		writeMetric(&b, "smart_llmrouter_fallbacks_total", labelText, values.Fallbacks)
	}
	return b.String()
}

func writeHelp(b *strings.Builder, name, help string) {
	fmt.Fprintf(b, "# HELP %s %s\n# TYPE %s counter\n", name, help, name)
}

func writeMetric(b *strings.Builder, name, labels string, value int64) {
	fmt.Fprintf(b, "%s{%s} %d\n", name, labels, value)
}

func prometheusLabels(labels metricLabels) string {
	parts := []string{
		`caller_id="` + escapeLabel(labels.CallerID) + `"`,
		`caller_user="` + escapeLabel(labels.CallerUser) + `"`,
		`caller_project="` + escapeLabel(labels.CallerProject) + `"`,
		`caller_environment="` + escapeLabel(labels.CallerEnvironment) + `"`,
		`token_id="` + escapeLabel(labels.TokenID) + `"`,
		`model_group="` + escapeLabel(labels.ModelGroup) + `"`,
		`target_provider="` + escapeLabel(labels.TargetProvider) + `"`,
		`target_model="` + escapeLabel(labels.TargetModel) + `"`,
		fmt.Sprintf(`status="%d"`, labels.Status),
	}
	return strings.Join(parts, ",")
}

func escapeLabel(v string) string {
	v = strings.ReplaceAll(v, `\`, `\\`)
	v = strings.ReplaceAll(v, "\n", `\n`)
	v = strings.ReplaceAll(v, `"`, `\"`)
	return v
}

func labelKey(labels metricLabels) string {
	return strings.Join([]string{
		labels.CallerID,
		labels.CallerUser,
		labels.CallerProject,
		labels.CallerEnvironment,
		labels.TokenID,
		labels.ModelGroup,
		labels.TargetProvider,
		labels.TargetModel,
		fmt.Sprint(labels.Status),
	}, "\x00")
}
