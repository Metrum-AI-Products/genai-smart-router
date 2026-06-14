package router

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type usageStore struct {
	db *sql.DB
}

type UsageReportOptions struct {
	DBPath  string
	LogPath string
	From    time.Time
	To      time.Time
}

type usageRow struct {
	TS                time.Time
	RequestID         string
	CallerID          string
	CallerUser        string
	CallerProject     string
	CallerEnvironment string
	TokenID           string
	Client            string
	InboundDialect    string
	RequestedModel    string
	ResolvedGroup     string
	Strategy          string
	TargetProvider    string
	TargetModel       string
	TargetDialect     string
	Stream            bool
	Cache             string
	Status            int
	Attempts          int
	FallbackUsed      bool
	LatencyMS         int64
	TTFBMS            *int64
	InputTokens       int
	OutputTokens      int
	TotalTokens       int
	QuotaState        string
	KeyState          string
	Error             string
}

type agg struct {
	Calls        int64
	Errors       int64
	Streams      int64
	CacheHits    int64
	CacheMisses  int64
	CacheBypass  int64
	Fallbacks    int64
	Attempts     int64
	InputTokens  int64
	OutputTokens int64
	TotalTokens  int64
	LatencyMS    int64
	MaxLatencyMS int64
	TTFBMS       int64
	TTFBCount    int64
	MaxTTFBMS    int64
}

func newUsageStore(cfg UsageDBConfig) (*usageStore, error) {
	if cfg.Enable != nil && !*cfg.Enable {
		return nil, nil
	}
	if cfg.Path == "" {
		return nil, nil
	}
	store, err := OpenUsageStore(cfg.Path)
	if err != nil {
		return nil, err
	}
	return store, nil
}

func OpenUsageStore(path string) (*usageStore, error) {
	if path == "" {
		return nil, errors.New("usage db path is required")
	}
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return nil, err
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	store := &usageStore{db: db}
	if err := store.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *usageStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *usageStore) migrate() error {
	stmts := []string{
		`PRAGMA journal_mode=WAL`,
		`CREATE TABLE IF NOT EXISTS request_usage (
			request_id TEXT PRIMARY KEY,
			ts TEXT NOT NULL,
			caller_id TEXT NOT NULL,
			caller_user TEXT NOT NULL,
			caller_project TEXT NOT NULL,
			caller_environment TEXT NOT NULL,
			token_id TEXT NOT NULL,
			client TEXT NOT NULL,
			inbound_dialect TEXT NOT NULL,
			requested_model TEXT NOT NULL,
			resolved_group TEXT NOT NULL,
			strategy TEXT NOT NULL,
			target_provider TEXT NOT NULL,
			target_model TEXT NOT NULL,
			target_dialect TEXT NOT NULL,
			stream INTEGER NOT NULL,
			cache TEXT NOT NULL,
			status INTEGER NOT NULL,
			attempts INTEGER NOT NULL,
			fallback_used INTEGER NOT NULL,
			latency_ms INTEGER NOT NULL,
			ttfb_ms INTEGER,
			input_tokens INTEGER NOT NULL,
			output_tokens INTEGER NOT NULL,
			total_tokens INTEGER NOT NULL,
			quota_state TEXT NOT NULL,
			key_state TEXT NOT NULL,
			error TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_request_usage_ts ON request_usage(ts)`,
		`CREATE INDEX IF NOT EXISTS idx_request_usage_token ON request_usage(token_id, ts)`,
		`CREATE INDEX IF NOT EXISTS idx_request_usage_provider_model ON request_usage(target_provider, target_model, ts)`,
		`CREATE INDEX IF NOT EXISTS idx_request_usage_group ON request_usage(resolved_group, ts)`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

func (s *usageStore) Emit(rec logRecord) {
	if s == nil || s.db == nil || rec.RequestID == "" {
		return
	}
	row := rowFromRecord(rec)
	_, _ = s.db.Exec(`INSERT OR IGNORE INTO request_usage (
		request_id, ts, caller_id, caller_user, caller_project, caller_environment, token_id,
		client, inbound_dialect, requested_model, resolved_group, strategy, target_provider,
		target_model, target_dialect, stream, cache, status, attempts, fallback_used,
		latency_ms, ttfb_ms, input_tokens, output_tokens, total_tokens, quota_state, key_state, error
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		row.RequestID, formatUsageTime(row.TS), row.CallerID, row.CallerUser, row.CallerProject,
		row.CallerEnvironment, row.TokenID, row.Client, row.InboundDialect, row.RequestedModel,
		row.ResolvedGroup, row.Strategy, row.TargetProvider, row.TargetModel, row.TargetDialect,
		boolInt(row.Stream), row.Cache, row.Status, row.Attempts, boolInt(row.FallbackUsed),
		row.LatencyMS, row.TTFBMS, row.InputTokens, row.OutputTokens, row.TotalTokens,
		row.QuotaState, row.KeyState, row.Error)
}

func rowFromRecord(rec logRecord) usageRow {
	ts, err := parseUsageTime(rec.TS)
	if err != nil {
		ts = time.Now().UTC()
	}
	errText := ""
	if rec.Error != nil {
		errText = *rec.Error
	}
	tokenID := rec.TokenID
	if rec.CallerID == "" {
		tokenID = defaultString(rec.TokenID, "unauthorized")
		if tokenID != "missing-token" {
			tokenID = "invalid-token"
		}
	} else {
		tokenID = publicTokenID(tokenID)
	}
	return usageRow{
		TS:                ts,
		RequestID:         rec.RequestID,
		CallerID:          rec.CallerID,
		CallerUser:        rec.CallerUser,
		CallerProject:     rec.CallerProject,
		CallerEnvironment: rec.CallerEnvironment,
		TokenID:           tokenID,
		Client:            rec.Client,
		InboundDialect:    rec.InboundDialect,
		RequestedModel:    rec.RequestedModel,
		ResolvedGroup:     defaultString(rec.ResolvedGroup, rec.RequestedModel),
		Strategy:          rec.Strategy,
		TargetProvider:    rec.TargetProvider,
		TargetModel:       rec.TargetModel,
		TargetDialect:     rec.TargetDialect,
		Stream:            rec.Stream,
		Cache:             defaultString(rec.Cache, "bypass"),
		Status:            rec.Status,
		Attempts:          rec.Attempts,
		FallbackUsed:      rec.FallbackUsed,
		LatencyMS:         rec.LatencyMS,
		TTFBMS:            rec.TTFBMS,
		InputTokens:       rec.Usage.InputTokens,
		OutputTokens:      rec.Usage.OutputTokens,
		TotalTokens:       rec.Usage.TotalTokens,
		QuotaState:        rec.QuotaState,
		KeyState:          rec.KeyState,
		Error:             errText,
	}
}

func ImportUsageJSONL(dbPath, logPath string) (int, error) {
	store, err := OpenUsageStore(dbPath)
	if err != nil {
		return 0, err
	}
	defer store.Close()

	f, err := os.Open(logPath)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	count := 0
	scanner := bufio.NewScanner(f)
	buf := make([]byte, 0, 1024*1024)
	scanner.Buffer(buf, 16*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var rec logRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			return count, fmt.Errorf("parse %s line %d: %w", logPath, count+1, err)
		}
		store.Emit(rec)
		count++
	}
	return count, scanner.Err()
}

func GenerateUsageMarkdown(opts UsageReportOptions) (string, error) {
	if opts.DBPath == "" {
		return "", errors.New("usage db path is required")
	}
	if opts.To.IsZero() {
		opts.To = time.Now().UTC()
	}
	if opts.From.IsZero() {
		opts.From = opts.To.Add(-24 * time.Hour)
	}
	if !opts.From.Before(opts.To) {
		return "", errors.New("from must be before to")
	}
	if opts.LogPath != "" {
		if _, err := ImportUsageJSONL(opts.DBPath, opts.LogPath); err != nil {
			return "", err
		}
	}
	store, err := OpenUsageStore(opts.DBPath)
	if err != nil {
		return "", err
	}
	defer store.Close()

	rows, err := store.rows(opts.From, opts.To)
	if err != nil {
		return "", err
	}
	return renderUsageMarkdown(opts.From, opts.To, rows), nil
}

func (s *usageStore) rows(from, to time.Time) ([]usageRow, error) {
	rs, err := s.db.Query(`SELECT
		ts, request_id, caller_id, caller_user, caller_project, caller_environment, token_id,
		client, inbound_dialect, requested_model, resolved_group, strategy, target_provider,
		target_model, target_dialect, stream, cache, status, attempts, fallback_used,
		latency_ms, ttfb_ms, input_tokens, output_tokens, total_tokens, quota_state, key_state, error
		FROM request_usage
		WHERE ts >= ? AND ts < ?
		ORDER BY ts ASC`, formatUsageTime(from), formatUsageTime(to))
	if err != nil {
		return nil, err
	}
	defer rs.Close()

	var out []usageRow
	for rs.Next() {
		var row usageRow
		var ts string
		var stream, fallback int
		var ttfb sql.NullInt64
		if err := rs.Scan(&ts, &row.RequestID, &row.CallerID, &row.CallerUser, &row.CallerProject,
			&row.CallerEnvironment, &row.TokenID, &row.Client, &row.InboundDialect, &row.RequestedModel,
			&row.ResolvedGroup, &row.Strategy, &row.TargetProvider, &row.TargetModel, &row.TargetDialect,
			&stream, &row.Cache, &row.Status, &row.Attempts, &fallback, &row.LatencyMS, &ttfb,
			&row.InputTokens, &row.OutputTokens, &row.TotalTokens, &row.QuotaState, &row.KeyState, &row.Error); err != nil {
			return nil, err
		}
		parsed, err := parseUsageTime(ts)
		if err != nil {
			return nil, err
		}
		row.TS = parsed
		row.Stream = stream != 0
		row.FallbackUsed = fallback != 0
		if ttfb.Valid {
			v := ttfb.Int64
			row.TTFBMS = &v
		}
		out = append(out, row)
	}
	return out, rs.Err()
}

func renderUsageMarkdown(from, to time.Time, rows []usageRow) string {
	total := &agg{}
	byToken := map[string]*agg{}
	byTokenMeta := map[string]usageRow{}
	byModel := map[string]*agg{}
	byGroup := map[string]*agg{}
	byClient := map[string]*agg{}
	byStatus := map[string]*agg{}
	byHour := map[string]*agg{}
	byDay := map[string]*agg{}

	for _, row := range rows {
		total.add(row)
		tokenKey := joinKey(row.TokenID, row.CallerUser, row.CallerProject, row.CallerEnvironment, row.CallerID)
		byToken[tokenKey] = getAgg(byToken, tokenKey)
		byToken[tokenKey].add(row)
		byTokenMeta[tokenKey] = row
		modelKey := joinKey(row.TargetProvider, row.TargetModel)
		byModel[modelKey] = getAgg(byModel, modelKey)
		byModel[modelKey].add(row)
		groupKey := defaultString(row.ResolvedGroup, row.RequestedModel)
		byGroup[groupKey] = getAgg(byGroup, groupKey)
		byGroup[groupKey].add(row)
		clientKey := defaultString(row.Client, "unknown")
		byClient[clientKey] = getAgg(byClient, clientKey)
		byClient[clientKey].add(row)
		statusKey := fmt.Sprint(row.Status)
		byStatus[statusKey] = getAgg(byStatus, statusKey)
		byStatus[statusKey].add(row)
		hourKey := row.TS.UTC().Truncate(time.Hour).Format("2006-01-02 15:00")
		byHour[hourKey] = getAgg(byHour, hourKey)
		byHour[hourKey].add(row)
		dayKey := row.TS.UTC().Format("2006-01-02")
		byDay[dayKey] = getAgg(byDay, dayKey)
		byDay[dayKey].add(row)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# Smart LLM Router Usage Report\n\n")
	fmt.Fprintf(&b, "- Period UTC: `%s` to `%s`\n", formatUsageTime(from), formatUsageTime(to))
	fmt.Fprintf(&b, "- Requests: `%d`\n", total.Calls)
	fmt.Fprintf(&b, "- Errors: `%d`\n", total.Errors)
	fmt.Fprintf(&b, "- Tokens: `%d` total, `%d` input, `%d` output\n", total.TotalTokens, total.InputTokens, total.OutputTokens)
	fmt.Fprintf(&b, "- Cache: `%d` hits, `%d` misses, `%d` bypass\n", total.CacheHits, total.CacheMisses, total.CacheBypass)
	fmt.Fprintf(&b, "- Upstream attempts: `%d`; fallbacks: `%d`; streaming requests: `%d`\n", total.Attempts, total.Fallbacks, total.Streams)
	fmt.Fprintf(&b, "- Latency: `%d ms` avg, `%d ms` max\n\n", avg(total.LatencyMS, total.Calls), total.MaxLatencyMS)

	writeTokenTable(&b, "Usage By Internal API Key", byToken, byTokenMeta)
	writeAggTable(&b, "Usage By External Model", []string{"Provider", "Model"}, byModel, splitKey2)
	writeAggTable(&b, "Usage By Router Model Group", []string{"Model Group"}, byGroup, splitKey1)
	writeAggTable(&b, "Usage By Client", []string{"Client"}, byClient, splitKey1)
	writeAggTable(&b, "Usage By Status", []string{"Status"}, byStatus, splitKey1)
	writeAggTable(&b, "Hourly Usage", []string{"Hour UTC"}, byHour, splitKey1)
	writeAggTable(&b, "Daily Usage", []string{"Day UTC"}, byDay, splitKey1)
	return b.String()
}

func (a *agg) add(row usageRow) {
	a.Calls++
	if row.Status >= 400 {
		a.Errors++
	}
	if row.Stream {
		a.Streams++
	}
	switch row.Cache {
	case "hit":
		a.CacheHits++
	case "miss":
		a.CacheMisses++
	default:
		a.CacheBypass++
	}
	if row.FallbackUsed {
		a.Fallbacks++
	}
	a.Attempts += int64(row.Attempts)
	a.InputTokens += int64(row.InputTokens)
	a.OutputTokens += int64(row.OutputTokens)
	total := row.TotalTokens
	if total == 0 {
		total = row.InputTokens + row.OutputTokens
	}
	a.TotalTokens += int64(total)
	a.LatencyMS += row.LatencyMS
	if row.LatencyMS > a.MaxLatencyMS {
		a.MaxLatencyMS = row.LatencyMS
	}
	if row.TTFBMS != nil {
		a.TTFBMS += *row.TTFBMS
		a.TTFBCount++
		if *row.TTFBMS > a.MaxTTFBMS {
			a.MaxTTFBMS = *row.TTFBMS
		}
	}
}

func writeTokenTable(b *strings.Builder, title string, data map[string]*agg, meta map[string]usageRow) {
	fmt.Fprintf(b, "## %s\n\n", title)
	fmt.Fprintln(b, "| Token ID | User | Project | Env | Caller ID | Calls | Errors | Tokens | Input | Output | Cache Hit | Cache Miss | Attempts | Fallbacks | Avg Latency ms | Max Latency ms |")
	fmt.Fprintln(b, "|---|---|---|---|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|")
	for _, key := range sortedAggKeys(data) {
		row := meta[key]
		a := data[key]
		fmt.Fprintf(b, "| `%s` | %s | %s | %s | `%s` | %d | %d | %d | %d | %d | %d | %d | %d | %d | %d | %d |\n",
			esc(row.TokenID), esc(row.CallerUser), esc(row.CallerProject), esc(row.CallerEnvironment), esc(row.CallerID),
			a.Calls, a.Errors, a.TotalTokens, a.InputTokens, a.OutputTokens, a.CacheHits, a.CacheMisses, a.Attempts,
			a.Fallbacks, avg(a.LatencyMS, a.Calls), a.MaxLatencyMS)
	}
	if len(data) == 0 {
		fmt.Fprintln(b, "| _none_ |  |  |  |  | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 |")
	}
	fmt.Fprintln(b)
}

func writeAggTable(b *strings.Builder, title string, keyHeaders []string, data map[string]*agg, split func(string) []string) {
	fmt.Fprintf(b, "## %s\n\n", title)
	for _, h := range keyHeaders {
		fmt.Fprintf(b, "| %s ", h)
	}
	fmt.Fprintln(b, "| Calls | Errors | Tokens | Input | Output | Cache Hit | Cache Miss | Cache Bypass | Attempts | Fallbacks | Streams | Avg Latency ms | Max Latency ms | Avg TTFB ms | Max TTFB ms |")
	for range keyHeaders {
		fmt.Fprint(b, "|---")
	}
	fmt.Fprintln(b, "|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|")
	for _, key := range sortedAggKeys(data) {
		parts := split(key)
		for _, part := range parts {
			fmt.Fprintf(b, "| %s ", esc(part))
		}
		a := data[key]
		fmt.Fprintf(b, "| %d | %d | %d | %d | %d | %d | %d | %d | %d | %d | %d | %d | %d | %d | %d |\n",
			a.Calls, a.Errors, a.TotalTokens, a.InputTokens, a.OutputTokens, a.CacheHits, a.CacheMisses, a.CacheBypass,
			a.Attempts, a.Fallbacks, a.Streams, avg(a.LatencyMS, a.Calls), a.MaxLatencyMS, avg(a.TTFBMS, a.TTFBCount), a.MaxTTFBMS)
	}
	if len(data) == 0 {
		for range keyHeaders {
			fmt.Fprint(b, "| _none_ ")
		}
		fmt.Fprintln(b, "| 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 |")
	}
	fmt.Fprintln(b)
}

func getAgg(m map[string]*agg, key string) *agg {
	if m[key] == nil {
		m[key] = &agg{}
	}
	return m[key]
}

func sortedAggKeys(m map[string]*agg) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		ai, aj := m[keys[i]], m[keys[j]]
		if ai.TotalTokens != aj.TotalTokens {
			return ai.TotalTokens > aj.TotalTokens
		}
		if ai.Calls != aj.Calls {
			return ai.Calls > aj.Calls
		}
		return keys[i] < keys[j]
	})
	return keys
}

func joinKey(parts ...string) string {
	return strings.Join(parts, "\x00")
}

func splitKey1(key string) []string {
	return []string{key}
}

func splitKey2(key string) []string {
	parts := strings.SplitN(key, "\x00", 2)
	if len(parts) == 1 {
		return []string{parts[0], ""}
	}
	return parts
}

func esc(v string) string {
	v = strings.ReplaceAll(v, "|", `\|`)
	v = strings.ReplaceAll(v, "\n", " ")
	if v == "" {
		return ""
	}
	return v
}

func avg(sum, n int64) int64 {
	if n <= 0 {
		return 0
	}
	return sum / n
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func formatUsageTime(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05.000Z")
}

func parseUsageTime(v string) (time.Time, error) {
	if v == "" {
		return time.Time{}, errors.New("empty time")
	}
	layouts := []string{"2006-01-02T15:04:05.000Z", time.RFC3339Nano, time.RFC3339}
	var last error
	for _, layout := range layouts {
		t, err := time.Parse(layout, v)
		if err == nil {
			return t.UTC(), nil
		}
		last = err
	}
	return time.Time{}, last
}
