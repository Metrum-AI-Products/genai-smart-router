VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS ?= -X smart-llmrouter/internal/buildinfo.Version=$(VERSION) -X smart-llmrouter/internal/buildinfo.Commit=$(COMMIT) -X smart-llmrouter/internal/buildinfo.BuildDate=$(BUILD_DATE)
DIST_DIR ?= dist
PKG_NAME ?= smart-llmrouter
GOOS ?= linux
GOARCH ?= $(shell go env GOARCH)
HOST_GOOS := $(shell go env GOHOSTOS)
HOST_GOARCH := $(shell go env GOHOSTARCH)
PYTHON ?= python3

# Inspect coding evaluations are deliberately opt-in: they call a live endpoint
# and may start Docker sandboxes.  They are never prerequisites of test/build.
EVAL_MODEL ?=
EVAL_BASE_URL ?=
EVAL_API ?= openai
EVAL_LIMIT ?= 8
EVAL_CONCURRENCY ?= 1
EVAL_TIMEOUT ?= 300
# Each Make process receives one isolated run directory, shared by explicitly
# requested suite/report targets in that same invocation. CI pins EVAL_RUN_ID
# across separate Make calls in one workflow run.
EVAL_LOG_ROOT ?= tmp/inspect-evals
ifeq ($(origin EVAL_RUN_ID), undefined)
EVAL_RUN_ID := $(shell date -u +%Y%m%dT%H%M%SZ)-$(shell printf '%s' $$$$)
endif
EVAL_LOG_DIR ?= $(EVAL_LOG_ROOT)/$(EVAL_RUN_ID)
EVAL_REASONING ?=
# Ordinary bounded evaluations leave this false.  Set it to true only for the
# protected reasoning-coverage path, which requires exactly one usage config
# source and a dedicated caller identity.
EVAL_REQUIRE_REASONING_COVERAGE ?= false
# Path to the aggregate-only reasoning coverage JSON exported from the protected
# usage DB; never point this at raw request or Inspect logs.
EVAL_REASONING_COVERAGE_FILE ?=
EVAL_USAGE_CONFIG_YAML ?=
EVAL_USAGE_CONFIG_FILE ?=
EVAL_USAGE_CALLER_ID ?=
EVAL_USAGE_CLIENT ?=
EVAL_USAGE_REPORT_BIN ?= go
EVAL_USAGE_EXPORT_TIMEOUT ?= 60
EVAL_MODEL_KIND ?= router-group
EVAL_SUITE ?= humaneval
EVAL_POLICY ?= config/evaluation-policy.example.json
EVAL_INSPECT ?= inspect
EVAL_CI_REPORT_DIR ?= docs/evaluation-reports/inspect
EVAL_CI_REPORT_TIMESTAMP ?=
EVAL_SAVE_CI_REPORT ?= false
export EVAL_MODEL EVAL_BASE_URL EVAL_API EVAL_LIMIT EVAL_CONCURRENCY EVAL_TIMEOUT EVAL_LOG_ROOT EVAL_RUN_ID EVAL_LOG_DIR EVAL_REASONING EVAL_REQUIRE_REASONING_COVERAGE EVAL_REASONING_COVERAGE_FILE EVAL_USAGE_CONFIG_YAML EVAL_USAGE_CONFIG_FILE EVAL_USAGE_CALLER_ID EVAL_USAGE_CLIENT EVAL_USAGE_REPORT_BIN EVAL_USAGE_EXPORT_TIMEOUT EVAL_MODEL_KIND EVAL_SUITE EVAL_BASELINE_JSON EVAL_POLICY EVAL_INSPECT EVAL_CI_REPORT_DIR EVAL_CI_REPORT_TIMESTAMP EVAL_SAVE_CI_REPORT

# Keep the repository's historical validation contract for bare `make` even
# though the EKS help target appears earlier in this file.
.DEFAULT_GOAL := test

# Explicit inputs for all EKS commands. No target reads the current kubectl
# context; scripts/eks_delivery.py creates and removes its own kubeconfig.
# The AWS account, region, ECR repository, cluster, namespace, overlay,
# workload, and role are
# not Make variables: they are pinned in the reviewed target policy and its
# independently protected SSM copy.
# Delivery and discovery use different AWS profiles. Keep the legacy
# EKS_AWS_PROFILE default below for discovery/session bootstrap; delivery must
# never overwrite it because those targets create the discovery-role profile.
EKS_DELIVERY_AWS_PROFILE ?= genai-smart-router-eks-staging-delivery
IMAGE_DIGEST ?=
EKS_CONFIRM ?=
ROLLBACK_POD_TEMPLATE_SHA256 ?=
EKS_EVIDENCE_DIR ?= tmp/eks-evidence
# The promotion-plan reducer writes its fixed safe handoff here. It must not
# overlap raw EKS_EVIDENCE_DIR; it supplies only the staging projection later
# copied into the separately assembled protected production evidence bundle.
EKS_PROMOTION_EVIDENCE_DIR ?= tmp/eks-promotion-evidence
# Export only the path to an owner-only (0600) local/CI shell script. Its
# contents must never be passed through Make expansion or a Python argv value;
# the runner opens the validated file once and executes that bound descriptor.
EKS_SMOKE_COMMAND_FILE ?=
EKS_SUPPLY_CHAIN_DIR ?= tmp/eks-supply-chain
# EKS inputs can originate in CI/environment values. Export them and expand
# only in the recipe shell: Make interpolation inside shell quotes would allow
# a malicious value to alter shell syntax before Python can validate it.
export EKS_DELIVERY_AWS_PROFILE IMAGE_DIGEST EKS_CONFIRM ROLLBACK_POD_TEMPLATE_SHA256 \
	EKS_EVIDENCE_DIR EKS_PROMOTION_EVIDENCE_DIR EKS_SMOKE_COMMAND_FILE EKS_SUPPLY_CHAIN_DIR
EKS_DELIVERY = $(PYTHON) scripts/eks_delivery.py
EKS_ARGS = --aws-profile "$${EKS_DELIVERY_AWS_PROFILE}" --image-digest "$${IMAGE_DIGEST}" --confirm "$${EKS_CONFIRM}" --rollback-pod-template-sha256 "$${ROLLBACK_POD_TEMPLATE_SHA256}" --evidence-dir "$${EKS_EVIDENCE_DIR}"
EKS_PROMOTION_ARGS = $(EKS_ARGS) --promotion-evidence-dir "$${EKS_PROMOTION_EVIDENCE_DIR}"

DOCKER ?= docker
DOCKER_BUILDX ?= $(DOCKER) buildx
DOCKER_PLATFORM ?= linux/$(GOARCH)
IMAGE_NAME ?= smart-llmrouter
IMAGE_TAG ?= $(VERSION)-$(GOOS)-$(GOARCH)
DOCS_SITE_DIR ?= docs-site
DOCS_EMBED_DIR ?= internal/router/docsdist
PACKAGE_DOC_ALLOWLIST ?= scripts/package_docs_allowlist.txt
EKS_AWS_PROFILE ?= genai-smart-router-eks-discovery
EKS_ACCOUNT_ID ?=
EKS_REGION ?=
EKS_NAMESPACE ?=
EKS_LINKERD_NAMESPACE ?=
EKS_INGRESS_NAMESPACE ?=
EKS_INGRESS_SERVICE_ACCOUNT ?=
EKS_INGRESS_DEPLOYMENT ?=
EKS_LINKERD_TRUST_DOMAIN ?=
EKS_ECR_REPOSITORY ?=
EKS_DISCOVERY_OUTPUT ?=
EKS_LINKERD_POLICY_OUTPUT ?=
EKS_INGRESS_NETWORK_POLICY_OUTPUT ?=
EKS_POLICY_AWS_PROFILE ?=
EKS_POLICY_KUBECONFIG ?=
EKS_POLICY_CONTEXT ?=
EKS_POLICY_APPLY_CONFIRM ?=
EKS_ADMIN_PROFILE ?= default
EKS_SOURCE_USER ?= smartrouter
EKS_MFA_SERIAL ?=
EKS_MFA_KEYCHAIN_SERVICE ?=
EKS_MFA_KEYCHAIN_ACCOUNT ?= smartrouter
EKS_SESSION_DURATION ?= 3600
COPYFILE_DISABLE ?= 1
export VERSION COMMIT BUILD_DATE DIST_DIR PKG_NAME GOOS GOARCH IMAGE_NAME IMAGE_TAG PYTHON AWS_REGION EKS_CLUSTER K8S_NAMESPACE KUSTOMIZE_OVERLAY ENVIRONMENT IMAGE_DIGEST EKS_CONFIRM EKS_EVIDENCE_DIR EKS_PROMOTION_EVIDENCE_DIR EKS_SMOKE_COMMAND EKS_DELIVERY_AWS_PROFILE EKS_AWS_PROFILE EKS_ACCOUNT_ID EKS_REGION EKS_NAMESPACE EKS_LINKERD_NAMESPACE EKS_INGRESS_NAMESPACE EKS_INGRESS_SERVICE_ACCOUNT EKS_INGRESS_DEPLOYMENT EKS_LINKERD_TRUST_DOMAIN EKS_ECR_REPOSITORY EKS_DISCOVERY_OUTPUT EKS_LINKERD_POLICY_OUTPUT EKS_INGRESS_NETWORK_POLICY_OUTPUT EKS_POLICY_AWS_PROFILE EKS_POLICY_KUBECONFIG EKS_POLICY_CONTEXT EKS_POLICY_APPLY_CONFIRM EKS_ADMIN_PROFILE EKS_SOURCE_USER EKS_MFA_SERIAL EKS_MFA_KEYCHAIN_SERVICE EKS_MFA_KEYCHAIN_ACCOUNT EKS_SESSION_DURATION
export COPYFILE_DISABLE
TAR_ENV := COPYFILE_DISABLE=1

BUILD_LDFLAGS = -X smart-llmrouter/internal/buildinfo.Version=$${VERSION} -X smart-llmrouter/internal/buildinfo.Commit=$${COMMIT} -X smart-llmrouter/internal/buildinfo.BuildDate=$${BUILD_DATE}

.PHONY: help eks-help eks-preflight eks-render eks-plan eks-apply-staging eks-rollout-status eks-smoke-staging eks-rollback-staging eks-release-evidence eks-promotion-plan eks-supply-chain-validate production-promotion-validate ci-eks-staging-contract test test-reasoning-telemetry-postgres test-usage-schema-postgres-indexes capability-smoke capability-smoke-unit capability-smoke-live api-compat-bootstrap api-compat-bootstrap-go-provision api-compat-mock api-compat-mock-offline api-compat-live outcome-calibrated-demo outcome-calibrated-synthetic-demo secret-check validate-build-metadata validate-release-clean release-validation-matrix release-notes-from-git docs-diag-schema docs-diag-schema-check docs-qa docs-build docs-dev docs-clean admin-build admin-e2e build build-go-only build-all package package-one package-one-no-docs package-all docker-image docker-image-no-docs package-docker package-docker-one package-docker-one-no-docs package-docker-all compose-security-check eks-session-bootstrap eks-session-recovery-status eks-identity-check eks-discovery-validate eks-discover eks-render-ingress-network-policy eks-render-linkerd-policy eks-validate-tenant-network-policies eks-apply-tenant-network-policies e2e-mock e2e-live-c e2e-live-full e2e-compose-live eval-humaneval eval-bigcodebench eval-report eval-ci-smoke eval-ci-full livecodebench-contract-test livecodebench-target-test livecodebench-validate livecodebench-run clean

help: eks-help

test-reasoning-telemetry-postgres:
	bash scripts/test_reasoning_telemetry_postgres.sh

test-usage-schema-postgres-indexes:
	bash scripts/test_usage_schema_postgres_indexes.sh

eval-humaneval:
	$(PYTHON) scripts/inspect_coding_eval.py run --suite humaneval

eval-bigcodebench:
	$(PYTHON) scripts/inspect_coding_eval.py run --suite bigcodebench

eval-report:
	$(PYTHON) scripts/inspect_coding_eval.py report --log-dir "$${EVAL_LOG_DIR}" --suite "$${EVAL_SUITE}" --policy "$${EVAL_POLICY}"

# A small, authenticated router-group check for explicitly enabled CI only.
eval-ci-smoke:
	$(PYTHON) scripts/inspect_coding_eval.py smoke

# Full CI sequencing is implemented in the reusable wrapper, not in a CI YAML
# shell block. It requires protected per-suite baselines and remains opt-in.
eval-ci-full:
	$(PYTHON) scripts/inspect_coding_eval.py ci-full

# LiveCodeBench is opt-in. It needs an evaluator checkout created from the
# pinned contract and can download the official public dataset; it is never a
# prerequisite of make/test/build.
LCB_ROOT ?=
LCB_PYTHON ?= $(PYTHON)
LCB_RUNNER_COMMAND_FILE ?=
LCB_ROOT_ABS := $(abspath $(LCB_ROOT))
LCB_RUNNER_COMMAND_FILE_ABS := $(abspath $(LCB_RUNNER_COMMAND_FILE))
livecodebench-contract-test: livecodebench-target-test
	$(PYTHON) scripts/livecodebench_eval_test.py

livecodebench-target-test:
	$(PYTHON) scripts/livecodebench_make_targets_test.py

livecodebench-validate:
	@test -n "$(LCB_ROOT)" || { echo "LCB_ROOT must name the pinned LiveCodeBench checkout" >&2; exit 2; }
	cd "$(LCB_ROOT_ABS)" && PYTHONPATH="$(LCB_ROOT_ABS)$${PYTHONPATH:+:$${PYTHONPATH}}" $(LCB_PYTHON) "$(CURDIR)/scripts/livecodebench_eval.py" validate --lcb-root "$(LCB_ROOT_ABS)"

livecodebench-run:
	@test -n "$(LCB_ROOT)" && test -n "$(LCB_RUNNER_COMMAND_FILE)" || { echo "LCB_ROOT and LCB_RUNNER_COMMAND_FILE are required" >&2; exit 2; }
	cd "$(LCB_ROOT_ABS)" && PYTHONPATH="$(LCB_ROOT_ABS)$${PYTHONPATH:+:$${PYTHONPATH}}" $(LCB_PYTHON) "$(CURDIR)/scripts/livecodebench_eval.py" run --lcb-root "$(LCB_ROOT_ABS)" --runner-command-file "$(LCB_RUNNER_COMMAND_FILE_ABS)"

eks-help:
	@echo "EKS delivery targets (approved target policy; no default kubeconfig/context):"
	@echo "  eks-preflight          read-only: tools, AWS session, cluster, namespace RBAC"
	@echo "  eks-render             read-only: deterministic digest-pinned manifest -> evidence"
	@echo "  eks-plan               read-only: render plus server-side dry-run -> evidence"
	@echo "  eks-apply-staging      mutating staging only: requires EKS_CONFIRM=STAGING_APPLY"
	@echo "  eks-rollout-status     read-only: namespace workload status -> evidence"
	@echo "  eks-smoke-staging      protected arbitrary-script smoke; requires EKS_CONFIRM=STAGING_APPLY"
	@echo "  eks-rollback-staging   mutating staging only: requires validated supply-chain evidence, confirmation, digest, approved pod-template SHA-256"
	@echo "  eks-promotion-plan     read-only: requires passed apply + smoke evidence; never applies production"
	@echo "  production-promotion-validate read-only: validates evidence manifest; never deploys production"
	@echo "Required: an approved EKS_DELIVERY_AWS_PROFILE and protected staging target policy Parameter."
	@echo "  eks-supply-chain-validate read-only: validates digest/release-binding/SBOM/provenance/signature/scan evidence"
	@echo "eks-supply-chain-validate also requires EKS_SUPPLY_CHAIN_DIR; architecture comes only from the approved target policy."
	@echo "Render/plan/apply/rollback/smoke/promotion-plan require IMAGE_DIGEST=<approved ECR repository>@sha256:<64 hex>."
	@echo "Apply, rollback, and protected smoke require EKS_CONFIRM=STAGING_APPLY; rollback also requires ROLLBACK_POD_TEMPLATE_SHA256."
	@echo "Raw staging evidence: EKS_EVIDENCE_DIR (default tmp/eks-evidence). Promotion handoff: separate EKS_PROMOTION_EVIDENCE_DIR (default tmp/eks-promotion-evidence) contains only evidence-promotion-plan.safe.json."

eks-preflight:
	$(EKS_DELIVERY) preflight $(EKS_ARGS)

eks-render:
	$(EKS_DELIVERY) render $(EKS_ARGS)

eks-plan:
	$(EKS_DELIVERY) plan $(EKS_ARGS)

eks-apply-staging: eks-supply-chain-validate
	$(EKS_DELIVERY) apply $(EKS_ARGS)

eks-rollout-status:
	$(EKS_DELIVERY) status $(EKS_ARGS)

eks-smoke-staging:
	@test -n "$${EKS_SMOKE_COMMAND_FILE:-}" || { echo "EKS_SMOKE_COMMAND_FILE must name a protected mode-0600 smoke script" >&2; exit 2; }
	@$(EKS_DELIVERY) smoke $(EKS_ARGS) --smoke-command-file "$${EKS_SMOKE_COMMAND_FILE}"

eks-rollback-staging: eks-supply-chain-validate
	$(EKS_DELIVERY) rollback $(EKS_ARGS)

eks-release-evidence:
	$(EKS_DELIVERY) promotion-plan $(EKS_PROMOTION_ARGS)
	@printf '%s\n' 'Safe promotion handoff: evidence-promotion-plan.safe.json is in the separate EKS_PROMOTION_EVIDENCE_DIR; raw evidence remains in EKS_EVIDENCE_DIR.'

eks-promotion-plan:
	$(EKS_DELIVERY) promotion-plan $(EKS_PROMOTION_ARGS)

# Release automation must provide independently generated, safe evidence in this
# ignored directory. This target performs no image build, registry, AWS, or
# Kubernetes operation; it fails closed before the Make delivery contract runs.
eks-supply-chain-validate:
	$(PYTHON) scripts/validate_staging_supply_chain.py --image-digest "$${IMAGE_DIGEST}" --evidence-dir "$${EKS_SUPPLY_CHAIN_DIR}"

production-promotion-validate:
	@$(PYTHON) scripts/validate_production_promotion.py --from-make-environment

ci-eks-staging-contract:
	$(PYTHON) scripts/eks_delivery_contract_test.py
	$(PYTHON) scripts/makefile_security_test.py
	$(PYTHON) scripts/validate_staging_supply_chain_test.py
	$(PYTHON) scripts/validate_eks_staging_workflow_test.py
	$(PYTHON) scripts/validate_production_promotion_test.py
	$(PYTHON) scripts/eks_promotion_evidence_integration_test.py
	$(PYTHON) scripts/validate_eks_staging_workflow.py

capability-smoke: capability-smoke-unit

# This mock-first contract is offline and credential-free. SKIP_TESTS=true is
# the only bypass; it is explicit, noisy, and cannot enable provider traffic.
capability-smoke-unit:
	@if [ "$${SKIP_TESTS:-false}" = "true" ]; then \
		echo "WARNING: SKIP_TESTS=true skips capability-smoke-unit (synthetic manifests, evidence verifier, redaction checks)"; \
	elif [ "$${SKIP_TESTS:-false}" = "false" ]; then \
		$(PYTHON) scripts/provider_capability_smoke.py unit; \
		$(PYTHON) scripts/provider_capability_smoke_test.py; \
		GOOS=$(HOST_GOOS) GOARCH=$(HOST_GOARCH) go test ./internal/router -run 'TestVerifyCapability'; \
	else \
		echo "SKIP_TESTS must be true or false" >&2; exit 2; \
	fi

# Reserved for a separately reviewed protected-live implementation. It always
# fails closed and the invoked script has no network, credential, or write path.
capability-smoke-live:
	@$(PYTHON) scripts/provider_capability_smoke.py live

test: secret-check capability-smoke-unit
	go test ./...
	python3 scripts/outcome_calibrated_policy_test.py
	python3 scripts/api_compat_bootstrap_test.py
	$(MAKE) api-compat-mock \
		$(call api_compat_make_data,API_COMPAT_BOOTSTRAP_GO_PROXY) \
		$(call api_compat_make_data,API_COMPAT_BOOTSTRAP_GO_SUMDB)

# Provision the locked Python and Go dependency sets before entering the
# isolated conformance run. API_COMPAT_BOOTSTRAP_GO_PROXY and
# API_COMPAT_BOOTSTRAP_GO_SUMDB permit an approved internal mirror; they apply
# only here, never while the suite is running offline.
API_COMPAT_BOOTSTRAP_GO_PROXY ?= https://proxy.golang.org
API_COMPAT_BOOTSTRAP_GO_SUMDB ?= sum.golang.org
# Command-line variables otherwise propagate through Make's recursive
# environment handling, which may expand their Make syntax before this recipe.
# Keep them Make-local and inject the literal values only into `go mod download`.
unexport API_COMPAT_BOOTSTRAP_GO_PROXY API_COMPAT_BOOTSTRAP_GO_SUMDB
# Quote raw Make values as one POSIX-shell word at the sole consumer. In
# particular, $(value ...) prevents a command-line override containing Make
# syntax from being expanded before the shell sees it.
api_compat_shell_data = '$(subst ','"'"',$(value $(1)))'
api_compat_make_data = $(1)=$(call api_compat_shell_data,$(1))
api-compat-bootstrap:
	@api_compat_root=$${API_COMPAT_BOOTSTRAP_ROOT:-$$(mktemp -d)}; \
	cd tests/api_compat && \
	env -u API_COMPAT_BOOTSTRAP_GO_PROXY -u API_COMPAT_BOOTSTRAP_GO_SUMDB \
	UV_CACHE_DIR=$${UV_CACHE_DIR:-$$api_compat_root/uv-cache} \
	UV_PROJECT_ENVIRONMENT=$${UV_PROJECT_ENVIRONMENT:-$$api_compat_root/venv} \
	uv sync --locked && \
	API_COMPAT_BOOTSTRAP_ROOT="$$api_compat_root" \
	API_COMPAT_BOOTSTRAP_GO_PROXY=$(call api_compat_shell_data,API_COMPAT_BOOTSTRAP_GO_PROXY) \
	API_COMPAT_BOOTSTRAP_GO_SUMDB=$(call api_compat_shell_data,API_COMPAT_BOOTSTRAP_GO_SUMDB) \
	"$${MAKE:-make}" --no-print-directory -C ../.. api-compat-bootstrap-go-provision

# The approved mirror settings are consumed only by Go provisioning. Keep the
# #694 shell-data quoting at that sole consumer so explicit Make command-line
# values remain literal configuration data.
api-compat-bootstrap-go-provision:
	@GOMODCACHE=$${GOMODCACHE:-$${API_COMPAT_BOOTSTRAP_ROOT}/go-mod-cache} \
	GOCACHE=$${GOCACHE:-$${API_COMPAT_BOOTSTRAP_ROOT}/go-build-cache} \
	GOPROXY=$(call api_compat_shell_data,API_COMPAT_BOOTSTRAP_GO_PROXY) GOSUMDB=$(call api_compat_shell_data,API_COMPAT_BOOTSTRAP_GO_SUMDB) go mod download

# Deterministic caller-boundary tests: the locally built router and fake
# upstream bind to 127.0.0.1 only. This target is fail-closed: its Python and
# Go dependency resolution cannot use the network. It intentionally has no
# bootstrap prerequisite so clean-cache failure remains directly testable.
api-compat-mock-offline:
	cd tests/api_compat && PYTHONDONTWRITEBYTECODE=1 UV_OFFLINE=1 GOPROXY=off GOSUMDB=off uv run --locked --offline pytest -p no:cacheprovider

# Keep the default target usable on a clean supported runner while preserving
# the separately invokable fail-closed offline conformance phase. The shared
# disposable directory prevents ordinary invocations from creating .venv or
# dependency caches in the source tree.
api-compat-mock:
	@api_compat_root=$$(mktemp -d); \
	trap 'rm -rf "$$api_compat_root"' EXIT; \
	$(MAKE) api-compat-bootstrap API_COMPAT_BOOTSTRAP_ROOT="$$api_compat_root" \
		$(call api_compat_make_data,API_COMPAT_BOOTSTRAP_GO_PROXY) \
		$(call api_compat_make_data,API_COMPAT_BOOTSTRAP_GO_SUMDB) && \
	UV_CACHE_DIR="$$api_compat_root/uv-cache" \
	UV_PROJECT_ENVIRONMENT="$$api_compat_root/venv" \
	GOMODCACHE="$$api_compat_root/go-mod-cache" \
	GOCACHE="$$api_compat_root/go-build-cache" \
	$(MAKE) api-compat-mock-offline

# Deliberately not a normal test/build/package target. Live execution awaits a
# human-approved non-production matrix and least-privilege caller identity.
API_COMPAT_LIVE_MATRIX ?=
API_COMPAT_LIVE_ENVIRONMENT ?=
API_COMPAT_LIVE_CALLER ?=
API_COMPAT_LIVE_BASE_URL ?=
API_COMPAT_LIVE_CREDENTIAL_FILE ?=
API_COMPAT_LIVE_CONFIRM ?=
export API_COMPAT_LIVE_MATRIX API_COMPAT_LIVE_ENVIRONMENT API_COMPAT_LIVE_CALLER API_COMPAT_LIVE_BASE_URL API_COMPAT_LIVE_CREDENTIAL_FILE API_COMPAT_LIVE_CONFIRM
api-compat-live:
	@test -n "$${API_COMPAT_LIVE_MATRIX}" && test "$${API_COMPAT_LIVE_MATRIX}" != "default" && test "$${API_COMPAT_LIVE_MATRIX}" != "all" || { echo "API_COMPAT_LIVE_MATRIX must name an approved matrix" >&2; exit 2; }
	@test -n "$${API_COMPAT_LIVE_ENVIRONMENT}" && test "$${API_COMPAT_LIVE_ENVIRONMENT}" != "production" && test "$${API_COMPAT_LIVE_ENVIRONMENT}" != "default" || { echo "API_COMPAT_LIVE_ENVIRONMENT must name a non-production environment" >&2; exit 2; }
	@test -n "$${API_COMPAT_LIVE_CALLER}" && test "$${API_COMPAT_LIVE_CALLER}" != "default" && test "$${API_COMPAT_LIVE_CALLER}" != "all" || { echo "API_COMPAT_LIVE_CALLER must name a least-privilege caller" >&2; exit 2; }
	@test -n "$${API_COMPAT_LIVE_BASE_URL}" || { echo "API_COMPAT_LIVE_BASE_URL is required" >&2; exit 2; }
	@test -f "$${API_COMPAT_LIVE_CREDENTIAL_FILE}" && test "$$(stat -c '%a' "$${API_COMPAT_LIVE_CREDENTIAL_FILE}")" = 600 || { echo "API_COMPAT_LIVE_CREDENTIAL_FILE must be a mode-0600 protected file" >&2; exit 2; }
	@test "$${API_COMPAT_LIVE_CONFIRM}" = "$${API_COMPAT_LIVE_MATRIX}:$${API_COMPAT_LIVE_ENVIRONMENT}" || { echo "API_COMPAT_LIVE_CONFIRM must bind the selected matrix and environment" >&2; exit 2; }
	@echo "api-compat-live is intentionally unavailable until its human-approved matrix is implemented." >&2; exit 2

outcome-calibrated-demo: outcome-calibrated-synthetic-demo

outcome-calibrated-synthetic-demo:
	python3 scripts/run_outcome_calibrated_demo.py --out-dir tmp/outcome-calibrated-demo

secret-check:
	python3 scripts/check_env_example_secrets.py
	python3 scripts/check_env_example_secrets_test.py
	python3 scripts/validate_package_contents_test.py
	python3 scripts/validate_release_clean_test.py

	python3 scripts/validate_docker_context.py
	python3 scripts/harness_security_test.py
	python3 scripts/makefile_security_test.py
	python3 scripts/bootstrap_eks_session_test.py
	python3 scripts/eks_discover_test.py
	python3 scripts/validate_eks_make_args_test.py
	python3 scripts/validate_eks_bootstrap_assets.py
	python3 scripts/render_tenant_ingress_network_policy_test.py
	python3 scripts/render_tenant_linkerd_policy_test.py
	python3 scripts/apply_tenant_network_policies_test.py
	python3 scripts/eks_delivery_contract_test.py
	python3 scripts/validate_production_promotion_test.py
	python3 scripts/eks_promotion_evidence_integration_test.py
	python3 scripts/check_license_skus.py
	$(MAKE) validate-build-metadata


validate-build-metadata:
	python3 scripts/validate_build_metadata.py

validate-release-clean:
	python3 scripts/validate_release_clean.py

release-notes-from-git:
	python3 scripts/release_notes_from_git.py

release-validation-matrix:
	python3 scripts/validate_release_matrix.py

docs-diag-schema:
	go run ./cmd/docs-diag-schema --write

docs-diag-schema-check:
	go run ./cmd/docs-diag-schema --check

docs-qa: docs-diag-schema-check
	python3 scripts/check_docs_public_face.py
	python3 scripts/validate_docs_versioning.py
	python3 scripts/check_docs_sidebar.py
	go run ./cmd/docs-config-example-check

docs-build: docs-qa
	cd "$(DOCS_SITE_DIR)" && npm ci && DOCS_ROUTER_VERSION="$${VERSION}" DOCS_ROUTER_BUILD_DATE="$${BUILD_DATE}" npm run build
	find "$(DOCS_EMBED_DIR)" -mindepth 1 ! -name .keep -exec rm -rf {} +
	cp -R "$(DOCS_SITE_DIR)/build/." "$(DOCS_EMBED_DIR)/"

docs-dev:
	cd "$(DOCS_SITE_DIR)" && npm install && npm run start

docs-clean:
	rm -rf "$(DOCS_SITE_DIR)/build" "$(DOCS_SITE_DIR)/.docusaurus"
	find "$(DOCS_EMBED_DIR)" -mindepth 1 ! -name .keep -exec rm -rf {} +

admin-build:
	rm -rf internal/router/admindist/static/assets
	npm ci --prefix internal/router/admindist/web
	npm run build --prefix internal/router/admindist/web

admin-e2e: admin-build
	npx --prefix internal/router/admindist/web playwright install chromium
	npm run e2e --prefix internal/router/admindist/web

build: docs-build admin-build capability-smoke-unit
	$(MAKE) validate-build-metadata
	go build -ldflags "$(BUILD_LDFLAGS)" -o router ./cmd/router
	go build -ldflags "$(BUILD_LDFLAGS)" -o router-token-gen ./cmd/router-token-gen
	go build -ldflags "$(BUILD_LDFLAGS)" -o router-usage-report ./cmd/router-usage-report
	go build -ldflags "$(BUILD_LDFLAGS)" -o metrum-smartrouterctl ./cmd/metrum-smartrouterctl

build-go-only: capability-smoke-unit
	$(MAKE) validate-build-metadata
	go build -ldflags "$(BUILD_LDFLAGS)" -o router ./cmd/router
	go build -ldflags "$(BUILD_LDFLAGS)" -o router-token-gen ./cmd/router-token-gen
	go build -ldflags "$(BUILD_LDFLAGS)" -o router-usage-report ./cmd/router-usage-report
	go build -ldflags "$(BUILD_LDFLAGS)" -o metrum-smartrouterctl ./cmd/metrum-smartrouterctl

build-all: docs-build admin-build capability-smoke-unit
	$(MAKE) validate-build-metadata
	mkdir -p "$${DIST_DIR}/build/linux-amd64" "$${DIST_DIR}/build/linux-arm64"
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "$(BUILD_LDFLAGS)" -o "$${DIST_DIR}/build/linux-amd64/router" ./cmd/router
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "$(BUILD_LDFLAGS)" -o "$${DIST_DIR}/build/linux-amd64/router-token-gen" ./cmd/router-token-gen
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "$(BUILD_LDFLAGS)" -o "$${DIST_DIR}/build/linux-amd64/router-usage-report" ./cmd/router-usage-report
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "$(BUILD_LDFLAGS)" -o "$${DIST_DIR}/build/linux-amd64/metrum-smartrouterctl" ./cmd/metrum-smartrouterctl
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags "$(BUILD_LDFLAGS)" -o "$${DIST_DIR}/build/linux-arm64/router" ./cmd/router
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags "$(BUILD_LDFLAGS)" -o "$${DIST_DIR}/build/linux-arm64/router-token-gen" ./cmd/router-token-gen
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags "$(BUILD_LDFLAGS)" -o "$${DIST_DIR}/build/linux-arm64/router-usage-report" ./cmd/router-usage-report
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags "$(BUILD_LDFLAGS)" -o "$${DIST_DIR}/build/linux-arm64/metrum-smartrouterctl" ./cmd/metrum-smartrouterctl

package: package-all

package-one: docs-build admin-build capability-smoke-unit package-one-no-docs

package-one-no-docs: capability-smoke-unit
	$(MAKE) validate-release-clean
	$(MAKE) validate-build-metadata
	pkg_dir="$${DIST_DIR}/pkg/$${PKG_NAME}-$${VERSION}-$${GOOS}-$${GOARCH}"; \
	rm -rf "$${pkg_dir}"; \
	mkdir -p "$${pkg_dir}/bin" "$${pkg_dir}/config/scripts" "$${pkg_dir}/docs" "$${pkg_dir}/caddy"; \
	CGO_ENABLED=0 GOOS="$${GOOS}" GOARCH="$${GOARCH}" go build -ldflags "$(BUILD_LDFLAGS)" -o "$${pkg_dir}/bin/router" ./cmd/router; \
	CGO_ENABLED=0 GOOS="$${GOOS}" GOARCH="$${GOARCH}" go build -ldflags "$(BUILD_LDFLAGS)" -o "$${pkg_dir}/bin/router-token-gen" ./cmd/router-token-gen; \
	CGO_ENABLED=0 GOOS="$${GOOS}" GOARCH="$${GOARCH}" go build -ldflags "$(BUILD_LDFLAGS)" -o "$${pkg_dir}/bin/router-usage-report" ./cmd/router-usage-report; \
	CGO_ENABLED=0 GOOS="$${GOOS}" GOARCH="$${GOARCH}" go build -ldflags "$(BUILD_LDFLAGS)" -o "$${pkg_dir}/bin/metrum-smartrouterctl" ./cmd/metrum-smartrouterctl; \
	cp config.example.yaml "$${pkg_dir}/config/config.example.yaml"; \
	cp env.example.json "$${pkg_dir}/config/env.example.json"; \
	cp scripts/router.ts "$${pkg_dir}/config/scripts/router.ts"; \
	cp deploy/Caddyfile "$${pkg_dir}/caddy/Caddyfile"
	while IFS= read -r doc; do \
		case "$$doc" in ""|\#*) continue ;; esac; \
		cp "$$doc" "$${DIST_DIR}/pkg/$${PKG_NAME}-$${VERSION}-$${GOOS}-$${GOARCH}/docs/$$(basename "$$doc")"; \
	done < "$(PACKAGE_DOC_ALLOWLIST)"
	find "$${DIST_DIR}/pkg/$${PKG_NAME}-$${VERSION}-$${GOOS}-$${GOARCH}" -type d -exec chmod 0755 {} \;
	find "$${DIST_DIR}/pkg/$${PKG_NAME}-$${VERSION}-$${GOOS}-$${GOARCH}" -type f -exec chmod 0644 {} \;
	chmod 0755 "$${DIST_DIR}/pkg/$${PKG_NAME}-$${VERSION}-$${GOOS}-$${GOARCH}/bin/router" "$${DIST_DIR}/pkg/$${PKG_NAME}-$${VERSION}-$${GOOS}-$${GOARCH}/bin/router-token-gen" "$${DIST_DIR}/pkg/$${PKG_NAME}-$${VERSION}-$${GOOS}-$${GOARCH}/bin/router-usage-report" "$${DIST_DIR}/pkg/$${PKG_NAME}-$${VERSION}-$${GOOS}-$${GOARCH}/bin/metrum-smartrouterctl"
	$(TAR_ENV) tar --owner=0 --group=0 --numeric-owner -C "$${DIST_DIR}/pkg" -czf "$${DIST_DIR}/$${PKG_NAME}-$${VERSION}-$${GOOS}-$${GOARCH}.tar.gz" "$${PKG_NAME}-$${VERSION}-$${GOOS}-$${GOARCH}"
	python3 scripts/validate_package_contents.py --allowlist "$(PACKAGE_DOC_ALLOWLIST)" "$${DIST_DIR}/$${PKG_NAME}-$${VERSION}-$${GOOS}-$${GOARCH}.tar.gz"

package-all: docs-build admin-build capability-smoke-unit
	GOOS=linux GOARCH=amd64 $(MAKE) package-one-no-docs
	GOOS=linux GOARCH=arm64 $(MAKE) package-one-no-docs

docker-image: docs-build admin-build capability-smoke-unit docker-image-no-docs

docker-image-no-docs: capability-smoke-unit
	$(MAKE) validate-build-metadata
	$(DOCKER_BUILDX) build --platform "$(DOCKER_PLATFORM)" --load --build-arg "VERSION=$${VERSION}" --build-arg "COMMIT=$${COMMIT}" --build-arg "BUILD_DATE=$${BUILD_DATE}" -t "$${IMAGE_NAME}:$${IMAGE_TAG}" .

package-docker: package-docker-all

package-docker-one: docs-build admin-build capability-smoke-unit package-docker-one-no-docs

package-docker-one-no-docs: capability-smoke-unit
	$(MAKE) validate-release-clean
	$(MAKE) validate-build-metadata
	docker_pkg_dir="$${DIST_DIR}/docker/$${PKG_NAME}-$${VERSION}-docker-$${GOOS}-$${GOARCH}"; \
	rm -rf "$${docker_pkg_dir}"; \
	mkdir -p "$${docker_pkg_dir}/images" "$${docker_pkg_dir}/compose" "$${docker_pkg_dir}/config/scripts" "$${docker_pkg_dir}/docs"
	GOOS="$${GOOS}" GOARCH="$${GOARCH}" DOCKER_PLATFORM="linux/$${GOARCH}" IMAGE_TAG="$${VERSION}-$${GOOS}-$${GOARCH}" $(MAKE) docker-image-no-docs
	docker_pkg_dir="$${DIST_DIR}/docker/$${PKG_NAME}-$${VERSION}-docker-$${GOOS}-$${GOARCH}"; \
	$(DOCKER) save "$${IMAGE_NAME}:$${VERSION}-$${GOOS}-$${GOARCH}" -o "$${docker_pkg_dir}/images/$${IMAGE_NAME}-$${VERSION}-$${GOOS}-$${GOARCH}.tar"; \
	cp deploy/docker-compose.yml "$${docker_pkg_dir}/compose/docker-compose.yml"; \
	cp deploy/docker-compose.postgres-localhost.yml "$${docker_pkg_dir}/compose/docker-compose.postgres-localhost.yml"; \
	cp deploy/Caddyfile.compose "$${docker_pkg_dir}/compose/Caddyfile.compose"; \
	cp deploy/compose.env.example "$${docker_pkg_dir}/compose/.env.example"; \
	sed "s/^SMART_LLMROUTER_VERSION=.*/SMART_LLMROUTER_VERSION=$${VERSION}-$${GOOS}-$${GOARCH}/" deploy/compose.env.example > "$${docker_pkg_dir}/compose/.env"; \
	cp config.example.yaml "$${docker_pkg_dir}/config/config.example.yaml"; \
	cp env.example.json "$${docker_pkg_dir}/config/env.example.json"; \
	cp scripts/router.ts "$${docker_pkg_dir}/config/scripts/router.ts"
	while IFS= read -r doc; do \
		case "$$doc" in ""|\#*) continue ;; esac; \
		cp "$$doc" "$${DIST_DIR}/docker/$${PKG_NAME}-$${VERSION}-docker-$${GOOS}-$${GOARCH}/docs/$$(basename "$$doc")"; \
	done < "$(PACKAGE_DOC_ALLOWLIST)"
	find "$${DIST_DIR}/docker/$${PKG_NAME}-$${VERSION}-docker-$${GOOS}-$${GOARCH}" -type d -exec chmod 0755 {} \;
	find "$${DIST_DIR}/docker/$${PKG_NAME}-$${VERSION}-docker-$${GOOS}-$${GOARCH}" -type f -exec chmod 0644 {} \;
	$(TAR_ENV) tar --owner=0 --group=0 --numeric-owner -C "$${DIST_DIR}/docker" -czf "$${DIST_DIR}/$${PKG_NAME}-$${VERSION}-docker-$${GOOS}-$${GOARCH}.tar.gz" "$${PKG_NAME}-$${VERSION}-docker-$${GOOS}-$${GOARCH}"
	python3 scripts/validate_package_contents.py --allowlist "$(PACKAGE_DOC_ALLOWLIST)" "$${DIST_DIR}/$${PKG_NAME}-$${VERSION}-docker-$${GOOS}-$${GOARCH}.tar.gz"

package-docker-all: docs-build admin-build capability-smoke-unit
	GOOS=linux GOARCH=amd64 $(MAKE) package-docker-one-no-docs
	GOOS=linux GOARCH=arm64 $(MAKE) package-docker-one-no-docs

compose-security-check:
	bash scripts/check_compose_security.sh

eks-session-bootstrap:
	@test -n "$$EKS_CLEANUP_RECORD" || (echo "EKS_CLEANUP_RECORD is required" >&2; exit 2)
	python3 scripts/bootstrap_eks_session.py --admin-profile "$$EKS_ADMIN_PROFILE" --source-user "$$EKS_SOURCE_USER" --mfa-serial "$$EKS_MFA_SERIAL" --macos-keychain-service "$$EKS_MFA_KEYCHAIN_SERVICE" --macos-keychain-account "$$EKS_MFA_KEYCHAIN_ACCOUNT" --session-profile smartrouter --role-profile "$$EKS_AWS_PROFILE" --role-arn "arn:aws:iam::$$EKS_ACCOUNT_ID:role/genai-smart-router-eks-discovery" --region "$$EKS_REGION" --duration-seconds "$$EKS_SESSION_DURATION" --cleanup-record "$$EKS_CLEANUP_RECORD"

eks-session-recovery-status:
	@test -n "$$EKS_CLEANUP_RECORD" || (echo "EKS_CLEANUP_RECORD is required" >&2; exit 2)
	python3 scripts/bootstrap_eks_session.py --recovery-status "$$EKS_CLEANUP_RECORD"

eks-identity-check:
	python3 scripts/validate_eks_make_args.py --identity-only --profile "$$EKS_AWS_PROFILE" --account-id "$$EKS_ACCOUNT_ID" --region "$$EKS_REGION"
	@identity="$$(aws --profile "$$EKS_AWS_PROFILE" --region "$$EKS_REGION" sts get-caller-identity --query Arn --output text)"; \
	case "$$identity" in \
	"arn:aws:sts::$$EKS_ACCOUNT_ID:assumed-role/genai-smart-router-eks-discovery/"*) ;; \
	*) echo "EKS identity error: expected the genai-smart-router-eks-discovery assumed role" >&2; exit 1 ;; \
	esac

eks-discovery-validate:
	python3 scripts/validate_eks_make_args.py --profile "$$EKS_AWS_PROFILE" --account-id "$$EKS_ACCOUNT_ID" --region "$$EKS_REGION" --cluster "$$EKS_CLUSTER" --namespace "$$EKS_NAMESPACE" --linkerd-namespace "$$EKS_LINKERD_NAMESPACE" --ingress-namespace "$$EKS_INGRESS_NAMESPACE" --ingress-service-account "$$EKS_INGRESS_SERVICE_ACCOUNT" --ingress-deployment "$$EKS_INGRESS_DEPLOYMENT" --linkerd-trust-domain "$$EKS_LINKERD_TRUST_DOMAIN" --ecr-repository "$$EKS_ECR_REPOSITORY" --output "$$EKS_DISCOVERY_OUTPUT"

eks-discover: eks-discovery-validate
	if [ -n "$$EKS_LINKERD_NAMESPACE" ]; then \
		python3 scripts/eks_discover.py --profile "$$EKS_AWS_PROFILE" --account-id "$$EKS_ACCOUNT_ID" --region "$$EKS_REGION" --cluster "$$EKS_CLUSTER" --namespace "$$EKS_NAMESPACE" --linkerd-namespace "$$EKS_LINKERD_NAMESPACE" --ingress-namespace "$$EKS_INGRESS_NAMESPACE" --ingress-service-account "$$EKS_INGRESS_SERVICE_ACCOUNT" --ingress-deployment "$$EKS_INGRESS_DEPLOYMENT" --linkerd-trust-domain "$$EKS_LINKERD_TRUST_DOMAIN" --ecr-repository "$$EKS_ECR_REPOSITORY" --output "$$EKS_DISCOVERY_OUTPUT"; \
	else \
		python3 scripts/eks_discover.py --profile "$$EKS_AWS_PROFILE" --account-id "$$EKS_ACCOUNT_ID" --region "$$EKS_REGION" --cluster "$$EKS_CLUSTER" --namespace "$$EKS_NAMESPACE" --ingress-namespace "$$EKS_INGRESS_NAMESPACE" --ecr-repository "$$EKS_ECR_REPOSITORY" --output "$$EKS_DISCOVERY_OUTPUT"; \
	fi

eks-render-ingress-network-policy:
	@test -n "$$EKS_DISCOVERY_OUTPUT" || (echo "EKS_DISCOVERY_OUTPUT is required" >&2; exit 2)
	@test -n "$$EKS_INGRESS_NETWORK_POLICY_OUTPUT" || (echo "EKS_INGRESS_NETWORK_POLICY_OUTPUT is required" >&2; exit 2)
	python3 scripts/render_tenant_ingress_network_policy.py --discovery-report "$$EKS_DISCOVERY_OUTPUT" --output "$$EKS_INGRESS_NETWORK_POLICY_OUTPUT"

eks-render-linkerd-policy: eks-render-ingress-network-policy
	@test -n "$$EKS_LINKERD_POLICY_OUTPUT" || (echo "EKS_LINKERD_POLICY_OUTPUT is required" >&2; exit 2)
	python3 scripts/render_tenant_linkerd_policy.py --discovery-report "$$EKS_DISCOVERY_OUTPUT" --output "$$EKS_LINKERD_POLICY_OUTPUT"

eks-validate-tenant-network-policies:
	@test -n "$$EKS_DISCOVERY_OUTPUT" || (echo "EKS_DISCOVERY_OUTPUT is required" >&2; exit 2)
	@test -n "$$EKS_POLICY_AWS_PROFILE" || (echo "EKS_POLICY_AWS_PROFILE is required" >&2; exit 2)
	@test -n "$$EKS_POLICY_KUBECONFIG" || (echo "EKS_POLICY_KUBECONFIG is required" >&2; exit 2)
	@test -n "$$EKS_POLICY_CONTEXT" || (echo "EKS_POLICY_CONTEXT is required" >&2; exit 2)
	@test -n "$$EKS_INGRESS_NETWORK_POLICY_OUTPUT" || (echo "EKS_INGRESS_NETWORK_POLICY_OUTPUT is required" >&2; exit 2)
	@if [ -n "$$EKS_LINKERD_POLICY_OUTPUT" ]; then \
		python3 scripts/apply_tenant_network_policies.py --discovery-report "$$EKS_DISCOVERY_OUTPUT" --profile "$$EKS_POLICY_AWS_PROFILE" --kubeconfig "$$EKS_POLICY_KUBECONFIG" --context "$$EKS_POLICY_CONTEXT" --ingress-policy "$$EKS_INGRESS_NETWORK_POLICY_OUTPUT" --linkerd-policy "$$EKS_LINKERD_POLICY_OUTPUT"; \
	else \
		python3 scripts/apply_tenant_network_policies.py --discovery-report "$$EKS_DISCOVERY_OUTPUT" --profile "$$EKS_POLICY_AWS_PROFILE" --kubeconfig "$$EKS_POLICY_KUBECONFIG" --context "$$EKS_POLICY_CONTEXT" --ingress-policy "$$EKS_INGRESS_NETWORK_POLICY_OUTPUT"; \
	fi

eks-apply-tenant-network-policies:
	@test "$$EKS_POLICY_APPLY_CONFIRM" = "apply" || (echo "EKS_POLICY_APPLY_CONFIRM=apply is required" >&2; exit 2)
	@test -n "$$EKS_DISCOVERY_OUTPUT" || (echo "EKS_DISCOVERY_OUTPUT is required" >&2; exit 2)
	@test -n "$$EKS_POLICY_AWS_PROFILE" || (echo "EKS_POLICY_AWS_PROFILE is required" >&2; exit 2)
	@test -n "$$EKS_POLICY_KUBECONFIG" || (echo "EKS_POLICY_KUBECONFIG is required" >&2; exit 2)
	@test -n "$$EKS_POLICY_CONTEXT" || (echo "EKS_POLICY_CONTEXT is required" >&2; exit 2)
	@test -n "$$EKS_INGRESS_NETWORK_POLICY_OUTPUT" || (echo "EKS_INGRESS_NETWORK_POLICY_OUTPUT is required" >&2; exit 2)
	@if [ -n "$$EKS_LINKERD_POLICY_OUTPUT" ]; then \
		python3 scripts/apply_tenant_network_policies.py --discovery-report "$$EKS_DISCOVERY_OUTPUT" --profile "$$EKS_POLICY_AWS_PROFILE" --kubeconfig "$$EKS_POLICY_KUBECONFIG" --context "$$EKS_POLICY_CONTEXT" --ingress-policy "$$EKS_INGRESS_NETWORK_POLICY_OUTPUT" --linkerd-policy "$$EKS_LINKERD_POLICY_OUTPUT" --apply; \
	else \
		python3 scripts/apply_tenant_network_policies.py --discovery-report "$$EKS_DISCOVERY_OUTPUT" --profile "$$EKS_POLICY_AWS_PROFILE" --kubeconfig "$$EKS_POLICY_KUBECONFIG" --context "$$EKS_POLICY_CONTEXT" --ingress-policy "$$EKS_INGRESS_NETWORK_POLICY_OUTPUT" --apply; \
	fi

e2e-mock:
	$(MAKE) -C examples/cli-e2e-c clean test

e2e-live-c: build
	bash scripts/live_cli_c_e2e.sh

e2e-live-full: build
	bash scripts/live_full_e2e.sh

e2e-compose-live:
	bash scripts/compose_live_e2e.sh

clean:
	rm -rf router router-token router-token-gen router-usage-report examples/cli-e2e-c/cli-e2e "$${DIST_DIR}"
	rm -rf "$(DOCS_SITE_DIR)/build" "$(DOCS_SITE_DIR)/.docusaurus"
	find "$(DOCS_EMBED_DIR)" -mindepth 1 ! -name .keep -exec rm -rf {} +
