package router

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

type usageStore struct {
	db *gorm.DB
}

type UsageReportOptions struct {
	Driver            string
	DBPath            string
	DSN               string
	LogPath           string
	From              time.Time
	To                time.Time
	TokenID           string
	TokenIDPrefix     string
	CallerProject     string
	CallerEnvironment string
	ResolvedGroup     string
	Client            string
}

type usageRow struct {
	TS                                 time.Time
	RequestID                          string
	CallerID                           string
	CallerUser                         string
	CallerProject                      string
	CallerEnvironment                  string
	CallerIP                           string
	TokenID                            string
	Client                             string
	InboundDialect                     string
	RequestedModel                     string
	ResolvedGroup                      string
	Strategy                           string
	TargetProvider                     string
	TargetModel                        string
	TargetDialect                      string
	Stream                             bool
	Cache                              string
	Status                             int
	Attempts                           int
	FallbackUsed                       bool
	LatencyMS                          int64
	TTFBMS                             *int64
	UpstreamMS                         *int64
	DownstreamMS                       *int64
	UpstreamOutputTPS                  *float64
	UpstreamTotalTPS                   *float64
	DownstreamOutputTPS                *float64
	DownstreamTotalTPS                 *float64
	InputTokens                        int
	OutputTokens                       int
	TotalTokens                        int
	InputHasImage                      bool
	InputImageCount                    int
	InputImageTokens                   int
	PIIFilterApplied                   bool
	PIIFilterMode                      string
	PIIFilterReplacements              int
	PIIFilterRuleCount                 int
	InputPricePerMillionUSD            float64
	OutputPricePerMillionUSD           float64
	ImageInputPricePerMillionTokensUSD float64
	ImageInputPricePerImageUSD         float64
	InputCostUSD                       float64
	ImageCostUSD                       float64
	OutputCostUSD                      float64
	TotalCostUSD                       float64
	UpstreamReportedInputCostUSD       float64
	UpstreamReportedOutputCostUSD      float64
	UpstreamReportedTotalCostUSD       float64
	PricingSource                      string
	PricingUpdatedAt                   string
	CacheEnabled                       bool
	CacheItems                         int64
	CacheBytes                         int64
	CacheMaxBytes                      int64
	CacheOccupancyPct                  float64
	QuotaState                         string
	KeyState                           string
	Error                              string
}

type usageRecord struct {
	RequestID                          string   `gorm:"column:request_id;primaryKey;type:text"`
	TS                                 string   `gorm:"column:ts;type:text;not null;index:idx_request_usage_ts"`
	CallerID                           string   `gorm:"column:caller_id;type:text;not null"`
	CallerUser                         string   `gorm:"column:caller_user;type:text;not null"`
	CallerProject                      string   `gorm:"column:caller_project;type:text;not null"`
	CallerEnvironment                  string   `gorm:"column:caller_environment;type:text;not null"`
	CallerIP                           string   `gorm:"column:caller_ip;type:text;index:idx_request_usage_caller_ip,priority:1"`
	TokenID                            string   `gorm:"column:token_id;type:text;not null;index:idx_request_usage_token,priority:1"`
	Client                             string   `gorm:"column:client;type:text;not null"`
	InboundDialect                     string   `gorm:"column:inbound_dialect;type:text;not null"`
	RequestedModel                     string   `gorm:"column:requested_model;type:text;not null"`
	ResolvedGroup                      string   `gorm:"column:resolved_group;type:text;not null;index:idx_request_usage_group,priority:1"`
	Strategy                           string   `gorm:"column:strategy;type:text;not null"`
	TargetProvider                     string   `gorm:"column:target_provider;type:text;not null;index:idx_request_usage_provider_model,priority:1"`
	TargetModel                        string   `gorm:"column:target_model;type:text;not null;index:idx_request_usage_provider_model,priority:2"`
	TargetDialect                      string   `gorm:"column:target_dialect;type:text;not null"`
	Stream                             bool     `gorm:"column:stream;not null"`
	Cache                              string   `gorm:"column:cache;type:text;not null"`
	Status                             int      `gorm:"column:status;not null"`
	Attempts                           int      `gorm:"column:attempts;not null"`
	FallbackUsed                       bool     `gorm:"column:fallback_used;not null"`
	LatencyMS                          int64    `gorm:"column:latency_ms;not null"`
	TTFBMS                             *int64   `gorm:"column:ttfb_ms"`
	UpstreamMS                         *int64   `gorm:"column:upstream_duration_ms"`
	DownstreamMS                       *int64   `gorm:"column:downstream_duration_ms"`
	UpstreamOutputTPS                  *float64 `gorm:"column:upstream_output_tokens_per_sec"`
	UpstreamTotalTPS                   *float64 `gorm:"column:upstream_total_tokens_per_sec"`
	DownstreamOutputTPS                *float64 `gorm:"column:downstream_output_tokens_per_sec"`
	DownstreamTotalTPS                 *float64 `gorm:"column:downstream_total_tokens_per_sec"`
	InputTokens                        int      `gorm:"column:input_tokens;not null"`
	OutputTokens                       int      `gorm:"column:output_tokens;not null"`
	TotalTokens                        int      `gorm:"column:total_tokens;not null"`
	InputHasImage                      bool     `gorm:"column:input_has_image;not null;default:false;index:idx_request_usage_input_image"`
	InputImageCount                    int      `gorm:"column:input_image_count;not null;default:0"`
	InputImageTokens                   int      `gorm:"column:input_image_tokens;not null;default:0"`
	PIIFilterApplied                   bool     `gorm:"column:pii_filter_applied;not null;default:false;index:idx_request_usage_pii_filter"`
	PIIFilterMode                      string   `gorm:"column:pii_filter_mode;type:text;not null;default:''"`
	PIIFilterReplacements              int      `gorm:"column:pii_filter_replacements;not null;default:0"`
	PIIFilterRuleCount                 int      `gorm:"column:pii_filter_rule_count;not null;default:0"`
	InputPricePerMillionUSD            float64  `gorm:"column:input_price_per_million_usd;not null;default:0"`
	OutputPricePerMillionUSD           float64  `gorm:"column:output_price_per_million_usd;not null;default:0"`
	ImageInputPricePerMillionTokensUSD float64  `gorm:"column:image_input_price_per_million_tokens_usd;not null;default:0"`
	ImageInputPricePerImageUSD         float64  `gorm:"column:image_input_price_per_image_usd;not null;default:0"`
	InputCostUSD                       float64  `gorm:"column:input_cost_usd;not null;default:0"`
	ImageCostUSD                       float64  `gorm:"column:image_cost_usd;not null;default:0"`
	OutputCostUSD                      float64  `gorm:"column:output_cost_usd;not null;default:0"`
	TotalCostUSD                       float64  `gorm:"column:total_cost_usd;not null;default:0"`
	UpstreamReportedInputCostUSD       float64  `gorm:"column:upstream_reported_input_cost_usd;not null;default:0"`
	UpstreamReportedOutputCostUSD      float64  `gorm:"column:upstream_reported_output_cost_usd;not null;default:0"`
	UpstreamReportedTotalCostUSD       float64  `gorm:"column:upstream_reported_total_cost_usd;not null;default:0"`
	PricingSource                      string   `gorm:"column:pricing_source;type:text;not null;default:''"`
	PricingUpdatedAt                   string   `gorm:"column:pricing_updated_at;type:text;not null;default:''"`
	CacheEnabled                       bool     `gorm:"column:cache_enabled;not null"`
	CacheItems                         int64    `gorm:"column:cache_items;not null"`
	CacheBytes                         int64    `gorm:"column:cache_bytes;not null"`
	CacheMaxBytes                      int64    `gorm:"column:cache_max_bytes;not null"`
	CacheOccupancyPct                  float64  `gorm:"column:cache_occupancy_pct;not null"`
	QuotaState                         string   `gorm:"column:quota_state;type:text;not null"`
	KeyState                           string   `gorm:"column:key_state;type:text;not null"`
	Error                              string   `gorm:"column:error;type:text;not null"`
}

func (usageRecord) TableName() string {
	return "request_usage"
}

type requestAttemptRecord struct {
	RequestID        string `gorm:"column:request_id;primaryKey;type:text;index:idx_request_attempt_request"`
	AttemptIndex     int    `gorm:"column:attempt_index;primaryKey;not null"`
	TS               string `gorm:"column:ts;type:text;not null;index:idx_request_attempt_ts"`
	Provider         string `gorm:"column:provider;type:text;not null;index:idx_request_attempt_provider_model,priority:1"`
	Model            string `gorm:"column:model;type:text;not null;index:idx_request_attempt_provider_model,priority:2"`
	Dialect          string `gorm:"column:dialect;type:text;not null"`
	EndpointHost     string `gorm:"column:endpoint_host;type:text;not null"`
	DurationMS       int64  `gorm:"column:duration_ms;not null"`
	StatusCode       int    `gorm:"column:status_code;not null;index:idx_request_attempt_status"`
	ErrorClass       string `gorm:"column:error_class;type:text;not null;index:idx_request_attempt_error"`
	ErrorMessage     string `gorm:"column:error_message;type:text;not null"`
	Retryable        bool   `gorm:"column:retryable;not null"`
	TimedOut         bool   `gorm:"column:timed_out;not null;index:idx_request_attempt_timeout"`
	ClientCanceled   bool   `gorm:"column:client_canceled;not null;index:idx_request_attempt_cancel"`
	Selected         bool   `gorm:"column:selected;not null"`
	FallbackReason   string `gorm:"column:fallback_reason;type:text;not null"`
	RequestBytes     int64  `gorm:"column:request_bytes;not null"`
	ResponseBytes    int64  `gorm:"column:response_bytes;not null"`
	AttemptTimeoutMS int    `gorm:"column:attempt_timeout_ms;not null"`
}

func (requestAttemptRecord) TableName() string {
	return "request_attempts"
}

type requestTraceEventRecord struct {
	RequestID  string `gorm:"column:request_id;primaryKey;type:text;index:idx_request_trace_request"`
	Seq        int    `gorm:"column:seq;primaryKey;not null"`
	TS         string `gorm:"column:ts;type:text;not null;index:idx_request_trace_ts"`
	Event      string `gorm:"column:event;type:text;not null;index:idx_request_trace_event"`
	Message    string `gorm:"column:message;type:text;not null"`
	Provider   string `gorm:"column:provider;type:text;not null"`
	Model      string `gorm:"column:model;type:text;not null"`
	Dialect    string `gorm:"column:dialect;type:text;not null"`
	DurationMS int64  `gorm:"column:duration_ms;not null"`
	StatusCode int    `gorm:"column:status_code;not null"`
	ErrorClass string `gorm:"column:error_class;type:text;not null;index:idx_request_trace_error"`
	Retryable  bool   `gorm:"column:retryable;not null"`
	Attempt    int    `gorm:"column:attempt;not null"`
}

func (requestTraceEventRecord) TableName() string {
	return "request_trace_events"
}

type requestErrorRecord struct {
	RequestID    string `gorm:"column:request_id;primaryKey;type:text"`
	TS           string `gorm:"column:ts;type:text;not null;index:idx_request_error_ts"`
	Status       int    `gorm:"column:status;not null;index:idx_request_error_status"`
	ErrorType    string `gorm:"column:error_type;type:text;not null;index:idx_request_error_type"`
	ErrorClass   string `gorm:"column:error_class;type:text;not null;index:idx_request_error_class"`
	ErrorMessage string `gorm:"column:error_message;type:text;not null"`
	Retryable    bool   `gorm:"column:retryable;not null"`
	Attempts     int    `gorm:"column:attempts;not null"`
	Provider     string `gorm:"column:provider;type:text;not null"`
	Model        string `gorm:"column:model;type:text;not null"`
	Dialect      string `gorm:"column:dialect;type:text;not null"`
}

func (requestErrorRecord) TableName() string {
	return "request_errors"
}

type agg struct {
	Calls                    int64
	Errors                   int64
	Streams                  int64
	CacheHits                int64
	CacheMisses              int64
	CacheBypass              int64
	Fallbacks                int64
	Attempts                 int64
	InputTokens              int64
	OutputTokens             int64
	TotalTokens              int64
	InputCostUSD             float64
	OutputCostUSD            float64
	TotalCostUSD             float64
	LatencyMS                int64
	MaxLatencyMS             int64
	TTFBMS                   int64
	TTFBCount                int64
	MaxTTFBMS                int64
	UpstreamMS               int64
	UpstreamMSCount          int64
	DownstreamMS             int64
	DownstreamMSCount        int64
	UpstreamOutputTPS        float64
	UpstreamOutputTPSCount   int64
	UpstreamTotalTPS         float64
	UpstreamTotalTPSCount    int64
	DownstreamOutputTPS      float64
	DownstreamOutputTPSCount int64
	DownstreamTotalTPS       float64
	DownstreamTotalTPSCount  int64
	CacheItemsLatest         int64
	CacheBytesLatest         int64
	CacheMaxBytesLatest      int64
	CacheOccupancyLatest     float64
	CacheItemsMax            int64
	CacheBytesMax            int64
	CacheOccupancyMax        float64
	CacheBytesSum            int64
	CacheOccupancySum        float64
	CacheSnapshotCount       int64
}

func newUsageStore(cfg UsageDBConfig) (*usageStore, error) {
	if cfg.Enable != nil && !*cfg.Enable {
		return nil, nil
	}
	driver := strings.ToLower(defaultString(cfg.Driver, "sqlite"))
	if driver == "postgres" && cfg.DSN == "" {
		return nil, errors.New("usage_db.dsn is required for postgres")
	}
	if driver == "sqlite" && cfg.Path == "" {
		return nil, nil
	}
	store, err := OpenUsageStore(cfg)
	if err != nil {
		return nil, err
	}
	return store, nil
}

func OpenUsageStore(cfg UsageDBConfig) (*usageStore, error) {
	db, err := openUsageDB(cfg)
	if err != nil {
		return nil, err
	}
	store := &usageStore{db: db}
	if err := store.migrate(); err != nil {
		_ = store.Close()
		return nil, err
	}
	return store, nil
}

func OpenUsageStorePath(path string) (*usageStore, error) {
	return OpenUsageStore(UsageDBConfig{Driver: "sqlite", Path: path})
}

func openUsageDB(cfg UsageDBConfig) (*gorm.DB, error) {
	driver := strings.ToLower(defaultString(cfg.Driver, "sqlite"))
	switch driver {
	case "sqlite":
		if cfg.Path == "" {
			return nil, errors.New("usage db path is required")
		}
		if dir := filepath.Dir(cfg.Path); dir != "." {
			if err := os.MkdirAll(dir, 0700); err != nil {
				return nil, err
			}
		}
		return gorm.Open(sqlite.Open(cfg.Path), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	case "postgres", "postgresql":
		if cfg.DSN == "" {
			return nil, errors.New("usage db dsn is required")
		}
		return gorm.Open(postgres.Open(cfg.DSN), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	default:
		return nil, fmt.Errorf("unsupported usage db driver %q", cfg.Driver)
	}
}

func (s *usageStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	sqlDB, err := s.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

func (s *usageStore) migrate() error {
	if s.db.Dialector.Name() == "sqlite" {
		if err := s.db.Exec("PRAGMA journal_mode=WAL").Error; err != nil {
			return err
		}
	}
	if err := s.db.AutoMigrate(&usageRecord{}, &requestAttemptRecord{}, &requestTraceEventRecord{}, &requestErrorRecord{}); err != nil {
		return err
	}
	return ensureUsageRelationalSchema(s.db)
}

func ensureUsageRelationalSchema(db *gorm.DB) error {
	type columnInfo struct {
		Name string
		Type string
	}
	var columns []columnInfo
	switch db.Dialector.Name() {
	case "sqlite":
		for _, table := range []string{"request_usage", "request_attempts", "request_trace_events", "request_errors"} {
			var tableColumns []columnInfo
			if err := db.Raw(`SELECT name, type FROM pragma_table_info(?)`, table).Scan(&tableColumns).Error; err != nil {
				return err
			}
			for i := range tableColumns {
				tableColumns[i].Name = table + "." + tableColumns[i].Name
			}
			columns = append(columns, tableColumns...)
		}
	default:
		if err := db.Raw(`SELECT column_name AS name, data_type AS type
			FROM information_schema.columns
			WHERE table_name IN ('request_usage', 'request_attempts', 'request_trace_events', 'request_errors')`).Scan(&columns).Error; err != nil {
			return err
		}
	}
	for _, col := range columns {
		t := strings.ToLower(col.Type)
		if strings.Contains(t, "json") || strings.Contains(t, "array") || strings.HasSuffix(t, "[]") {
			return fmt.Errorf("request_usage.%s uses forbidden non-relational type %q", col.Name, col.Type)
		}
	}
	return nil
}

func (s *usageStore) Emit(rec logRecord) {
	if s == nil || s.db == nil || rec.RequestID == "" {
		return
	}
	row := rowFromRecord(rec)
	_ = s.db.Clauses(clause.OnConflict{DoNothing: true}).Create(recordFromRow(row)).Error
	for _, attempt := range rec.AttemptsDetail {
		_ = s.db.Clauses(clause.OnConflict{DoNothing: true}).Create(attemptRecordFromLog(rec.RequestID, attempt)).Error
	}
	for _, event := range rec.TraceEvents {
		_ = s.db.Clauses(clause.OnConflict{DoNothing: true}).Create(traceRecordFromLog(rec.RequestID, event)).Error
	}
	if rec.Error != nil {
		_ = s.db.Clauses(clause.OnConflict{DoNothing: true}).Create(errorRecordFromLog(rec)).Error
	}
}

func attemptRecordFromLog(requestID string, rec attemptLogRecord) *requestAttemptRecord {
	return &requestAttemptRecord{
		RequestID:        requestID,
		AttemptIndex:     rec.Index,
		TS:               rec.TS,
		Provider:         rec.Provider,
		Model:            rec.Model,
		Dialect:          rec.Dialect,
		EndpointHost:     rec.EndpointHost,
		DurationMS:       rec.DurationMS,
		StatusCode:       rec.StatusCode,
		ErrorClass:       rec.ErrorClass,
		ErrorMessage:     rec.ErrorMessage,
		Retryable:        rec.Retryable,
		TimedOut:         rec.TimedOut,
		ClientCanceled:   rec.ClientCanceled,
		Selected:         rec.Selected,
		FallbackReason:   rec.FallbackReason,
		RequestBytes:     rec.RequestBytes,
		ResponseBytes:    rec.ResponseBytes,
		AttemptTimeoutMS: rec.AttemptTimeoutMS,
	}
}

func traceRecordFromLog(requestID string, rec traceLogRecord) *requestTraceEventRecord {
	return &requestTraceEventRecord{
		RequestID:  requestID,
		Seq:        rec.Seq,
		TS:         rec.TS,
		Event:      rec.Event,
		Message:    rec.Message,
		Provider:   rec.Provider,
		Model:      rec.Model,
		Dialect:    rec.Dialect,
		DurationMS: rec.DurationMS,
		StatusCode: rec.StatusCode,
		ErrorClass: rec.ErrorClass,
		Retryable:  rec.Retryable,
		Attempt:    rec.Attempt,
	}
}

func errorRecordFromLog(rec logRecord) *requestErrorRecord {
	errType := ""
	if rec.Error != nil {
		errType = *rec.Error
	}
	return &requestErrorRecord{
		RequestID:    rec.RequestID,
		TS:           rec.TS,
		Status:       rec.Status,
		ErrorType:    errType,
		ErrorClass:   rec.ErrorClass,
		ErrorMessage: rec.ErrorMessage,
		Retryable:    errorTypeRetryable(errType),
		Attempts:     rec.Attempts,
		Provider:     rec.TargetProvider,
		Model:        rec.TargetModel,
		Dialect:      rec.TargetDialect,
	}
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
		TS:                                 ts,
		RequestID:                          rec.RequestID,
		CallerID:                           rec.CallerID,
		CallerUser:                         rec.CallerUser,
		CallerProject:                      rec.CallerProject,
		CallerEnvironment:                  rec.CallerEnvironment,
		CallerIP:                           rec.CallerIP,
		TokenID:                            tokenID,
		Client:                             rec.Client,
		InboundDialect:                     rec.InboundDialect,
		RequestedModel:                     rec.RequestedModel,
		ResolvedGroup:                      defaultString(rec.ResolvedGroup, rec.RequestedModel),
		Strategy:                           rec.Strategy,
		TargetProvider:                     rec.TargetProvider,
		TargetModel:                        rec.TargetModel,
		TargetDialect:                      rec.TargetDialect,
		Stream:                             rec.Stream,
		Cache:                              defaultString(rec.Cache, "bypass"),
		Status:                             rec.Status,
		Attempts:                           rec.Attempts,
		FallbackUsed:                       rec.FallbackUsed,
		LatencyMS:                          rec.LatencyMS,
		TTFBMS:                             rec.TTFBMS,
		UpstreamMS:                         rec.UpstreamMS,
		DownstreamMS:                       rec.DownstreamMS,
		UpstreamOutputTPS:                  rec.UpstreamOutputTPS,
		UpstreamTotalTPS:                   rec.UpstreamTotalTPS,
		DownstreamOutputTPS:                rec.DownstreamOutputTPS,
		DownstreamTotalTPS:                 rec.DownstreamTotalTPS,
		InputTokens:                        rec.Usage.InputTokens,
		OutputTokens:                       rec.Usage.OutputTokens,
		TotalTokens:                        rec.Usage.TotalTokens,
		InputHasImage:                      rec.InputHasImage,
		InputImageCount:                    rec.InputImageCount,
		InputImageTokens:                   rec.InputImageTokens,
		PIIFilterApplied:                   rec.PIIFilterApplied,
		PIIFilterMode:                      rec.PIIFilterMode,
		PIIFilterReplacements:              rec.PIIFilterReplacements,
		PIIFilterRuleCount:                 rec.PIIFilterRuleCount,
		InputPricePerMillionUSD:            rec.InputPricePerMillionUSD,
		OutputPricePerMillionUSD:           rec.OutputPricePerMillionUSD,
		ImageInputPricePerMillionTokensUSD: rec.ImageInputPricePerMillionTokensUSD,
		ImageInputPricePerImageUSD:         rec.ImageInputPricePerImageUSD,
		InputCostUSD:                       rec.InputCostUSD,
		ImageCostUSD:                       rec.ImageCostUSD,
		OutputCostUSD:                      rec.OutputCostUSD,
		TotalCostUSD:                       rec.TotalCostUSD,
		UpstreamReportedInputCostUSD:       rec.UpstreamReportedInputCostUSD,
		UpstreamReportedOutputCostUSD:      rec.UpstreamReportedOutputCostUSD,
		UpstreamReportedTotalCostUSD:       rec.UpstreamReportedTotalCostUSD,
		PricingSource:                      rec.PricingSource,
		PricingUpdatedAt:                   rec.PricingUpdatedAt,
		CacheEnabled:                       rec.CacheEnabled,
		CacheItems:                         rec.CacheItems,
		CacheBytes:                         rec.CacheBytes,
		CacheMaxBytes:                      rec.CacheMaxBytes,
		CacheOccupancyPct:                  rec.CacheOccupancyPct,
		QuotaState:                         rec.QuotaState,
		KeyState:                           rec.KeyState,
		Error:                              errText,
	}
}

func recordFromRow(row usageRow) *usageRecord {
	return &usageRecord{
		RequestID:                          row.RequestID,
		TS:                                 formatUsageTime(row.TS),
		CallerID:                           row.CallerID,
		CallerUser:                         row.CallerUser,
		CallerProject:                      row.CallerProject,
		CallerEnvironment:                  row.CallerEnvironment,
		CallerIP:                           row.CallerIP,
		TokenID:                            row.TokenID,
		Client:                             row.Client,
		InboundDialect:                     row.InboundDialect,
		RequestedModel:                     row.RequestedModel,
		ResolvedGroup:                      row.ResolvedGroup,
		Strategy:                           row.Strategy,
		TargetProvider:                     row.TargetProvider,
		TargetModel:                        row.TargetModel,
		TargetDialect:                      row.TargetDialect,
		Stream:                             row.Stream,
		Cache:                              row.Cache,
		Status:                             row.Status,
		Attempts:                           row.Attempts,
		FallbackUsed:                       row.FallbackUsed,
		LatencyMS:                          row.LatencyMS,
		TTFBMS:                             row.TTFBMS,
		UpstreamMS:                         row.UpstreamMS,
		DownstreamMS:                       row.DownstreamMS,
		UpstreamOutputTPS:                  row.UpstreamOutputTPS,
		UpstreamTotalTPS:                   row.UpstreamTotalTPS,
		DownstreamOutputTPS:                row.DownstreamOutputTPS,
		DownstreamTotalTPS:                 row.DownstreamTotalTPS,
		InputTokens:                        row.InputTokens,
		OutputTokens:                       row.OutputTokens,
		TotalTokens:                        row.TotalTokens,
		InputHasImage:                      row.InputHasImage,
		InputImageCount:                    row.InputImageCount,
		InputImageTokens:                   row.InputImageTokens,
		PIIFilterApplied:                   row.PIIFilterApplied,
		PIIFilterMode:                      row.PIIFilterMode,
		PIIFilterReplacements:              row.PIIFilterReplacements,
		PIIFilterRuleCount:                 row.PIIFilterRuleCount,
		InputPricePerMillionUSD:            row.InputPricePerMillionUSD,
		OutputPricePerMillionUSD:           row.OutputPricePerMillionUSD,
		ImageInputPricePerMillionTokensUSD: row.ImageInputPricePerMillionTokensUSD,
		ImageInputPricePerImageUSD:         row.ImageInputPricePerImageUSD,
		InputCostUSD:                       row.InputCostUSD,
		ImageCostUSD:                       row.ImageCostUSD,
		OutputCostUSD:                      row.OutputCostUSD,
		TotalCostUSD:                       row.TotalCostUSD,
		UpstreamReportedInputCostUSD:       row.UpstreamReportedInputCostUSD,
		UpstreamReportedOutputCostUSD:      row.UpstreamReportedOutputCostUSD,
		UpstreamReportedTotalCostUSD:       row.UpstreamReportedTotalCostUSD,
		PricingSource:                      row.PricingSource,
		PricingUpdatedAt:                   row.PricingUpdatedAt,
		CacheEnabled:                       row.CacheEnabled,
		CacheItems:                         row.CacheItems,
		CacheBytes:                         row.CacheBytes,
		CacheMaxBytes:                      row.CacheMaxBytes,
		CacheOccupancyPct:                  row.CacheOccupancyPct,
		QuotaState:                         row.QuotaState,
		KeyState:                           row.KeyState,
		Error:                              row.Error,
	}
}

func rowFromUsageRecord(record usageRecord) (usageRow, error) {
	ts, err := parseUsageTime(record.TS)
	if err != nil {
		return usageRow{}, err
	}
	return usageRow{
		TS:                                 ts,
		RequestID:                          record.RequestID,
		CallerID:                           record.CallerID,
		CallerUser:                         record.CallerUser,
		CallerProject:                      record.CallerProject,
		CallerEnvironment:                  record.CallerEnvironment,
		CallerIP:                           record.CallerIP,
		TokenID:                            record.TokenID,
		Client:                             record.Client,
		InboundDialect:                     record.InboundDialect,
		RequestedModel:                     record.RequestedModel,
		ResolvedGroup:                      record.ResolvedGroup,
		Strategy:                           record.Strategy,
		TargetProvider:                     record.TargetProvider,
		TargetModel:                        record.TargetModel,
		TargetDialect:                      record.TargetDialect,
		Stream:                             record.Stream,
		Cache:                              record.Cache,
		Status:                             record.Status,
		Attempts:                           record.Attempts,
		FallbackUsed:                       record.FallbackUsed,
		LatencyMS:                          record.LatencyMS,
		TTFBMS:                             record.TTFBMS,
		UpstreamMS:                         record.UpstreamMS,
		DownstreamMS:                       record.DownstreamMS,
		UpstreamOutputTPS:                  record.UpstreamOutputTPS,
		UpstreamTotalTPS:                   record.UpstreamTotalTPS,
		DownstreamOutputTPS:                record.DownstreamOutputTPS,
		DownstreamTotalTPS:                 record.DownstreamTotalTPS,
		InputTokens:                        record.InputTokens,
		OutputTokens:                       record.OutputTokens,
		TotalTokens:                        record.TotalTokens,
		InputHasImage:                      record.InputHasImage,
		InputImageCount:                    record.InputImageCount,
		InputImageTokens:                   record.InputImageTokens,
		PIIFilterApplied:                   record.PIIFilterApplied,
		PIIFilterMode:                      record.PIIFilterMode,
		PIIFilterReplacements:              record.PIIFilterReplacements,
		PIIFilterRuleCount:                 record.PIIFilterRuleCount,
		InputPricePerMillionUSD:            record.InputPricePerMillionUSD,
		OutputPricePerMillionUSD:           record.OutputPricePerMillionUSD,
		ImageInputPricePerMillionTokensUSD: record.ImageInputPricePerMillionTokensUSD,
		ImageInputPricePerImageUSD:         record.ImageInputPricePerImageUSD,
		InputCostUSD:                       record.InputCostUSD,
		ImageCostUSD:                       record.ImageCostUSD,
		OutputCostUSD:                      record.OutputCostUSD,
		TotalCostUSD:                       record.TotalCostUSD,
		UpstreamReportedInputCostUSD:       record.UpstreamReportedInputCostUSD,
		UpstreamReportedOutputCostUSD:      record.UpstreamReportedOutputCostUSD,
		UpstreamReportedTotalCostUSD:       record.UpstreamReportedTotalCostUSD,
		PricingSource:                      record.PricingSource,
		PricingUpdatedAt:                   record.PricingUpdatedAt,
		CacheEnabled:                       record.CacheEnabled,
		CacheItems:                         record.CacheItems,
		CacheBytes:                         record.CacheBytes,
		CacheMaxBytes:                      record.CacheMaxBytes,
		CacheOccupancyPct:                  record.CacheOccupancyPct,
		QuotaState:                         record.QuotaState,
		KeyState:                           record.KeyState,
		Error:                              record.Error,
	}, nil
}

func ImportUsageJSONL(dbPath, logPath string) (int, error) {
	return ImportUsageJSONLTo(UsageDBConfig{Driver: "sqlite", Path: dbPath}, logPath)
}

func ImportUsageJSONLTo(cfg UsageDBConfig, logPath string) (int, error) {
	store, err := OpenUsageStore(cfg)
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
	driver := strings.ToLower(defaultString(opts.Driver, "sqlite"))
	if driver == "sqlite" && opts.DBPath == "" {
		return "", errors.New("usage db path is required")
	}
	if (driver == "postgres" || driver == "postgresql") && opts.DSN == "" {
		return "", errors.New("usage db dsn is required")
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
	cfg := UsageDBConfig{Driver: driver, Path: opts.DBPath, DSN: opts.DSN}
	if opts.LogPath != "" {
		if _, err := ImportUsageJSONLTo(cfg, opts.LogPath); err != nil {
			return "", err
		}
	}
	store, err := OpenUsageStore(cfg)
	if err != nil {
		return "", err
	}
	defer store.Close()

	rows, err := store.rows(opts)
	if err != nil {
		return "", err
	}
	return renderUsageMarkdown(opts.From, opts.To, rows), nil
}

func (s *usageStore) rows(opts UsageReportOptions) ([]usageRow, error) {
	var records []usageRecord
	q := s.db.Where("ts >= ? AND ts < ?", formatUsageTime(opts.From), formatUsageTime(opts.To))
	if opts.TokenID != "" {
		q = q.Where("token_id = ?", opts.TokenID)
	}
	if opts.TokenIDPrefix != "" {
		q = q.Where("token_id LIKE ?", opts.TokenIDPrefix+"%")
	}
	if opts.CallerProject != "" {
		q = q.Where("caller_project = ?", opts.CallerProject)
	}
	if opts.CallerEnvironment != "" {
		q = q.Where("caller_environment = ?", opts.CallerEnvironment)
	}
	if opts.ResolvedGroup != "" {
		q = q.Where("resolved_group = ?", opts.ResolvedGroup)
	}
	if opts.Client != "" {
		q = q.Where("client = ?", opts.Client)
	}
	if err := q.Order("ts ASC").Find(&records).Error; err != nil {
		return nil, err
	}
	out := make([]usageRow, 0, len(records))
	for _, record := range records {
		row, err := rowFromUsageRecord(record)
		if err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, nil
}

func renderUsageMarkdown(from, to time.Time, rows []usageRow) string {
	total := &agg{}
	byToken := map[string]*agg{}
	byTokenMeta := map[string]usageRow{}
	byModel := map[string]*agg{}
	byGroup := map[string]*agg{}
	byClient := map[string]*agg{}
	byCallerIP := map[string]*agg{}
	byStatus := map[string]*agg{}
	byHour := map[string]*agg{}
	byHourIP := map[string]*agg{}
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
		ipKey := defaultString(row.CallerIP, "unknown")
		byCallerIP[ipKey] = getAgg(byCallerIP, ipKey)
		byCallerIP[ipKey].add(row)
		statusKey := fmt.Sprint(row.Status)
		byStatus[statusKey] = getAgg(byStatus, statusKey)
		byStatus[statusKey].add(row)
		hourKey := row.TS.UTC().Truncate(time.Hour).Format("2006-01-02 15:00")
		byHour[hourKey] = getAgg(byHour, hourKey)
		byHour[hourKey].add(row)
		hourIPKey := joinKey(hourKey, ipKey)
		byHourIP[hourIPKey] = getAgg(byHourIP, hourIPKey)
		byHourIP[hourIPKey].add(row)
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
	fmt.Fprintf(&b, "- Cost: `$%s` total, `$%s` input, `$%s` output\n", fmtUSD(total.TotalCostUSD), fmtUSD(total.InputCostUSD), fmtUSD(total.OutputCostUSD))
	fmt.Fprintf(&b, "- Cache: `%d` hits, `%d` misses, `%d` bypass\n", total.CacheHits, total.CacheMisses, total.CacheBypass)
	fmt.Fprintf(&b, "- Upstream attempts: `%d`; fallbacks: `%d`; streaming requests: `%d`\n", total.Attempts, total.Fallbacks, total.Streams)
	fmt.Fprintf(&b, "- Latency: `%d ms` avg, `%d ms` max\n", avg(total.LatencyMS, total.Calls), total.MaxLatencyMS)
	fmt.Fprintf(&b, "- Throughput: upstream `%s` output tok/s / `%s` total tok/s; downstream `%s` output tok/s / `%s` total tok/s\n",
		fmtFloat(avgFloat(total.UpstreamOutputTPS, total.UpstreamOutputTPSCount)),
		fmtFloat(avgFloat(total.UpstreamTotalTPS, total.UpstreamTotalTPSCount)),
		fmtFloat(avgFloat(total.DownstreamOutputTPS, total.DownstreamOutputTPSCount)),
		fmtFloat(avgFloat(total.DownstreamTotalTPS, total.DownstreamTotalTPSCount)))
	fmt.Fprintf(&b, "- Cache occupancy: latest `%s`, avg `%s`, max `%s`\n\n",
		fmtPct(total.CacheOccupancyLatest), fmtPct(avgFloat(total.CacheOccupancySum, total.CacheSnapshotCount)), fmtPct(total.CacheOccupancyMax))

	writeCacheSummary(&b, total)
	writeRequestThroughputTable(&b, rows)
	writeTokenTable(&b, "Usage By Internal API Key", byToken, byTokenMeta)
	writeAggTable(&b, "Usage By External Model", []string{"Provider", "Model"}, byModel, splitKey2)
	writeAggTable(&b, "Usage By Router Model Group", []string{"Model Group"}, byGroup, splitKey1)
	writeAggTable(&b, "Usage By Client", []string{"Client"}, byClient, splitKey1)
	writeAggTable(&b, "Usage By Caller IP", []string{"Caller IP"}, byCallerIP, splitKey1)
	writeAggTable(&b, "Usage By Status", []string{"Status"}, byStatus, splitKey1)
	writeAggTable(&b, "Hourly Usage", []string{"Hour UTC"}, byHour, splitKey1)
	writeAggTable(&b, "Hourly Usage By Caller IP", []string{"Hour UTC", "Caller IP"}, byHourIP, splitKey2)
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
	a.InputCostUSD += row.InputCostUSD
	a.OutputCostUSD += row.OutputCostUSD
	a.TotalCostUSD += row.TotalCostUSD
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
	if row.UpstreamMS != nil {
		a.UpstreamMS += *row.UpstreamMS
		a.UpstreamMSCount++
	}
	if row.DownstreamMS != nil {
		a.DownstreamMS += *row.DownstreamMS
		a.DownstreamMSCount++
	}
	addFloat(row.UpstreamOutputTPS, &a.UpstreamOutputTPS, &a.UpstreamOutputTPSCount)
	addFloat(row.UpstreamTotalTPS, &a.UpstreamTotalTPS, &a.UpstreamTotalTPSCount)
	addFloat(row.DownstreamOutputTPS, &a.DownstreamOutputTPS, &a.DownstreamOutputTPSCount)
	addFloat(row.DownstreamTotalTPS, &a.DownstreamTotalTPS, &a.DownstreamTotalTPSCount)
	if row.CacheEnabled || row.CacheMaxBytes > 0 {
		a.CacheSnapshotCount++
		a.CacheItemsLatest = row.CacheItems
		a.CacheBytesLatest = row.CacheBytes
		a.CacheMaxBytesLatest = row.CacheMaxBytes
		a.CacheOccupancyLatest = row.CacheOccupancyPct
		a.CacheBytesSum += row.CacheBytes
		a.CacheOccupancySum += row.CacheOccupancyPct
		if row.CacheItems > a.CacheItemsMax {
			a.CacheItemsMax = row.CacheItems
		}
		if row.CacheBytes > a.CacheBytesMax {
			a.CacheBytesMax = row.CacheBytes
		}
		if row.CacheOccupancyPct > a.CacheOccupancyMax {
			a.CacheOccupancyMax = row.CacheOccupancyPct
		}
	}
}

func writeTokenTable(b *strings.Builder, title string, data map[string]*agg, meta map[string]usageRow) {
	fmt.Fprintf(b, "## %s\n\n", title)
	fmt.Fprintln(b, "| Token ID | User | Project | Env | Caller ID | Calls | Errors | Tokens | Input | Output | Cost USD | Cache Hit | Cache Miss | Attempts | Fallbacks | Avg Upstream Output tok/s | Avg Downstream Output tok/s | Avg Latency ms | Max Latency ms |")
	fmt.Fprintln(b, "|---|---|---|---|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|")
	for _, key := range sortedAggKeys(data) {
		row := meta[key]
		a := data[key]
		fmt.Fprintf(b, "| `%s` | %s | %s | %s | `%s` | %d | %d | %d | %d | %d | $%s | %d | %d | %d | %d | %s | %s | %d | %d |\n",
			esc(row.TokenID), esc(row.CallerUser), esc(row.CallerProject), esc(row.CallerEnvironment), esc(row.CallerID),
			a.Calls, a.Errors, a.TotalTokens, a.InputTokens, a.OutputTokens, fmtUSD(a.TotalCostUSD), a.CacheHits, a.CacheMisses, a.Attempts,
			a.Fallbacks, fmtFloat(avgFloat(a.UpstreamOutputTPS, a.UpstreamOutputTPSCount)),
			fmtFloat(avgFloat(a.DownstreamOutputTPS, a.DownstreamOutputTPSCount)), avg(a.LatencyMS, a.Calls), a.MaxLatencyMS)
	}
	if len(data) == 0 {
		fmt.Fprintln(b, "| _none_ |  |  |  |  | 0 | 0 | 0 | 0 | 0 | $0.000000 | 0 | 0 | 0 | 0 | n/a | n/a | 0 | 0 |")
	}
	fmt.Fprintln(b)
}

func writeAggTable(b *strings.Builder, title string, keyHeaders []string, data map[string]*agg, split func(string) []string) {
	fmt.Fprintf(b, "## %s\n\n", title)
	for _, h := range keyHeaders {
		fmt.Fprintf(b, "| %s ", h)
	}
	fmt.Fprintln(b, "| Calls | Errors | Tokens | Input | Output | Cost USD | Cache Hit | Cache Miss | Cache Bypass | Attempts | Fallbacks | Streams | Avg Upstream Output tok/s | Avg Upstream Total tok/s | Avg Downstream Output tok/s | Avg Downstream Total tok/s | Avg Latency ms | Max Latency ms | Avg TTFB ms | Max TTFB ms |")
	for range keyHeaders {
		fmt.Fprint(b, "|---")
	}
	fmt.Fprintln(b, "|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|")
	for _, key := range sortedAggKeys(data) {
		parts := split(key)
		for _, part := range parts {
			fmt.Fprintf(b, "| %s ", esc(part))
		}
		a := data[key]
		fmt.Fprintf(b, "| %d | %d | %d | %d | %d | $%s | %d | %d | %d | %d | %d | %d | %s | %s | %s | %s | %d | %d | %d | %d |\n",
			a.Calls, a.Errors, a.TotalTokens, a.InputTokens, a.OutputTokens, fmtUSD(a.TotalCostUSD), a.CacheHits, a.CacheMisses, a.CacheBypass,
			a.Attempts, a.Fallbacks, a.Streams,
			fmtFloat(avgFloat(a.UpstreamOutputTPS, a.UpstreamOutputTPSCount)),
			fmtFloat(avgFloat(a.UpstreamTotalTPS, a.UpstreamTotalTPSCount)),
			fmtFloat(avgFloat(a.DownstreamOutputTPS, a.DownstreamOutputTPSCount)),
			fmtFloat(avgFloat(a.DownstreamTotalTPS, a.DownstreamTotalTPSCount)),
			avg(a.LatencyMS, a.Calls), a.MaxLatencyMS, avg(a.TTFBMS, a.TTFBCount), a.MaxTTFBMS)
	}
	if len(data) == 0 {
		for range keyHeaders {
			fmt.Fprint(b, "| _none_ ")
		}
		fmt.Fprintln(b, "| 0 | 0 | 0 | 0 | 0 | $0.000000 | 0 | 0 | 0 | 0 | 0 | 0 | n/a | n/a | n/a | n/a | 0 | 0 | 0 | 0 |")
	}
	fmt.Fprintln(b)
}

func writeCacheSummary(b *strings.Builder, total *agg) {
	cacheable := total.CacheHits + total.CacheMisses
	fmt.Fprintln(b, "## Cache Summary")
	fmt.Fprintln(b)
	fmt.Fprintln(b, "| Requests | Cacheable | Hits | Misses | Bypass | Hit Rate | Bypass Rate | Latest Items | Latest Bytes | Max Bytes | Latest Occupancy | Avg Occupancy | Max Occupancy |")
	fmt.Fprintln(b, "|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|")
	fmt.Fprintf(b, "| %d | %d | %d | %d | %d | %s | %s | %d | %d | %d | %s | %s | %s |\n\n",
		total.Calls, cacheable, total.CacheHits, total.CacheMisses, total.CacheBypass,
		fmtPct(ratioPct(total.CacheHits, cacheable)), fmtPct(ratioPct(total.CacheBypass, total.Calls)),
		total.CacheItemsLatest, total.CacheBytesLatest, total.CacheMaxBytesLatest,
		fmtPct(total.CacheOccupancyLatest), fmtPct(avgFloat(total.CacheOccupancySum, total.CacheSnapshotCount)), fmtPct(total.CacheOccupancyMax))
}

func writeRequestThroughputTable(b *strings.Builder, rows []usageRow) {
	fmt.Fprintln(b, "## Per-Request Throughput")
	fmt.Fprintln(b)
	fmt.Fprintln(b, "| Time UTC | Caller IP | Request ID | Token ID | Model Group | Provider | Model | Status | Cache | Output | Total | Cost USD | Upstream ms | Downstream ms | Upstream Output tok/s | Upstream Total tok/s | Downstream Output tok/s | Downstream Total tok/s |")
	fmt.Fprintln(b, "|---|---|---|---|---|---|---|---:|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|")
	for _, row := range rows {
		fmt.Fprintf(b, "| %s | `%s` | `%s` | `%s` | %s | %s | %s | %d | %s | %d | %d | $%s | %s | %s | %s | %s | %s | %s |\n",
			formatUsageTime(row.TS), esc(defaultString(row.CallerIP, "unknown")), esc(row.RequestID), esc(row.TokenID), esc(defaultString(row.ResolvedGroup, row.RequestedModel)),
			esc(row.TargetProvider), esc(row.TargetModel), row.Status, esc(row.Cache), row.OutputTokens, totalTokens(Usage{InputTokens: row.InputTokens, OutputTokens: row.OutputTokens, TotalTokens: row.TotalTokens}),
			fmtUSD(row.TotalCostUSD), fmtIntPtr(row.UpstreamMS), fmtIntPtr(row.DownstreamMS), fmtFloatPtr(row.UpstreamOutputTPS), fmtFloatPtr(row.UpstreamTotalTPS),
			fmtFloatPtr(row.DownstreamOutputTPS), fmtFloatPtr(row.DownstreamTotalTPS))
	}
	if len(rows) == 0 {
		fmt.Fprintln(b, "| _none_ |  |  |  |  |  |  | 0 |  | 0 | 0 | $0.000000 | n/a | n/a | n/a | n/a | n/a | n/a |")
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

func addFloat(v *float64, sum *float64, count *int64) {
	if v == nil {
		return
	}
	*sum += *v
	*count++
}

func avgFloat(sum float64, n int64) float64 {
	if n <= 0 {
		return -1
	}
	return sum / float64(n)
}

func ratioPct(part, total int64) float64 {
	if total <= 0 {
		return -1
	}
	return float64(part) * 100 / float64(total)
}

func fmtIntPtr(v *int64) string {
	if v == nil {
		return "n/a"
	}
	return fmt.Sprint(*v)
}

func fmtFloatPtr(v *float64) string {
	if v == nil {
		return "n/a"
	}
	return fmtFloat(*v)
}

func fmtFloat(v float64) string {
	if v < 0 {
		return "n/a"
	}
	return fmt.Sprintf("%.2f", v)
}

func fmtUSD(v float64) string {
	if v < 0 {
		return "n/a"
	}
	return fmt.Sprintf("%.6f", v)
}

func fmtPct(v float64) string {
	if v < 0 {
		return "n/a"
	}
	return fmt.Sprintf("%.2f%%", v)
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
