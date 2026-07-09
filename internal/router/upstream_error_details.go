package router

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

const upstreamErrorDetailMaxValueBytes = 512

var upstreamErrorDetailFieldAllowlist = map[string]bool{
	"code":                true,
	"error":               true,
	"error_code":          true,
	"error_type":          true,
	"message":             true,
	"param":               true,
	"provider_request_id": true,
	"request_id":          true,
	"status":              true,
	"type":                true,
}

var upstreamErrorDetailBlockedFields = map[string]bool{
	"authorization":    true,
	"body":             true,
	"content":          true,
	"input":            true,
	"messages":         true,
	"prompt":           true,
	"provider_key":     true,
	"provider_api_key": true,
	"raw_body":         true,
	"request":          true,
	"response":         true,
	"response_body":    true,
	"secret":           true,
	"token":            true,
	"token_hash":       true,
	"tool":             true,
	"tools":            true,
}

var upstreamErrorDetailSafeIdentifierRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:/\[\]-]{0,127}$`)

func (s *Service) extractUpstreamErrorDetails(raw []byte, status int, class string, attemptIndex int, ts string) []upstreamErrorDetailLogRecord {
	if s == nil || s.cfg == nil || !s.diagnosticsEnabled() || !s.storeSanitizedUpstreamErrors() || len(raw) == 0 {
		return nil
	}
	maxBytes := s.diagnosticMaxErrorBytes()
	values := extractAllowedUpstreamErrorFields(raw, maxBytes)
	if len(values) == 0 {
		values = append(values, upstreamErrorDetailValue{
			Name:      "body_summary",
			Value:     "non-json upstream error body redacted",
			Source:    "text",
			Truncated: len(raw) > maxBytes && maxBytes > 0,
		})
	}
	sort.SliceStable(values, func(i, j int) bool {
		return values[i].Name < values[j].Name
	})
	out := make([]upstreamErrorDetailLogRecord, 0, len(values))
	for _, value := range values {
		out = append(out, upstreamErrorDetailLogRecord{
			Seq:          len(out) + 1,
			TS:           ts,
			AttemptIndex: attemptIndex,
			StatusCode:   status,
			ErrorClass:   class,
			FieldName:    value.Name,
			FieldValue:   value.Value,
			Source:       value.Source,
			Truncated:    value.Truncated,
		})
	}
	return out
}

type upstreamErrorDetailValue struct {
	Name      string
	Value     string
	Source    string
	Truncated bool
}

func extractAllowedUpstreamErrorFields(raw []byte, maxBytes int) []upstreamErrorDetailValue {
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil
	}
	fields := map[string]upstreamErrorDetailValue{}
	collectUpstreamErrorFields(decoded, "", fields, maxBytes)
	out := make([]upstreamErrorDetailValue, 0, len(fields))
	for _, value := range fields {
		out = append(out, value)
	}
	return out
}

func collectUpstreamErrorFields(value any, path string, fields map[string]upstreamErrorDetailValue, maxBytes int) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			name := normalizeUpstreamErrorFieldName(key)
			if name == "" || upstreamErrorDetailBlockedFields[name] {
				continue
			}
			childPath := name
			if path != "" {
				childPath = path + "." + name
			}
			if upstreamErrorDetailFieldAllowlist[name] {
				if scalar, ok := upstreamErrorDetailScalar(child); ok {
					sanitized, truncated := sanitizeUpstreamErrorDetailValue(name, scalar, maxBytes)
					if sanitized != "" {
						candidate := upstreamErrorDetailValue{Name: name, Value: sanitized, Source: childPath, Truncated: truncated}
						if existing, ok := fields[name]; !ok || upstreamErrorDetailSourcePriority(candidate.Source) > upstreamErrorDetailSourcePriority(existing.Source) {
							fields[name] = candidate
						}
					}
					continue
				}
			}
			collectUpstreamErrorFields(child, childPath, fields, maxBytes)
		}
	case []any:
		for i, child := range typed {
			collectUpstreamErrorFields(child, fmt.Sprintf("%s.%d", path, i), fields, maxBytes)
		}
	}
}

func normalizeUpstreamErrorFieldName(name string) string {
	name = strings.TrimSpace(strings.ToLower(name))
	name = strings.NewReplacer("-", "_", " ", "_").Replace(name)
	return name
}

func upstreamErrorDetailSourcePriority(source string) int {
	switch {
	case strings.HasPrefix(source, "error."):
		return 3
	case strings.Contains(source, ".error."):
		return 2
	default:
		return 1
	}
}

func upstreamErrorDetailScalar(value any) (string, bool) {
	switch typed := value.(type) {
	case string:
		return typed, true
	case float64, bool:
		return fmt.Sprint(typed), true
	default:
		return "", false
	}
}

func sanitizeUpstreamErrorDetailValue(fieldName, value string, maxBytes int) (string, bool) {
	normalizedField := normalizeUpstreamErrorFieldName(fieldName)
	value = normalizeDiagnosticText(value)
	if value == "" {
		return "", false
	}
	if normalizedField == "message" || normalizedField == "error" {
		return safeUpstreamProviderMessageCategory(value), false
	}
	value = redactDiagnosticSecrets(value)
	value = redactDiagnosticJSONLikeFields(value)
	if upstreamErrorDetailBlockedFields[normalizedField] {
		return "[REDACTED]", false
	}
	if diagnosticContentFieldRe.MatchString(value) {
		value = diagnosticContentFieldRe.ReplaceAllString(value, "$1=[REDACTED]")
	}
	value = redactDiagnosticSecrets(value)
	if !upstreamErrorDetailSafeIdentifierRe.MatchString(value) || strings.Contains(value, "[REDACTED]") {
		return "[REDACTED]", false
	}
	limit := upstreamErrorDetailMaxValueBytes
	if maxBytes > 0 && maxBytes < limit {
		limit = maxBytes
	}
	if limit > 0 && len(value) > limit {
		return value[:limit], true
	}
	return value, false
}

func safeUpstreamProviderMessageCategory(value string) string {
	lower := strings.ToLower(value)
	switch {
	case strings.Contains(lower, "context") || strings.Contains(lower, "token limit") || strings.Contains(lower, "maximum context"):
		return "provider_message:context_limit"
	case strings.Contains(lower, "tool") && (strings.Contains(lower, "schema") || strings.Contains(lower, "invalid")):
		return "provider_message:tool_schema_rejected"
	case strings.Contains(lower, "unsupported") || strings.Contains(lower, "unknown field") || strings.Contains(lower, "unrecognized"):
		return "provider_message:unsupported_field"
	case strings.Contains(lower, "invalid") || strings.Contains(lower, "bad request"):
		return "provider_message:invalid_request"
	case strings.Contains(lower, "rate limit") || strings.Contains(lower, "too many requests"):
		return "provider_message:rate_limit"
	case strings.Contains(lower, "quota") || strings.Contains(lower, "balance") || strings.Contains(lower, "billing") || strings.Contains(lower, "credit"):
		return "provider_message:quota_or_billing"
	case strings.Contains(lower, "auth") || strings.Contains(lower, "api key") || strings.Contains(lower, "permission"):
		return "provider_message:auth"
	case strings.Contains(lower, "model") && (strings.Contains(lower, "not found") || strings.Contains(lower, "access")):
		return "provider_message:model_access"
	case strings.Contains(lower, "timeout") || strings.Contains(lower, "timed out"):
		return "provider_message:timeout"
	case strings.Contains(lower, "temporary") || strings.Contains(lower, "unavailable") || strings.Contains(lower, "outage"):
		return "provider_message:temporary_failure"
	default:
		return "provider_message:redacted"
	}
}
