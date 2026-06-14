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
		`{"ts":"2026-06-14T01:15:00.000Z","request_id":"req_1","caller_id":"sudarshan-prod","caller_user":"sudarshan","caller_project":"metrum-insights","caller_environment":"prod","token_id":"rtr_metrum_sudarshan_metrum-insights_prod_k20260614","client":"codex","inbound_dialect":"openai-responses","requested_model":"big-coder","resolved_group":"big-coder","strategy":"failover","target_provider":"openai","target_model":"gpt-5.4","target_dialect":"openai-responses","stream":false,"cache":"miss","status":200,"attempts":1,"fallback_used":false,"latency_ms":1200,"ttfb_ms":300,"usage":{"input_tokens":100,"output_tokens":40,"total_tokens":140},"quota_state":"ok","key_state":"active","warnings":[]}`,
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
