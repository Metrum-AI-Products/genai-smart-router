package router

// Phase one of the configuration control plane deliberately provides a small,
// read-only relational projection. YAML remains the serving default. The
// projection is useful for validating the migration contract and loading a
// simple active router configuration without introducing a write API, secret
// storage, PostgREST, or runtime hot reload.

import (
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"

	"gorm.io/gorm"
)

const configControlPlaneScope = "router_config"

const configControlPlanePhase1MigrationID = 2026072001

const configControlPlanePhase2MigrationID = 2026072002

const configControlPlanePhase3MigrationID = 2026072003

const configControlPlaneProviderHeaderNameConstraint = "router_config_provider_headers_non_secret_name_ck"

const configControlPlaneProviderHeaderNameCheck = "header_name = TRIM(header_name) AND LOWER(header_name) IN ('http-referer', 'user-agent', 'x-title')"

const configControlPlaneProviderHeaderNameCIIndex = "router_config_provider_headers_name_ci"

var configControlPlaneCompatibility = MigrationCompatibility{MinSchema: 0, MaxSchema: 3, MinData: 0, MaxData: 0}

var configControlPlaneMigrationDefinitions = []MigrationDefinition{
	{
		ID:              configControlPlanePhase1MigrationID,
		Scope:           configControlPlaneScope,
		Name:            "create relational read-only configuration projection",
		Release:         "2026.7",
		Checksum:        "a2c3e1ea4db3422e260a17b0b03ae67a8f7f07e0282ec2f39d0879b64f9d0f9a",
		SchemaVersion:   1,
		Transactional:   true,
		MaintenanceMode: "online",
		RollbackClass:   "restore-required",
		Apply:           applyConfigControlPlanePhase1,
		Verify:          verifyConfigControlPlanePhase1,
	},
	{
		ID:              configControlPlanePhase2MigrationID,
		Scope:           configControlPlaneScope,
		Name:            "enforce non-secret provider header names at database boundary",
		Release:         "2026.7",
		Checksum:        "714e7b191083cbb9ca0490990e394dfe54a0acc4a9da54760e4f85e0ba5f9baf",
		SchemaVersion:   2,
		Transactional:   true,
		MaintenanceMode: "maintenance",
		RollbackClass:   "restore-required",
		Apply:           applyConfigControlPlanePhase2,
		Verify:          verifyConfigControlPlanePhase2,
	},
	{
		ID:              configControlPlanePhase3MigrationID,
		Scope:           configControlPlaneScope,
		Name:            "enforce case-insensitive provider header uniqueness",
		Release:         "2026.7",
		Checksum:        "c2b317747db0dd6b43fee33e113ef4574d5304ffc24085b37c3ac7cf95f86959",
		SchemaVersion:   3,
		Transactional:   true,
		MaintenanceMode: "maintenance",
		RollbackClass:   "restore-required",
		Apply:           applyConfigControlPlanePhase3,
		Verify:          verifyConfigControlPlanePhase3,
	},
}

// ConfigControlPlaneMigrationRunner opens a dedicated config-control-plane
// database scope. It never initializes or changes the usage schema.
func ConfigControlPlaneMigrationRunner(cfg UsageDBConfig) (*migrationRunner, func() error, error) {
	db, err := openUsageDB(cfg)
	if err != nil {
		return nil, nil, err
	}
	r, err := NewMigrationRunner(db, configControlPlaneScope, configControlPlaneCompatibility, configControlPlaneMigrationDefinitions)
	if err != nil {
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
		return nil, nil, err
	}
	return r, func() error {
		sqlDB, err := db.DB()
		if err != nil {
			return err
		}
		return sqlDB.Close()
	}, nil
}

// LoadActiveConfigFromDB loads the one validated active set for runtimeScope.
// It is intentionally read-only and fails closed for unsupported relational
// fields rather than silently dropping configuration. Caller token hashes are
// retained for verification; raw caller tokens are never represented here.
func LoadActiveConfigFromDB(db *gorm.DB, runtimeScope string) (*Config, error) {
	if db == nil {
		return nil, errors.New("config control-plane database is required")
	}
	runtimeScope = strings.TrimSpace(runtimeScope)
	if runtimeScope == "" {
		return nil, errors.New("config runtime scope is required")
	}
	var set configSetRow
	if err := db.Where("runtime_scope = ? AND status = ? AND validation_status = ?", runtimeScope, "active", "valid").First(&set).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("no validated active config set for runtime scope %q", runtimeScope)
		}
		return nil, fmt.Errorf("load active config set: %w", err)
	}
	var server serverConfigRow
	if err := db.Where("config_set_id = ?", set.ID).First(&server).Error; err != nil {
		return nil, fmt.Errorf("load server config: %w", err)
	}
	cfg := &Config{Server: ServerConfig{
		Listen:            server.Listen,
		DefaultModelGroup: server.DefaultModelGroup,
		License: LicenseConfig{
			Enabled:    server.LicenseEnabled,
			Path:       server.LicensePath,
			StatePath:  server.LicenseStatePath,
			enabledSet: true,
		},
	}, StatePath: server.StatePath, Provider: map[string]ProviderConfig{}, Models: map[string]ModelGroup{}}
	var providers []providerRow
	if err := db.Where("config_set_id = ?", set.ID).Order("provider_name ASC").Find(&providers).Error; err != nil {
		return nil, err
	}
	for _, row := range providers {
		p := ProviderConfig{BaseURL: row.BaseURL, Dialect: row.Dialect, APIKeyEnv: row.APIKeyEnv, KeyID: row.KeyID, AuthScheme: row.AuthScheme, Headers: map[string]string{}, Models: map[string]ProviderModel{}}
		var headers []providerHeaderRow
		if err := db.Where("config_set_id = ? AND provider_name = ?", set.ID, row.ProviderName).Order("header_name ASC").Find(&headers).Error; err != nil {
			return nil, err
		}
		for _, h := range headers {
			if !controlPlaneAllowedProviderHeader(h.HeaderName) {
				return nil, fmt.Errorf("provider %q uses unsupported header %q; provider headers are limited to approved non-secret metadata headers and credentials must use api_key_env or a deployment secret reference", row.ProviderName, h.HeaderName)
			}
			p.Headers[h.HeaderName] = h.HeaderValue
		}
		var models []providerModelRow
		if err := db.Where("config_set_id = ? AND provider_name = ?", set.ID, row.ProviderName).Order("model_ref ASC").Find(&models).Error; err != nil {
			return nil, err
		}
		for _, model := range models {
			p.Models[model.ModelRef] = ProviderModel{Model: model.Model, Dialect: model.Dialect, DisplayName: model.DisplayName, ContextTokens: model.ContextTokens, InputPricePerMillionUSD: model.InputPricePerMillionUSD, OutputPricePerMillionUSD: model.OutputPricePerMillionUSD, PricingSource: model.PricingSource, PricingUpdatedAt: model.PricingUpdatedAt, PricingNotes: model.PricingNotes}
		}
		cfg.Provider[row.ProviderName] = p
	}
	var groups []modelGroupRow
	if err := db.Where("config_set_id = ?", set.ID).Order("group_name ASC").Find(&groups).Error; err != nil {
		return nil, err
	}
	for _, row := range groups {
		group := ModelGroup{Strategy: row.Strategy, AttemptTimeoutMS: row.AttemptTimeoutMS}
		var targets []modelGroupTargetRow
		if err := db.Where("config_set_id = ? AND group_name = ?", set.ID, row.GroupName).Order("sequence ASC").Find(&targets).Error; err != nil {
			return nil, err
		}
		for _, target := range targets {
			modelRef := ""
			if target.ModelRef.Valid {
				modelRef = target.ModelRef.String
			}
			group.Targets = append(group.Targets, Target{Provider: target.ProviderName, ModelRef: modelRef, Model: target.Model, Dialect: target.Dialect, Weight: target.Weight, RPM: target.RPM, Tier: target.Tier, Cost: target.Cost})
		}
		cfg.Models[row.GroupName] = group
	}
	var callers []callerRow
	if err := db.Where("config_set_id = ?", set.ID).Order("caller_id ASC").Find(&callers).Error; err != nil {
		return nil, err
	}
	for _, row := range callers {
		caller := CallerConfig{ID: row.CallerID, OwnerUser: row.OwnerUser, Project: row.Project, Environment: row.Environment, Status: row.Status, TokenSHA256: row.TokenSHA256, TokenID: row.TokenID, MetricsAdmin: row.MetricsAdmin, ContentAdmin: row.ContentAdmin, Rate: RateConfig{RPM: row.RPM, TPM: row.TPM, Concurrent: row.Concurrent}}
		var allowed []callerAllowedGroupRow
		if err := db.Where("config_set_id = ? AND caller_id = ?", set.ID, row.CallerID).Order("group_name ASC").Find(&allowed).Error; err != nil {
			return nil, err
		}
		for _, allow := range allowed {
			caller.Allow = append(caller.Allow, allow.GroupName)
		}
		cfg.Callers = append(cfg.Callers, caller)
	}
	cfg.setDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validate active config set %q: %w", set.ID, err)
	}
	return cfg, nil
}

func controlPlaneAllowedProviderHeader(name string) bool {
	if name != strings.TrimSpace(name) {
		return false
	}
	switch strings.ToLower(name) {
	case "http-referer", "user-agent", "x-title":
		return true
	default:
		return false
	}
}

func applyConfigControlPlanePhase1(tx *gorm.DB) error {
	for _, stmt := range configControlPlaneDDL {
		if err := tx.Exec(stmt).Error; err != nil {
			return fmt.Errorf("config control-plane bootstrap: %w", err)
		}
	}
	if tx.Dialector.Name() == "postgres" {
		for _, stmt := range configControlPlanePostgresComments {
			if err := tx.Exec(stmt).Error; err != nil {
				return fmt.Errorf("config control-plane comments: %w", err)
			}
		}
	}
	return nil
}

func verifyConfigControlPlanePhase1(tx *gorm.DB) error {
	for _, table := range configControlPlaneTables {
		if !tx.Migrator().HasTable(table) {
			return fmt.Errorf("required control-plane table %s is missing", table)
		}
	}
	for table, columns := range configControlPlaneRequiredColumns {
		for _, column := range columns {
			if !tx.Migrator().HasColumn(table, column) {
				return fmt.Errorf("required control-plane column %s.%s is missing", table, column)
			}
		}
	}
	for table, indexes := range configControlPlaneRequiredIndexes {
		for _, index := range indexes {
			if !tx.Migrator().HasIndex(table, index) {
				return fmt.Errorf("required control-plane index %s.%s is missing", table, index)
			}
		}
	}
	for table, constraints := range configControlPlaneRequiredConstraints {
		for _, constraint := range constraints {
			if !tx.Migrator().HasConstraint(table, constraint) {
				return fmt.Errorf("required control-plane foreign key %s.%s is missing", table, constraint)
			}
		}
	}
	for _, foreignKey := range configControlPlaneRequiredForeignKeys {
		if err := verifyConfigControlPlaneForeignKey(tx, foreignKey); err != nil {
			return err
		}
	}
	return nil
}

// applyConfigControlPlanePhase2 puts the provider-header allowlist in the
// database instead of relying solely on a read-path validation. That means a
// direct SQL import or any future write API cannot durably store a credential
// header that the router might later forward upstream.
func applyConfigControlPlanePhase2(tx *gorm.DB) error {
	switch tx.Dialector.Name() {
	case "sqlite":
		return applyConfigControlPlaneProviderHeaderConstraintSQLite(tx)
	case "postgres":
		stmt := fmt.Sprintf(`ALTER TABLE router_config_provider_headers ADD CONSTRAINT %s CHECK (%s)`, configControlPlaneProviderHeaderNameConstraint, configControlPlaneProviderHeaderNameCheck)
		if err := tx.Exec(stmt).Error; err != nil {
			return fmt.Errorf("add provider-header allowlist constraint: %w", err)
		}
		return nil
	default:
		return fmt.Errorf("unsupported control-plane database driver %q", tx.Dialector.Name())
	}
}

func applyConfigControlPlaneProviderHeaderConstraintSQLite(tx *gorm.DB) error {
	constraint := fmt.Sprintf("CONSTRAINT %s CHECK (%s)", configControlPlaneProviderHeaderNameConstraint, configControlPlaneProviderHeaderNameCheck)
	for _, stmt := range []string{
		`CREATE TABLE router_config_provider_headers_rebuild (config_set_id TEXT NOT NULL, provider_name TEXT NOT NULL, header_name TEXT NOT NULL, header_value TEXT NOT NULL, ` + constraint + `, PRIMARY KEY (config_set_id, provider_name, header_name), FOREIGN KEY (config_set_id, provider_name) REFERENCES router_config_providers(config_set_id, provider_name))`,
		`INSERT INTO router_config_provider_headers_rebuild (config_set_id, provider_name, header_name, header_value) SELECT config_set_id, provider_name, header_name, header_value FROM router_config_provider_headers`,
		`DROP TABLE router_config_provider_headers`,
		`ALTER TABLE router_config_provider_headers_rebuild RENAME TO router_config_provider_headers`,
	} {
		if err := tx.Exec(stmt).Error; err != nil {
			return fmt.Errorf("rebuild provider-header table with non-secret allowlist: %w", err)
		}
	}
	return nil
}

func verifyConfigControlPlanePhase2(tx *gorm.DB) error {
	if err := verifyConfigControlPlanePhase1(tx); err != nil {
		return err
	}
	if !tx.Migrator().HasConstraint("router_config_provider_headers", configControlPlaneProviderHeaderNameConstraint) {
		return fmt.Errorf("required control-plane constraint router_config_provider_headers.%s is missing", configControlPlaneProviderHeaderNameConstraint)
	}
	return nil
}

// applyConfigControlPlanePhase3 prevents case variants such as X-Title and
// x-title from becoming two map keys that later collapse to one HTTP header.
// The expression is supported by both PostgreSQL and SQLite. Existing
// ambiguous rows make this migration fail and require an explicit reviewed
// cleanup rather than silently selecting one metadata value.
func applyConfigControlPlanePhase3(tx *gorm.DB) error {
	stmt := fmt.Sprintf(`CREATE UNIQUE INDEX %s ON router_config_provider_headers(config_set_id, provider_name, LOWER(header_name))`, configControlPlaneProviderHeaderNameCIIndex)
	if err := tx.Exec(stmt).Error; err != nil {
		return fmt.Errorf("add case-insensitive provider-header uniqueness: %w", err)
	}
	return nil
}

func verifyConfigControlPlanePhase3(tx *gorm.DB) error {
	if err := verifyConfigControlPlanePhase2(tx); err != nil {
		return err
	}
	if !tx.Migrator().HasIndex("router_config_provider_headers", configControlPlaneProviderHeaderNameCIIndex) {
		return fmt.Errorf("required control-plane index router_config_provider_headers.%s is missing", configControlPlaneProviderHeaderNameCIIndex)
	}
	return nil
}

var configControlPlaneTables = []string{"router_config_sets", "router_config_server", "router_config_providers", "router_config_provider_headers", "router_config_provider_models", "router_config_model_groups", "router_config_model_group_targets", "router_config_callers", "router_config_caller_allowed_groups"}

var configControlPlaneRequiredColumns = map[string][]string{
	"router_config_sets":                  {"id", "runtime_scope", "name", "status", "validation_status", "created_by", "created_at", "activated_at"},
	"router_config_server":                {"config_set_id", "listen", "default_model_group", "state_path", "license_enabled", "license_path", "license_state_path"},
	"router_config_providers":             {"config_set_id", "provider_name", "base_url", "dialect", "api_key_env", "key_id", "auth_scheme"},
	"router_config_provider_headers":      {"config_set_id", "provider_name", "header_name", "header_value"},
	"router_config_provider_models":       {"config_set_id", "provider_name", "model_ref", "model", "dialect", "display_name", "context_tokens", "input_price_per_million_usd", "output_price_per_million_usd", "pricing_source", "pricing_updated_at", "pricing_notes"},
	"router_config_model_groups":          {"config_set_id", "group_name", "strategy", "attempt_timeout_ms"},
	"router_config_model_group_targets":   {"config_set_id", "group_name", "sequence", "provider_name", "model_ref", "model", "dialect", "weight", "rpm", "tier", "cost"},
	"router_config_callers":               {"config_set_id", "caller_id", "owner_user", "project", "environment", "status", "token_sha256", "token_id", "metrics_admin", "content_admin", "rpm", "tpm", "concurrent"},
	"router_config_caller_allowed_groups": {"config_set_id", "caller_id", "group_name"},
}

var configControlPlaneRequiredIndexes = map[string][]string{
	"router_config_sets":                {"router_config_one_active_set_per_scope"},
	"router_config_model_group_targets": {"router_config_targets_provider_model"},
}

var configControlPlaneRequiredConstraints = map[string][]string{
	"router_config_model_group_targets": {"router_config_targets_group_fk", "router_config_targets_provider_fk", "router_config_targets_provider_model_fk"},
}

type configControlPlaneForeignKey struct {
	Table             string
	Columns           []string
	ReferencedTable   string
	ReferencedColumns []string
}

type configControlPlaneForeignKeyRow struct {
	ConstraintID     string `gorm:"column:constraint_id"`
	Sequence         int    `gorm:"column:sequence"`
	ReferencedTable  string `gorm:"column:referenced_table"`
	LocalColumn      string `gorm:"column:local_column"`
	ReferencedColumn string `gorm:"column:referenced_column"`
}

var configControlPlaneRequiredForeignKeys = []configControlPlaneForeignKey{
	{Table: "router_config_server", Columns: []string{"config_set_id"}, ReferencedTable: "router_config_sets", ReferencedColumns: []string{"id"}},
	{Table: "router_config_providers", Columns: []string{"config_set_id"}, ReferencedTable: "router_config_sets", ReferencedColumns: []string{"id"}},
	{Table: "router_config_provider_headers", Columns: []string{"config_set_id", "provider_name"}, ReferencedTable: "router_config_providers", ReferencedColumns: []string{"config_set_id", "provider_name"}},
	{Table: "router_config_provider_models", Columns: []string{"config_set_id", "provider_name"}, ReferencedTable: "router_config_providers", ReferencedColumns: []string{"config_set_id", "provider_name"}},
	{Table: "router_config_model_groups", Columns: []string{"config_set_id"}, ReferencedTable: "router_config_sets", ReferencedColumns: []string{"id"}},
	{Table: "router_config_model_group_targets", Columns: []string{"config_set_id", "group_name"}, ReferencedTable: "router_config_model_groups", ReferencedColumns: []string{"config_set_id", "group_name"}},
	{Table: "router_config_model_group_targets", Columns: []string{"config_set_id", "provider_name"}, ReferencedTable: "router_config_providers", ReferencedColumns: []string{"config_set_id", "provider_name"}},
	{Table: "router_config_model_group_targets", Columns: []string{"config_set_id", "provider_name", "model_ref"}, ReferencedTable: "router_config_provider_models", ReferencedColumns: []string{"config_set_id", "provider_name", "model_ref"}},
	{Table: "router_config_callers", Columns: []string{"config_set_id"}, ReferencedTable: "router_config_sets", ReferencedColumns: []string{"id"}},
	{Table: "router_config_caller_allowed_groups", Columns: []string{"config_set_id", "caller_id"}, ReferencedTable: "router_config_callers", ReferencedColumns: []string{"config_set_id", "caller_id"}},
	{Table: "router_config_caller_allowed_groups", Columns: []string{"config_set_id", "group_name"}, ReferencedTable: "router_config_model_groups", ReferencedColumns: []string{"config_set_id", "group_name"}},
}

func verifyConfigControlPlaneForeignKey(tx *gorm.DB, required configControlPlaneForeignKey) error {
	var rows []configControlPlaneForeignKeyRow
	switch tx.Dialector.Name() {
	case "sqlite":
		// required.Table comes from the checked-in allowlist above, never a caller
		// value. SQLite reports every foreign-key column with a stable numeric id
		// and zero-based sequence.
		query := fmt.Sprintf(`SELECT id AS constraint_id, seq AS sequence, "table" AS referenced_table, "from" AS local_column, "to" AS referenced_column FROM pragma_foreign_key_list('%s')`, required.Table)
		if err := tx.Raw(query).Scan(&rows).Error; err != nil {
			return fmt.Errorf("inspect control-plane foreign keys for %s: %w", required.Table, err)
		}
	case "postgres":
		const query = `SELECT con.conname AS constraint_id,
			local_key.ordinality AS sequence,
			referenced_table.relname AS referenced_table,
			local_column.attname AS local_column,
			referenced_column.attname AS referenced_column
		FROM pg_constraint con
		JOIN pg_class local_table ON local_table.oid = con.conrelid
		JOIN pg_namespace local_namespace ON local_namespace.oid = local_table.relnamespace
		JOIN pg_class referenced_table ON referenced_table.oid = con.confrelid
		JOIN unnest(con.conkey) WITH ORDINALITY AS local_key(attnum, ordinality) ON TRUE
		JOIN unnest(con.confkey) WITH ORDINALITY AS referenced_key(attnum, ordinality) ON referenced_key.ordinality = local_key.ordinality
		JOIN pg_attribute local_column ON local_column.attrelid = con.conrelid AND local_column.attnum = local_key.attnum
		JOIN pg_attribute referenced_column ON referenced_column.attrelid = con.confrelid AND referenced_column.attnum = referenced_key.attnum
		WHERE con.contype = 'f' AND local_namespace.nspname = current_schema() AND local_table.relname = ?
		ORDER BY con.conname, local_key.ordinality`
		if err := tx.Raw(query, required.Table).Scan(&rows).Error; err != nil {
			return fmt.Errorf("inspect control-plane foreign keys for %s: %w", required.Table, err)
		}
	default:
		return fmt.Errorf("unsupported control-plane database driver %q", tx.Dialector.Name())
	}
	byConstraint := map[string][]configControlPlaneForeignKeyRow{}
	for _, row := range rows {
		byConstraint[row.ConstraintID] = append(byConstraint[row.ConstraintID], row)
	}
	for _, candidate := range byConstraint {
		sort.Slice(candidate, func(i, j int) bool { return candidate[i].Sequence < candidate[j].Sequence })
		if len(candidate) != len(required.Columns) || candidate[0].ReferencedTable != required.ReferencedTable {
			continue
		}
		matched := true
		for index, row := range candidate {
			if row.LocalColumn != required.Columns[index] || row.ReferencedColumn != required.ReferencedColumns[index] {
				matched = false
				break
			}
		}
		if matched {
			return nil
		}
	}
	return fmt.Errorf("required control-plane foreign key %s(%s) -> %s(%s) is missing", required.Table, strings.Join(required.Columns, ","), required.ReferencedTable, strings.Join(required.ReferencedColumns, ","))
}

var configControlPlaneDDL = []string{
	`CREATE TABLE IF NOT EXISTS router_config_sets (id TEXT PRIMARY KEY, runtime_scope TEXT NOT NULL, name TEXT NOT NULL, status TEXT NOT NULL, validation_status TEXT NOT NULL, created_by TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL, activated_at TEXT NOT NULL DEFAULT '', UNIQUE (runtime_scope, name))`,
	`CREATE UNIQUE INDEX IF NOT EXISTS router_config_one_active_set_per_scope ON router_config_sets(runtime_scope) WHERE status = 'active'`,
	`CREATE TABLE IF NOT EXISTS router_config_server (config_set_id TEXT PRIMARY KEY REFERENCES router_config_sets(id), listen TEXT NOT NULL DEFAULT '', default_model_group TEXT NOT NULL DEFAULT '', state_path TEXT NOT NULL DEFAULT '', license_enabled BOOLEAN NOT NULL DEFAULT TRUE, license_path TEXT NOT NULL DEFAULT '', license_state_path TEXT NOT NULL DEFAULT '')`,
	`CREATE TABLE IF NOT EXISTS router_config_providers (config_set_id TEXT NOT NULL REFERENCES router_config_sets(id), provider_name TEXT NOT NULL, base_url TEXT NOT NULL, dialect TEXT NOT NULL, api_key_env TEXT NOT NULL DEFAULT '', key_id TEXT NOT NULL DEFAULT '', auth_scheme TEXT NOT NULL DEFAULT '', PRIMARY KEY (config_set_id, provider_name))`,
	`CREATE TABLE IF NOT EXISTS router_config_provider_headers (config_set_id TEXT NOT NULL, provider_name TEXT NOT NULL, header_name TEXT NOT NULL, header_value TEXT NOT NULL, PRIMARY KEY (config_set_id, provider_name, header_name), FOREIGN KEY (config_set_id, provider_name) REFERENCES router_config_providers(config_set_id, provider_name))`,
	`CREATE TABLE IF NOT EXISTS router_config_provider_models (config_set_id TEXT NOT NULL, provider_name TEXT NOT NULL, model_ref TEXT NOT NULL, model TEXT NOT NULL, dialect TEXT NOT NULL DEFAULT '', display_name TEXT NOT NULL DEFAULT '', context_tokens BIGINT NOT NULL DEFAULT 0, input_price_per_million_usd DOUBLE PRECISION NOT NULL DEFAULT 0, output_price_per_million_usd DOUBLE PRECISION NOT NULL DEFAULT 0, pricing_source TEXT NOT NULL DEFAULT '', pricing_updated_at TEXT NOT NULL DEFAULT '', pricing_notes TEXT NOT NULL DEFAULT '', PRIMARY KEY (config_set_id, provider_name, model_ref), FOREIGN KEY (config_set_id, provider_name) REFERENCES router_config_providers(config_set_id, provider_name))`,
	`CREATE TABLE IF NOT EXISTS router_config_model_groups (config_set_id TEXT NOT NULL REFERENCES router_config_sets(id), group_name TEXT NOT NULL, strategy TEXT NOT NULL DEFAULT '', attempt_timeout_ms BIGINT NOT NULL DEFAULT 0, PRIMARY KEY (config_set_id, group_name))`,
	`CREATE TABLE IF NOT EXISTS router_config_model_group_targets (config_set_id TEXT NOT NULL, group_name TEXT NOT NULL, sequence BIGINT NOT NULL, provider_name TEXT NOT NULL, model_ref TEXT, model TEXT NOT NULL DEFAULT '', dialect TEXT NOT NULL DEFAULT '', weight BIGINT NOT NULL DEFAULT 0, rpm BIGINT NOT NULL DEFAULT 0, tier TEXT NOT NULL DEFAULT '', cost BIGINT NOT NULL DEFAULT 0, PRIMARY KEY (config_set_id, group_name, sequence), CONSTRAINT router_config_targets_group_fk FOREIGN KEY (config_set_id, group_name) REFERENCES router_config_model_groups(config_set_id, group_name), CONSTRAINT router_config_targets_provider_fk FOREIGN KEY (config_set_id, provider_name) REFERENCES router_config_providers(config_set_id, provider_name), CONSTRAINT router_config_targets_provider_model_fk FOREIGN KEY (config_set_id, provider_name, model_ref) REFERENCES router_config_provider_models(config_set_id, provider_name, model_ref))`,
	`CREATE INDEX IF NOT EXISTS router_config_targets_provider_model ON router_config_model_group_targets(config_set_id, provider_name, model_ref)`,
	`CREATE TABLE IF NOT EXISTS router_config_callers (config_set_id TEXT NOT NULL REFERENCES router_config_sets(id), caller_id TEXT NOT NULL, owner_user TEXT NOT NULL DEFAULT '', project TEXT NOT NULL DEFAULT '', environment TEXT NOT NULL DEFAULT '', status TEXT NOT NULL DEFAULT '', token_sha256 TEXT NOT NULL DEFAULT '', token_id TEXT NOT NULL DEFAULT '', metrics_admin BOOLEAN NOT NULL DEFAULT FALSE, content_admin BOOLEAN NOT NULL DEFAULT FALSE, rpm BIGINT NOT NULL DEFAULT 0, tpm BIGINT NOT NULL DEFAULT 0, concurrent BIGINT NOT NULL DEFAULT 0, PRIMARY KEY (config_set_id, caller_id))`,
	`CREATE TABLE IF NOT EXISTS router_config_caller_allowed_groups (config_set_id TEXT NOT NULL, caller_id TEXT NOT NULL, group_name TEXT NOT NULL, PRIMARY KEY (config_set_id, caller_id, group_name), FOREIGN KEY (config_set_id, caller_id) REFERENCES router_config_callers(config_set_id, caller_id), FOREIGN KEY (config_set_id, group_name) REFERENCES router_config_model_groups(config_set_id, group_name))`,
}

var configControlPlanePostgresComments = []string{
	`COMMENT ON TABLE router_config_sets IS 'Versioned configuration set metadata; exactly one validated active set is expected per runtime scope.'`,
	`COMMENT ON COLUMN router_config_sets.id IS 'Opaque configuration-set identifier.'`,
	`COMMENT ON COLUMN router_config_sets.runtime_scope IS 'Deployment/runtime scope owning the active configuration.'`,
	`COMMENT ON COLUMN router_config_sets.name IS 'Human-readable version name unique in the runtime scope.'`,
	`COMMENT ON COLUMN router_config_sets.status IS 'Draft, active, or archived lifecycle state.'`,
	`COMMENT ON COLUMN router_config_sets.validation_status IS 'Validation state; only valid sets may be read as active.'`,
	`COMMENT ON COLUMN router_config_sets.created_by IS 'Sanitized actor identifier that created the set.'`,
	`COMMENT ON COLUMN router_config_sets.created_at IS 'UTC creation timestamp.'`,
	`COMMENT ON COLUMN router_config_sets.activated_at IS 'UTC activation timestamp when applicable.'`,
	`COMMENT ON TABLE router_config_server IS 'Scalar server settings for one configuration set.'`,
	`COMMENT ON COLUMN router_config_server.config_set_id IS 'Owning configuration-set identifier.'`,
	`COMMENT ON COLUMN router_config_server.listen IS 'Router listener address.'`,
	`COMMENT ON COLUMN router_config_server.default_model_group IS 'Default deployment-defined model group.'`,
	`COMMENT ON COLUMN router_config_server.state_path IS 'Runtime state path, not a secret.'`,
	`COMMENT ON COLUMN router_config_server.license_enabled IS 'Whether license enforcement is enabled for this configuration set.'`,
	`COMMENT ON COLUMN router_config_server.license_path IS 'Deployment-managed path to the signed license file.'`,
	`COMMENT ON COLUMN router_config_server.license_state_path IS 'Deployment-managed path to non-secret license state.'`,
	`COMMENT ON TABLE router_config_providers IS 'Provider endpoint and secret-reference metadata; no raw provider secret is stored.'`,
	`COMMENT ON COLUMN router_config_providers.config_set_id IS 'Owning configuration-set identifier.'`,
	`COMMENT ON COLUMN router_config_providers.provider_name IS 'Deployment-local provider reference.'`,
	`COMMENT ON COLUMN router_config_providers.base_url IS 'Provider API base URL.'`,
	`COMMENT ON COLUMN router_config_providers.dialect IS 'Validated provider API dialect.'`,
	`COMMENT ON COLUMN router_config_providers.api_key_env IS 'Environment-variable name for the provider credential.'`,
	`COMMENT ON COLUMN router_config_providers.key_id IS 'Non-secret provider credential identifier.'`,
	`COMMENT ON COLUMN router_config_providers.auth_scheme IS 'Provider authentication scheme.'`,
	`COMMENT ON TABLE router_config_provider_headers IS 'One deployment-owned upstream header per provider.'`,
	`COMMENT ON COLUMN router_config_provider_headers.config_set_id IS 'Owning configuration-set identifier.'`,
	`COMMENT ON COLUMN router_config_provider_headers.provider_name IS 'Referenced provider name.'`,
	`COMMENT ON COLUMN router_config_provider_headers.header_name IS 'Configured non-secret upstream header name.'`,
	`COMMENT ON COLUMN router_config_provider_headers.header_value IS 'Configured non-secret upstream header value.'`,
	`COMMENT ON TABLE router_config_provider_models IS 'Catalogued provider models with scalar pricing metadata.'`,
	`COMMENT ON COLUMN router_config_provider_models.config_set_id IS 'Owning configuration-set identifier.'`,
	`COMMENT ON COLUMN router_config_provider_models.provider_name IS 'Referenced provider name.'`,
	`COMMENT ON COLUMN router_config_provider_models.model_ref IS 'Deployment-local provider model reference.'`,
	`COMMENT ON COLUMN router_config_provider_models.model IS 'Exact upstream model identifier.'`,
	`COMMENT ON COLUMN router_config_provider_models.dialect IS 'Optional model-specific API dialect.'`,
	`COMMENT ON COLUMN router_config_provider_models.display_name IS 'Non-secret operator display name.'`,
	`COMMENT ON COLUMN router_config_provider_models.context_tokens IS 'Advertised context-window tokens.'`,
	`COMMENT ON COLUMN router_config_provider_models.input_price_per_million_usd IS 'Input price per million tokens in USD.'`,
	`COMMENT ON COLUMN router_config_provider_models.output_price_per_million_usd IS 'Output price per million tokens in USD.'`,
	`COMMENT ON COLUMN router_config_provider_models.pricing_source IS 'Primary pricing evidence location.'`,
	`COMMENT ON COLUMN router_config_provider_models.pricing_updated_at IS 'Pricing evidence date.'`,
	`COMMENT ON COLUMN router_config_provider_models.pricing_notes IS 'Operator pricing caveats.'`,
	`COMMENT ON TABLE router_config_model_groups IS 'Deployment-defined model-group routing metadata.'`,
	`COMMENT ON COLUMN router_config_model_groups.config_set_id IS 'Owning configuration-set identifier.'`,
	`COMMENT ON COLUMN router_config_model_groups.group_name IS 'Deployment-defined router model group.'`,
	`COMMENT ON COLUMN router_config_model_groups.strategy IS 'Configured routing strategy.'`,
	`COMMENT ON COLUMN router_config_model_groups.attempt_timeout_ms IS 'Per-attempt timeout in milliseconds.'`,
	`COMMENT ON TABLE router_config_model_group_targets IS 'Ordered weighted targets for a model group.'`,
	`COMMENT ON COLUMN router_config_model_group_targets.config_set_id IS 'Owning configuration-set identifier.'`,
	`COMMENT ON COLUMN router_config_model_group_targets.group_name IS 'Referenced model group.'`,
	`COMMENT ON COLUMN router_config_model_group_targets.sequence IS 'Stable target ordering value.'`,
	`COMMENT ON COLUMN router_config_model_group_targets.provider_name IS 'Referenced provider name.'`,
	`COMMENT ON COLUMN router_config_model_group_targets.model_ref IS 'Referenced provider-model record when supplied.'`,
	`COMMENT ON COLUMN router_config_model_group_targets.model IS 'Inline exact upstream model identifier when supplied.'`,
	`COMMENT ON COLUMN router_config_model_group_targets.dialect IS 'Optional target-specific API dialect.'`,
	`COMMENT ON COLUMN router_config_model_group_targets.weight IS 'Group-local routing weight.'`,
	`COMMENT ON COLUMN router_config_model_group_targets.rpm IS 'Target rate limit in requests per minute.'`,
	`COMMENT ON COLUMN router_config_model_group_targets.tier IS 'Deployment-local target tier.'`,
	`COMMENT ON COLUMN router_config_model_group_targets.cost IS 'Deployment-local relative cost class.'`,
	`COMMENT ON TABLE router_config_callers IS 'Caller identity and token hash metadata; raw caller tokens are never stored.'`,
	`COMMENT ON COLUMN router_config_callers.config_set_id IS 'Owning configuration-set identifier.'`,
	`COMMENT ON COLUMN router_config_callers.caller_id IS 'Stable caller identity.'`,
	`COMMENT ON COLUMN router_config_callers.owner_user IS 'Normalized owning user identifier.'`,
	`COMMENT ON COLUMN router_config_callers.project IS 'Normalized owning project identifier.'`,
	`COMMENT ON COLUMN router_config_callers.environment IS 'Normalized deployment environment identifier.'`,
	`COMMENT ON COLUMN router_config_callers.status IS 'Caller lifecycle status.'`,
	`COMMENT ON COLUMN router_config_callers.token_sha256 IS 'Write-only SHA-256 hash of the caller token.'`,
	`COMMENT ON COLUMN router_config_callers.token_id IS 'Non-secret public caller-token identifier.'`,
	`COMMENT ON COLUMN router_config_callers.metrics_admin IS 'Whether caller may access global metrics.'`,
	`COMMENT ON COLUMN router_config_callers.content_admin IS 'Whether caller may access governed content administration.'`,
	`COMMENT ON COLUMN router_config_callers.rpm IS 'Caller request rate limit.'`,
	`COMMENT ON COLUMN router_config_callers.tpm IS 'Caller token rate limit.'`,
	`COMMENT ON COLUMN router_config_callers.concurrent IS 'Caller concurrent request limit.'`,
	`COMMENT ON TABLE router_config_caller_allowed_groups IS 'One allowed model group per caller.'`,
	`COMMENT ON COLUMN router_config_caller_allowed_groups.config_set_id IS 'Owning configuration-set identifier.'`,
	`COMMENT ON COLUMN router_config_caller_allowed_groups.caller_id IS 'Referenced caller identifier.'`,
	`COMMENT ON COLUMN router_config_caller_allowed_groups.group_name IS 'Referenced allowed model group.'`,
}

type configSetRow struct {
	ID               string `gorm:"column:id"`
	RuntimeScope     string `gorm:"column:runtime_scope"`
	Status           string `gorm:"column:status"`
	ValidationStatus string `gorm:"column:validation_status"`
}

func (configSetRow) TableName() string { return "router_config_sets" }

type serverConfigRow struct {
	ConfigSetID       string `gorm:"column:config_set_id"`
	Listen            string `gorm:"column:listen"`
	DefaultModelGroup string `gorm:"column:default_model_group"`
	StatePath         string `gorm:"column:state_path"`
	LicenseEnabled    bool   `gorm:"column:license_enabled"`
	LicensePath       string `gorm:"column:license_path"`
	LicenseStatePath  string `gorm:"column:license_state_path"`
}

func (serverConfigRow) TableName() string { return "router_config_server" }

type providerRow struct {
	ConfigSetID  string `gorm:"column:config_set_id"`
	ProviderName string `gorm:"column:provider_name"`
	BaseURL      string `gorm:"column:base_url"`
	Dialect      string `gorm:"column:dialect"`
	APIKeyEnv    string `gorm:"column:api_key_env"`
	KeyID        string `gorm:"column:key_id"`
	AuthScheme   string `gorm:"column:auth_scheme"`
}

func (providerRow) TableName() string { return "router_config_providers" }

type providerHeaderRow struct {
	HeaderName  string `gorm:"column:header_name"`
	HeaderValue string `gorm:"column:header_value"`
}

func (providerHeaderRow) TableName() string { return "router_config_provider_headers" }

type providerModelRow struct {
	ModelRef                 string  `gorm:"column:model_ref"`
	Model                    string  `gorm:"column:model"`
	Dialect                  string  `gorm:"column:dialect"`
	DisplayName              string  `gorm:"column:display_name"`
	ContextTokens            int     `gorm:"column:context_tokens"`
	InputPricePerMillionUSD  float64 `gorm:"column:input_price_per_million_usd"`
	OutputPricePerMillionUSD float64 `gorm:"column:output_price_per_million_usd"`
	PricingSource            string  `gorm:"column:pricing_source"`
	PricingUpdatedAt         string  `gorm:"column:pricing_updated_at"`
	PricingNotes             string  `gorm:"column:pricing_notes"`
}

func (providerModelRow) TableName() string { return "router_config_provider_models" }

type modelGroupRow struct {
	GroupName        string `gorm:"column:group_name"`
	Strategy         string `gorm:"column:strategy"`
	AttemptTimeoutMS int    `gorm:"column:attempt_timeout_ms"`
}

func (modelGroupRow) TableName() string { return "router_config_model_groups" }

type modelGroupTargetRow struct {
	Sequence     int            `gorm:"column:sequence"`
	ProviderName string         `gorm:"column:provider_name"`
	ModelRef     sql.NullString `gorm:"column:model_ref"`
	Model        string         `gorm:"column:model"`
	Dialect      string         `gorm:"column:dialect"`
	Weight       int            `gorm:"column:weight"`
	RPM          int            `gorm:"column:rpm"`
	Tier         string         `gorm:"column:tier"`
	Cost         int            `gorm:"column:cost"`
}

func (modelGroupTargetRow) TableName() string { return "router_config_model_group_targets" }

type callerRow struct {
	CallerID     string `gorm:"column:caller_id"`
	OwnerUser    string `gorm:"column:owner_user"`
	Project      string `gorm:"column:project"`
	Environment  string `gorm:"column:environment"`
	Status       string `gorm:"column:status"`
	TokenSHA256  string `gorm:"column:token_sha256"`
	TokenID      string `gorm:"column:token_id"`
	MetricsAdmin bool   `gorm:"column:metrics_admin"`
	ContentAdmin bool   `gorm:"column:content_admin"`
	RPM          int    `gorm:"column:rpm"`
	TPM          int    `gorm:"column:tpm"`
	Concurrent   int    `gorm:"column:concurrent"`
}

func (callerRow) TableName() string { return "router_config_callers" }

type callerAllowedGroupRow struct {
	GroupName string `gorm:"column:group_name"`
}

func (callerAllowedGroupRow) TableName() string { return "router_config_caller_allowed_groups" }

// ConfigControlPlaneTableNames returns a sorted copy for schema validation
// tests and operator inspection without exposing configuration values.
func ConfigControlPlaneTableNames() []string {
	names := append([]string(nil), configControlPlaneTables...)
	sort.Strings(names)
	return names
}
