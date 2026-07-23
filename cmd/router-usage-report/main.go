package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
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
	usageDBConfigPath := flag.String("usage-db-config", "", "protected router config path from which to read usage DB settings")
	logPath := flag.String("log", "", "optional JSONL request log to import before reporting")
	fromText := flag.String("from", "", "report start time, RFC3339 or 2006-01-02T15:04:05Z")
	toText := flag.String("to", "", "report end time, RFC3339 or 2006-01-02T15:04:05Z; defaults to now")
	sinceText := flag.String("since", "24h", "relative report duration when --from is omitted, such as 24h, 7d, 30d")
	outPath := flag.String("out", "", "optional markdown output path; defaults to stdout")
	reasoningCoverageOut := flag.String("reasoning-coverage-out", "", "write aggregate-only reasoning coverage JSON for a protected evaluation report")
	tokenID := flag.String("token-id", "", "filter report to one public router token id")
	callerID := flag.String("caller-id", "", "filter report to one configured caller id")
	tokenIDPrefix := flag.String("token-id-prefix", "", "filter report to public router token ids with this prefix")
	callerUser := flag.String("caller-user", "", "filter report to one caller owner user")
	callerProject := flag.String("caller-project", "", "filter report to one caller project")
	callerEnvironment := flag.String("caller-environment", "", "filter report to one caller environment")
	resolvedGroup := flag.String("resolved-group", "", "filter report to one resolved router model group")
	provider := flag.String("provider", "", "filter report to one upstream provider")
	targetModel := flag.String("target-model", "", "filter report to one upstream target model")
	client := flag.String("client", "", "filter report to one client, such as codex or claude-code")
	trafficTuningAdvisor := flag.Bool("traffic-tuning-advisor", false, "generate a traffic-shaping tuning advisor instead of the full usage report")
	trafficShapedOnly := flag.Bool("traffic-shaped-only", false, "filter report to requests with caller or upstream traffic shaping events")
	trafficShapeBucket := flag.String("traffic-shape-bucket", "", "filter report to one traffic-shaping bucket")
	trafficShapeScope := flag.String("traffic-shape-scope", "", "filter report to one traffic-shaping scope")
	rollup := flag.Bool("rollup", false, "generate a daily usage rollup instead of markdown")
	rollupType := flag.String("rollup-type", "daily", "rollup granularity: hourly, daily, or monthly")
	rollupFinalize := flag.Bool("rollup-finalize", false, "finalize the generated rollup window; finalized windows are immutable")
	baselineID := flag.String("baseline-id", "", "optional savings baseline id to store on rollup rows")
	baselineName := flag.String("baseline-name", "", "optional savings baseline name to store on rollup rows")
	baselineVersion := flag.String("baseline-version", "", "optional savings baseline version/source date to store on rollup rows")
	baselineInputPrice := flag.Float64("baseline-input-price-per-million-usd", 0, "optional baseline input price in USD per million tokens")
	baselineOutputPrice := flag.Float64("baseline-output-price-per-million-usd", 0, "optional baseline output price in USD per million tokens")
	retentionStatus := flag.Bool("retention-status", false, "record a dry-run retention status job from --config")
	retentionRun := flag.Bool("retention-run", false, "run retention from --config; deletes one batch per supported table only when config dry_run=false")
	configPath := flag.String("config", "", "router config path for --retention-status or --retention-run")
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

	if *retentionStatus && *retentionRun {
		die("choose only one of --retention-status or --retention-run")
	}
	if *retentionStatus || *retentionRun {
		if *configPath == "" {
			die("--retention-status or --retention-run requires --config")
		}
		cfg, err := router.LoadConfig(*configPath)
		if err != nil {
			die("load config: %v", err)
		}
		opts := router.RetentionStatusOptions{
			Driver:      cfg.Server.UsageDB.Driver,
			DBPath:      cfg.Server.UsageDB.Path,
			DSN:         cfg.Server.UsageDB.DSN,
			Config:      cfg.Server.Retention,
			Now:         to,
			RequestedBy: "router-usage-report",
		}
		var result router.RetentionStatusResult
		if *retentionStatus {
			result, err = router.GenerateRetentionStatus(opts)
		} else {
			result, err = router.GenerateRetentionRun(opts)
		}
		if err != nil {
			die("retention: %v", err)
		}
		fmt.Printf("retention %s job_id=%d policy_version_id=%d completed_at=%s\n",
			result.Status, result.JobID, result.PolicyVersionID, result.CompletedAt.Format(time.RFC3339))
		for _, table := range result.TableResults {
			fmt.Printf("%s %s cutoff=%s candidates=%d held=%d eligible=%d blocked=%d deleted=%d status=%s\n",
				table.DataClass, table.TableName, table.Cutoff.Format(time.RFC3339), table.CandidateRows, table.HeldRows,
				table.EligibleRows, table.BlockedRows, table.DeletedRows, table.Status)
		}
		return
	}
	if *usageDBConfigPath != "" {
		cfg, err := router.LoadConfig(*usageDBConfigPath)
		if err != nil {
			die("load --usage-db-config: %v", err)
		}
		*driver = cfg.Server.UsageDB.Driver
		*dbPath = cfg.Server.UsageDB.Path
		*dsn = cfg.Server.UsageDB.DSN
	}

	if *rollup {
		result, err := router.GenerateUsageRollup(router.UsageRollupOptions{
			Driver:                           *driver,
			DBPath:                           *dbPath,
			DSN:                              *dsn,
			RollupType:                       *rollupType,
			From:                             from,
			To:                               to,
			Finalize:                         *rollupFinalize,
			BaselineID:                       *baselineID,
			BaselineName:                     *baselineName,
			BaselineVersion:                  *baselineVersion,
			BaselineInputPricePerMillionUSD:  *baselineInputPrice,
			BaselineOutputPricePerMillionUSD: *baselineOutputPrice,
		})
		if err != nil {
			die("generate rollup: %v", err)
		}
		fmt.Printf("usage rollup %s %s run_id=%d window=%s..%s source_requests=%d rollup_rows=%d decision_bucket_rows=%d checksum=%s\n",
			result.RollupType, result.Status, result.RunID, result.WindowStart.Format(time.RFC3339), result.WindowEnd.Format(time.RFC3339),
			result.SourceRequestCount, result.RollupRows, result.DecisionBucketRows, result.SourceChecksum)
		return
	}

	reportOpts := router.UsageReportOptions{
		Driver:             *driver,
		DBPath:             *dbPath,
		DSN:                *dsn,
		LogPath:            *logPath,
		From:               from,
		To:                 to,
		CallerID:           *callerID,
		TokenID:            *tokenID,
		TokenIDPrefix:      *tokenIDPrefix,
		CallerUser:         *callerUser,
		CallerProject:      *callerProject,
		CallerEnvironment:  *callerEnvironment,
		ResolvedGroup:      *resolvedGroup,
		TargetProvider:     *provider,
		TargetModel:        *targetModel,
		Client:             *client,
		TrafficShapedOnly:  *trafficShapedOnly,
		TrafficShapeBucket: *trafficShapeBucket,
		TrafficShapeScope:  *trafficShapeScope,
	}
	if *reasoningCoverageOut != "" {
		coverage, err := router.ExportReasoningCoverage(reportOpts)
		if err != nil {
			die("export reasoning coverage: %v", err)
		}
		payload, err := json.MarshalIndent(coverage, "", "  ")
		if err != nil {
			die("encode reasoning coverage: %v", err)
		}
		if err := writePrivateFile(*reasoningCoverageOut, append(payload, '\n')); err != nil {
			die("write --reasoning-coverage-out: %v", err)
		}
		return
	}
	var md string
	if *trafficTuningAdvisor {
		md, err = router.GenerateTrafficTuningAdvisorMarkdown(reportOpts)
	} else {
		md, err = router.GenerateUsageMarkdown(reportOpts)
	}
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

func writePrivateFile(path string, contents []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".router-usage-report-")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(contents); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
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
