package router

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestUsageReportImportsJSONLAndRendersMarkdown(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "requests.jsonl")
	dbPath := filepath.Join(dir, "usage.sqlite")
	raw := strings.Join([]string{
		`{"ts":"2026-06-14T01:15:00.000Z","request_id":"req_1","caller_id":"sudarshan-prod","caller_user":"sudarshan","caller_project":"metrum-insights","caller_environment":"prod","caller_ip":"203.0.113.10","token_id":"rtr_metrum_sudarshan_metrum-insights_prod_k20260614","client":"codex","inbound_dialect":"openai-responses","requested_model":"big-coder","resolved_group":"big-coder","strategy":"weighted","target_provider":"openai","target_model":"gpt-5.4","target_dialect":"openai-responses","stream":false,"cache":"miss","status":200,"attempts":1,"fallback_used":false,"latency_ms":1200,"ttfb_ms":300,"usage":{"input_tokens":100,"output_tokens":40,"total_tokens":140},"quota_state":"ok","key_state":"active","warnings":[]}`,
		`{"ts":"2026-06-14T02:05:00.000Z","request_id":"req_2","caller_id":"clay-prod","caller_user":"clay","caller_project":"metrum-insights","caller_environment":"prod","token_id":"rtr_metrum_clay_metrum-insights_prod_k20260614","client":"claude-code","inbound_dialect":"anthropic","requested_model":"small","resolved_group":"small","strategy":"weighted","target_provider":"openai","target_model":"gpt-5.4-nano","target_dialect":"openai-chat","stream":true,"cache":"hit","status":200,"attempts":0,"fallback_used":false,"latency_ms":30,"usage":{"input_tokens":12,"output_tokens":6,"total_tokens":18},"quota_state":"ok","key_state":"active","warnings":[]}`,
		`{"ts":"2026-06-14T02:45:00.000Z","request_id":"req_3","caller_id":"clay-prod","caller_user":"clay","caller_project":"metrum-insights","caller_environment":"prod","token_id":"rtr_metrum_clay_metrum-insights_prod_k20260614","client":"claude-code","inbound_dialect":"anthropic","requested_model":"small","resolved_group":"small","strategy":"weighted","target_provider":"minimax","target_model":"MiniMax-M3","target_dialect":"openai-chat","stream":false,"cache":"miss","status":502,"attempts":2,"fallback_used":true,"latency_ms":900,"usage":{"input_tokens":20,"output_tokens":0,"total_tokens":20},"quota_state":"ok","key_state":"active","warnings":[],"error":"upstream-failed"}`,
		"",
	}, "\n")
	if err := os.WriteFile(logPath, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}

	md, err := GenerateUsageMarkdown(UsageReportOptions{
		DBPath:  dbPath,
		LogPath: logPath,
		From:    time.Date(2026, 6, 14, 0, 0, 0, 0, time.UTC),
		To:      time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"# Smart LLM Router Usage Report",
		"Requests: `3`",
		"Errors: `1`",
		"Tokens: `178` total, `132` input, `46` output",
		"rtr_metrum_clay_metrum-insights_prod_k20260614",
		"| openai | gpt-5.4 | 1 | 0 | 140 |",
		"| minimax | MiniMax-M3 | 1 | 1 | 20 |",
		"| 2026-06-14 02:00 | 2 | 1 | 38 |",
		"## Usage By Caller IP",
		"| 203.0.113.10 | 1 | 0 | 140 |",
		"## Hourly Usage By Caller IP",
		"| 2026-06-14 01:00 | 203.0.113.10 | 1 | 0 | 140 |",
		"## Cache Summary",
		"## Per-Request Throughput",
	} {
		if !strings.Contains(md, want) {
			t.Fatalf("report missing %q:\n%s", want, md)
		}
	}

	imported, err := ImportUsageJSONL(dbPath, logPath)
	if err != nil {
		t.Fatal(err)
	}
	if imported != 3 {
		t.Fatalf("imported=%d", imported)
	}
	md, err = GenerateUsageMarkdown(UsageReportOptions{
		DBPath: dbPath,
		From:   time.Date(2026, 6, 14, 0, 0, 0, 0, time.UTC),
		To:     time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(md, "Requests: `3`") {
		t.Fatalf("duplicate import changed request count:\n%s", md)
	}
}

func TestUsageReportRendersThroughputAndCacheSnapshots(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "requests.jsonl")
	dbPath := filepath.Join(dir, "usage.sqlite")
	raw := strings.Join([]string{
		`{"ts":"2026-06-14T01:15:00.000Z","request_id":"req_tps","caller_id":"alice","caller_user":"alice","caller_project":"metrum-insights","caller_environment":"test","caller_ip":"198.51.100.22","token_id":"rtr_alice_test","client":"codex","inbound_dialect":"openai-responses","requested_model":"default","resolved_group":"default","strategy":"static","target_provider":"openai","target_model":"gpt-5.5","target_dialect":"openai-responses","stream":false,"cache":"miss","status":200,"attempts":1,"fallback_used":false,"latency_ms":250,"upstream_duration_ms":200,"downstream_duration_ms":25,"upstream_output_tokens_per_sec":50,"upstream_total_tokens_per_sec":75,"downstream_output_tokens_per_sec":400,"downstream_total_tokens_per_sec":600,"usage":{"input_tokens":5,"output_tokens":10,"total_tokens":15},"cache_enabled":true,"cache_items":2,"cache_bytes":1024,"cache_max_bytes":4096,"cache_occupancy_pct":25,"quota_state":"ok","key_state":"active","warnings":[]}`,
		"",
	}, "\n")
	if err := os.WriteFile(logPath, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	md, err := GenerateUsageMarkdown(UsageReportOptions{
		DBPath:  dbPath,
		LogPath: logPath,
		From:    time.Date(2026, 6, 14, 0, 0, 0, 0, time.UTC),
		To:      time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"upstream `50.00` output tok/s / `75.00` total tok/s",
		"downstream `400.00` output tok/s / `600.00` total tok/s",
		"| 1 | 1 | 0 | 1 | 0 | 0.00% | 0.00% | 2 | 1024 | 4096 | 25.00% | 25.00% | 25.00% |",
		"| 2026-06-14T01:15:00.000Z | `198.51.100.22` | `req_tps` | `rtr_alice_test` | default | openai | gpt-5.5 | 200 | miss | 10 | 15 | 200 | 25 | 50.00 | 75.00 | 400.00 | 600.00 |",
	} {
		if !strings.Contains(md, want) {
			t.Fatalf("report missing %q:\n%s", want, md)
		}
	}
}

func TestUsageReportFiltersRows(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "requests.jsonl")
	dbPath := filepath.Join(dir, "usage.sqlite")
	raw := strings.Join([]string{
		`{"ts":"2026-06-14T01:00:00.000Z","request_id":"req_codex_small","caller_id":"codex-small","caller_user":"codex-small","caller_project":"harbor-algotune-pca","caller_environment":"case-1","caller_ip":"203.0.113.10","token_id":"rtr_metrum_codex-small_harbor-algotune-pca_case-1_k20260614","client":"codex","inbound_dialect":"openai-responses","requested_model":"small","resolved_group":"small","strategy":"weighted","target_provider":"openrouter","target_model":"deepseek/deepseek-v4-flash:nitro","target_dialect":"openai-chat","stream":false,"cache":"miss","status":200,"attempts":1,"fallback_used":false,"latency_ms":1200,"usage":{"input_tokens":100,"output_tokens":40,"total_tokens":140},"quota_state":"ok","key_state":"active","warnings":[]}`,
		`{"ts":"2026-06-14T01:15:00.000Z","request_id":"req_claude_small","caller_id":"claude-small","caller_user":"claude-small","caller_project":"harbor-algotune-pca","caller_environment":"case-1","caller_ip":"203.0.113.11","token_id":"rtr_metrum_claude-small_harbor-algotune-pca_case-1_k20260614","client":"claude-code","inbound_dialect":"anthropic","requested_model":"small","resolved_group":"small","strategy":"weighted","target_provider":"minimax","target_model":"MiniMax-M3","target_dialect":"openai-chat","stream":false,"cache":"miss","status":200,"attempts":1,"fallback_used":false,"latency_ms":900,"usage":{"input_tokens":80,"output_tokens":20,"total_tokens":100},"quota_state":"ok","key_state":"active","warnings":[]}`,
		`{"ts":"2026-06-14T01:30:00.000Z","request_id":"req_other","caller_id":"other","caller_user":"other","caller_project":"metrum-insights","caller_environment":"prod","caller_ip":"203.0.113.12","token_id":"rtr_metrum_other_metrum-insights_prod_k20260614","client":"codex","inbound_dialect":"openai-responses","requested_model":"high","resolved_group":"high","strategy":"weighted","target_provider":"openai","target_model":"gpt-5.5","target_dialect":"openai-responses","stream":false,"cache":"miss","status":200,"attempts":1,"fallback_used":false,"latency_ms":1500,"usage":{"input_tokens":300,"output_tokens":100,"total_tokens":400},"quota_state":"ok","key_state":"active","warnings":[]}`,
		"",
	}, "\n")
	if err := os.WriteFile(logPath, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	md, err := GenerateUsageMarkdown(UsageReportOptions{
		DBPath:            dbPath,
		LogPath:           logPath,
		From:              time.Date(2026, 6, 14, 0, 0, 0, 0, time.UTC),
		To:                time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC),
		CallerProject:     "harbor-algotune-pca",
		CallerEnvironment: "case-1",
		ResolvedGroup:     "small",
		Client:            "codex",
		TokenIDPrefix:     "rtr_metrum_codex",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Requests: `1`",
		"Tokens: `140` total, `100` input, `40` output",
		"rtr_metrum_codex-small_harbor-algotune-pca_case-1_k20260614",
		"| openrouter | deepseek/deepseek-v4-flash:nitro | 1 | 0 | 140 |",
	} {
		if !strings.Contains(md, want) {
			t.Fatalf("filtered report missing %q:\n%s", want, md)
		}
	}
	for _, notWant := range []string{
		"rtr_metrum_claude-small_harbor-algotune-pca_case-1_k20260614",
		"rtr_metrum_other_metrum-insights_prod_k20260614",
		"gpt-5.5",
	} {
		if strings.Contains(md, notWant) {
			t.Fatalf("filtered report unexpectedly contains %q:\n%s", notWant, md)
		}
	}
}

func TestUsageDBSchemaIsRelationalOnly(t *testing.T) {
	store, err := OpenUsageStorePath(filepath.Join(t.TempDir(), "usage.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	type col struct {
		Name string
		Type string
	}
	var cols []col
	if err := store.db.Raw(`SELECT name, type FROM pragma_table_info('request_usage')`).Scan(&cols).Error; err != nil {
		t.Fatal(err)
	}
	if len(cols) == 0 {
		t.Fatal("request_usage schema not found")
	}
	for _, c := range cols {
		typ := strings.ToLower(c.Type)
		if strings.Contains(typ, "json") || strings.Contains(typ, "array") || strings.HasSuffix(typ, "[]") {
			t.Fatalf("non-relational column %s type %s", c.Name, c.Type)
		}
	}
}
