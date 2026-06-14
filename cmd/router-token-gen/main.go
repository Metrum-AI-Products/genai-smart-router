package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"

	"smart-llmrouter/internal/router"
)

func main() {
	if len(os.Args) < 2 || os.Args[1] != "generate" {
		usage()
		os.Exit(2)
	}
	fs := flag.NewFlagSet("generate", flag.ExitOnError)
	user := fs.String("user", "", "caller user name")
	project := fs.String("project", "", "caller project name")
	environment := fs.String("env", "dev", "caller environment")
	keySlug := fs.String("key", "", "visible key slug; defaults to kYYYYMMDD")
	allowRaw := fs.String("allow", "default", "comma-separated allowed internal model groups")
	format := fs.String("format", "yaml", "output format: yaml, json, or env")
	if err := fs.Parse(os.Args[2:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	generated, err := router.GenerateCallerToken(router.TokenGenerateOptions{
		User:        *user,
		Project:     *project,
		Environment: *environment,
		KeySlug:     *keySlug,
		Allow:       splitCSV(*allowRaw),
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	switch strings.ToLower(strings.TrimSpace(*format)) {
	case "yaml", "":
		printYAML(generated)
	case "json":
		printJSON(generated)
	case "env":
		printEnv(generated)
	default:
		fmt.Fprintf(os.Stderr, "unsupported format %q\n", *format)
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: router-token-gen generate --user USER --project PROJECT [--env ENV] [--key KEY] [--allow default,fast,small] [--format yaml|json|env]")
}

func splitCSV(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func printYAML(g router.GeneratedToken) {
	out := struct {
		Token       string                `yaml:"token"`
		TokenID     string                `yaml:"token_id"`
		TokenSHA256 string                `yaml:"token_sha256"`
		Caller      []router.CallerConfig `yaml:"callers"`
	}{
		Token:       g.Token,
		TokenID:     g.TokenID,
		TokenSHA256: g.TokenSHA256,
		Caller:      []router.CallerConfig{g.Caller},
	}
	raw, err := yaml.Marshal(out)
	if err != nil {
		panic(err)
	}
	fmt.Print(string(raw))
}

func printJSON(g router.GeneratedToken) {
	raw, err := json.MarshalIndent(g, "", "  ")
	if err != nil {
		panic(err)
	}
	fmt.Println(string(raw))
}

func printEnv(g router.GeneratedToken) {
	fmt.Printf("ROUTER_TOKEN=%q\n", g.Token)
	fmt.Printf("ROUTER_TOKEN_ID=%q\n", g.TokenID)
	fmt.Printf("ROUTER_TOKEN_SHA256=%q\n", g.TokenSHA256)
}
