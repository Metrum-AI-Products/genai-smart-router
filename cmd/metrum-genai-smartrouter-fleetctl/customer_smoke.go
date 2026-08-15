package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type smokeOptions struct {
	TokenFile         string
	Model             string
	SkipChat          bool
	TimeoutSec        int
	ExpectModelsCount int // -1 means unset
}

func resolveSmokeToken(ws customerWorkspace, tokenFile string) (string, error) {
	candidates := []string{}
	if strings.TrimSpace(tokenFile) != "" {
		candidates = append(candidates, expandHome(tokenFile))
	}
	candidates = append(candidates,
		filepath.Join(ws.Home, "CALLER_TOKEN.txt"),
		filepath.Join(ws.Home, "CALLER_TOKEN_ADMIN.txt"),
	)
	for _, path := range candidates {
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		token := strings.TrimSpace(string(raw))
		if token != "" {
			return token, nil
		}
	}
	return "", fmt.Errorf("no caller token file found; pass --token-file or grant-caller first")
}

func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

func httpJSON(method, url, token string, body any, timeout time.Duration) (int, map[string]any, error) {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return 0, nil, err
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Accept", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return 0, map[string]any{"error": err.Error()}, nil
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var parsed map[string]any
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &parsed); err != nil {
			parsed = map[string]any{"_raw_len": len(raw)}
		}
	}
	if parsed == nil {
		parsed = map[string]any{}
	}
	return resp.StatusCode, parsed, nil
}

func chatSmokeSucceeded(code int, content string) bool {
	return code == http.StatusOK && strings.TrimSpace(content) == "OK"
}

func hasModelID(ids []string, want string) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}

func runCustomerSmoke(ws customerWorkspace, opts smokeOptions) error {
	state := ws.loadState()
	hostname, _ := state["hostname"].(string)
	if strings.TrimSpace(hostname) == "" {
		hostname = customerHostname(ws.CustomerID)
	}
	base := "https://" + hostname
	timeoutSec := opts.TimeoutSec
	if timeoutSec <= 0 {
		timeoutSec = 180
	}
	group := strings.TrimSpace(opts.Model)
	if group == "" && !opts.SkipChat {
		return fmt.Errorf("--model is required unless --skip-chat is set; use an allowed deployment-defined group from /v1/models")
	}
	lastErr := "smoke not attempted"
	deadline := time.Now().Add(time.Duration(timeoutSec) * time.Second)
	for time.Now().Before(deadline) {
		code, ready, _ := httpJSON(http.MethodGet, base+"/readyz", "", nil, 20*time.Second)
		ok, _ := ready["ok"].(bool)
		if code != 200 || !ok {
			lastErr = fmt.Sprintf("readyz http=%d", code)
			time.Sleep(5 * time.Second)
			continue
		}
		token, err := resolveSmokeToken(ws, opts.TokenFile)
		if err != nil {
			return err
		}
		code, models, _ := httpJSON(http.MethodGet, base+"/v1/models", token, nil, 30*time.Second)
		ids := modelIDs(models)
		if code != 200 || len(ids) == 0 {
			lastErr = fmt.Sprintf("models http=%d", code)
			time.Sleep(5 * time.Second)
			continue
		}
		fmt.Println(mustJSON(map[string]any{
			"readyz": map[string]any{"http": 200, "ok": true, "version": ready["version"]},
		}))
		sample := ids
		if len(sample) > 8 {
			sample = sample[:8]
		}
		fmt.Println(mustJSON(map[string]any{
			"models": map[string]any{"http": code, "count": len(ids), "sample": sample},
		}))
		if opts.ExpectModelsCount >= 0 && len(ids) != opts.ExpectModelsCount {
			lastErr = fmt.Sprintf("models count=%d want=%d", len(ids), opts.ExpectModelsCount)
			time.Sleep(5 * time.Second)
			continue
		}
		if opts.SkipChat {
			return nil
		}
		if !hasModelID(ids, group) {
			return fmt.Errorf("requested model group %q is not visible to the caller", group)
		}
		code, chat, _ := httpJSON(http.MethodPost, base+"/v1/chat/completions", token, map[string]any{
			"model": group,
			"messages": []map[string]string{
				{"role": "user", "content": "Reply with exactly: OK"},
			},
			"max_tokens": 64,
			"stream":     false,
		}, 90*time.Second)
		content := chatMessageContent(chat)
		fmt.Println(mustJSON(map[string]any{
			"chat": map[string]any{
				"http":        code,
				"model":       chat["model"],
				"group":       group,
				"content_len": len(content),
				"has_OK":      strings.Contains(content, "OK"),
			},
		}))
		if chatSmokeSucceeded(code, content) {
			return nil
		}
		lastErr = fmt.Sprintf("chat http=%d expected_exact_OK=%t", code, chatSmokeSucceeded(code, content))
		time.Sleep(5 * time.Second)
	}
	return fmt.Errorf("smoke failed after retries: %s", lastErr)
}

func modelIDs(models map[string]any) []string {
	data, _ := models["data"].([]any)
	ids := make([]string, 0, len(data))
	for _, row := range data {
		m, ok := row.(map[string]any)
		if !ok {
			continue
		}
		id, _ := m["id"].(string)
		if id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

func chatMessageContent(chat map[string]any) string {
	choices, _ := chat["choices"].([]any)
	if len(choices) == 0 {
		return ""
	}
	first, _ := choices[0].(map[string]any)
	msg, _ := first["message"].(map[string]any)
	content, _ := msg["content"].(string)
	return content
}
