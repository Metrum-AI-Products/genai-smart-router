package router

import (
	"encoding/json"
	"regexp"
	"strings"
)

var (
	diagnosticBearerRe        = regexp.MustCompile(`(?i)\bbearer\s+[A-Za-z0-9._~+/=-]{8,}`)
	diagnosticNamedSecretRe   = regexp.MustCompile(`(?i)\b(api[_-]?key|authorization|provider[_-]?key|secret|token|token[_-]?hash|x-api-key)\b\s*[:=]\s*["']?[^"',\s}]{6,}`)
	diagnosticKnownSecretRe   = regexp.MustCompile(`\b(?:sk-[A-Za-z0-9_-]{8,}|rtr_[A-Za-z0-9_-]{8,}|gh[pousr]_[A-Za-z0-9_]{8,})\b`)
	diagnosticSHA256PrefixRe  = regexp.MustCompile(`(?i)\bsha256:[a-f0-9]{12,64}\b`)
	diagnosticHexHashRe       = regexp.MustCompile(`(?i)\b[a-f0-9]{64}\b`)
	diagnosticContentFieldRe  = regexp.MustCompile(`(?i)\b(prompt|messages?|input|content|request|raw[_-]?body|response[_-]?body|body)\b\s*[:=]\s*[^;]+`)
	diagnosticJSONLikeFieldRe = regexp.MustCompile(`(?i)("?(?:api[_-]?key|authorization|provider[_-]?(?:api[_-]?)?key|secret|token|token[_-]?hash|x-api-key|prompt|messages?|input|content|request|raw[_-]?body|response[_-]?body|body)"?\s*:\s*)(?:"(?:\\.|[^"\\])*"?|[^,}\]]*)`)
)

func (s *Service) sanitizeDiagnosticError(text string) string {
	storeUpstreamSnippet := false
	if s != nil && s.cfg != nil {
		storeUpstreamSnippet = s.cfg.Server.Diagnostics.StoreSanitizedUpstreamError
	}
	return sanitizeDiagnosticText(text, storeUpstreamSnippet, s.diagnosticMaxErrorBytes())
}

func (s *Service) sanitizeDiagnosticTraceMessage(event, text string) string {
	if event == "routing_decision" {
		return sanitizeInternalDiagnosticText(text, s.diagnosticMaxErrorBytes())
	}
	return s.sanitizeDiagnosticError(text)
}

func (s *Service) sanitizeDiagnosticRecord(rec *logRecord) {
	if s == nil || rec == nil {
		return
	}
	rec.ErrorMessage = s.sanitizeDiagnosticError(rec.ErrorMessage)
	for i := range rec.AttemptsDetail {
		rec.AttemptsDetail[i].ErrorMessage = s.sanitizeDiagnosticError(rec.AttemptsDetail[i].ErrorMessage)
	}
	for i := range rec.TraceEvents {
		rec.TraceEvents[i].Message = s.sanitizeDiagnosticTraceMessage(rec.TraceEvents[i].Event, rec.TraceEvents[i].Message)
	}
}

func sanitizePersistedDiagnosticText(text string) string {
	return sanitizeDiagnosticText(text, true, 2048)
}

func sanitizePersistedTraceMessage(event, text string) string {
	if event == "routing_decision" {
		return sanitizeInternalDiagnosticText(text, 2048)
	}
	return sanitizePersistedDiagnosticText(text)
}

func sanitizeInternalDiagnosticText(text string, maxBytes int) string {
	text = normalizeDiagnosticText(text)
	if text == "" {
		return ""
	}
	text = redactDiagnosticSecrets(text)
	return truncateDiagnosticText(text, maxBytes)
}

func sanitizeDiagnosticText(text string, storeUpstreamSnippet bool, maxBytes int) string {
	text = normalizeDiagnosticText(text)
	if text == "" {
		return ""
	}
	text = redactDiagnosticSecrets(text)
	if !storeUpstreamSnippet {
		if i := strings.Index(text, ":"); i > 0 {
			text = strings.TrimSpace(text[:i])
		}
		return truncateDiagnosticText(text, maxBytes)
	}
	text = redactDiagnosticJSONBody(text)
	text = redactDiagnosticJSONLikeFields(text)
	text = diagnosticContentFieldRe.ReplaceAllString(text, "$1=[REDACTED]")
	text = redactDiagnosticSecrets(text)
	return truncateDiagnosticText(text, maxBytes)
}

func normalizeDiagnosticText(text string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
}

func redactDiagnosticJSONBody(text string) string {
	if json.Valid([]byte(text)) {
		return "upstream error body redacted"
	}
	idx := strings.Index(text, ":")
	if idx <= 0 || idx == len(text)-1 {
		return text
	}
	prefix := strings.TrimSpace(text[:idx])
	body := strings.TrimSpace(text[idx+1:])
	if body == "" {
		return text
	}
	if (strings.HasPrefix(body, "{") || strings.HasPrefix(body, "[")) && json.Valid([]byte(body)) {
		return prefix + ": upstream error body redacted"
	}
	return text
}

func redactDiagnosticJSONLikeFields(text string) string {
	return diagnosticJSONLikeFieldRe.ReplaceAllString(text, "${1}[REDACTED]")
}

func redactDiagnosticSecrets(text string) string {
	text = diagnosticBearerRe.ReplaceAllString(text, "Bearer [REDACTED]")
	text = diagnosticNamedSecretRe.ReplaceAllStringFunc(text, func(match string) string {
		for _, sep := range []string{":", "="} {
			if idx := strings.Index(match, sep); idx >= 0 {
				return strings.TrimSpace(match[:idx]) + sep + "[REDACTED]"
			}
		}
		return "[REDACTED]"
	})
	text = diagnosticKnownSecretRe.ReplaceAllString(text, "[REDACTED_SECRET]")
	text = diagnosticSHA256PrefixRe.ReplaceAllString(text, "sha256:[REDACTED]")
	text = diagnosticHexHashRe.ReplaceAllString(text, "[REDACTED_HASH]")
	return text
}

func truncateDiagnosticText(text string, maxBytes int) string {
	if maxBytes > 0 && len(text) > maxBytes {
		return text[:maxBytes]
	}
	return text
}
