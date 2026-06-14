package router

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dop251/goja"
	"github.com/evanw/esbuild/pkg/api"
)

type scriptStrategy struct {
	path    string
	program *goja.Program
}

type scriptInput struct {
	Group   string         `json:"group"`
	Request *IRRequest     `json:"request"`
	Targets []scriptTarget `json:"targets"`
	Caller  *scriptCaller  `json:"caller,omitempty"`
	Text    string         `json:"text"`
	Now     string         `json:"now"`
}

type scriptCaller struct {
	ID          string   `json:"id"`
	User        string   `json:"user"`
	Project     string   `json:"project"`
	Environment string   `json:"environment"`
	TokenID     string   `json:"tokenId"`
	Allow       []string `json:"allow"`
}

type scriptTarget struct {
	Provider      string `json:"provider"`
	Model         string `json:"model"`
	ModelRef      string `json:"modelRef,omitempty"`
	DisplayName   string `json:"displayName,omitempty"`
	Dialect       string `json:"dialect"`
	BaseURL       string `json:"baseUrl"`
	Weight        int    `json:"weight"`
	RPM           int    `json:"rpm,omitempty"`
	Tier          string `json:"tier,omitempty"`
	Cost          int    `json:"cost,omitempty"`
	KeyID         string `json:"keyId,omitempty"`
	APIKeyEnv     string `json:"apiKeyEnv,omitempty"`
	KeyConfigured bool   `json:"keyConfigured"`
}

type scriptOutput struct {
	Target          any    `json:"target"`
	TargetIndex     int    `json:"targetIndex"`
	HasTargetIndex  bool   `json:"-"`
	Fallbacks       []any  `json:"fallbacks"`
	FallbackIndexes []int  `json:"fallbackIndexes"`
	ClassLabel      string `json:"classLabel"`
}

func loadScriptStrategy(baseDir, scriptPath string) (*scriptStrategy, error) {
	if scriptPath == "" {
		return nil, fmt.Errorf("missing script path")
	}
	resolved := scriptPath
	if !filepath.IsAbs(resolved) {
		if baseDir == "" {
			baseDir = "."
		}
		resolved = filepath.Join(baseDir, scriptPath)
	}
	source, err := os.ReadFile(resolved)
	if err != nil {
		return nil, err
	}
	result := api.Transform(string(source), api.TransformOptions{
		Loader:            api.LoaderTS,
		Format:            api.FormatIIFE,
		GlobalName:        "routerScript",
		Target:            api.ES2018,
		Sourcemap:         api.SourceMapNone,
		LegalComments:     api.LegalCommentsNone,
		Supported:         map[string]bool{"dynamic-import": false},
		Sourcefile:        filepath.Base(resolved),
		Platform:          api.PlatformNeutral,
		TreeShaking:       api.TreeShakingTrue,
		KeepNames:         true,
		MinifyWhitespace:  false,
		MinifyIdentifiers: false,
		MinifySyntax:      false,
	})
	if len(result.Errors) > 0 {
		return nil, fmt.Errorf("typescript transform: %s", result.Errors[0].Text)
	}
	program, err := goja.Compile(resolved, string(result.Code), false)
	if err != nil {
		return nil, err
	}
	return &scriptStrategy{path: resolved, program: program}, nil
}

func (s *scriptStrategy) Pick(group string, req *IRRequest, targets []Target, providers map[string]ProviderConfig, caller *callerRuntime, tokenID string) (decision, error) {
	vm := goja.New()
	timer := time.AfterFunc(50*time.Millisecond, func() {
		vm.Interrupt("script routing timed out")
	})
	defer timer.Stop()
	if _, err := vm.RunProgram(s.program); err != nil {
		return decision{}, fmt.Errorf("run %s: %w", s.path, err)
	}
	mod := vm.Get("routerScript").ToObject(vm)
	fn, ok := goja.AssertFunction(mod.Get("route"))
	if !ok {
		return decision{}, fmt.Errorf("%s must export function route(ctx)", s.path)
	}
	input := scriptInput{
		Group:   group,
		Request: req,
		Targets: buildScriptTargets(targets, providers),
		Caller:  buildScriptCaller(caller, tokenID),
		Text:    requestText(req),
		Now:     time.Now().UTC().Format(time.RFC3339),
	}
	ctxValue, err := jsonValue(input)
	if err != nil {
		return decision{}, err
	}
	val, err := fn(goja.Undefined(), vm.ToValue(ctxValue))
	if err != nil {
		return decision{}, fmt.Errorf("route(ctx): %w", err)
	}
	out, err := exportScriptOutput(val)
	if err != nil {
		return decision{}, err
	}
	primary, err := resolveScriptTarget(out, targets)
	if err != nil {
		return decision{}, err
	}
	fallbacks, err := resolveScriptFallbacks(out, primary, targets)
	if err != nil {
		return decision{}, err
	}
	var classLabel *string
	if out.ClassLabel != "" {
		classLabel = &out.ClassLabel
	}
	return decision{
		Target:     targets[primary],
		Fallbacks:  fallbacks,
		ClassLabel: classLabel,
		Strategy:   "script",
		GroupName:  group,
	}, nil
}

func buildScriptCaller(caller *callerRuntime, tokenID string) *scriptCaller {
	if caller == nil {
		return nil
	}
	allow := append([]string(nil), caller.cfg.Allow...)
	return &scriptCaller{
		ID:          caller.cfg.ID,
		User:        callerUser(caller.cfg),
		Project:     callerProject(caller.cfg),
		Environment: callerEnvironment(caller.cfg),
		TokenID:     tokenID,
		Allow:       allow,
	}
}

func buildScriptTargets(targets []Target, providers map[string]ProviderConfig) []scriptTarget {
	out := make([]scriptTarget, 0, len(targets))
	for _, target := range targets {
		provider := providers[target.Provider]
		weight := target.Weight
		if weight == 0 {
			weight = 1
		}
		out = append(out, scriptTarget{
			Provider:      target.Provider,
			Model:         target.Model,
			ModelRef:      target.ModelRef,
			DisplayName:   target.DisplayName,
			Dialect:       targetDialect(provider, target),
			BaseURL:       provider.BaseURL,
			Weight:        weight,
			RPM:           target.RPM,
			Tier:          target.Tier,
			Cost:          target.Cost,
			KeyID:         provider.KeyID,
			APIKeyEnv:     provider.APIKeyEnv,
			KeyConfigured: provider.APIKey != "",
		})
	}
	return out
}

func exportScriptOutput(val goja.Value) (scriptOutput, error) {
	exported := val.Export()
	raw, err := json.Marshal(exported)
	if err != nil {
		return scriptOutput{}, fmt.Errorf("route(ctx) returned invalid decision: %w", err)
	}
	fields := map[string]json.RawMessage{}
	if err := json.Unmarshal(raw, &fields); err != nil {
		return scriptOutput{}, fmt.Errorf("route(ctx) returned invalid decision: %w", err)
	}
	var out scriptOutput
	if rawTargetIndex, ok := fields["targetIndex"]; ok {
		if err := json.Unmarshal(rawTargetIndex, &out.TargetIndex); err != nil {
			return scriptOutput{}, fmt.Errorf("route(ctx) targetIndex must be a number")
		}
		out.HasTargetIndex = true
	}
	if rawTarget, ok := fields["target"]; ok {
		if err := json.Unmarshal(rawTarget, &out.Target); err != nil {
			return scriptOutput{}, fmt.Errorf("route(ctx) target is invalid")
		}
	}
	if rawFallbackIndexes, ok := fields["fallbackIndexes"]; ok {
		if err := json.Unmarshal(rawFallbackIndexes, &out.FallbackIndexes); err != nil {
			return scriptOutput{}, fmt.Errorf("route(ctx) fallbackIndexes must be numbers")
		}
	}
	if rawFallbacks, ok := fields["fallbacks"]; ok {
		if err := json.Unmarshal(rawFallbacks, &out.Fallbacks); err != nil {
			return scriptOutput{}, fmt.Errorf("route(ctx) fallbacks are invalid")
		}
	}
	if rawClassLabel, ok := fields["classLabel"]; ok {
		if err := json.Unmarshal(rawClassLabel, &out.ClassLabel); err != nil {
			return scriptOutput{}, fmt.Errorf("route(ctx) classLabel must be a string")
		}
	}
	return out, nil
}

func jsonValue(v any) (any, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var out any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func resolveScriptTarget(out scriptOutput, targets []Target) (int, error) {
	if out.HasTargetIndex {
		return validateTargetIndex(out.TargetIndex, targets)
	}
	if out.Target != nil {
		return targetSelectorIndex(out.Target, targets)
	}
	return 0, fmt.Errorf("script route(ctx) must return targetIndex or target")
}

func resolveScriptFallbacks(out scriptOutput, primary int, targets []Target) ([]Target, error) {
	seen := map[int]bool{primary: true}
	indexes := []int{}
	for _, idx := range out.FallbackIndexes {
		valid, err := validateTargetIndex(idx, targets)
		if err != nil {
			return nil, err
		}
		if !seen[valid] {
			indexes = append(indexes, valid)
			seen[valid] = true
		}
	}
	for _, selector := range out.Fallbacks {
		idx, err := targetSelectorIndex(selector, targets)
		if err != nil {
			return nil, err
		}
		if !seen[idx] {
			indexes = append(indexes, idx)
			seen[idx] = true
		}
	}
	for i := range targets {
		if !seen[i] {
			indexes = append(indexes, i)
		}
	}
	fallbacks := make([]Target, 0, len(indexes))
	for _, idx := range indexes {
		fallbacks = append(fallbacks, targets[idx])
	}
	return fallbacks, nil
}

func validateTargetIndex(idx int, targets []Target) (int, error) {
	if idx < 0 || idx >= len(targets) {
		return 0, fmt.Errorf("script selected target index %d outside configured targets", idx)
	}
	return idx, nil
}

func targetSelectorIndex(selector any, targets []Target) (int, error) {
	switch v := selector.(type) {
	case int:
		return validateTargetIndex(v, targets)
	case int64:
		return validateTargetIndex(int(v), targets)
	case float64:
		return validateTargetIndex(int(v), targets)
	case string:
		return findTargetByString(v, targets)
	case map[string]any:
		return findTargetByMap(v, targets)
	default:
		raw, _ := json.Marshal(selector)
		m := map[string]any{}
		if err := json.Unmarshal(raw, &m); err == nil && len(m) > 0 {
			return findTargetByMap(m, targets)
		}
		return 0, fmt.Errorf("unsupported script target selector %T", selector)
	}
}

func findTargetByString(selector string, targets []Target) (int, error) {
	for i, target := range targets {
		if selector == target.Provider || selector == target.Provider+":"+target.Model {
			return i, nil
		}
	}
	return 0, fmt.Errorf("script selected unknown target %q", selector)
}

func findTargetByMap(selector map[string]any, targets []Target) (int, error) {
	provider, _ := selector["provider"].(string)
	model, _ := selector["model"].(string)
	modelRef, _ := selector["modelRef"].(string)
	if modelRef == "" {
		modelRef, _ = selector["model_ref"].(string)
	}
	for i, target := range targets {
		if provider != "" && provider != target.Provider {
			continue
		}
		if model != "" && model != target.Model {
			continue
		}
		if modelRef != "" && modelRef != target.ModelRef {
			continue
		}
		if provider != "" || model != "" || modelRef != "" {
			return i, nil
		}
	}
	raw, _ := json.Marshal(selector)
	return 0, fmt.Errorf("script selected target outside configured list: %s", strings.TrimSpace(string(raw)))
}
