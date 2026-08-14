package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"smart-llmrouter/internal/buildinfo"
	"smart-llmrouter/internal/router"
)

const defaultCatalogPath = "docs/enterprise-license-skus.json"

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
	case "inspect", "safe-summary":
		err = safeSummary(os.Args[2:])
	case "template":
		err = templateCommand(os.Args[2:])
	case "issue":
		err = issue(os.Args[2:])
	case "validate":
		err = validate(os.Args[2:])
	case "renew":
		err = renew(os.Args[2:])
	case "top-up":
		err = topUp(os.Args[2:])
	case "revocation":
		err = revocationCommand(os.Args[2:])
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
	fmt.Fprintln(os.Stderr, "usage: router-license generate-keypair|sign|verify|inspect|safe-summary|template list|template render|issue|validate|renew|top-up|revocation create|revocation validate|revocation safe-summary")
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
	if err := writeFile0600(*pubPath, []byte(base64.StdEncoding.EncodeToString(pub)+"\n")); err != nil {
		return err
	}
	return writeFile0600(*privPath, []byte(base64.StdEncoding.EncodeToString(priv)+"\n"))
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
	payload, err := readPayload(*payloadPath)
	if err != nil {
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
	return writeFile0600(*outPath, append(out, '\n'))
}

func verify(args []string) error {
	fs := flag.NewFlagSet("verify", flag.ExitOnError)
	licensePath := fs.String("license", "", "license envelope path")
	publicKeyPath := fs.String("public-key", "", "base64 Ed25519 public key")
	keyID := fs.String("key-id", "", "public key id; defaults to license payload key_id")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *licensePath == "" || *publicKeyPath == "" {
		return fmt.Errorf("license and public-key are required")
	}
	env, err := readEnvelope(*licensePath)
	if err != nil {
		return err
	}
	pub, err := readPublicKey(*publicKeyPath)
	if err != nil {
		return err
	}
	id := strings.TrimSpace(*keyID)
	if id == "" {
		id = env.Payload.KeyID
	}
	keys := []router.LicensePublicKey{{KeyID: id, Algorithm: "ed25519", PublicKey: pub}}
	if err := router.VerifyLicenseEnvelope(env, keys, time.Now().UTC()); err != nil {
		return err
	}
	fmt.Println("valid")
	return nil
}

func safeSummary(args []string) error {
	fs := flag.NewFlagSet("safe-summary", flag.ExitOnError)
	licensePath := fs.String("license", "", "license envelope path")
	payloadPath := fs.String("payload", "", "unsigned payload path")
	outPath := fs.String("out", "", "optional output path")
	format := fs.String("format", "json", "json or markdown")
	if err := fs.Parse(args); err != nil {
		return err
	}
	payload, err := payloadFromInputs(*licensePath, *payloadPath)
	if err != nil {
		return err
	}
	out, err := renderSafeSummary(payload, *format)
	if err != nil {
		return err
	}
	if *outPath != "" {
		return writeFile0600(*outPath, out)
	}
	fmt.Print(string(out))
	if !strings.HasSuffix(string(out), "\n") {
		fmt.Println()
	}
	return nil
}

func revocationCommand(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("revocation subcommand is required")
	}
	switch args[0] {
	case "create":
		return revocationCreate(args[1:])
	case "validate":
		return revocationValidate(args[1:])
	case "safe-summary":
		return revocationSafeSummary(args[1:])
	default:
		return fmt.Errorf("unknown revocation subcommand %q", args[0])
	}
}

func revocationCreate(args []string) error {
	fs := flag.NewFlagSet("revocation create", flag.ExitOnError)
	setID := fs.String("set-id", "", "revocation set id")
	epoch := fs.Int64("epoch", 0, "monotonic revocation epoch")
	licenseIDs := fs.String("license-id", "", "comma-separated license ids")
	status := fs.String("status", "revoked", "entry status: revoked, suspended, or superseded")
	reason := fs.String("reason", "", "safe revocation reason")
	supersededBy := fs.String("superseded-by", "", "replacement license id for superseded entries")
	effectiveAt := fs.String("effective-at", "", "entry effective time RFC3339; defaults to now")
	notBefore := fs.String("not-before", "", "bundle not-before RFC3339; defaults to now")
	expiresAt := fs.String("expires-at", "", "bundle expiry RFC3339; optional")
	keyPath := fs.String("key", "", "base64 Ed25519 private key")
	keyID := fs.String("key-id", "", "signing key id")
	outPath := fs.String("out", "", "signed revocation bundle output")
	publicKeyPath := fs.String("public-key", "", "optional public key for verification")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *setID == "" || *epoch <= 0 || *licenseIDs == "" || *keyPath == "" || *keyID == "" || *outPath == "" {
		return fmt.Errorf("set-id, epoch, license-id, key, key-id, and out are required")
	}
	entryStatus := strings.ToLower(strings.TrimSpace(*status))
	switch entryStatus {
	case "revoked", "suspended", "superseded":
	default:
		return fmt.Errorf("unsupported revocation status %q", *status)
	}
	now := time.Now().UTC()
	nb, err := parseOptionalTime(*notBefore, now)
	if err != nil {
		return err
	}
	exp := time.Time{}
	if strings.TrimSpace(*expiresAt) != "" {
		exp, err = parseRequiredTime(*expiresAt)
		if err != nil {
			return err
		}
	}
	eff, err := parseOptionalTime(*effectiveAt, now)
	if err != nil {
		return err
	}
	entries := make([]router.LicenseRevocationEntry, 0)
	for _, id := range strings.Split(*licenseIDs, ",") {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		entries = append(entries, router.LicenseRevocationEntry{
			LicenseID:    id,
			Status:       entryStatus,
			Reason:       strings.TrimSpace(*reason),
			EffectiveAt:  eff,
			SupersededBy: strings.TrimSpace(*supersededBy),
		})
	}
	if len(entries) == 0 {
		return fmt.Errorf("at least one license-id is required")
	}
	payload := router.LicenseRevocationPayload{
		SchemaVersion:   1,
		Issuer:          router.LicenseIssuer,
		Product:         router.LicenseProduct,
		RevocationSetID: strings.TrimSpace(*setID),
		RevocationEpoch: *epoch,
		IssuedAt:        now,
		NotBefore:       nb,
		ExpiresAt:       exp,
		KeyID:           strings.TrimSpace(*keyID),
		Entries:         entries,
	}
	priv, err := readPrivateKey(*keyPath)
	if err != nil {
		return err
	}
	env, err := router.SignLicenseRevocationPayload(payload, priv)
	if err != nil {
		return err
	}
	raw, err := router.MarshalLicenseRevocationEnvelope(env)
	if err != nil {
		return err
	}
	if err := writeFile0600(*outPath, append(raw, '\n')); err != nil {
		return err
	}
	if *publicKeyPath != "" {
		pub, err := readPublicKey(*publicKeyPath)
		if err != nil {
			return err
		}
		if err := router.VerifyLicenseRevocationEnvelope(env, []router.LicensePublicKey{{KeyID: *keyID, Algorithm: "ed25519", PublicKey: pub}}, time.Now().UTC()); err != nil {
			return err
		}
	}
	fmt.Printf("created revocation set %s\n", payload.RevocationSetID)
	return nil
}

func revocationValidate(args []string) error {
	fs := flag.NewFlagSet("revocation validate", flag.ExitOnError)
	bundlePath := fs.String("bundle", "", "signed revocation bundle")
	publicKeyPath := fs.String("public-key", "", "base64 Ed25519 public key")
	keyID := fs.String("key-id", "", "public key id; defaults to bundle payload key_id")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *bundlePath == "" || *publicKeyPath == "" {
		return fmt.Errorf("bundle and public-key are required")
	}
	env, err := readRevocationEnvelope(*bundlePath)
	if err != nil {
		return err
	}
	pub, err := readPublicKey(*publicKeyPath)
	if err != nil {
		return err
	}
	id := strings.TrimSpace(*keyID)
	if id == "" {
		id = env.Payload.KeyID
	}
	if err := router.VerifyLicenseRevocationEnvelope(env, []router.LicensePublicKey{{KeyID: id, Algorithm: "ed25519", PublicKey: pub}}, time.Now().UTC()); err != nil {
		return err
	}
	fmt.Println("valid")
	return nil
}

func revocationSafeSummary(args []string) error {
	fs := flag.NewFlagSet("revocation safe-summary", flag.ExitOnError)
	bundlePath := fs.String("bundle", "", "signed revocation bundle")
	outPath := fs.String("out", "", "optional output path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *bundlePath == "" {
		return fmt.Errorf("bundle is required")
	}
	env, err := readRevocationEnvelope(*bundlePath)
	if err != nil {
		return err
	}
	summary := map[string]any{
		"schema_version":    env.Payload.SchemaVersion,
		"product":           env.Payload.Product,
		"issuer":            env.Payload.Issuer,
		"revocation_set_id": env.Payload.RevocationSetID,
		"revocation_epoch":  env.Payload.RevocationEpoch,
		"not_before":        env.Payload.NotBefore.UTC().Format(time.RFC3339),
		"expires_at":        formatOptionalCLIUTC(env.Payload.ExpiresAt),
		"key_id":            env.Payload.KeyID,
		"entries":           safeRevocationEntries(env.Payload.Entries),
	}
	raw, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return err
	}
	if *outPath != "" {
		return writeFile0600(*outPath, append(raw, '\n'))
	}
	fmt.Println(string(raw))
	return nil
}

func templateCommand(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("template subcommand is required")
	}
	switch args[0] {
	case "list":
		return templateList(args[1:])
	case "render":
		return templateRender(args[1:])
	default:
		return fmt.Errorf("unknown template subcommand %q", args[0])
	}
}

func templateList(args []string) error {
	fs := flag.NewFlagSet("template list", flag.ExitOnError)
	catalogPath := fs.String("catalog", defaultCatalogPath, "SKU catalog path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	catalog, err := router.LoadLicenseSKUCatalog(*catalogPath)
	if err != nil {
		return err
	}
	for _, sku := range catalog.SKUs {
		fmt.Printf("%s\t%s\t%s\n", sku.SKU, sku.SupportTier, sku.Notes)
	}
	return nil
}

func templateRender(args []string) error {
	fs := flag.NewFlagSet("template render", flag.ExitOnError)
	catalogPath := fs.String("catalog", defaultCatalogPath, "SKU catalog path")
	entitlementPath := fs.String("entitlement", "", "entitlement YAML or JSON")
	outPath := fs.String("out", "", "payload output path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *entitlementPath == "" {
		return fmt.Errorf("entitlement is required")
	}
	payload, err := renderEntitlement(*catalogPath, *entitlementPath)
	if err != nil {
		return err
	}
	if err := validatePayloadForIssuance(payload, nil, false); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	if *outPath != "" {
		return writeFile0600(*outPath, append(raw, '\n'))
	}
	fmt.Println(string(raw))
	return nil
}

func issue(args []string) error {
	fs := flag.NewFlagSet("issue", flag.ExitOnError)
	catalogPath := fs.String("catalog", defaultCatalogPath, "SKU catalog path")
	entitlementPath := fs.String("entitlement", "", "entitlement YAML or JSON")
	keyPath := fs.String("key", "", "base64 Ed25519 private key")
	outPath := fs.String("out", "", "signed license output path")
	payloadOut := fs.String("payload-out", "", "optional unsigned payload audit output path")
	summaryOut := fs.String("summary-out", "", "optional safe summary output path")
	checklistOut := fs.String("checklist-out", "", "optional operator checklist output path")
	publicKeyPath := fs.String("public-key", "", "optional public key for verification")
	allowUnknown := fs.Bool("allow-unknown-runtime-key", false, "allow key IDs absent from embedded runtime public keys")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *entitlementPath == "" || *keyPath == "" || *outPath == "" {
		return fmt.Errorf("entitlement, key, and out are required")
	}
	payload, err := renderEntitlement(*catalogPath, *entitlementPath)
	if err != nil {
		return err
	}
	priv, err := readPrivateKey(*keyPath)
	if err != nil {
		return err
	}
	verifyKeys, err := verificationKeys(payload.KeyID, priv, *publicKeyPath, *allowUnknown)
	if err != nil {
		return err
	}
	if err := validatePayloadForIssuance(payload, verifyKeys.runtime, *allowUnknown); err != nil {
		return err
	}
	return writeIssuedArtifacts(payload, priv, verifyKeys.verify, *outPath, *payloadOut, *summaryOut, *checklistOut)
}

func validate(args []string) error {
	fs := flag.NewFlagSet("validate", flag.ExitOnError)
	licensePath := fs.String("license", "", "license envelope path")
	payloadPath := fs.String("payload", "", "unsigned payload path")
	publicKeyPath := fs.String("public-key", "", "optional public key for signature verification")
	keyID := fs.String("key-id", "", "public key id; defaults to payload key_id")
	allowUnknown := fs.Bool("allow-unknown-runtime-key", false, "allow key IDs absent from embedded runtime public keys")
	if err := fs.Parse(args); err != nil {
		return err
	}
	payload, err := payloadFromInputs(*licensePath, *payloadPath)
	if err != nil {
		return err
	}
	keys := router.DefaultLicensePublicKeys()
	if *publicKeyPath != "" {
		pub, err := readPublicKey(*publicKeyPath)
		if err != nil {
			return err
		}
		id := strings.TrimSpace(*keyID)
		if id == "" {
			id = payload.KeyID
		}
		keys = append(keys, router.LicensePublicKey{KeyID: id, Algorithm: "ed25519", PublicKey: pub})
	}
	if err := validatePayloadForIssuance(payload, keys, *allowUnknown); err != nil {
		return err
	}
	if *licensePath != "" && *publicKeyPath != "" {
		env, err := readEnvelope(*licensePath)
		if err != nil {
			return err
		}
		if err := router.VerifyLicenseEnvelope(env, keys, time.Now().UTC()); err != nil {
			return err
		}
	}
	fmt.Println("valid")
	return nil
}

func renew(args []string) error {
	fs := flag.NewFlagSet("renew", flag.ExitOnError)
	licensePath := fs.String("license", "", "previous license envelope path")
	keyPath := fs.String("key", "", "base64 Ed25519 private key")
	outPath := fs.String("out", "", "signed license output path")
	licenseID := fs.String("license-id", "", "new license id")
	issuedAt := fs.String("issued-at", "", "issued_at RFC3339; defaults to now")
	notBefore := fs.String("not-before", "", "not_before RFC3339; defaults to issued_at")
	expiresAt := fs.String("expires-at", "", "expires_at RFC3339")
	summaryOut := fs.String("summary-out", "", "optional safe summary output path")
	publicKeyPath := fs.String("public-key", "", "optional public key for verification")
	allowUnknown := fs.Bool("allow-unknown-runtime-key", false, "allow key IDs absent from embedded runtime public keys")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *licensePath == "" || *keyPath == "" || *outPath == "" || *expiresAt == "" {
		return fmt.Errorf("license, key, out, and expires-at are required")
	}
	prev, err := readEnvelope(*licensePath)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	issued, err := parseOptionalTime(*issuedAt, now)
	if err != nil {
		return err
	}
	nb, err := parseOptionalTime(*notBefore, issued)
	if err != nil {
		return err
	}
	exp, err := parseRequiredTime(*expiresAt)
	if err != nil {
		return err
	}
	id := strings.TrimSpace(*licenseID)
	if id == "" {
		id = router.GenerateLicenseID(prev.Payload.SKU)
	}
	payload, err := router.RenewLicensePayload(prev.Payload, id, issued, nb, exp)
	if err != nil {
		return err
	}
	priv, err := readPrivateKey(*keyPath)
	if err != nil {
		return err
	}
	verifyKeys, err := verificationKeys(payload.KeyID, priv, *publicKeyPath, *allowUnknown)
	if err != nil {
		return err
	}
	if err := validatePayloadForIssuance(payload, verifyKeys.runtime, *allowUnknown); err != nil {
		return err
	}
	return writeIssuedArtifacts(payload, priv, verifyKeys.verify, *outPath, "", *summaryOut, "")
}

func topUp(args []string) error {
	fs := flag.NewFlagSet("top-up", flag.ExitOnError)
	catalogPath := fs.String("catalog", defaultCatalogPath, "SKU catalog path")
	licensePath := fs.String("license", "", "previous license envelope path")
	sku := fs.String("sku", "", "credit-pack SKU")
	keyPath := fs.String("key", "", "base64 Ed25519 private key")
	outPath := fs.String("out", "", "signed license output path")
	licenseID := fs.String("license-id", "", "new license id")
	expiresAt := fs.String("expires-at", "", "optional expires_at RFC3339")
	summaryOut := fs.String("summary-out", "", "optional safe summary output path")
	publicKeyPath := fs.String("public-key", "", "optional public key for verification")
	allowUnknown := fs.Bool("allow-unknown-runtime-key", false, "allow key IDs absent from embedded runtime public keys")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *licensePath == "" || *sku == "" || *keyPath == "" || *outPath == "" {
		return fmt.Errorf("license, sku, key, and out are required")
	}
	prev, err := readEnvelope(*licensePath)
	if err != nil {
		return err
	}
	ent := router.LicenseEntitlement{
		LicenseID:    strings.TrimSpace(*licenseID),
		CustomerID:   prev.Payload.CustomerID,
		CustomerName: prev.Payload.CustomerName,
		SKU:          *sku,
		Deployment:   deploymentMap(prev.Payload.Deployment),
	}
	ent.Signing.KeyID = prev.Payload.KeyID
	ent.Term.IssuedAt = time.Now().UTC()
	ent.Term.NotBefore = ent.Term.IssuedAt
	if strings.TrimSpace(*expiresAt) != "" {
		ent.Term.ExpiresAt, err = parseRequiredTime(*expiresAt)
		if err != nil {
			return err
		}
	}
	creditPack, err := renderEntitlementFromValue(*catalogPath, ent)
	if err != nil {
		return err
	}
	payload, err := router.TopUpLicensePayload(prev.Payload, creditPack, creditPack.LicenseID)
	if err != nil {
		return err
	}
	payload.Notes = "Credit-pack top-up replacement. License-wide counters reset because license_id changed."
	priv, err := readPrivateKey(*keyPath)
	if err != nil {
		return err
	}
	verifyKeys, err := verificationKeys(payload.KeyID, priv, *publicKeyPath, *allowUnknown)
	if err != nil {
		return err
	}
	if err := validatePayloadForIssuance(payload, verifyKeys.runtime, *allowUnknown); err != nil {
		return err
	}
	return writeIssuedArtifacts(payload, priv, verifyKeys.verify, *outPath, "", *summaryOut, "")
}

type keySets struct {
	runtime []router.LicensePublicKey
	verify  []router.LicensePublicKey
}

func verificationKeys(keyID string, priv ed25519.PrivateKey, publicKeyPath string, allowUnknown bool) (keySets, error) {
	runtimeKeys := router.DefaultLicensePublicKeys()
	verifyKeys := append([]router.LicensePublicKey(nil), runtimeKeys...)
	if publicKeyPath != "" {
		pub, err := readPublicKey(publicKeyPath)
		if err != nil {
			return keySets{}, err
		}
		verifyKeys = append(verifyKeys, router.LicensePublicKey{KeyID: keyID, Algorithm: "ed25519", PublicKey: pub})
		if allowUnknown {
			runtimeKeys = append(runtimeKeys, router.LicensePublicKey{KeyID: keyID, Algorithm: "ed25519", PublicKey: pub})
		}
	} else if allowUnknown {
		pub := priv.Public().(ed25519.PublicKey)
		runtimeKeys = append(runtimeKeys, router.LicensePublicKey{KeyID: keyID, Algorithm: "ed25519", PublicKey: pub})
		verifyKeys = append(verifyKeys, router.LicensePublicKey{KeyID: keyID, Algorithm: "ed25519", PublicKey: pub})
	}
	return keySets{runtime: runtimeKeys, verify: verifyKeys}, nil
}

func writeIssuedArtifacts(payload router.LicensePayload, priv ed25519.PrivateKey, keys []router.LicensePublicKey, licensePath, payloadOut, summaryOut, checklistOut string) error {
	env, err := router.SignLicensePayload(payload, priv)
	if err != nil {
		return err
	}
	if err := router.VerifyLicenseEnvelope(env, keys, time.Now().UTC()); err != nil {
		return err
	}
	raw, err := router.MarshalLicenseEnvelope(env)
	if err != nil {
		return err
	}
	if err := writeFile0600(licensePath, append(raw, '\n')); err != nil {
		return err
	}
	if payloadOut != "" {
		payloadRaw, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			return err
		}
		if err := writeFile0600(payloadOut, append(payloadRaw, '\n')); err != nil {
			return err
		}
	}
	if summaryOut != "" {
		out, err := renderSafeSummary(payload, "json")
		if err != nil {
			return err
		}
		if err := writeFile0600(summaryOut, out); err != nil {
			return err
		}
	}
	if checklistOut != "" {
		if err := writeFile0600(checklistOut, []byte(operatorChecklist(payload))); err != nil {
			return err
		}
	}
	fmt.Printf("issued %s for %s (%s)\n", payload.LicenseID, payload.CustomerID, payload.SKU)
	return nil
}

func validatePayloadForIssuance(payload router.LicensePayload, keys []router.LicensePublicKey, allowUnknown bool) error {
	if allowUnknown {
		keys = nil
	}
	when := payload.NotBefore
	if when.IsZero() {
		when = time.Now().UTC()
	}
	return router.ValidateLicensePayload(payload, nil, keys, when)
}

func renderEntitlement(catalogPath, entitlementPath string) (router.LicensePayload, error) {
	ent, err := router.LoadLicenseEntitlement(entitlementPath)
	if err != nil {
		return router.LicensePayload{}, err
	}
	return renderEntitlementFromValue(catalogPath, ent)
}

func renderEntitlementFromValue(catalogPath string, ent router.LicenseEntitlement) (router.LicensePayload, error) {
	catalog, err := router.LoadLicenseSKUCatalog(catalogPath)
	if err != nil {
		return router.LicensePayload{}, err
	}
	return router.RenderLicensePayload(ent, catalog, router.LicenseRenderOptions{Now: time.Now().UTC()})
}

func payloadFromInputs(licensePath, payloadPath string) (router.LicensePayload, error) {
	if licensePath == "" && payloadPath == "" {
		return router.LicensePayload{}, fmt.Errorf("license or payload is required")
	}
	if licensePath != "" {
		env, err := readEnvelope(licensePath)
		if err != nil {
			return router.LicensePayload{}, err
		}
		return env.Payload, nil
	}
	return readPayload(payloadPath)
}

func readPayload(path string) (router.LicensePayload, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return router.LicensePayload{}, err
	}
	var payload router.LicensePayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return router.LicensePayload{}, err
	}
	return payload, nil
}

func readEnvelope(path string) (router.LicenseEnvelope, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return router.LicenseEnvelope{}, err
	}
	return router.ParseLicenseEnvelope(raw)
}

func readRevocationEnvelope(path string) (router.LicenseRevocationEnvelope, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return router.LicenseRevocationEnvelope{}, err
	}
	return router.ParseLicenseRevocationEnvelope(raw)
}

func renderSafeSummary(payload router.LicensePayload, format string) ([]byte, error) {
	summary := router.SafeLicenseSummary(payload)
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "", "json":
		raw, err := json.MarshalIndent(summary, "", "  ")
		if err != nil {
			return nil, err
		}
		return append(raw, '\n'), nil
	case "markdown", "md":
		return []byte(fmt.Sprintf("# License Safe Summary\n\n- License ID: `%s`\n- Customer ID: `%s`\n- SKU: `%s`\n- Product: `%s`\n- Key ID: `%s`\n- Issuer: `%s`\n- Not before: `%s`\n- Expires at: `%s`\n", summary.LicenseID, summary.CustomerID, summary.SKU, summary.Product, summary.KeyID, summary.Issuer, summary.NotBefore, summary.ExpiresAt)), nil
	default:
		return nil, fmt.Errorf("unsupported summary format %q", format)
	}
}

func safeRevocationEntries(entries []router.LicenseRevocationEntry) []map[string]any {
	out := make([]map[string]any, 0, len(entries))
	for _, entry := range entries {
		item := map[string]any{
			"license_id":   entry.LicenseID,
			"status":       entry.Status,
			"reason":       entry.Reason,
			"effective_at": formatOptionalCLIUTC(entry.EffectiveAt),
		}
		if entry.SupersededBy != "" {
			item["superseded_by"] = entry.SupersededBy
		}
		out = append(out, item)
	}
	return out
}

func formatOptionalCLIUTC(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}

func operatorChecklist(payload router.LicensePayload) string {
	return fmt.Sprintf("# License Operator Checklist\n\n"+
		"- Confirm entitlement approval is recorded for customer `%s`.\n"+
		"- Deliver only the signed license file `%s` through an approved secure channel.\n"+
		"- Tell the operator to install the file at the configured `server.license.path`.\n"+
		"- Verify `/readyz`, authorized `/admin/license/status`, metrics-admin license gauges, and one licensed caller smoke.\n"+
		"- Record safe status only: license ID `%s`, SKU `%s`, key ID `%s`, expiry `%s`.\n",
		payload.CustomerID, filepath.Base("license.json"), payload.LicenseID, payload.SKU, payload.KeyID, payload.ExpiresAt.UTC().Format(time.RFC3339))
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

func writeFile0600(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil && filepath.Dir(path) != "." {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func parseOptionalTime(value string, fallback time.Time) (time.Time, error) {
	if strings.TrimSpace(value) == "" {
		return fallback.UTC(), nil
	}
	return parseRequiredTime(value)
}

func parseRequiredTime(value string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(value))
	if err != nil {
		return time.Time{}, err
	}
	return t.UTC(), nil
}

func deploymentMap(deployment router.LicenseDeployment) map[string]any {
	raw, _ := json.Marshal(deployment)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return out
}
