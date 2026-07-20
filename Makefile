VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS ?= -X smart-llmrouter/internal/buildinfo.Version=$(VERSION) -X smart-llmrouter/internal/buildinfo.Commit=$(COMMIT) -X smart-llmrouter/internal/buildinfo.BuildDate=$(BUILD_DATE)
DIST_DIR ?= dist
PKG_NAME ?= smart-llmrouter
GOOS ?= linux
GOARCH ?= $(shell go env GOARCH)
PYTHON ?= python3

# Explicit inputs for all EKS commands. No target reads the current kubectl
# context; scripts/eks_delivery.py creates and removes its own kubeconfig.
# The AWS account, region, cluster, namespace, overlay, workload, and role are
# not Make variables: they are pinned in the reviewed target policy and its
# independently protected SSM copy.
EKS_AWS_PROFILE ?= genai-smart-router-eks-staging-delivery
IMAGE_DIGEST ?=
EKS_CONFIRM ?=
EKS_EVIDENCE_DIR ?= tmp/eks-evidence
EKS_SMOKE_COMMAND ?=
EKS_DELIVERY = $(PYTHON) scripts/eks_delivery.py
EKS_ARGS = --aws-profile "$(EKS_AWS_PROFILE)" --image-digest "$(IMAGE_DIGEST)" --confirm "$(EKS_CONFIRM)" --evidence-dir "$(EKS_EVIDENCE_DIR)"

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
export VERSION COMMIT BUILD_DATE DIST_DIR PKG_NAME GOOS GOARCH IMAGE_NAME IMAGE_TAG PYTHON AWS_REGION EKS_CLUSTER K8S_NAMESPACE KUSTOMIZE_OVERLAY ENVIRONMENT IMAGE_DIGEST EKS_CONFIRM EKS_EVIDENCE_DIR EKS_SMOKE_COMMAND EKS_AWS_PROFILE EKS_ACCOUNT_ID EKS_REGION EKS_NAMESPACE EKS_LINKERD_NAMESPACE EKS_INGRESS_NAMESPACE EKS_INGRESS_SERVICE_ACCOUNT EKS_INGRESS_DEPLOYMENT EKS_LINKERD_TRUST_DOMAIN EKS_ECR_REPOSITORY EKS_DISCOVERY_OUTPUT EKS_LINKERD_POLICY_OUTPUT EKS_INGRESS_NETWORK_POLICY_OUTPUT EKS_POLICY_AWS_PROFILE EKS_POLICY_KUBECONFIG EKS_POLICY_CONTEXT EKS_POLICY_APPLY_CONFIRM EKS_ADMIN_PROFILE EKS_SOURCE_USER EKS_MFA_SERIAL EKS_MFA_KEYCHAIN_SERVICE EKS_MFA_KEYCHAIN_ACCOUNT EKS_SESSION_DURATION
export COPYFILE_DISABLE
TAR_ENV := COPYFILE_DISABLE=1

BUILD_LDFLAGS = -X smart-llmrouter/internal/buildinfo.Version=$${VERSION} -X smart-llmrouter/internal/buildinfo.Commit=$${COMMIT} -X smart-llmrouter/internal/buildinfo.BuildDate=$${BUILD_DATE}

.PHONY: help eks-help eks-preflight eks-render eks-plan eks-apply-staging eks-rollout-status eks-smoke-staging eks-rollback-staging eks-release-evidence eks-promotion-plan test outcome-calibrated-demo outcome-calibrated-synthetic-demo secret-check validate-build-metadata validate-release-clean release-validation-matrix release-notes-from-git docs-diag-schema docs-diag-schema-check docs-qa docs-build docs-dev docs-clean admin-build admin-e2e build build-go-only build-all package package-one package-one-no-docs package-all docker-image docker-image-no-docs package-docker package-docker-one package-docker-one-no-docs package-docker-all compose-security-check eks-session-bootstrap eks-session-recovery-status eks-identity-check eks-discovery-validate eks-discover eks-render-ingress-network-policy eks-render-linkerd-policy eks-validate-tenant-network-policies eks-apply-tenant-network-policies e2e-mock e2e-live-c e2e-live-full e2e-compose-live clean

help: eks-help

eks-help:
	@echo "EKS delivery targets (approved target policy; no default kubeconfig/context):"
	@echo "  eks-preflight          read-only: tools, AWS session, cluster, namespace RBAC"
	@echo "  eks-render             read-only: deterministic digest-pinned manifest -> evidence"
	@echo "  eks-plan               read-only: render plus server-side dry-run -> evidence"
	@echo "  eks-apply-staging      mutating staging only: requires EKS_CONFIRM=STAGING_APPLY"
	@echo "  eks-rollout-status     read-only: namespace workload status -> evidence"
	@echo "  eks-smoke-staging      read-only smoke using protected EKS_SMOKE_COMMAND"
	@echo "  eks-rollback-staging   mutating staging only: requires EKS_CONFIRM=STAGING_APPLY"
	@echo "  eks-promotion-plan     read-only: requires passed apply + smoke evidence; never applies production"
	@echo "Required: an approved EKS_AWS_PROFILE and protected staging target policy Parameter."
	@echo "Render/plan/apply/smoke/promotion-plan require IMAGE_DIGEST=registry/image@sha256:<64 hex>."
	@echo "Evidence: EKS_EVIDENCE_DIR (default tmp/eks-evidence); redacted JSON/Markdown bind digest, rendered config fingerprint, and live pod-template state."

eks-preflight:
	$(EKS_DELIVERY) preflight $(EKS_ARGS)

eks-render:
	$(EKS_DELIVERY) render $(EKS_ARGS)

eks-plan:
	$(EKS_DELIVERY) plan $(EKS_ARGS)

eks-apply-staging:
	$(EKS_DELIVERY) apply $(EKS_ARGS)

eks-rollout-status:
	$(EKS_DELIVERY) status $(EKS_ARGS)

eks-smoke-staging:
	$(EKS_DELIVERY) smoke $(EKS_ARGS) --smoke-command "$(EKS_SMOKE_COMMAND)"

eks-rollback-staging:
	$(EKS_DELIVERY) rollback $(EKS_ARGS)

eks-release-evidence:
	$(EKS_DELIVERY) promotion-plan $(EKS_ARGS)
	@echo "Safe evidence: $(EKS_EVIDENCE_DIR)/evidence-{apply,smoke}.json and matching Markdown summaries"

eks-promotion-plan:
	$(EKS_DELIVERY) promotion-plan $(EKS_ARGS)

test: secret-check
	go test ./...
	python3 scripts/outcome_calibrated_policy_test.py

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

build: docs-build admin-build
	$(MAKE) validate-build-metadata
	go build -ldflags "$(BUILD_LDFLAGS)" -o router ./cmd/router
	go build -ldflags "$(BUILD_LDFLAGS)" -o router-token-gen ./cmd/router-token-gen
	go build -ldflags "$(BUILD_LDFLAGS)" -o router-usage-report ./cmd/router-usage-report

build-go-only:
	$(MAKE) validate-build-metadata
	go build -ldflags "$(BUILD_LDFLAGS)" -o router ./cmd/router
	go build -ldflags "$(BUILD_LDFLAGS)" -o router-token-gen ./cmd/router-token-gen
	go build -ldflags "$(BUILD_LDFLAGS)" -o router-usage-report ./cmd/router-usage-report

build-all: docs-build admin-build
	$(MAKE) validate-build-metadata
	mkdir -p "$${DIST_DIR}/build/linux-amd64" "$${DIST_DIR}/build/linux-arm64"
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "$(BUILD_LDFLAGS)" -o "$${DIST_DIR}/build/linux-amd64/router" ./cmd/router
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "$(BUILD_LDFLAGS)" -o "$${DIST_DIR}/build/linux-amd64/router-token-gen" ./cmd/router-token-gen
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "$(BUILD_LDFLAGS)" -o "$${DIST_DIR}/build/linux-amd64/router-usage-report" ./cmd/router-usage-report
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags "$(BUILD_LDFLAGS)" -o "$${DIST_DIR}/build/linux-arm64/router" ./cmd/router
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags "$(BUILD_LDFLAGS)" -o "$${DIST_DIR}/build/linux-arm64/router-token-gen" ./cmd/router-token-gen
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags "$(BUILD_LDFLAGS)" -o "$${DIST_DIR}/build/linux-arm64/router-usage-report" ./cmd/router-usage-report

package: package-all

package-one: docs-build admin-build package-one-no-docs

package-one-no-docs:
	$(MAKE) validate-release-clean
	$(MAKE) validate-build-metadata
	pkg_dir="$${DIST_DIR}/pkg/$${PKG_NAME}-$${VERSION}-$${GOOS}-$${GOARCH}"; \
	rm -rf "$${pkg_dir}"; \
	mkdir -p "$${pkg_dir}/bin" "$${pkg_dir}/config/scripts" "$${pkg_dir}/docs" "$${pkg_dir}/caddy"; \
	CGO_ENABLED=0 GOOS="$${GOOS}" GOARCH="$${GOARCH}" go build -ldflags "$(BUILD_LDFLAGS)" -o "$${pkg_dir}/bin/router" ./cmd/router; \
	CGO_ENABLED=0 GOOS="$${GOOS}" GOARCH="$${GOARCH}" go build -ldflags "$(BUILD_LDFLAGS)" -o "$${pkg_dir}/bin/router-token-gen" ./cmd/router-token-gen; \
	CGO_ENABLED=0 GOOS="$${GOOS}" GOARCH="$${GOARCH}" go build -ldflags "$(BUILD_LDFLAGS)" -o "$${pkg_dir}/bin/router-usage-report" ./cmd/router-usage-report; \
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
	chmod 0755 "$${DIST_DIR}/pkg/$${PKG_NAME}-$${VERSION}-$${GOOS}-$${GOARCH}/bin/router" "$${DIST_DIR}/pkg/$${PKG_NAME}-$${VERSION}-$${GOOS}-$${GOARCH}/bin/router-token-gen" "$${DIST_DIR}/pkg/$${PKG_NAME}-$${VERSION}-$${GOOS}-$${GOARCH}/bin/router-usage-report"
	$(TAR_ENV) tar --owner=0 --group=0 --numeric-owner -C "$${DIST_DIR}/pkg" -czf "$${DIST_DIR}/$${PKG_NAME}-$${VERSION}-$${GOOS}-$${GOARCH}.tar.gz" "$${PKG_NAME}-$${VERSION}-$${GOOS}-$${GOARCH}"
	python3 scripts/validate_package_contents.py --allowlist "$(PACKAGE_DOC_ALLOWLIST)" "$${DIST_DIR}/$${PKG_NAME}-$${VERSION}-$${GOOS}-$${GOARCH}.tar.gz"

package-all: docs-build admin-build
	GOOS=linux GOARCH=amd64 $(MAKE) package-one-no-docs
	GOOS=linux GOARCH=arm64 $(MAKE) package-one-no-docs

docker-image: docs-build admin-build docker-image-no-docs

docker-image-no-docs:
	$(MAKE) validate-build-metadata
	$(DOCKER_BUILDX) build --platform "$(DOCKER_PLATFORM)" --load --build-arg "VERSION=$${VERSION}" --build-arg "COMMIT=$${COMMIT}" --build-arg "BUILD_DATE=$${BUILD_DATE}" -t "$${IMAGE_NAME}:$${IMAGE_TAG}" .

package-docker: package-docker-all

package-docker-one: docs-build admin-build package-docker-one-no-docs

package-docker-one-no-docs:
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

package-docker-all: docs-build admin-build
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
