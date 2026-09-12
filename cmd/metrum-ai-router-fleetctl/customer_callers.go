// Copyright 2026 Metrum AI
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type callerListRow struct {
	ID                string         `json:"id"`
	TokenID           string         `json:"token_id,omitempty"`
	Status            string         `json:"status,omitempty"`
	Environment       string         `json:"environment,omitempty"`
	Allow             []string       `json:"allow,omitempty"`
	MetricsAdmin      bool           `json:"metrics_admin"`
	ContentAdmin      bool           `json:"content_admin"`
	OwnerUser         string         `json:"owner_user"`
	UserName          string         `json:"user_name,omitempty"`
	UserStatus        string         `json:"user_status,omitempty"`
	UserType          string         `json:"user_type,omitempty"`
	Project           string         `json:"project"`
	ProjectName       string         `json:"project_name,omitempty"`
	ProjectStatus     string         `json:"project_status,omitempty"`
	MembershipRole    string         `json:"membership_role,omitempty"`
	MembershipStatus  string         `json:"membership_status,omitempty"`
	Rate              map[string]any `json:"rate,omitempty"`
	Quota             map[string]any `json:"quota,omitempty"`
	KeyLifetimeTokens int64          `json:"key_lifetime_tokens,omitempty"`
	KeySoftPct        int            `json:"key_soft_pct,omitempty"`
	KeyOnExhaust      string         `json:"key_on_exhaust,omitempty"`
}

var callerRevokeStatuses = map[string]bool{
	"disabled":  true,
	"rotated":   true,
	"expired":   true,
	"suspended": true,
}

func parseConfigRoot(configYAML string) (map[string]any, error) {
	var cfg any
	if err := yaml.Unmarshal([]byte(configYAML), &cfg); err != nil {
		return nil, fmt.Errorf("parse config.yaml: %w", err)
	}
	root, ok := asStringMap(cfg)
	if !ok {
		return nil, fmt.Errorf("config.yaml must be a mapping")
	}
	return root, nil
}

func listCallersFromConfig(configYAML string) ([]callerListRow, error) {
	root, err := parseConfigRoot(configYAML)
	if err != nil {
		return nil, err
	}
	users := accountIndex(root["users"], "id")
	projects := accountIndex(root["projects"], "id")
	memberships := membershipIndex(root["project_memberships"])
	callers, ok := asAnySlice(root["callers"])
	if !ok {
		return []callerListRow{}, nil
	}
	out := make([]callerListRow, 0, len(callers))
	for _, raw := range callers {
		m, ok := asStringMap(raw)
		if !ok {
			continue
		}
		id := stringField(m, "id")
		if id == "" {
			continue
		}
		owner := stringField(m, "owner_user")
		if owner == "" {
			owner = stringField(m, "user")
		}
		project := stringField(m, "project")
		row := callerListRow{
			ID:           id,
			TokenID:      stringField(m, "token_id"),
			Status:       stringField(m, "status"),
			Environment:  stringField(m, "environment"),
			Allow:        stringSliceField(m["allow"]),
			MetricsAdmin: boolField(m, "metrics_admin"),
			ContentAdmin: boolField(m, "content_admin"),
			OwnerUser:    owner,
			Project:      project,
		}
		if u, ok := users[owner]; ok {
			row.UserName = stringField(u, "name")
			row.UserStatus = stringField(u, "status")
			row.UserType = stringField(u, "type")
		}
		if p, ok := projects[project]; ok {
			row.ProjectName = stringField(p, "name")
			row.ProjectStatus = stringField(p, "status")
		}
		if mem, ok := memberships[owner+"\x00"+project]; ok {
			row.MembershipRole = stringField(mem, "role")
			row.MembershipStatus = stringField(mem, "status")
		}
		if rate, ok := asStringMap(m["rate"]); ok {
			row.Rate = map[string]any{
				"rpm":        intField(rate, "rpm"),
				"tpm":        intField(rate, "tpm"),
				"concurrent": intField(rate, "concurrent"),
			}
		}
		if quota, ok := asStringMap(m["quota"]); ok {
			row.Quota = map[string]any{
				"day":      budgetMap(quota["day"]),
				"month":    budgetMap(quota["month"]),
				"soft_pct": intField(quota, "soft_pct"),
			}
		}
		if key, ok := asStringMap(m["key"]); ok {
			row.KeyLifetimeTokens = int64Field(key, "lifetime_tokens")
			row.KeySoftPct = intField(key, "soft_pct")
			row.KeyOnExhaust = stringField(key, "on_exhaust")
		}
		out = append(out, row)
	}
	return out, nil
}

func callersMatching(configYAML string, callerIDs, ownerUsers []string) ([]map[string]any, error) {
	root, err := parseConfigRoot(configYAML)
	if err != nil {
		return nil, err
	}
	callers, ok := asAnySlice(root["callers"])
	if !ok {
		return nil, fmt.Errorf("config.yaml has no callers list")
	}
	wantIDs := setLower(callerIDs)
	wantUsers := setLower(ownerUsers)
	var matched []map[string]any
	seen := map[string]bool{}
	for _, raw := range callers {
		m, ok := asStringMap(raw)
		if !ok {
			continue
		}
		id := stringField(m, "id")
		if id == "" {
			continue
		}
		owner := stringField(m, "owner_user")
		if owner == "" {
			owner = stringField(m, "user")
		}
		idHit := len(wantIDs) > 0 && wantIDs[strings.ToLower(id)]
		userHit := len(wantUsers) > 0 && wantUsers[strings.ToLower(owner)]
		if !idHit && !userHit {
			continue
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		matched = append(matched, cloneStringMap(m))
	}
	return matched, nil
}

func accountIndex(v any, idKey string) map[string]map[string]any {
	out := map[string]map[string]any{}
	if sl, ok := asAnySlice(v); ok {
		for _, raw := range sl {
			m, ok := asStringMap(raw)
			if !ok {
				continue
			}
			id := stringField(m, idKey)
			if id == "" {
				continue
			}
			out[id] = m
		}
		return out
	}
	if m, ok := asStringMap(v); ok {
		for id, raw := range m {
			row, ok := asStringMap(raw)
			if !ok {
				row = map[string]any{}
			}
			if stringField(row, idKey) == "" {
				row[idKey] = id
			}
			out[id] = row
		}
	}
	return out
}

func membershipIndex(v any) map[string]map[string]any {
	out := map[string]map[string]any{}
	sl, ok := asAnySlice(v)
	if !ok {
		return out
	}
	for _, raw := range sl {
		m, ok := asStringMap(raw)
		if !ok {
			continue
		}
		user := stringField(m, "user_id")
		project := stringField(m, "project")
		if user == "" || project == "" {
			continue
		}
		out[user+"\x00"+project] = m
	}
	return out
}

func budgetMap(v any) map[string]any {
	m, ok := asStringMap(v)
	if !ok {
		return map[string]any{"requests": 0, "tokens": 0}
	}
	return map[string]any{
		"requests": int64Field(m, "requests"),
		"tokens":   int64Field(m, "tokens"),
	}
}

func stringField(m map[string]any, key string) string {
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	default:
		return strings.TrimSpace(fmt.Sprint(t))
	}
}

func boolField(m map[string]any, key string) bool {
	v, ok := m[key]
	if !ok {
		return false
	}
	b, ok := v.(bool)
	return ok && b
}

func intField(m map[string]any, key string) int {
	return int(int64Field(m, key))
}

func int64Field(m map[string]any, key string) int64 {
	v, ok := m[key]
	if !ok || v == nil {
		return 0
	}
	switch t := v.(type) {
	case int:
		return int64(t)
	case int64:
		return t
	case uint64:
		return int64(t)
	case float64:
		return int64(t)
	case string:
		n, _ := strconv.ParseInt(strings.TrimSpace(t), 10, 64)
		return n
	default:
		n, _ := strconv.ParseInt(fmt.Sprint(t), 10, 64)
		return n
	}
}

func stringSliceField(v any) []string {
	sl, ok := asAnySlice(v)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(sl))
	for _, item := range sl {
		s := strings.TrimSpace(fmt.Sprint(item))
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

func setLower(in []string) map[string]bool {
	out := map[string]bool{}
	for _, v := range in {
		v = strings.ToLower(strings.TrimSpace(v))
		if v != "" {
			out[v] = true
		}
	}
	return out
}

type repeatableStrings []string

func (s *repeatableStrings) String() string {
	return strings.Join(*s, ",")
}

func (s *repeatableStrings) Set(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return fmt.Errorf("value must not be empty")
	}
	*s = append(*s, value)
	return nil
}
