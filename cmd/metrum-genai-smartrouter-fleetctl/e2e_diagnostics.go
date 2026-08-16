package main

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

var (
	protectedRefPattern = regexp.MustCompile(`(?i)aws-(?:ssm|secretsmanager):///\S+`)
	credentialCanaries  = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\bsk-[A-Za-z0-9_-]{8,}\b`),
		regexp.MustCompile(`(?i)\brtr_metrum_[A-Za-z0-9_]+\b`),
		regexp.MustCompile(`(?i)\bBearer\s+[A-Za-z0-9._\-]+\b`),
		regexp.MustCompile(`(?i)\bAKIA[0-9A-Z]{16}\b`),
	}
	configBlobMarkers = []string{
		"config.yaml:",
		"env.json:",
		"ROUTER_USAGE_DB_DSN=",
		"OPENROUTER_API_KEY",
		"secret-like-input-must-not-be-echoed",
	}
	safeStatusKeys = map[string]struct{}{
		"job_id":         {},
		"state":          {},
		"error_class":    {},
		"next_action":    {},
		"observed_state": {},
		"retryable":      {},
		"customer_id":    {},
		"environment":    {},
		"namespace":      {},
		"hostname":       {},
	}
)

// sanitizePackageE2ECommandDiagnostics redacts protected refs, credential-shaped
// tokens, and config/env blobs while keeping short CLI error text and allowlisted
// status JSON scalars for actionable packaged E2E failures.
func sanitizePackageE2ECommandDiagnostics(output []byte) string {
	raw := strings.TrimSpace(string(output))
	if raw == "" {
		return ""
	}
	var parts []string
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if sanitized := sanitizePackageE2ELine(line); sanitized != "" {
			parts = append(parts, sanitized)
		}
	}
	return strings.Join(parts, "\n")
}

func sanitizePackageE2ELine(line string) string {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return ""
	}
	for _, marker := range configBlobMarkers {
		if strings.Contains(trimmed, marker) {
			return "[redacted-config-or-secret-marker]"
		}
	}
	if strings.HasPrefix(trimmed, "{") && strings.Contains(trimmed, "\"") {
		if safe := extractSafeStatusScalars(trimmed); safe != "" {
			return safe
		}
	}
	out := protectedRefPattern.ReplaceAllString(trimmed, "[redacted-protected-ref]")
	for _, re := range credentialCanaries {
		out = re.ReplaceAllString(out, "[redacted-credential]")
	}
	return strings.TrimSpace(out)
}

func extractSafeStatusScalars(line string) string {
	var payload map[string]any
	if err := json.Unmarshal([]byte(line), &payload); err != nil {
		return ""
	}
	safe := make(map[string]any, len(safeStatusKeys))
	for key, value := range payload {
		if _, ok := safeStatusKeys[key]; !ok {
			continue
		}
		switch value.(type) {
		case string, float64, bool, nil:
			safe[key] = value
		}
	}
	if len(safe) == 0 {
		return ""
	}
	encoded, err := json.Marshal(safe)
	if err != nil {
		return ""
	}
	return string(encoded)
}

// formatPackageE2ECommandFailure builds a test failure message that keeps exit
// status and sanitized CLI diagnostics without dumping protected inputs.
func formatPackageE2ECommandFailure(command string, err error, output []byte) string {
	var b strings.Builder
	fmt.Fprintf(&b, "release package %s failed", command)
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			fmt.Fprintf(&b, ": exit status %d", exitErr.ExitCode())
		} else {
			fmt.Fprintf(&b, ": %v", err)
		}
	}
	if sanitized := sanitizePackageE2ECommandDiagnostics(output); sanitized != "" {
		fmt.Fprintf(&b, "\n%s", sanitized)
	}
	return b.String()
}
