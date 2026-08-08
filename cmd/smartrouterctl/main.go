// smartrouterctl is the customer-local Router operations CLI. It has no cloud,
// deployment, configuration-activation, key-rotation, or license-signing authority.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"smart-llmrouter/internal/buildinfo"
	"smart-llmrouter/internal/router"
)

func main() {
	if len(os.Args) == 2 && os.Args[1] == "--version" {
		fmt.Println(buildinfo.Text())
		return
	}
	if len(os.Args) < 2 {
		die("usage: smartrouterctl <version|config|callers|status|license|models|usage> [flags]")
	}
	switch os.Args[1] {
	case "version":
		fmt.Println(buildinfo.Text())
	case "config":
		configCommand(os.Args[2:])
	case "callers":
		callersCommand(os.Args[2:])
	case "status":
		statusCommand(os.Args[2:])
	case "license":
		licenseCommand(os.Args[2:])
	case "models":
		modelsCommand(os.Args[2:])
	case "usage":
		usageCommand(os.Args[2:])
	default:
		die("unsupported command %q", os.Args[1])
	}
}

func configCommand(args []string) {
	if len(args) == 0 {
		die("usage: smartrouterctl config <validate|diff> [flags]")
	}
	switch args[0] {
	case "validate":
		fs := flag.NewFlagSet("config validate", flag.ExitOnError)
		path := fs.String("config", "", "router configuration path")
		fs.Parse(args[1:])
		cfg := loadConfig(*path)
		writeJSON(configSummary(cfg))
	case "diff":
		fs := flag.NewFlagSet("config diff", flag.ExitOnError)
		from := fs.String("from", "", "existing router configuration path")
		to := fs.String("to", "", "candidate router configuration path")
		fs.Parse(args[1:])
		before, after := loadConfig(*from), loadConfig(*to)
		writeJSON(configDiff(before, after))
	default:
		die("unsupported config command %q", args[0])
	}
}

func callersCommand(args []string) {
	if len(args) == 0 || args[0] != "generate" {
		die("usage: smartrouterctl callers generate [flags]")
	}
	fs := flag.NewFlagSet("callers generate", flag.ExitOnError)
	owner := fs.String("owner-user", "", "caller owner user id")
	project := fs.String("project", "", "caller project name")
	environment := fs.String("env", "dev", "caller environment")
	key := fs.String("key", "", "visible key slug")
	allow := fs.String("allow", "", "comma-separated allowed model groups")
	tokenOut := fs.String("token-out", "", "new mode-0600 token file; must not already exist")
	fs.Parse(args[1:])
	if strings.TrimSpace(*tokenOut) == "" {
		die("token-out is required")
	}
	generated, err := router.GenerateCallerToken(router.TokenGenerateOptions{OwnerUser: *owner, Project: *project, Environment: *environment, KeySlug: *key, Allow: splitCSV(*allow)})
	if err != nil {
		die("generate caller token: %v", err)
	}
	if err := writeNewPrivateFile(*tokenOut, []byte(generated.Token+"\n")); err != nil {
		die("write token: %v", err)
	}
	writeJSON(map[string]any{
		"schema": "metrum.ai/smartrouter-caller-grant/v1", "caller_id": generated.Caller.ID,
		"token_id": generated.TokenID, "allowed_model_groups": generated.Caller.Allow,
		"token_file": filepath.Base(*tokenOut), "activation": "configuration-controller-required",
	})
}

func statusCommand(args []string) {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	path := fs.String("config", "", "router configuration path")
	fs.Parse(args)
	cfg := loadConfig(*path)
	writeJSON(map[string]any{"schema": "metrum.ai/smartrouter-customer-status/v1", "config": configSummary(cfg), "authority": "local-read-only"})
}

func licenseCommand(args []string) {
	if len(args) == 0 || args[0] != "status" {
		die("usage: smartrouterctl license status --config PATH")
	}
	fs := flag.NewFlagSet("license status", flag.ExitOnError)
	path := fs.String("config", "", "router configuration path")
	fs.Parse(args[1:])
	cfg := loadConfig(*path)
	if !cfg.Server.License.Enabled {
		writeJSON(map[string]any{"schema": "metrum.ai/smartrouter-license-status/v1", "enabled": false, "valid": true, "code": "license-disabled"})
		return
	}
	raw, err := os.ReadFile(cfg.Server.License.Path)
	if err != nil {
		die("read license: %v", err)
	}
	envelope, err := router.ParseLicenseEnvelope(raw)
	if err != nil {
		die("parse license: %v", err)
	}
	if err := router.VerifyLicenseEnvelope(envelope, router.DefaultLicensePublicKeys(), time.Now().UTC()); err != nil {
		die("verify license: %v", err)
	}
	if err := router.ValidateLicensePayload(envelope.Payload, cfg, router.DefaultLicensePublicKeys(), time.Now().UTC()); err != nil {
		die("validate license: %v", err)
	}
	features := append([]string(nil), envelope.Payload.Features...)
	sort.Strings(features)
	writeJSON(map[string]any{"schema": "metrum.ai/smartrouter-license-status/v1", "enabled": true, "valid": true, "code": "license-valid", "expires_at": envelope.Payload.ExpiresAt, "features": features})
}

func modelsCommand(args []string) {
	if len(args) == 0 || args[0] != "list" {
		die("usage: smartrouterctl models list --config PATH")
	}
	fs := flag.NewFlagSet("models list", flag.ExitOnError)
	path := fs.String("config", "", "router configuration path")
	fs.Parse(args[1:])
	cfg := loadConfig(*path)
	groups := make([]map[string]any, 0, len(cfg.Models))
	for name, group := range cfg.Models {
		groups = append(groups, map[string]any{"name": name, "strategy": group.Strategy, "target_count": len(group.Targets)})
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i]["name"].(string) < groups[j]["name"].(string) })
	writeJSON(map[string]any{"schema": "metrum.ai/smartrouter-model-list/v1", "data": groups})
}

func usageCommand(args []string) {
	if len(args) == 0 || args[0] != "summary" {
		die("usage: smartrouterctl usage summary --config PATH [--since 24h]")
	}
	fs := flag.NewFlagSet("usage summary", flag.ExitOnError)
	path := fs.String("config", "", "router configuration path")
	since := fs.Duration("since", 24*time.Hour, "summary window")
	fs.Parse(args[1:])
	cfg := loadConfig(*path)
	if cfg.Server.UsageDB.Enable == nil || !*cfg.Server.UsageDB.Enable {
		die("usage database is disabled")
	}
	to := time.Now().UTC()
	markdown, err := router.GenerateUsageMarkdown(router.UsageReportOptions{Driver: cfg.Server.UsageDB.Driver, DBPath: cfg.Server.UsageDB.Path, DSN: cfg.Server.UsageDB.DSN, MigrationPolicy: cfg.Server.UsageDB.MigrationPolicy, From: to.Add(-*since), To: to})
	if err != nil {
		die("generate usage summary: %v", err)
	}
	fmt.Print(markdown)
}

func loadConfig(path string) *router.Config {
	if strings.TrimSpace(path) == "" {
		die("config is required")
	}
	cfg, err := router.LoadConfig(path)
	if err != nil {
		die("load config: %v", err)
	}
	return cfg
}

func configSummary(cfg *router.Config) map[string]any {
	groups, callers := sortedGroups(cfg), sortedCallerIDs(cfg)
	return map[string]any{"schema": "metrum.ai/smartrouter-config-summary/v1", "valid": true, "model_groups": groups, "caller_ids": callers, "provider_count": len(cfg.Provider)}
}

func configDiff(before, after *router.Config) map[string]any {
	return map[string]any{"schema": "metrum.ai/smartrouter-config-diff/v1", "model_groups": setDiff(sortedGroups(before), sortedGroups(after)), "caller_ids": setDiff(sortedCallerIDs(before), sortedCallerIDs(after)), "provider_count": map[string]int{"before": len(before.Provider), "after": len(after.Provider)}}
}

func sortedGroups(cfg *router.Config) []string {
	out := make([]string, 0, len(cfg.Models))
	for name := range cfg.Models {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}
func sortedCallerIDs(cfg *router.Config) []string {
	out := make([]string, 0, len(cfg.Callers))
	for _, caller := range cfg.Callers {
		out = append(out, caller.ID)
	}
	sort.Strings(out)
	return out
}
func setDiff(before, after []string) map[string][]string {
	return map[string][]string{"added": difference(after, before), "removed": difference(before, after)}
}
func difference(left, right []string) []string {
	seen := make(map[string]struct{}, len(right))
	for _, value := range right {
		seen[value] = struct{}{}
	}
	out := make([]string, 0)
	for _, value := range left {
		if _, ok := seen[value]; !ok {
			out = append(out, value)
		}
	}
	return out
}

func splitCSV(raw string) []string {
	values := strings.Split(raw, ",")
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			out = append(out, value)
		}
	}
	return out
}
func writeNewPrivateFile(path string, contents []byte) error {
	if strings.TrimSpace(path) == "" || filepath.Base(path) == "." {
		return errors.New("token output path is invalid")
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := f.Chmod(0600); err != nil {
		return err
	}
	if _, err := f.Write(contents); err != nil {
		return err
	}
	return f.Sync()
}
func writeJSON(value any) {
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		die("encode safe output: %v", err)
	}
	fmt.Println(string(body))
}
func die(format string, args ...any) { fmt.Fprintf(os.Stderr, format+"\n", args...); os.Exit(2) }
