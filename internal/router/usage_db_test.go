package router

import (
	"math"
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
		"Total Tokens: `178`; Input Tokens: `132`; Output Tokens: `46`",
		"Cost: `$0.000531` total, `$0.000207` input, `$0.000000` image, `$0.000324` output",
		"rtr_metrum_clay_metrum-insights_prod_k20260614",
		"| Token ID | Owner User | Project | Env | Caller ID |",
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
		"downstream write `400.00` output tok/s / `600.00` total tok/s",
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

func TestUsageReportRendersDecisionTelemetrySummary(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "requests.jsonl")
	dbPath := filepath.Join(dir, "usage.sqlite")
	raw := strings.Join([]string{
		`{"ts":"2026-06-14T01:15:00.000Z","request_id":"req_decision","caller_id":"alice","caller_user":"alice","caller_project":"metrum-insights","caller_environment":"test","token_id":"rtr_alice_test","client":"codex","inbound_dialect":"openai-chat","requested_model":"default","resolved_group":"default","strategy":"static","target_provider":"mock","target_model":"mock-model","target_dialect":"openai-chat","stream":false,"cache":"bypass","status":200,"attempts":1,"fallback_used":false,"latency_ms":25,"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2},"quota_state":"ok","key_state":"active","warnings":[],"decision_shape_features":[{"seq":1,"name":"has_tools","bool_value":true}],"decision_candidates":[{"candidate_index":0,"group_target_index":0,"provider":"mock","model":"mock-model","dialect":"openai-chat","eligible":true,"selected":true}],"decision_filter_reasons":[{"seq":1,"candidate_index":1,"stage":"request_shape","reason":"tool-support"}],"routing_decisions":[{"seq":1,"strategy":"static","selected_candidate_index":0,"provider":"mock","model":"mock-model","dialect":"openai-chat","fallback_count":0}],"cache_reasons":[{"seq":1,"status":"bypass","reason":"cache-tool-request","candidate_index":0,"provider":"mock","model":"mock-model","dialect":"openai-chat"}]}`,
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
		"## Decision Telemetry Summary",
		"| 1 | 1 | 1 | 1 | 1 |",
		"### Routing Decisions By Strategy",
		"| static | 1 |",
		"### Target Filter Reasons",
		"| tool-support | 1 |",
		"### Cache Decision Reasons",
		"| cache-tool-request | 1 |",
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
		"Total Tokens: `140`; Input Tokens: `100`; Output Tokens: `40`",
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

func TestUsageRollupDailyTotalsDraftRerunAndFinalize(t *testing.T) {
	store, err := OpenUsageStorePath(filepath.Join(t.TempDir(), "usage.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	from := time.Date(2026, 6, 14, 0, 0, 0, 0, time.UTC)
	to := from.Add(24 * time.Hour)
	ttfb := int64(25)
	upstream := int64(90)
	downstream := int64(10)
	upstreamTPS := 12.5
	downstreamTPS := 25.0
	rows := []usageRow{
		{
			TS:                            from.Add(time.Hour),
			RequestID:                     "req_rollup_1",
			CallerID:                      "caller-alice",
			CallerUser:                    "alice",
			CallerProject:                 "analytics",
			CallerEnvironment:             "prod",
			TokenID:                       "rtr_alice",
			Client:                        "codex",
			InboundDialect:                "openai-chat",
			RequestedModel:                "default",
			ResolvedGroup:                 "default",
			Strategy:                      "weighted",
			TargetProvider:                "mock",
			TargetModel:                   "model-a",
			TargetDialect:                 "openai-chat",
			Cache:                         "miss",
			Status:                        200,
			Attempts:                      1,
			LatencyMS:                     100,
			TTFBMS:                        &ttfb,
			UpstreamMS:                    &upstream,
			DownstreamMS:                  &downstream,
			UpstreamOutputTPS:             &upstreamTPS,
			DownstreamOutputTPS:           &downstreamTPS,
			InputTokens:                   10,
			OutputTokens:                  5,
			TotalTokens:                   15,
			InputCostUSD:                  0.10,
			ImageCostUSD:                  0.02,
			OutputCostUSD:                 0.20,
			TotalCostUSD:                  0.32,
			UpstreamReportedInputCostUSD:  0.11,
			UpstreamReportedOutputCostUSD: 0.21,
			UpstreamReportedTotalCostUSD:  0.33,
			InputHasImage:                 true,
			InputImageCount:               2,
			InputImageTokens:              321,
			PIIFilterApplied:              true,
			ContractBucket:                "ok",
			TargetValidationStatus:        "validated",
			CacheEnabled:                  true,
			CacheItems:                    3,
			CacheBytes:                    1000,
			CacheMaxBytes:                 4000,
			CacheOccupancyPct:             25,
		},
		{
			TS:                from.Add(2 * time.Hour),
			RequestID:         "req_rollup_2",
			CallerID:          "caller-bob",
			CallerUser:        "bob",
			CallerProject:     "platform",
			CallerEnvironment: "prod",
			TokenID:           "rtr_bob",
			Client:            "claude-code",
			InboundDialect:    "anthropic",
			RequestedModel:    "default",
			ResolvedGroup:     "default",
			Strategy:          "failover",
			TargetProvider:    "mock",
			TargetModel:       "model-b",
			TargetDialect:     "openai-chat",
			Stream:            true,
			Cache:             "hit",
			Status:            502,
			Attempts:          2,
			FallbackUsed:      true,
			LatencyMS:         250,
			InputTokens:       4,
			OutputTokens:      6,
			InputCostUSD:      0.04,
			OutputCostUSD:     0.06,
			TotalCostUSD:      0.10,
		},
		{
			TS:             to.Add(time.Hour),
			RequestID:      "req_rollup_outside",
			CallerUser:     "outside",
			TokenID:        "rtr_outside",
			RequestedModel: "default",
			ResolvedGroup:  "default",
			Cache:          "miss",
			Status:         200,
			Attempts:       1,
			LatencyMS:      999,
			InputTokens:    100,
			OutputTokens:   100,
			TotalTokens:    200,
		},
	}
	for _, row := range rows {
		if err := store.db.Create(recordFromRow(row)).Error; err != nil {
			t.Fatal(err)
		}
	}

	result, err := store.generateUsageRollup(UsageRollupOptions{From: from, To: to})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "draft" || result.SourceRequestCount != 2 || result.DailyRows != 2 {
		t.Fatalf("unexpected result: %#v", result)
	}
	var dailyRows []usageRollupDailyRecord
	if err := store.db.Where("run_id = ?", result.RunID).Order("token_id ASC").Find(&dailyRows).Error; err != nil {
		t.Fatal(err)
	}
	if len(dailyRows) != 2 {
		t.Fatalf("got %d rollup rows, want 2: %#v", len(dailyRows), dailyRows)
	}
	alice := dailyRows[0]
	bob := dailyRows[1]
	if alice.TokenID != "rtr_alice" || alice.CallerUser != "alice" || alice.CallerProject != "analytics" || alice.Client != "codex" || alice.TargetModel != "model-a" || !alice.InputHasImage || !alice.PIIFilterApplied || alice.ContractBucket != "ok" || alice.TargetValidationStatus != "validated" {
		t.Fatalf("alice dimension row lost fields: %#v", alice)
	}
	if bob.TokenID != "rtr_bob" || bob.CallerUser != "bob" || bob.CallerProject != "platform" || bob.Client != "claude-code" || bob.TargetModel != "model-b" || bob.StatusClass != "5xx" || !bob.Stream || bob.Cache != "hit" {
		t.Fatalf("bob dimension row lost fields: %#v", bob)
	}
	if alice.DayUTC != "2026-06-14" || alice.SourceRequestCount != 1 || alice.ErrorCount != 0 || alice.StreamCount != 0 {
		t.Fatalf("unexpected alice counts: %#v", alice)
	}
	if bob.DayUTC != "2026-06-14" || bob.SourceRequestCount != 1 || bob.ErrorCount != 1 || bob.StreamCount != 1 {
		t.Fatalf("unexpected bob counts: %#v", bob)
	}
	if alice.CacheMissCount != 1 || bob.CacheHitCount != 1 || bob.FallbackCount != 1 || alice.AttemptCount+bob.AttemptCount != 3 {
		t.Fatalf("unexpected cache/fallback/attempt rollup: alice=%#v bob=%#v", alice, bob)
	}
	if alice.InputTokens+bob.InputTokens != 14 || alice.OutputTokens+bob.OutputTokens != 11 || alice.TotalTokens+bob.TotalTokens != 25 {
		t.Fatalf("unexpected token rollup: alice=%#v bob=%#v", alice, bob)
	}
	if alice.InputImageCount+bob.InputImageCount != 2 || alice.InputImageTokens+bob.InputImageTokens != 321 {
		t.Fatalf("unexpected image rollup: alice=%#v bob=%#v", alice, bob)
	}
	assertNear(t, alice.InputCostUSD+bob.InputCostUSD, 0.14, "input cost")
	assertNear(t, alice.ImageCostUSD+bob.ImageCostUSD, 0.02, "image cost")
	assertNear(t, alice.OutputCostUSD+bob.OutputCostUSD, 0.26, "output cost")
	assertNear(t, alice.TotalCostUSD+bob.TotalCostUSD, 0.42, "total cost")
	assertNear(t, alice.UpstreamReportedInputCostUSD+bob.UpstreamReportedInputCostUSD, 0.11, "upstream input cost")
	assertNear(t, alice.UpstreamReportedOutputCostUSD+bob.UpstreamReportedOutputCostUSD, 0.21, "upstream output cost")
	assertNear(t, alice.UpstreamReportedTotalCostUSD+bob.UpstreamReportedTotalCostUSD, 0.33, "upstream total cost")
	if alice.LatencyMSSum+bob.LatencyMSSum != 350 || maxInt64(alice.LatencyMSMax, bob.LatencyMSMax) != 250 || alice.TTFBMSSum+bob.TTFBMSSum != 25 || alice.TTFBMSCount+bob.TTFBMSCount != 1 || maxInt64(alice.TTFBMSMax, bob.TTFBMSMax) != 25 {
		t.Fatalf("unexpected latency rollup: alice=%#v bob=%#v", alice, bob)
	}
	if alice.UpstreamMSSum+bob.UpstreamMSSum != 90 || alice.UpstreamMSCount+bob.UpstreamMSCount != 1 || maxInt64(alice.UpstreamMSMax, bob.UpstreamMSMax) != 90 || alice.DownstreamMSSum+bob.DownstreamMSSum != 10 || alice.DownstreamMSCount+bob.DownstreamMSCount != 1 || maxInt64(alice.DownstreamMSMax, bob.DownstreamMSMax) != 10 {
		t.Fatalf("unexpected duration rollup: alice=%#v bob=%#v", alice, bob)
	}
	if alice.UpstreamOutputTokensPerSecSum+bob.UpstreamOutputTokensPerSecSum != 12.5 || alice.UpstreamOutputTokensPerSecCount+bob.UpstreamOutputTokensPerSecCount != 1 || alice.DownstreamOutputTokensPerSecSum+bob.DownstreamOutputTokensPerSecSum != 25 || alice.DownstreamOutputTokensPerSecCount+bob.DownstreamOutputTokensPerSecCount != 1 {
		t.Fatalf("unexpected throughput rollup: alice=%#v bob=%#v", alice, bob)
	}
	if alice.CacheSnapshotCount+bob.CacheSnapshotCount != 1 || maxInt64(alice.CacheItemsMax, bob.CacheItemsMax) != 3 || alice.CacheBytesSum+bob.CacheBytesSum != 1000 || maxInt64(alice.CacheBytesMax, bob.CacheBytesMax) != 1000 || alice.CacheMaxBytesLatest != 4000 || alice.CacheOccupancyPctSum+bob.CacheOccupancyPctSum != 25 || maxFloat64(alice.CacheOccupancyPctMax, bob.CacheOccupancyPctMax) != 25 {
		t.Fatalf("unexpected cache snapshot rollup: alice=%#v bob=%#v", alice, bob)
	}

	third := rows[1]
	third.RequestID = "req_rollup_3"
	third.TS = from.Add(3 * time.Hour)
	if err := store.db.Create(recordFromRow(third)).Error; err != nil {
		t.Fatal(err)
	}
	rerun, err := store.generateUsageRollup(UsageRollupOptions{From: from, To: to})
	if err != nil {
		t.Fatal(err)
	}
	if rerun.RunID != result.RunID || rerun.SourceRequestCount != 3 {
		t.Fatalf("draft rerun did not replace same run: first=%#v rerun=%#v", result, rerun)
	}
	var dailyCount int64
	if err := store.db.Model(&usageRollupDailyRecord{}).Where("run_id = ?", result.RunID).Count(&dailyCount).Error; err != nil {
		t.Fatal(err)
	}
	if dailyCount != 2 {
		t.Fatalf("draft rerun left %d daily rows, want 2", dailyCount)
	}

	finalized, err := store.generateUsageRollup(UsageRollupOptions{From: from, To: to, Finalize: true})
	if err != nil {
		t.Fatal(err)
	}
	if finalized.RunID != result.RunID || finalized.Status != "finalized" || finalized.SourceRequestCount != 3 {
		t.Fatalf("unexpected finalized result: %#v", finalized)
	}
	if _, err := store.generateUsageRollup(UsageRollupOptions{From: from, To: to}); err == nil || !strings.Contains(err.Error(), "overlaps a finalized window") {
		t.Fatalf("draft rerun after finalize err=%v, want finalized immutability", err)
	}
	if _, err := store.generateUsageRollup(UsageRollupOptions{From: from.Add(12 * time.Hour), To: to.Add(12 * time.Hour), Finalize: true}); err == nil || !strings.Contains(err.Error(), "overlaps a finalized window") {
		t.Fatalf("overlapping finalized window err=%v, want overlap rejection", err)
	}
}

func assertNear(t *testing.T, got, want float64, label string) {
	t.Helper()
	if math.Abs(got-want) > 0.000000001 {
		t.Fatalf("%s=%f, want %f", label, got, want)
	}
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func maxFloat64(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
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
	for _, table := range []string{
		"request_usage",
		"request_attempts",
		"request_trace_events",
		"request_decision_shape_features",
		"request_target_candidates",
		"request_target_filter_reasons",
		"request_routing_decisions",
		"request_cache_reasons",
		"request_errors",
		"request_content_captures",
		"request_content_headers",
		"request_content_audit_events",
		"security_access_events",
		"usage_rollup_runs",
		"usage_rollup_daily",
		"retention_policy_versions",
		"retention_policy_rules",
		"retention_jobs",
		"retention_job_table_results",
		"legal_holds",
		"legal_hold_audit_events",
	} {
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

func TestRetentionDryRunWritesJobRecordsAndHonorsLegalHold(t *testing.T) {
	store, err := OpenUsageStorePath(filepath.Join(t.TempDir(), "usage.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)
	old := now.Add(-10 * 24 * time.Hour)
	fresh := now.Add(-24 * time.Hour)
	for _, row := range []contentCaptureRecord{
		{
			RequestID:      "req_retention_held",
			TS:             formatUsageTime(old),
			Scope:          contentCaptureScopeRequest,
			CallerID:       "alice",
			TokenID:        "rtr_alice_test",
			ResolvedGroup:  "default",
			InboundDialect: "openai-chat",
			ContentType:    "application/json",
			ContentText:    "held",
			ContentBytes:   len("held"),
			RetentionUntil: formatUsageTime(old.Add(24 * time.Hour)),
		},
		{
			RequestID:      "req_retention_free",
			TS:             formatUsageTime(old.Add(time.Hour)),
			Scope:          contentCaptureScopeResponse,
			CallerID:       "alice",
			TokenID:        "rtr_alice_test",
			ResolvedGroup:  "default",
			InboundDialect: "openai-chat",
			ContentType:    "application/json",
			ContentText:    "free",
			ContentBytes:   len("free"),
			RetentionUntil: formatUsageTime(old.Add(24 * time.Hour)),
		},
		{
			RequestID:      "req_retention_fresh",
			TS:             formatUsageTime(fresh),
			Scope:          contentCaptureScopeRequest,
			CallerID:       "alice",
			TokenID:        "rtr_alice_test",
			ResolvedGroup:  "default",
			InboundDialect: "openai-chat",
			ContentType:    "application/json",
			ContentText:    "fresh",
			ContentBytes:   len("fresh"),
			RetentionUntil: formatUsageTime(fresh.Add(24 * time.Hour)),
		},
	} {
		if err := store.db.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := store.db.Create(&legalHoldRecord{
		HoldID:     "hold-content-1",
		DataClass:  retentionDataClassContentCapture,
		Active:     true,
		StartTS:    formatUsageTime(old.Add(-time.Hour)),
		EndTS:      formatUsageTime(old.Add(30 * time.Minute)),
		ReasonCode: "litigation",
		Subject:    "matter-121",
		CreatedBy:  "unit-test",
		CreatedAt:  formatUsageTime(now),
	}).Error; err != nil {
		t.Fatal(err)
	}
	cfg := retentionTestConfig(retentionDataClassContentCapture, 7)
	result, err := store.runRetentionStatus(RetentionStatusOptions{Config: cfg, Now: now, RequestedBy: "unit-test"})
	if err != nil {
		t.Fatal(err)
	}
	if result.JobID == 0 || result.PolicyVersionID == 0 || len(result.TableResults) != 1 {
		t.Fatalf("unexpected result: %#v", result)
	}
	table := result.TableResults[0]
	if table.TableName != "request_content_captures" || table.CandidateRows != 2 || table.HeldRows != 1 || table.EligibleRows != 1 || table.BlockedRows != 0 || table.Status != "dry_run" {
		t.Fatalf("unexpected table result: %#v", table)
	}
	var job retentionJobRecord
	if err := store.db.First(&job, result.JobID).Error; err != nil {
		t.Fatal(err)
	}
	if job.Status != "completed" || !job.DryRun || job.RequestedBy != "unit-test" {
		t.Fatalf("unexpected job: %#v", job)
	}
	var resultRow retentionJobTableResultRecord
	if err := store.db.Where("job_id = ? AND table_name = ?", result.JobID, "request_content_captures").First(&resultRow).Error; err != nil {
		t.Fatal(err)
	}
	if resultRow.CandidateRows != 2 || resultRow.HeldRows != 1 || resultRow.EligibleRows != 1 {
		t.Fatalf("unexpected stored result: %#v", resultRow)
	}
	var rules int64
	if err := store.db.Model(&retentionPolicyRuleRecord{}).Where("policy_version_id = ?", result.PolicyVersionID).Count(&rules).Error; err != nil {
		t.Fatal(err)
	}
	if rules != 1 {
		t.Fatalf("policy rules=%d, want 1", rules)
	}
}

func TestRetentionUsageDetailRequiresFinalizedRollupBeforeFutureDelete(t *testing.T) {
	store, err := OpenUsageStorePath(filepath.Join(t.TempDir(), "usage.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)
	old := now.Add(-10 * 24 * time.Hour)
	cutoff := now.Add(-7 * 24 * time.Hour)
	if err := store.db.Create(recordFromRow(usageRow{
		TS:             old,
		RequestID:      "req_usage_detail_old",
		CallerID:       "alice",
		CallerUser:     "alice",
		TokenID:        "rtr_alice",
		RequestedModel: "default",
		ResolvedGroup:  "default",
		Cache:          "miss",
		Status:         200,
		Attempts:       1,
	})).Error; err != nil {
		t.Fatal(err)
	}
	cfg := retentionTestConfig(retentionDataClassUsageDetail, 7)
	blocked, err := store.runRetentionStatus(RetentionStatusOptions{Config: cfg, Now: now, RequestedBy: "unit-test"})
	if err != nil {
		t.Fatal(err)
	}
	if len(blocked.TableResults) != 1 {
		t.Fatalf("blocked results: %#v", blocked)
	}
	if got := blocked.TableResults[0]; got.Status != "blocked_rollup_required" || got.CandidateRows != 1 || got.EligibleRows != 0 || got.BlockedRows != 1 {
		t.Fatalf("unexpected blocked usage_detail result: %#v", got)
	}
	mid := old.Add(24 * time.Hour)
	if _, err := store.generateUsageRollup(UsageRollupOptions{From: old.Add(-time.Hour), To: mid, Finalize: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.generateUsageRollup(UsageRollupOptions{From: mid, To: cutoff, Finalize: true}); err != nil {
		t.Fatal(err)
	}
	ready, err := store.runRetentionStatus(RetentionStatusOptions{Config: cfg, Now: now, RequestedBy: "unit-test"})
	if err != nil {
		t.Fatal(err)
	}
	if got := ready.TableResults[0]; got.Status != "dry_run" || got.CandidateRows != 1 || got.EligibleRows != 1 || got.BlockedRows != 0 {
		t.Fatalf("unexpected ready usage_detail result: %#v", got)
	}
}

func retentionTestConfig(dataClass string, retentionDays int) RetentionConfig {
	cfg := RetentionConfig{
		Enabled:          true,
		DryRun:           boolPtr(true),
		DefaultBatchSize: 100,
		Classes: []RetentionClassConfig{{
			DataClass:     dataClass,
			RetentionDays: retentionDays,
			BatchSize:     50,
		}},
	}
	if dataClass == retentionDataClassUsageDetail {
		cfg.Classes[0].RequireFinalizedRollup = true
	}
	return cfg
}

func TestDecisionTelemetryTablesReferenceRequestUsage(t *testing.T) {
	store, err := OpenUsageStorePath(filepath.Join(t.TempDir(), "usage.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	for _, table := range []string{
		"request_decision_shape_features",
		"request_target_candidates",
		"request_target_filter_reasons",
		"request_routing_decisions",
		"request_cache_reasons",
	} {
		assertRequestUsageForeignKey(t, store, table)
	}
}

func assertRequestUsageForeignKey(t *testing.T, store *usageStore, table string) {
	t.Helper()
	type fkRow struct {
		Table string
		From  string
		To    string
	}
	var rows []fkRow
	query := map[string]string{
		"request_decision_shape_features": "PRAGMA foreign_key_list(request_decision_shape_features)",
		"request_target_candidates":       "PRAGMA foreign_key_list(request_target_candidates)",
		"request_target_filter_reasons":   "PRAGMA foreign_key_list(request_target_filter_reasons)",
		"request_routing_decisions":       "PRAGMA foreign_key_list(request_routing_decisions)",
		"request_cache_reasons":           "PRAGMA foreign_key_list(request_cache_reasons)",
	}[table]
	if query == "" {
		t.Fatalf("unknown table %s", table)
	}
	if err := store.db.Raw(query).Scan(&rows).Error; err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row.Table == "request_usage" && row.From == "request_id" && row.To == "request_id" {
			return
		}
	}
	t.Fatalf("%s does not reference request_usage(request_id): %#v", table, rows)
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
