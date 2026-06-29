package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

var (
	fenceRE            = regexp.MustCompile("(?s)```yaml([^\\n`]*)\\n(.*?)\\n```")
	forbiddenPricingRE = regexp.MustCompile(`(?m)^\s*pricing_(?:source|updated_at)\s*:`)
)

func main() {
	root, err := findRepoRoot()
	must(err)
	docsRoot := filepath.Join(root, "docs-site", "docs")
	configDocs := filepath.Join(docsRoot, "configuration")

	sampleBytes, err := os.ReadFile(filepath.Join(root, "config.example.yaml"))
	must(err)
	var sample any
	must(yaml.Unmarshal(sampleBytes, &sample))
	sample = normalizeYAML(sample)

	var errors []string
	must(filepath.WalkDir(docsRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		ext := filepath.Ext(path)
		if ext != ".md" && ext != ".mdx" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if loc := forbiddenPricingRE.FindIndex(data); loc != nil {
			errors = append(errors, fmt.Sprintf("%s:%d: do not embed pricing_source or pricing_updated_at values in docs-site; link to config.example.yaml as the source of truth", rel(root, path), lineNumber(data, loc[0])))
		}
		return nil
	}))

	var configPaths []string
	must(filepath.WalkDir(configDocs, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(path) != ".md" {
			return err
		}
		configPaths = append(configPaths, path)
		return nil
	}))
	sort.Strings(configPaths)
	for _, path := range configPaths {
		data, err := os.ReadFile(path)
		must(err)
		matches := fenceRE.FindAllSubmatchIndex(data, -1)
		for _, match := range matches {
			meta := string(data[match[2]:match[3]])
			if !strings.Contains(meta, `title="config.example.yaml"`) && !strings.Contains(meta, `title="config.example.yaml"`) {
				continue
			}
			body := data[match[4]:match[5]]
			var snippet any
			if err := yaml.Unmarshal(body, &snippet); err != nil {
				errors = append(errors, fmt.Sprintf("%s:%d: invalid YAML: %v", rel(root, path), lineNumber(data, match[0]), err))
				continue
			}
			if snippet == nil {
				continue
			}
			if !isSubset(normalizeYAML(snippet), sample) {
				errors = append(errors, fmt.Sprintf("%s:%d: canonical config.example.yaml snippet drifted from config.example.yaml", rel(root, path), lineNumber(data, match[0])))
			}
		}
	}

	if len(errors) > 0 {
		fmt.Fprintln(os.Stderr, "docs config example check failed:")
		for _, err := range errors {
			fmt.Fprintf(os.Stderr, "  - %s\n", err)
		}
		os.Exit(1)
	}
	fmt.Println("docs config examples ok")
}

func findRepoRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(wd, "go.mod")); err == nil {
			return wd, nil
		}
		parent := filepath.Dir(wd)
		if parent == wd {
			return "", fmt.Errorf("go.mod not found")
		}
		wd = parent
	}
}

func normalizeYAML(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, v := range x {
			out[k] = normalizeYAML(v)
		}
		return out
	case map[any]any:
		out := make(map[string]any, len(x))
		for k, v := range x {
			out[fmt.Sprint(k)] = normalizeYAML(v)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, v := range x {
			out[i] = normalizeYAML(v)
		}
		return out
	default:
		return x
	}
}

func isSubset(expected, actual any) bool {
	switch e := expected.(type) {
	case map[string]any:
		a, ok := actual.(map[string]any)
		if !ok {
			return false
		}
		for k, v := range e {
			av, ok := a[k]
			if !ok || !isSubset(v, av) {
				return false
			}
		}
		return true
	case []any:
		a, ok := actual.([]any)
		if !ok || len(e) > len(a) {
			return false
		}
		for i := range e {
			if !isSubset(e[i], a[i]) {
				return false
			}
		}
		return true
	default:
		return fmt.Sprint(expected) == fmt.Sprint(actual)
	}
}

func lineNumber(data []byte, offset int) int {
	return bytes.Count(data[:offset], []byte("\n")) + 1
}

func rel(root, path string) string {
	r, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(r)
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
