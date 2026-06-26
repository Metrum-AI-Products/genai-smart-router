package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"smart-llmrouter/internal/buildinfo"
	"smart-llmrouter/internal/router"
)

func main() {
	if len(os.Args) == 2 && os.Args[1] == "--version" {
		fmt.Println(buildinfo.Text())
		return
	}
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "generate-keypair":
		err = generateKeypair(os.Args[2:])
	case "sign":
		err = sign(os.Args[2:])
	case "verify":
		err = verify(os.Args[2:])
	case "inspect":
		err = inspect(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: router-license generate-keypair --public-key-out PUB --private-key-out KEY | sign --payload PAYLOAD --key KEY --key-id ID --out LICENSE | verify --license LICENSE --public-key PUB | inspect --license LICENSE")
}

func generateKeypair(args []string) error {
	fs := flag.NewFlagSet("generate-keypair", flag.ExitOnError)
	pubPath := fs.String("public-key-out", "", "path to write base64 Ed25519 public key")
	privPath := fs.String("private-key-out", "", "path to write base64 Ed25519 private key")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*pubPath) == "" || strings.TrimSpace(*privPath) == "" {
		return fmt.Errorf("public-key-out and private-key-out are required")
	}
	pub, priv, err := router.GenerateLicenseKeypair()
	if err != nil {
		return err
	}
	if err := os.WriteFile(*pubPath, []byte(base64.StdEncoding.EncodeToString(pub)+"\n"), 0o644); err != nil {
		return err
	}
	return os.WriteFile(*privPath, []byte(base64.StdEncoding.EncodeToString(priv)+"\n"), 0o600)
}

func sign(args []string) error {
	fs := flag.NewFlagSet("sign", flag.ExitOnError)
	payloadPath := fs.String("payload", "", "license payload JSON")
	keyPath := fs.String("key", "", "base64 Ed25519 private key")
	keyID := fs.String("key-id", "", "license verification key id")
	outPath := fs.String("out", "", "output license envelope path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *payloadPath == "" || *keyPath == "" || *keyID == "" || *outPath == "" {
		return fmt.Errorf("payload, key, key-id, and out are required")
	}
	raw, err := os.ReadFile(*payloadPath)
	if err != nil {
		return err
	}
	var payload router.LicensePayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return err
	}
	payload.KeyID = *keyID
	priv, err := readPrivateKey(*keyPath)
	if err != nil {
		return err
	}
	env, err := router.SignLicensePayload(payload, priv)
	if err != nil {
		return err
	}
	out, err := router.MarshalLicenseEnvelope(env)
	if err != nil {
		return err
	}
	return os.WriteFile(*outPath, append(out, '\n'), 0o644)
}

func verify(args []string) error {
	fs := flag.NewFlagSet("verify", flag.ExitOnError)
	licensePath := fs.String("license", "", "license envelope path")
	publicKeyPath := fs.String("public-key", "", "base64 Ed25519 public key")
	keyID := fs.String("key-id", "test-license-key", "public key id")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *licensePath == "" || *publicKeyPath == "" {
		return fmt.Errorf("license and public-key are required")
	}
	raw, err := os.ReadFile(*licensePath)
	if err != nil {
		return err
	}
	env, err := router.ParseLicenseEnvelope(raw)
	if err != nil {
		return err
	}
	pub, err := readPublicKey(*publicKeyPath)
	if err != nil {
		return err
	}
	keys := []router.LicensePublicKey{{KeyID: *keyID, Algorithm: "ed25519", PublicKey: pub}}
	if err := router.VerifyLicenseEnvelope(env, keys, time.Now().UTC()); err != nil {
		return err
	}
	fmt.Println("valid")
	return nil
}

func inspect(args []string) error {
	fs := flag.NewFlagSet("inspect", flag.ExitOnError)
	licensePath := fs.String("license", "", "license envelope path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *licensePath == "" {
		return fmt.Errorf("license is required")
	}
	raw, err := os.ReadFile(*licensePath)
	if err != nil {
		return err
	}
	env, err := router.ParseLicenseEnvelope(raw)
	if err != nil {
		return err
	}
	summary := map[string]any{
		"schema_version": env.Payload.SchemaVersion,
		"license_id":     env.Payload.LicenseID,
		"customer_id":    env.Payload.CustomerID,
		"product":        env.Payload.Product,
		"sku":            env.Payload.SKU,
		"features":       env.Payload.Features,
		"not_before":     env.Payload.NotBefore.Format(time.RFC3339),
		"expires_at":     env.Payload.ExpiresAt.Format(time.RFC3339),
		"key_id":         env.Payload.KeyID,
		"issuer":         env.Payload.Issuer,
	}
	out, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(out))
	return nil
}

func readPublicKey(path string) (ed25519.PublicKey, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil {
		return nil, err
	}
	if len(decoded) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("public key has length %d, want %d", len(decoded), ed25519.PublicKeySize)
	}
	return ed25519.PublicKey(decoded), nil
}

func readPrivateKey(path string) (ed25519.PrivateKey, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil {
		return nil, err
	}
	if len(decoded) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("private key has length %d, want %d", len(decoded), ed25519.PrivateKeySize)
	}
	return ed25519.PrivateKey(decoded), nil
}
