package customerlifecycle

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	fleetEKSStateRoot   = "/var/lib/smart-llmrouter"
	fleetEKSLicenseRoot = "/etc/smart-llmrouter-license"
)

// ValidateConfigRoutesBYOK ensures config_file activates the BYOK provider/model/api_key_env.
func ValidateConfigRoutesBYOK(configPath string, byok BYOKSpec, smokeModel string) error {
	root, err := loadConfigYAML(configPath)
	if err != nil {
		return err
	}
	providers, _ := root["providers"].(map[string]any)
	if providers == nil {
		return fmt.Errorf("config_file missing providers")
	}
	providerRaw, ok := providers[byok.Provider]
	if !ok {
		return fmt.Errorf("config_file missing providers.%s for BYOK", byok.Provider)
	}
	provider, _ := providerRaw.(map[string]any)
	if provider == nil {
		return fmt.Errorf("providers.%s must be a mapping", byok.Provider)
	}
	apiKeyEnv, _ := provider["api_key_env"].(string)
	if strings.TrimSpace(apiKeyEnv) != byok.APIKeyEnv {
		return fmt.Errorf("providers.%s.api_key_env must be %s", byok.Provider, byok.APIKeyEnv)
	}
	// Router LoadConfig expands ${ENV} into providers.*.api_key after loading env.json.
	// api_key_env alone does not populate Authorization; omit api_key → upstream 401.
	apiKey, _ := provider["api_key"].(string)
	wantExpand := "${" + byok.APIKeyEnv + "}"
	if strings.TrimSpace(apiKey) != wantExpand {
		return fmt.Errorf("providers.%s.api_key must be %s (LoadConfig expands env into the credential field)", byok.Provider, wantExpand)
	}
	models, _ := provider["models"].(map[string]any)
	if models == nil {
		return fmt.Errorf("providers.%s.models is required", byok.Provider)
	}
	foundModel := false
	for ref, modelRaw := range models {
		modelMap, _ := modelRaw.(map[string]any)
		upstream, _ := modelMap["model"].(string)
		if strings.TrimSpace(upstream) == byok.Model || strings.TrimSpace(ref) == byok.Model {
			foundModel = true
			break
		}
	}
	if !foundModel {
		return fmt.Errorf("providers.%s.models must include BYOK model %s", byok.Provider, byok.Model)
	}
	groupName := strings.TrimSpace(smokeModel)
	if groupName == "" {
		groupName = "default"
	}
	modelGroups, _ := root["models"].(map[string]any)
	if modelGroups == nil {
		return fmt.Errorf("config_file missing models groups")
	}
	groupRaw, ok := modelGroups[groupName]
	if !ok {
		return fmt.Errorf("config_file missing models.%s smoke group", groupName)
	}
	group, _ := groupRaw.(map[string]any)
	if group == nil {
		return fmt.Errorf("models.%s must be a mapping", groupName)
	}
	targets, _ := group["targets"].([]any)
	if len(targets) == 0 {
		return fmt.Errorf("models.%s.targets must route the BYOK model", groupName)
	}
	matched := false
	for _, t := range targets {
		tm, _ := t.(map[string]any)
		if tm == nil {
			continue
		}
		p, _ := tm["provider"].(string)
		m, _ := tm["model"].(string)
		if strings.TrimSpace(p) == byok.Provider && (strings.TrimSpace(m) == byok.Model || modelRefMatches(models, m, byok.Model)) {
			matched = true
			break
		}
	}
	if !matched {
		return fmt.Errorf("models.%s must include a target for provider=%s model=%s", groupName, byok.Provider, byok.Model)
	}
	if err := validateBootstrapAccounts(root); err != nil {
		return err
	}
	return nil
}

// validateBootstrapAccounts ensures project_memberships use router yaml keys (user_id, not user).
// A wrong key unmarshals to empty UserID and CrashLoops with "project membership missing user_id or project".
func validateBootstrapAccounts(root map[string]any) error {
	raw, ok := root["project_memberships"]
	if !ok {
		return nil
	}
	list, _ := raw.([]any)
	if len(list) == 0 {
		return nil
	}
	for i, item := range list {
		m, _ := item.(map[string]any)
		if m == nil {
			return fmt.Errorf("project_memberships[%d] must be a mapping", i)
		}
		if _, hasLegacy := m["user"]; hasLegacy {
			if _, hasID := m["user_id"]; !hasID {
				return fmt.Errorf("project_memberships[%d] uses user; router requires user_id", i)
			}
		}
		userID, _ := m["user_id"].(string)
		project, _ := m["project"].(string)
		if strings.TrimSpace(userID) == "" || strings.TrimSpace(project) == "" {
			return fmt.Errorf("project_memberships[%d] requires non-empty user_id and project", i)
		}
	}
	return nil
}

// ValidateFleetEKSProvisionPaths fail-closes when Fleet provision config lacks durable
// EKS paths. Lifecycle always publishes with --rewrite-paths fleet-eks; rewrite only
// remaps /app/state and /app/logs, so relative or /app/config license paths still break pods.
// Router Config.state_path is TOP-LEVEL yaml (not server.state_path); a nested server.state_path
// is ignored and defaults to relative router-state.json, which CrashLoops on read-only /.
func ValidateFleetEKSProvisionPaths(configPath string) error {
	root, err := loadConfigYAML(configPath)
	if err != nil {
		return err
	}
	if server, _ := root["server"].(map[string]any); server != nil {
		if nested, _ := server["state_path"].(string); strings.TrimSpace(nested) != "" {
			return fmt.Errorf("server.state_path is ignored by the router; set top-level state_path under %s for fleet-eks provision", fleetEKSStateRoot)
		}
	}
	statePath, _ := root["state_path"].(string)
	if err := requirePathUnder("state_path", statePath, fleetEKSStateRoot); err != nil {
		return err
	}
	server, _ := root["server"].(map[string]any)
	if server == nil {
		return fmt.Errorf("config_file missing server mapping required for fleet-eks provision")
	}
	if logging, ok := server["logging"].(map[string]any); ok {
		if p, _ := logging["path"].(string); strings.TrimSpace(p) != "" {
			if err := requirePathUnder("server.logging.path", p, fleetEKSStateRoot); err != nil {
				return err
			}
		}
	}
	if usage, ok := server["usage_db"].(map[string]any); ok {
		if p, _ := usage["path"].(string); strings.TrimSpace(p) != "" {
			if err := requirePathUnder("server.usage_db.path", p, fleetEKSStateRoot); err != nil {
				return err
			}
		}
	}
	license, _ := server["license"].(map[string]any)
	if license == nil {
		return fmt.Errorf("config_file missing server.license for fleet-eks provision")
	}
	licensePath, _ := license["path"].(string)
	if err := requirePathUnder("server.license.path", licensePath, fleetEKSLicenseRoot); err != nil {
		return err
	}
	licenseState, _ := license["state_path"].(string)
	if err := requirePathUnder("server.license.state_path", licenseState, fleetEKSStateRoot); err != nil {
		return err
	}
	if rev, ok := license["revocation"].(map[string]any); ok {
		if p, _ := rev["path"].(string); strings.TrimSpace(p) != "" {
			if err := requirePathUnder("server.license.revocation.path", p, fleetEKSLicenseRoot); err != nil {
				return err
			}
		}
	}
	return nil
}

// ValidateConfigRetentionForSKU fail-closes when configured retention exceeds the
// SKU catalog max_retention_days (readyz returns license-retention-limit-exceeded).
// Mirrors router maxConfiguredRetentionDays defaults: unset diagnostics.retention_days
// defaults to 30, so eval SKUs must set an explicit value at or below the limit.
func ValidateConfigRetentionForSKU(configPath, catalogPath, sku string) error {
	sku = strings.TrimSpace(sku)
	if sku == "" {
		return fmt.Errorf("sku is required for retention validation")
	}
	maxDays, err := catalogMaxRetentionDays(catalogPath, sku)
	if err != nil {
		return err
	}
	if maxDays <= 0 {
		return nil
	}
	root, err := loadConfigYAML(configPath)
	if err != nil {
		return err
	}
	configured := configuredRetentionDays(root)
	if configured > maxDays {
		return fmt.Errorf("config retention %d days exceeds sku %s max_retention_days %d (set server.diagnostics.retention_days and retention classes <= %d)", configured, sku, maxDays, maxDays)
	}
	return nil
}

func catalogMaxRetentionDays(catalogPath, sku string) (int, error) {
	catalogPath = expandHome(catalogPath)
	raw, err := os.ReadFile(catalogPath)
	if err != nil {
		return 0, fmt.Errorf("read license catalog: %w", err)
	}
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		return 0, fmt.Errorf("license catalog must be JSON: %w", err)
	}
	skus, _ := root["skus"].([]any)
	for _, item := range skus {
		m, _ := item.(map[string]any)
		if m == nil {
			continue
		}
		if strings.TrimSpace(fmt.Sprint(m["sku"])) != sku {
			continue
		}
		limits, _ := m["limits"].(map[string]any)
		if limits == nil {
			return 0, nil
		}
		switch v := limits["max_retention_days"].(type) {
		case float64:
			return int(v), nil
		case int:
			return v, nil
		case json.Number:
			n, _ := v.Int64()
			return int(n), nil
		default:
			return 0, nil
		}
	}
	return 0, fmt.Errorf("sku %q not found in license catalog", sku)
}

func configuredRetentionDays(root map[string]any) int {
	maxDays := 0
	server, _ := root["server"].(map[string]any)
	if server == nil {
		// Router defaults diagnostics to enabled with retention_days 30.
		return 30
	}
	if ret, ok := server["retention"].(map[string]any); ok {
		enabled, _ := ret["enabled"].(bool)
		if enabled {
			classes, _ := ret["classes"].([]any)
			for _, c := range classes {
				cm, _ := c.(map[string]any)
				if cm == nil {
					continue
				}
				if en, ok := cm["enabled"].(bool); ok && !en {
					continue
				}
				if d := anyToInt(cm["retention_days"]); d > maxDays {
					maxDays = d
				}
			}
		}
	}
	if cc, ok := server["content_capture"].(map[string]any); ok {
		if en, _ := cc["enabled"].(bool); en {
			if d := anyToInt(cc["retention_days"]); d > maxDays {
				maxDays = d
			}
		}
	}
	diag, _ := server["diagnostics"].(map[string]any)
	diagEnabled := true
	if diag != nil {
		if en, ok := diag["enabled"].(bool); ok {
			diagEnabled = en
		}
	}
	if diagEnabled {
		days := 30 // router default when unset
		if diag != nil {
			if _, ok := diag["retention_days"]; ok {
				days = anyToInt(diag["retention_days"])
			}
		}
		if days > maxDays {
			maxDays = days
		}
	}
	if admin, ok := server["admin_reports"].(map[string]any); ok {
		if en, _ := admin["enabled"].(bool); en {
			if sec, ok := admin["security"].(map[string]any); ok {
				if sen, _ := sec["enabled"].(bool); sen {
					days := 90
					if _, ok := sec["retention_days"]; ok {
						days = anyToInt(sec["retention_days"])
					}
					if days > maxDays {
						maxDays = days
					}
				}
			}
		}
	}
	return maxDays
}

func anyToInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	case json.Number:
		i, _ := n.Int64()
		return int(i)
	default:
		return 0
	}
}

func loadConfigYAML(configPath string) (map[string]any, error) {
	raw, err := os.ReadFile(expandHome(configPath))
	if err != nil {
		return nil, fmt.Errorf("read config_file: %w", err)
	}
	var root map[string]any
	if err := yaml.Unmarshal(raw, &root); err != nil {
		return nil, fmt.Errorf("config_file must be YAML: %w", err)
	}
	if root == nil {
		return nil, fmt.Errorf("config_file is empty")
	}
	return root, nil
}

func requirePathUnder(field, value, root string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return fmt.Errorf("%s is required for fleet-eks provision (must be under %s)", field, root)
	}
	if !path.IsAbs(value) {
		return fmt.Errorf("%s must be an absolute path under %s (got %q)", field, root, value)
	}
	clean := path.Clean(value)
	if clean != root && !strings.HasPrefix(clean, root+"/") {
		return fmt.Errorf("%s must be under %s for fleet-eks provision (got %q)", field, root, value)
	}
	return nil
}

func modelRefMatches(models map[string]any, ref, upstream string) bool {
	ref = strings.TrimSpace(ref)
	upstream = strings.TrimSpace(upstream)
	if ref == upstream {
		return true
	}
	modelRaw, ok := models[ref]
	if !ok {
		return false
	}
	modelMap, _ := modelRaw.(map[string]any)
	got, _ := modelMap["model"].(string)
	return strings.TrimSpace(got) == upstream
}
