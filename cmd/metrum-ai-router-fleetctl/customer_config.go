// Copyright 2026 Metrum AI
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

func deepMerge(base, patch any) any {
	baseMap, baseOK := asStringMap(base)
	patchMap, patchOK := asStringMap(patch)
	if baseOK && patchOK {
		out := make(map[string]any, len(baseMap)+len(patchMap))
		for k, v := range baseMap {
			out[k] = v
		}
		for key, value := range patchMap {
			if key == "callers" {
				existing, existingOK := asAnySlice(out["callers"])
				patchList, patchListOK := asAnySlice(value)
				if existingOK && patchListOK {
					merged, err := mergeCallers(existing, patchList)
					if err != nil {
						die("%v", err)
					}
					out[key] = merged
					continue
				}
			}
			if existing, ok := out[key]; ok {
				out[key] = deepMerge(existing, value)
			} else {
				out[key] = value
			}
		}
		return out
	}
	return patch
}

func mergeCallers(existing, patch []any) ([]any, error) {
	byID := map[string]map[string]any{}
	order := make([]string, 0, len(existing)+len(patch))
	for _, row := range existing {
		m, ok := asStringMap(row)
		if !ok {
			continue
		}
		cid, _ := m["id"].(string)
		if cid == "" {
			continue
		}
		byID[cid] = cloneStringMap(m)
		order = append(order, cid)
	}
	for _, row := range patch {
		m, ok := asStringMap(row)
		if !ok {
			continue
		}
		cid, _ := m["id"].(string)
		if cid == "" {
			return nil, fmt.Errorf("caller patch entries require id")
		}
		if cur, ok := byID[cid]; ok {
			merged := deepMerge(cur, m)
			byID[cid] = asStringMapMust(merged)
		} else {
			byID[cid] = cloneStringMap(m)
			order = append(order, cid)
		}
	}
	out := make([]any, 0, len(order))
	for _, cid := range order {
		out = append(out, byID[cid])
	}
	return out, nil
}

func applyConfigPatch(configYAML string, patch map[string]any) (string, error) {
	var cfg any
	if err := yaml.Unmarshal([]byte(configYAML), &cfg); err != nil {
		return "", fmt.Errorf("parse config.yaml: %w", err)
	}
	base, ok := asStringMap(cfg)
	if !ok {
		return "", fmt.Errorf("config.yaml must be a mapping")
	}
	merged := deepMerge(base, patch)
	out, err := yaml.Marshal(merged)
	if err != nil {
		return "", fmt.Errorf("encode patched config.yaml: %w", err)
	}
	return string(out), nil
}

func asStringMap(v any) (map[string]any, bool) {
	switch m := v.(type) {
	case map[string]any:
		return m, true
	case map[any]any:
		out := make(map[string]any, len(m))
		for k, val := range m {
			ks, ok := k.(string)
			if !ok {
				return nil, false
			}
			out[ks] = val
		}
		return out, true
	default:
		return nil, false
	}
}

func asStringMapMust(v any) map[string]any {
	m, ok := asStringMap(v)
	if !ok {
		die("expected mapping after merge")
	}
	return m
}

func asAnySlice(v any) ([]any, bool) {
	switch s := v.(type) {
	case []any:
		return s, true
	default:
		return nil, false
	}
}

func cloneStringMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// applyGrantCallerPatch merges a caller and, when the config already has an explicit
// account directory (users/projects/memberships), ensures matching directory rows exist.
// Explicit directories disable router synthetic accounts; a new owner/project without
// memberships fails config validation and CrashLoops the router after publish.
func applyGrantCallerPatch(configYAML string, callerRow map[string]any, ownerUser, project string) (string, error) {
	ownerUser = strings.TrimSpace(ownerUser)
	project = strings.TrimSpace(project)
	if ownerUser == "" || project == "" {
		return "", fmt.Errorf("grant-caller account directory patch requires owner-user and project")
	}
	if callerRow == nil {
		return "", fmt.Errorf("grant-caller requires caller row")
	}
	root, err := parseConfigRoot(configYAML)
	if err != nil {
		return "", err
	}
	usersIdx := accountIndex(root["users"], "id")
	projectsIdx := accountIndex(root["projects"], "id")
	membershipsIdx := membershipIndex(root["project_memberships"])
	hasExplicit := len(usersIdx) > 0 || len(projectsIdx) > 0 || len(membershipsIdx) > 0
	if hasExplicit {
		root["users"] = ensureAccountEntry(root["users"], ownerUser, map[string]any{
			"id":     ownerUser,
			"name":   ownerUser,
			"type":   "human",
			"status": "active",
		})
		root["projects"] = ensureAccountEntry(root["projects"], project, map[string]any{
			"id":     project,
			"name":   project,
			"status": "active",
		})
		root["project_memberships"] = ensureProjectMembership(root["project_memberships"], ownerUser, project)
	}
	existing, _ := asAnySlice(root["callers"])
	merged, err := mergeCallers(existing, []any{callerRow})
	if err != nil {
		return "", err
	}
	root["callers"] = merged
	out, err := yaml.Marshal(root)
	if err != nil {
		return "", fmt.Errorf("encode patched config.yaml: %w", err)
	}
	return string(out), nil
}

func ensureAccountEntry(v any, id string, row map[string]any) any {
	id = strings.TrimSpace(id)
	if id == "" {
		return v
	}
	if sl, ok := asAnySlice(v); ok {
		for _, raw := range sl {
			m, ok := asStringMap(raw)
			if !ok {
				continue
			}
			if stringField(m, "id") == id {
				return sl
			}
		}
		return append(append([]any{}, sl...), cloneStringMap(row))
	}
	if m, ok := asStringMap(v); ok {
		out := cloneStringMap(m)
		if _, exists := out[id]; !exists {
			entry := cloneStringMap(row)
			delete(entry, "id")
			out[id] = entry
		}
		return out
	}
	return []any{cloneStringMap(row)}
}

func ensureProjectMembership(v any, userID, project string) any {
	userID = strings.TrimSpace(userID)
	project = strings.TrimSpace(project)
	row := map[string]any{
		"user_id": userID,
		"project": project,
		"role":    "owner",
		"status":  "active",
	}
	sl, ok := asAnySlice(v)
	if !ok || sl == nil {
		return []any{row}
	}
	for _, raw := range sl {
		m, ok := asStringMap(raw)
		if !ok {
			continue
		}
		if stringField(m, "user_id") == userID && stringField(m, "project") == project {
			return sl
		}
	}
	return append(append([]any{}, sl...), row)
}
