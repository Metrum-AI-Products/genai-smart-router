package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"smart-llmrouter/internal/buildinfo"
	"smart-llmrouter/internal/router"
)

func main() {
	if len(os.Args) == 2 && os.Args[1] == "--version" {
		fmt.Println(buildinfo.Text())
		return
	}
	driver := flag.String("driver", "sqlite", "usage DB driver: sqlite or postgres")
	dbPath := flag.String("db", "usage.sqlite", "path to usage SQLite database")
	dsn := flag.String("dsn", "", "Postgres DSN when --driver=postgres")
	logPath := flag.String("log", "", "optional JSONL request log to import before reporting")
	fromText := flag.String("from", "", "report start time, RFC3339 or 2006-01-02T15:04:05Z")
	toText := flag.String("to", "", "report end time, RFC3339 or 2006-01-02T15:04:05Z; defaults to now")
	sinceText := flag.String("since", "24h", "relative report duration when --from is omitted, such as 24h, 7d, 30d")
	outPath := flag.String("out", "", "optional markdown output path; defaults to stdout")
	tokenID := flag.String("token-id", "", "filter report to one public router token id")
	tokenIDPrefix := flag.String("token-id-prefix", "", "filter report to public router token ids with this prefix")
	callerUser := flag.String("caller-user", "", "filter report to one caller owner user")
	callerProject := flag.String("caller-project", "", "filter report to one caller project")
	callerEnvironment := flag.String("caller-environment", "", "filter report to one caller environment")
	resolvedGroup := flag.String("resolved-group", "", "filter report to one resolved router model group")
	client := flag.String("client", "", "filter report to one client, such as codex or claude-code")
	rollup := flag.Bool("rollup", false, "generate a daily usage rollup instead of markdown")
	rollupFinalize := flag.Bool("rollup-finalize", false, "finalize the generated rollup window; finalized windows are immutable")
	flag.Parse()

	to := time.Now().UTC()
	var err error
	if *toText != "" {
		to, err = parseTime(*toText)
		if err != nil {
			die("parse --to: %v", err)
		}
	}

	var from time.Time
	if *fromText != "" {
		from, err = parseTime(*fromText)
		if err != nil {
			die("parse --from: %v", err)
		}
	} else {
		d, err := parseDuration(*sinceText)
		if err != nil {
			die("parse --since: %v", err)
		}
		from = to.Add(-d)
	}

	if *rollup {
		result, err := router.GenerateUsageRollup(router.UsageRollupOptions{
			Driver:   *driver,
			DBPath:   *dbPath,
			DSN:      *dsn,
			From:     from,
			To:       to,
			Finalize: *rollupFinalize,
		})
		if err != nil {
			die("generate rollup: %v", err)
		}
		fmt.Printf("usage rollup %s run_id=%d window=%s..%s source_requests=%d daily_rows=%d\n",
			result.Status, result.RunID, result.WindowStart.Format(time.RFC3339), result.WindowEnd.Format(time.RFC3339),
			result.SourceRequestCount, result.DailyRows)
		return
	}

	md, err := router.GenerateUsageMarkdown(router.UsageReportOptions{
		Driver:            *driver,
		DBPath:            *dbPath,
		DSN:               *dsn,
		LogPath:           *logPath,
		From:              from,
		To:                to,
		TokenID:           *tokenID,
		TokenIDPrefix:     *tokenIDPrefix,
		CallerUser:        *callerUser,
		CallerProject:     *callerProject,
		CallerEnvironment: *callerEnvironment,
		ResolvedGroup:     *resolvedGroup,
		Client:            *client,
	})
	if err != nil {
		die("generate report: %v", err)
	}
	if *outPath == "" {
		fmt.Print(md)
		return
	}
	if err := os.WriteFile(*outPath, []byte(md), 0600); err != nil {
		die("write --out: %v", err)
	}
}

func parseTime(v string) (time.Time, error) {
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05Z",
		"2006-01-02 15:04:05",
		"2006-01-02",
	}
	for _, layout := range layouts {
		t, err := time.Parse(layout, v)
		if err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("unsupported time %q", v)
}

func parseDuration(v string) (time.Duration, error) {
	if v == "" {
		return 24 * time.Hour, nil
	}
	if len(v) > 1 {
		suffix := v[len(v)-1]
		number := v[:len(v)-1]
		switch suffix {
		case 'd':
			h, err := time.ParseDuration(number + "h")
			if err != nil {
				return 0, err
			}
			return h * 24, nil
		case 'w':
			h, err := time.ParseDuration(number + "h")
			if err != nil {
				return 0, err
			}
			return h * 24 * 7, nil
		}
	}
	return time.ParseDuration(v)
}

func die(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(2)
}
