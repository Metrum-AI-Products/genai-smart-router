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
		`{"ts":"2026-06-14T01:15:00.000Z","request_id":"req_1","caller_id":"sudarshan-prod","caller_user":"sudarshan","caller_project":"metrum-insights","caller_environment":"prod","caller_ip":"203.0.113.10","token_id":"rtr_metrum_sudarshan_metrum-insights_prod_k20260614","client":"codex","inbound_dialect":"openai-responses","requested_model":"big-coder","resolved_group":"big-coder","strategy":"weighted","target_provider":"openai","target_model":"gpt-5.4","target_dialect":"openai-responses","stream":false,"cache":"miss","status":200,"attempts":1,"fallback_used":false,"latency_ms":1200,"ttfb_ms":300,"usage":{"input_tokens":100,"output_tokens":40,"total_tokens":140},"input_price_per_million_usd":2,"output_price_per_million_usd":8,"input_cost_usd":0.0002,"output_cost_usd":0.00032,"total_cost_usd":0.00052,"pricing_source":"https://example.test/openai","pricing_updated_at":"2026-06-14","quota_state":"ok","key_state":"active","warnings":[]}`,
		`{"ts":"2026-06-14T02:05:00.000Z","request_id":"req_2","caller_id":"clay-prod","caller_user":"clay","caller_project":"metrum-insights","caller_environment":"prod","token_id":"rtr_metrum_clay_metrum-insights_prod_k20260614","client":"claude-code","inbound_dialect":"anthropic","requested_model":"small","resolved_group":"small","strategy":"weighted","target_provider":"openai","target_model":"gpt-5.4-nano","target_dialect":"openai-chat","stream":true,"cache":"hit","status":200,"attempts":0,"fallback_used":false,"latency_ms":30,"usage":{"input_tokens":12,"output_tokens":6,"total_tokens":18},"input_price_per_million_usd":0.1,"output_price_per_million_usd":0.625,"input_cost_usd":0.0000012,"output_cost_usd":0.00000375,"total_cost_usd":0.00000495,"pricing_source":"https://example.test/openai","pricing_updated_at":"2026-06-14","quota_state":"ok","key_state":"active","warnings":[]}`,
		`{"ts":"2026-06-14T02:45:00.000Z","request_id":"req_3","caller_id":"clay-prod","caller_user":"clay","caller_project":"metrum-insights","caller_environment":"prod","token_id":"rtr_metrum_clay_metrum-insights_prod_k20260614","client":"claude-code","inbound_dialect":"anthropic","requested_model":"small","resolved_group":"small","strategy":"weighted","target_provider":"minimax","target_model":"MiniMax-M3","target_dialect":"openai-chat","stream":false,"cache":"miss","status":502,"attempts":2,"fallback_used":true,"latency_ms":900,"usage":{"input_tokens":20,"output_tokens":0,"total_tokens":20},"input_price_per_million_usd":0.3,"output_price_per_million_usd":1.2,"input_cost_usd":0.000006,"output_cost_usd":0,"total_cost_usd":0.000006,"pricing_source":"https://example.test/minimax","pricing_updated_at":"2026-06-14","quota_state":"ok","key_state":"active","warnings":[],"error":"upstream-failed"}`,
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
		"Cost: `$0.000531` total, `$0.000207` input, `$0.000324` output",
		"rtr_metrum_clay_metrum-insights_prod_k20260614",
		"| openai | gpt-5.4 | 1 | 0 | 140 | 100 | 40 | $0.000520 |",
		"| minimax | MiniMax-M3 | 1 | 1 | 20 | 20 | 0 | $0.000006 |",
		"| 2026-06-14 02:00 | 2 | 1 | 38 | 32 | 6 | $0.000011 |",
		"## Usage By Caller IP",
		"| 203.0.113.10 | 1 | 0 | 140 | 100 | 40 | $0.000520 |",
		"## Hourly Usage By Caller IP",
		"| 2026-06-14 01:00 | 203.0.113.10 | 1 | 0 | 140 | 100 | 40 | $0.000520 |",
		"## Cache Summary",
		"## Downstream User Performance",
		"## Upstream Endpoint Performance",
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
		`{"ts":"2026-06-14T01:15:00.000Z","request_id":"req_tps","caller_id":"alice","caller_user":"alice","caller_project":"metrum-insights","caller_environment":"test","caller_ip":"198.51.100.22","token_id":"rtr_alice_test","client":"codex","inbound_dialect":"openai-responses","requested_model":"default","resolved_group":"default","strategy":"static","target_provider":"openai","target_model":"gpt-5.4-nano","target_dialect":"openai-responses","stream":false,"cache":"miss","status":200,"attempts":1,"fallback_used":false,"latency_ms":250,"upstream_duration_ms":200,"downstream_duration_ms":25,"upstream_output_tokens_per_sec":50,"upstream_total_tokens_per_sec":75,"downstream_output_tokens_per_sec":400,"downstream_total_tokens_per_sec":600,"usage":{"input_tokens":5,"output_tokens":10,"total_tokens":15},"input_price_per_million_usd":0.2,"output_price_per_million_usd":1.25,"input_cost_usd":0.000001,"output_cost_usd":0.0000125,"total_cost_usd":0.0000135,"cache_enabled":true,"cache_items":2,"cache_bytes":1024,"cache_max_bytes":4096,"cache_occupancy_pct":25,"quota_state":"ok","key_state":"active","warnings":[]}`,
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
		"| alice | metrum-insights | test | codex | 1 | 0 | 0 | 15 | 10 | 250 | 250 | 0 | 0 | 25 | 25 | 400.00 | 600.00 | 0 |",
		"| openai | gpt-5.4-nano | openai-responses | 1 | 0 | 1 | 0 | 0 | 15 | 10 | $0.000013 | 200 | 200 | 250 | 250 | 0 | 0 | 50.00 | 75.00 |",
		"| 2026-06-14T01:15:00.000Z | `198.51.100.22` | `req_tps` | `rtr_alice_test` | default | openai | gpt-5.4-nano | 200 | miss | 10 | 15 | $0.000013 | 200 | 25 | 50.00 | 75.00 | 400.00 | 600.00 |",
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
		`{"ts":"2026-06-14T01:30:00.000Z","request_id":"req_other","caller_id":"other","caller_user":"other","caller_project":"metrum-insights","caller_environment":"prod","caller_ip":"203.0.113.12","token_id":"rtr_metrum_other_metrum-insights_prod_k20260614","client":"codex","inbound_dialect":"openai-responses","requested_model":"high","resolved_group":"high","strategy":"weighted","target_provider":"openai","target_model":"gpt-5.4-nano","target_dialect":"openai-responses","stream":false,"cache":"miss","status":200,"attempts":1,"fallback_used":false,"latency_ms":1500,"usage":{"input_tokens":300,"output_tokens":100,"total_tokens":400},"quota_state":"ok","key_state":"active","warnings":[]}`,
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
		"| openrouter | deepseek/deepseek-v4-flash:nitro | 1 | 0 | 140 | 100 | 40 | $0.000000 |",
	} {
		if !strings.Contains(md, want) {
			t.Fatalf("filtered report missing %q:\n%s", want, md)
		}
	}
	for _, notWant := range []string{
		"rtr_metrum_claude-small_harbor-algotune-pca_case-1_k20260614",
		"rtr_metrum_other_metrum-insights_prod_k20260614",
		"gpt-5.4-nano",
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
	for _, table := range []string{"request_usage", "request_attempts", "request_trace_events", "request_errors", "request_content_captures", "request_content_headers", "request_content_audit_events"} {
		var cols []col
		if err := store.db.Raw(`SELECT name, type FROM pragma_table_info(?)`, table).Scan(&cols).Error; err != nil {
			t.Fatal(err)
		}
		if len(cols) == 0 {
			t.Fatalf("%s schema not found", table)
		}
		for _, c := range cols {
			typ := strings.ToLower(c.Type)
			if strings.Contains(typ, "json") || strings.Contains(typ, "array") || strings.HasSuffix(typ, "[]") {
				t.Fatalf("non-relational column %s.%s type %s", table, c.Name, c.Type)
			}
		}
	}
}

func TestContentCaptureRetentionPurgeDeletesRowsAndAudits(t *testing.T) {
	store, err := OpenUsageStorePath(filepath.Join(t.TempDir(), "usage.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)
	expired := contentCaptureRecord{
		RequestID:      "req_expired",
		TS:             formatUsageTime(now.Add(-48 * time.Hour)),
		Scope:          contentCaptureScopeRequest,
		CallerID:       "alice",
		TokenID:        "rtr_alice_test",
		ResolvedGroup:  "default",
		InboundDialect: "openai-chat",
		ContentType:    "application/json",
		ContentText:    "expired",
		ContentBytes:   len("expired"),
		RetentionUntil: formatUsageTime(now.Add(-time.Hour)),
	}
	fresh := contentCaptureRecord{
		RequestID:      "req_fresh",
		TS:             formatUsageTime(now),
		Scope:          contentCaptureScopeRequest,
		CallerID:       "alice",
		TokenID:        "rtr_alice_test",
		ResolvedGroup:  "default",
		InboundDialect: "openai-chat",
		ContentType:    "application/json",
		ContentText:    "fresh",
		ContentBytes:   len("fresh"),
		RetentionUntil: formatUsageTime(now.Add(24 * time.Hour)),
	}
	if err := store.db.Create(&expired).Error; err != nil {
		t.Fatal(err)
	}
	if err := store.db.Create(&fresh).Error; err != nil {
		t.Fatal(err)
	}
	if err := store.db.Create(&contentCaptureHeaderRecord{CaptureID: expired.ID, RequestID: expired.RequestID, Scope: expired.Scope, Name: "User-Agent", Value: "test"}).Error; err != nil {
		t.Fatal(err)
	}

	deleted, err := store.PurgeExpiredContentCaptures(now, "content-admin", "rtr_content_admin", "unit-test")
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 1 {
		t.Fatalf("deleted=%d, want 1", deleted)
	}
	var captures []contentCaptureRecord
	if err := store.db.Order("request_id").Find(&captures).Error; err != nil {
		t.Fatal(err)
	}
	if len(captures) != 1 || captures[0].RequestID != "req_fresh" {
		t.Fatalf("remaining captures: %#v", captures)
	}
	var headerCount int64
	if err := store.db.Model(&contentCaptureHeaderRecord{}).Count(&headerCount).Error; err != nil {
		t.Fatal(err)
	}
	if headerCount != 0 {
		t.Fatalf("header rows=%d, want 0", headerCount)
	}
	var audit contentCaptureAuditRecord
	if err := store.db.Where("action = ?", "retention_purge").First(&audit).Error; err != nil {
		t.Fatal(err)
	}
	if audit.RequestID != "req_expired" || audit.RowsAffected != 1 || audit.ActorCallerID != "content-admin" {
		t.Fatalf("unexpected audit row: %#v", audit)
	}
}
