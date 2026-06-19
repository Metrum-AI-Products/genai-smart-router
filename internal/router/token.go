package router

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
	"time"
)

const tokenPrefixName = "rtr_metrum"

type TokenGenerateOptions struct {
	User        string
	Project     string
	Environment string
	KeySlug     string
	Allow       []string
	Reader      io.Reader
	Now         time.Time
}

type GeneratedToken struct {
	Token       string       `json:"token"`
	TokenID     string       `json:"token_id"`
	TokenSHA256 string       `json:"token_sha256"`
	Caller      CallerConfig `json:"caller"`
}

func GenerateCallerToken(opts TokenGenerateOptions) (GeneratedToken, error) {
	user := slugify(opts.User)
	project := slugify(opts.Project)
	environment := slugify(opts.Environment)
	if user == "" {
		return GeneratedToken{}, fmt.Errorf("user is required")
	}
	if project == "" {
		return GeneratedToken{}, fmt.Errorf("project is required")
	}
	if environment == "" {
		environment = "dev"
	}
	keySlug := slugify(opts.KeySlug)
	if keySlug == "" {
		now := opts.Now
		if now.IsZero() {
			now = time.Now().UTC()
		}
		keySlug = "k" + now.Format("20060102")
	}
	reader := opts.Reader
	if reader == nil {
		reader = rand.Reader
	}
	secretRaw := make([]byte, 32)
	if _, err := io.ReadFull(reader, secretRaw); err != nil {
		return GeneratedToken{}, fmt.Errorf("generate token secret: %w", err)
	}
	secret := base64.RawURLEncoding.EncodeToString(secretRaw)
	tokenID := strings.Join([]string{tokenPrefixName, user, project, environment, keySlug}, "_")
	token := tokenID + "_" + secret
	sum := sha256.Sum256([]byte(token))
	allow := append([]string(nil), opts.Allow...)
	if len(allow) == 0 {
		return GeneratedToken{}, fmt.Errorf("at least one allowed model group is required")
	}
	callerID := strings.Join([]string{user, project, environment}, "-")
	caller := CallerConfig{
		ID:          callerID,
		User:        user,
		Project:     project,
		Environment: environment,
		TokenSHA256: hex.EncodeToString(sum[:]),
		TokenID:     tokenID,
		Allow:       allow,
		Rate:        RateConfig{RPM: 120, TPM: 200000, Concurrent: 8},
		Quota:       QuotaConfig{Day: BudgetConfig{Requests: 5000, Tokens: 20000000}, Month: BudgetConfig{Tokens: 400000000}, SoftPct: 80},
		Key:         KeyConfig{LifetimeTokens: 2000000000, SoftPct: 90, OnExhaust: "disable"},
	}
	return GeneratedToken{Token: token, TokenID: tokenID, TokenSHA256: caller.TokenSHA256, Caller: caller}, nil
}

func publicTokenID(v string) string {
	v = strings.TrimSpace(v)
	if !strings.HasPrefix(v, tokenPrefixName+"_") {
		return v
	}
	parts := strings.Split(v, "_")
	if len(parts) <= 6 {
		return v
	}
	return strings.Join(parts[:6], "_")
}

func slugify(in string) string {
	in = strings.ToLower(strings.TrimSpace(in))
	var b strings.Builder
	lastSep := false
	for _, r := range in {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastSep = false
		case r == '-' || r == '_' || r == '.' || r == ' ' || r == '/':
			if b.Len() > 0 && !lastSep {
				b.WriteByte('-')
				lastSep = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	return out
}

func callerUser(c CallerConfig) string {
	if v := slugify(c.User); v != "" {
		return v
	}
	if v := slugify(c.ID); v != "" {
		return v
	}
	return "unknown"
}

func callerProject(c CallerConfig) string {
	if v := slugify(c.Project); v != "" {
		return v
	}
	return "unknown"
}

func callerEnvironment(c CallerConfig) string {
	if v := slugify(c.Environment); v != "" {
		return v
	}
	return "unknown"
}
