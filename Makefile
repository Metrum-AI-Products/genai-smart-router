VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS ?= -X smart-llmrouter/internal/buildinfo.Version=$(VERSION) -X smart-llmrouter/internal/buildinfo.Commit=$(COMMIT) -X smart-llmrouter/internal/buildinfo.BuildDate=$(BUILD_DATE)
DIST_DIR ?= dist
PKG_NAME ?= smart-llmrouter
GOOS ?= linux
GOARCH ?= $(shell go env GOARCH)

DOCKER ?= docker
DOCKER_BUILDX ?= $(DOCKER) buildx
DOCKER_PLATFORM ?= linux/$(GOARCH)
IMAGE_NAME ?= smart-llmrouter
IMAGE_TAG ?= $(VERSION)-$(GOOS)-$(GOARCH)
DOCS_SITE_DIR ?= docs-site
DOCS_EMBED_DIR ?= internal/router/docsdist
PACKAGE_DOC_ALLOWLIST ?= scripts/package_docs_allowlist.txt
COPYFILE_DISABLE ?= 1
export VERSION COMMIT BUILD_DATE DIST_DIR PKG_NAME GOOS GOARCH IMAGE_NAME IMAGE_TAG
export COPYFILE_DISABLE
TAR_ENV := COPYFILE_DISABLE=1

BUILD_LDFLAGS = -X smart-llmrouter/internal/buildinfo.Version=$${VERSION} -X smart-llmrouter/internal/buildinfo.Commit=$${COMMIT} -X smart-llmrouter/internal/buildinfo.BuildDate=$${BUILD_DATE}

.PHONY: test secret-check validate-build-metadata validate-release-clean docs-qa docs-build docs-dev docs-clean admin-build admin-e2e build build-go-only build-all package package-one package-one-no-docs package-all docker-image docker-image-no-docs package-docker package-docker-one package-docker-one-no-docs package-docker-all compose-security-check e2e-mock e2e-live-c e2e-live-full e2e-compose-live clean

test: secret-check
	go test ./...

secret-check:
	python3 scripts/check_env_example_secrets.py
	python3 scripts/check_env_example_secrets_test.py
	python3 scripts/validate_package_contents_test.py
	python3 scripts/validate_release_clean_test.py
	python3 scripts/validate_docker_context.py
	python3 scripts/harness_security_test.py
	python3 scripts/makefile_security_test.py
	python3 scripts/check_license_skus.py
	$(MAKE) validate-build-metadata

validate-build-metadata:
	python3 scripts/validate_build_metadata.py

validate-release-clean:
	python3 scripts/validate_release_clean.py

docs-qa:
	python3 scripts/check_docs_public_face.py

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
