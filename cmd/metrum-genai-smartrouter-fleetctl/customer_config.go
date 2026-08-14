package main

import (
	"fmt"

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
