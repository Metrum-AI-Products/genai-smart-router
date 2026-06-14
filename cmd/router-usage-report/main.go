package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"smart-llmrouter/internal/router"
)

func main() {
	driver := flag.String("driver", "sqlite", "usage DB driver: sqlite or postgres")
	dbPath := flag.String("db", "usage.sqlite", "path to usage SQLite database")
	dsn := flag.String("dsn", "", "Postgres DSN when --driver=postgres")
	logPath := flag.String("log", "", "optional JSONL request log to import before reporting")
	fromText := flag.String("from", "", "report start time, RFC3339 or 2006-01-02T15:04:05Z")
	toText := flag.String("to", "", "report end time, RFC3339 or 2006-01-02T15:04:05Z; defaults to now")
	sinceText := flag.String("since", "24h", "relative report duration when --from is omitted, such as 24h, 7d, 30d")
	outPath := flag.String("out", "", "optional markdown output path; defaults to stdout")
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

	md, err := router.GenerateUsageMarkdown(router.UsageReportOptions{
		Driver:  *driver,
		DBPath:  *dbPath,
		DSN:     *dsn,
		LogPath: *logPath,
		From:    from,
		To:      to,
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
