package router

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/dop251/goja"
	"github.com/evanw/esbuild/pkg/api"
)

type scriptStrategy struct {
	path       string
	program    *goja.Program
	httpConfig ScriptHTTPConfig
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

func loadScriptStrategy(baseDir, scriptPath string, httpConfig ScriptHTTPConfig) (*scriptStrategy, error) {
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
	result := api.Build(api.BuildOptions{
		EntryPoints:       []string{resolved},
		Bundle:            true,
		Write:             false,
		Format:            api.FormatIIFE,
		GlobalName:        "routerScript",
		Target:            api.ES2018,
		Sourcemap:         api.SourceMapNone,
		LegalComments:     api.LegalCommentsNone,
		Platform:          api.PlatformNeutral,
		TreeShaking:       api.TreeShakingTrue,
		KeepNames:         true,
		MinifyWhitespace:  false,
		MinifyIdentifiers: false,
		MinifySyntax:      false,
		AbsWorkingDir:     filepath.Dir(resolved),
		SourceRoot:        filepath.Dir(resolved),
		LogLevel:          api.LogLevelSilent,
	})
	if len(result.Errors) > 0 {
		return nil, fmt.Errorf("typescript transform: %s", result.Errors[0].Text)
	}
	if len(result.OutputFiles) == 0 {
		return nil, fmt.Errorf("typescript transform produced no output")
	}
	program, err := goja.Compile(resolved, string(result.OutputFiles[0].Contents), false)
	if err != nil {
		return nil, err
	}
	return &scriptStrategy{path: resolved, program: program, httpConfig: httpConfig}, nil
}

func (s *scriptStrategy) Pick(group string, req *IRRequest, targets []Target, providers map[string]ProviderConfig, caller *callerRuntime, tokenID string) (decision, error) {
	vm := goja.New()
	timer := time.AfterFunc(s.timeout(), func() {
		vm.Interrupt("script routing timed out")
	})
	defer timer.Stop()
	if err := s.installRouterAPI(vm); err != nil {
		return decision{}, err
	}
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

func (s *scriptStrategy) timeout() time.Duration {
	if !s.httpConfig.Enabled || s.httpConfig.TimeoutMS <= 0 {
		return 50 * time.Millisecond
	}
	timeout := time.Duration(s.httpConfig.TimeoutMS)*time.Millisecond + 50*time.Millisecond
	if timeout > 5050*time.Millisecond {
		return 5050 * time.Millisecond
	}
	return timeout
}

func (s *scriptStrategy) installRouterAPI(vm *goja.Runtime) error {
	routerAPI := vm.NewObject()
	if err := routerAPI.Set("fetchJSON", func(call goja.FunctionCall) goja.Value {
		if !s.httpConfig.Enabled {
			panic(vm.NewTypeError("router.fetchJSON is disabled for this model group"))
		}
		rawURL := call.Argument(0).String()
		options := map[string]any{}
		if len(call.Arguments) > 1 && !goja.IsUndefined(call.Argument(1)) && !goja.IsNull(call.Argument(1)) {
			if err := vm.ExportTo(call.Argument(1), &options); err != nil {
				panic(vm.NewTypeError("router.fetchJSON options must be an object"))
			}
		}
		resp, err := s.fetchJSON(rawURL, options)
		if err != nil {
			panic(vm.NewTypeError("router.fetchJSON: %s", err.Error()))
		}
		return vm.ToValue(resp)
	}); err != nil {
		return err
	}
	return vm.Set("router", routerAPI)
}

func (s *scriptStrategy) fetchJSON(rawURL string, options map[string]any) (map[string]any, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL")
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return nil, fmt.Errorf("URL scheme must be http or https")
	}
	if !scriptHostAllowed(u.Hostname(), s.httpConfig.AllowHosts) {
		return nil, fmt.Errorf("host %s is not allowed", u.Hostname())
	}
	method := "GET"
	if rawMethod, ok := options["method"].(string); ok && rawMethod != "" {
		method = strings.ToUpper(rawMethod)
	}
	if method != "GET" && method != "POST" {
		return nil, fmt.Errorf("method %s is not allowed", method)
	}
	var body io.Reader
	if rawBody, ok := options["body"]; ok && rawBody != nil {
		b, err := json.Marshal(rawBody)
		if err != nil {
			return nil, fmt.Errorf("body must be JSON-serializable")
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, u.String(), body)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if rawHeaders, ok := options["headers"].(map[string]any); ok {
		for name, value := range rawHeaders {
			if !scriptHeaderAllowed(name) {
				return nil, fmt.Errorf("header %s is not allowed", name)
			}
			req.Header.Set(name, fmt.Sprint(value))
		}
	}
	for name, value := range s.httpConfig.Headers {
		if !scriptConfigHeaderAllowed(name) {
			return nil, fmt.Errorf("configured header %s is not allowed", name)
		}
		req.Header.Set(name, value)
	}
	timeout := time.Duration(s.httpConfig.TimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = 200 * time.Millisecond
	}
	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	limit := s.httpConfig.MaxResponseBytes
	if limit <= 0 {
		limit = 64 << 10
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > limit {
		return nil, fmt.Errorf("response exceeds max_response_bytes")
	}
	var data any
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &data); err != nil {
			return nil, fmt.Errorf("response is not JSON")
		}
	}
	return map[string]any{
		"ok":     resp.StatusCode >= 200 && resp.StatusCode < 300,
		"status": resp.StatusCode,
		"body":   data,
	}, nil
}

func scriptHostAllowed(host string, allowHosts []string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	for _, allowed := range allowHosts {
		allowed = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(allowed), "."))
		if allowed != "" && host == allowed {
			return true
		}
	}
	return false
}

func scriptHeaderAllowed(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	return name == "content-type" || name == "accept" || strings.HasPrefix(name, "x-")
}

func scriptConfigHeaderAllowed(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	return scriptHeaderAllowed(name) || name == "authorization"
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
