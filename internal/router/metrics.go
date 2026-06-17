package router

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"smart-llmrouter/internal/buildinfo"
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
	Requests                 int64
	Errors                   int64
	InputTokens              int64
	OutputTokens             int64
	TotalTokens              int64
	LatencyMS                int64
	CacheHits                int64
	CacheMisses              int64
	CacheBypass              int64
	Attempts                 int64
	Fallbacks                int64
	UpstreamOutputTPSSum     float64
	UpstreamOutputTPSCount   int64
	DownstreamOutputTPSSum   float64
	DownstreamOutputTPSCount int64
	CacheEntries             int64
	CacheBytes               int64
	CacheMaxBytes            int64
	CacheOccupancyRatio      float64
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
	default:
		values.CacheBypass++
	}
	values.Attempts += int64(rec.Attempts)
	if rec.FallbackUsed {
		values.Fallbacks++
	}
	if rec.UpstreamOutputTPS != nil {
		values.UpstreamOutputTPSSum += *rec.UpstreamOutputTPS
		values.UpstreamOutputTPSCount++
	}
	if rec.DownstreamOutputTPS != nil {
		values.DownstreamOutputTPSSum += *rec.DownstreamOutputTPS
		values.DownstreamOutputTPSCount++
	}
	values.CacheEntries = rec.CacheItems
	values.CacheBytes = rec.CacheBytes
	values.CacheMaxBytes = rec.CacheMaxBytes
	values.CacheOccupancyRatio = rec.CacheOccupancyPct / 100
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
	writeHelp(&b, "smart_llmrouter_cache_bypass_total", "Total requests bypassing cache.")
	writeHelp(&b, "smart_llmrouter_upstream_attempts_total", "Total upstream attempts.")
	writeHelp(&b, "smart_llmrouter_fallbacks_total", "Total requests that used fallback targets.")
	writeHelp(&b, "smart_llmrouter_upstream_output_tokens_per_second_sum", "Sum of per-request upstream output token throughput.")
	writeHelp(&b, "smart_llmrouter_upstream_output_tokens_per_second_count", "Count of requests with upstream output token throughput.")
	writeHelp(&b, "smart_llmrouter_downstream_output_tokens_per_second_sum", "Sum of per-request downstream output token throughput.")
	writeHelp(&b, "smart_llmrouter_downstream_output_tokens_per_second_count", "Count of requests with downstream output token throughput.")
	writeHelpType(&b, "smart_llmrouter_cache_entries", "Latest observed cache entry count.", "gauge")
	writeHelpType(&b, "smart_llmrouter_cache_bytes", "Latest observed cache occupied bytes.", "gauge")
	writeHelpType(&b, "smart_llmrouter_cache_max_bytes", "Configured cache maximum bytes.", "gauge")
	writeHelpType(&b, "smart_llmrouter_cache_occupancy_ratio", "Latest observed cache occupancy ratio.", "gauge")
	writeHelpType(&b, "smart_llmrouter_build_info", "Build information for the running router binary.", "gauge")
	fmt.Fprintf(&b, "smart_llmrouter_build_info{%s} 1\n", buildInfoLabels())
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
		writeMetric(&b, "smart_llmrouter_cache_bypass_total", labelText, values.CacheBypass)
		writeMetric(&b, "smart_llmrouter_upstream_attempts_total", labelText, values.Attempts)
		writeMetric(&b, "smart_llmrouter_fallbacks_total", labelText, values.Fallbacks)
		writeFloatMetric(&b, "smart_llmrouter_upstream_output_tokens_per_second_sum", labelText, values.UpstreamOutputTPSSum)
		writeMetric(&b, "smart_llmrouter_upstream_output_tokens_per_second_count", labelText, values.UpstreamOutputTPSCount)
		writeFloatMetric(&b, "smart_llmrouter_downstream_output_tokens_per_second_sum", labelText, values.DownstreamOutputTPSSum)
		writeMetric(&b, "smart_llmrouter_downstream_output_tokens_per_second_count", labelText, values.DownstreamOutputTPSCount)
		writeMetric(&b, "smart_llmrouter_cache_entries", labelText, values.CacheEntries)
		writeMetric(&b, "smart_llmrouter_cache_bytes", labelText, values.CacheBytes)
		writeMetric(&b, "smart_llmrouter_cache_max_bytes", labelText, values.CacheMaxBytes)
		writeFloatMetric(&b, "smart_llmrouter_cache_occupancy_ratio", labelText, values.CacheOccupancyRatio)
	}
	return b.String()
}

func buildInfoLabels() string {
	info := buildinfo.Current()
	parts := []string{
		`version="` + escapeLabel(info.Version) + `"`,
		`commit="` + escapeLabel(info.Commit) + `"`,
		`build_date="` + escapeLabel(info.BuildDate) + `"`,
		`go_version="` + escapeLabel(info.GoVersion) + `"`,
		`goos="` + escapeLabel(info.GOOS) + `"`,
		`goarch="` + escapeLabel(info.GOARCH) + `"`,
	}
	return strings.Join(parts, ",")
}

func writeHelp(b *strings.Builder, name, help string) {
	writeHelpType(b, name, help, "counter")
}

func writeHelpType(b *strings.Builder, name, help, typ string) {
	fmt.Fprintf(b, "# HELP %s %s\n# TYPE %s %s\n", name, help, name, typ)
}

func writeMetric(b *strings.Builder, name, labels string, value int64) {
	fmt.Fprintf(b, "%s{%s} %d\n", name, labels, value)
}

func writeFloatMetric(b *strings.Builder, name, labels string, value float64) {
	fmt.Fprintf(b, "%s{%s} %.6f\n", name, labels, value)
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
