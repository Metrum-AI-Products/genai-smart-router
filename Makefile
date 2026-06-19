VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS ?= -X smart-llmrouter/internal/buildinfo.Version=$(VERSION) -X smart-llmrouter/internal/buildinfo.Commit=$(COMMIT) -X smart-llmrouter/internal/buildinfo.BuildDate=$(BUILD_DATE)
DIST_DIR ?= dist
PKG_NAME ?= smart-llmrouter
GOOS ?= linux
GOARCH ?= $(shell go env GOARCH)

DOCKER ?= docker
DOCKER_PLATFORM ?= linux/$(GOARCH)
IMAGE_NAME ?= smart-llmrouter
IMAGE_TAG ?= $(VERSION)-$(GOOS)-$(GOARCH)
DOCS_SITE_DIR ?= docs-site
DOCS_EMBED_DIR ?= internal/router/docsdist

.PHONY: test docs-build docs-dev docs-clean build build-go-only build-all package package-all docker-image package-docker package-docker-all e2e-mock e2e-live-c e2e-live-full e2e-compose-live clean

test:
	go test ./...

docs-build:
	cd $(DOCS_SITE_DIR) && npm ci && DOCS_ROUTER_VERSION=$(VERSION) DOCS_ROUTER_COMMIT=$(COMMIT) DOCS_ROUTER_BUILD_DATE=$(BUILD_DATE) npm run build
	find $(DOCS_EMBED_DIR) -mindepth 1 ! -name .keep -exec rm -rf {} +
	cp -R $(DOCS_SITE_DIR)/build/. $(DOCS_EMBED_DIR)/

docs-dev:
	cd $(DOCS_SITE_DIR) && npm install && npm run start

docs-clean:
	rm -rf $(DOCS_SITE_DIR)/build $(DOCS_SITE_DIR)/.docusaurus
	find $(DOCS_EMBED_DIR) -mindepth 1 ! -name .keep -exec rm -rf {} +

build: docs-build
	go build -ldflags "$(LDFLAGS)" -o router ./cmd/router
	go build -ldflags "$(LDFLAGS)" -o router-token-gen ./cmd/router-token-gen
	go build -ldflags "$(LDFLAGS)" -o router-usage-report ./cmd/router-usage-report

build-go-only:
	go build -ldflags "$(LDFLAGS)" -o router ./cmd/router
	go build -ldflags "$(LDFLAGS)" -o router-token-gen ./cmd/router-token-gen
	go build -ldflags "$(LDFLAGS)" -o router-usage-report ./cmd/router-usage-report

build-all: docs-build
	mkdir -p $(DIST_DIR)/build/linux-amd64 $(DIST_DIR)/build/linux-arm64
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/build/linux-amd64/router ./cmd/router
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/build/linux-amd64/router-token-gen ./cmd/router-token-gen
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/build/linux-amd64/router-usage-report ./cmd/router-usage-report
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/build/linux-arm64/router ./cmd/router
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/build/linux-arm64/router-token-gen ./cmd/router-token-gen
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/build/linux-arm64/router-usage-report ./cmd/router-usage-report

package: docs-build
	rm -rf $(DIST_DIR)/pkg/$(PKG_NAME)-$(VERSION)-$(GOOS)-$(GOARCH)
	mkdir -p $(DIST_DIR)/pkg/$(PKG_NAME)-$(VERSION)-$(GOOS)-$(GOARCH)/bin
	mkdir -p $(DIST_DIR)/pkg/$(PKG_NAME)-$(VERSION)-$(GOOS)-$(GOARCH)/config/scripts
	mkdir -p $(DIST_DIR)/pkg/$(PKG_NAME)-$(VERSION)-$(GOOS)-$(GOARCH)/docs
	mkdir -p $(DIST_DIR)/pkg/$(PKG_NAME)-$(VERSION)-$(GOOS)-$(GOARCH)/caddy
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/pkg/$(PKG_NAME)-$(VERSION)-$(GOOS)-$(GOARCH)/bin/router ./cmd/router
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/pkg/$(PKG_NAME)-$(VERSION)-$(GOOS)-$(GOARCH)/bin/router-token-gen ./cmd/router-token-gen
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/pkg/$(PKG_NAME)-$(VERSION)-$(GOOS)-$(GOARCH)/bin/router-usage-report ./cmd/router-usage-report
	cp config.example.yaml $(DIST_DIR)/pkg/$(PKG_NAME)-$(VERSION)-$(GOOS)-$(GOARCH)/config/config.example.yaml
	cp env.example.json $(DIST_DIR)/pkg/$(PKG_NAME)-$(VERSION)-$(GOOS)-$(GOARCH)/config/env.example.json
	cp scripts/router.ts $(DIST_DIR)/pkg/$(PKG_NAME)-$(VERSION)-$(GOOS)-$(GOARCH)/config/scripts/router.ts
	cp README.md $(DIST_DIR)/pkg/$(PKG_NAME)-$(VERSION)-$(GOOS)-$(GOARCH)/docs/README.md
	cp docs/*.md $(DIST_DIR)/pkg/$(PKG_NAME)-$(VERSION)-$(GOOS)-$(GOARCH)/docs/
	cp deploy/Caddyfile $(DIST_DIR)/pkg/$(PKG_NAME)-$(VERSION)-$(GOOS)-$(GOARCH)/caddy/Caddyfile
	find $(DIST_DIR)/pkg/$(PKG_NAME)-$(VERSION)-$(GOOS)-$(GOARCH) -type d -exec chmod 0755 {} \;
	find $(DIST_DIR)/pkg/$(PKG_NAME)-$(VERSION)-$(GOOS)-$(GOARCH) -type f -exec chmod 0644 {} \;
	chmod 0755 $(DIST_DIR)/pkg/$(PKG_NAME)-$(VERSION)-$(GOOS)-$(GOARCH)/bin/router $(DIST_DIR)/pkg/$(PKG_NAME)-$(VERSION)-$(GOOS)-$(GOARCH)/bin/router-token-gen $(DIST_DIR)/pkg/$(PKG_NAME)-$(VERSION)-$(GOOS)-$(GOARCH)/bin/router-usage-report
	tar --owner=0 --group=0 --numeric-owner -C $(DIST_DIR)/pkg -czf $(DIST_DIR)/$(PKG_NAME)-$(VERSION)-$(GOOS)-$(GOARCH).tar.gz $(PKG_NAME)-$(VERSION)-$(GOOS)-$(GOARCH)

package-all:
	$(MAKE) package GOOS=linux GOARCH=amd64
	$(MAKE) package GOOS=linux GOARCH=arm64

docker-image: docs-build
	$(DOCKER) build --platform $(DOCKER_PLATFORM) --build-arg VERSION=$(VERSION) --build-arg COMMIT=$(COMMIT) --build-arg BUILD_DATE=$(BUILD_DATE) -t $(IMAGE_NAME):$(IMAGE_TAG) .

package-docker:
	rm -rf $(DIST_DIR)/docker/$(PKG_NAME)-$(VERSION)-docker-$(GOOS)-$(GOARCH)
	mkdir -p $(DIST_DIR)/docker/$(PKG_NAME)-$(VERSION)-docker-$(GOOS)-$(GOARCH)/images
	mkdir -p $(DIST_DIR)/docker/$(PKG_NAME)-$(VERSION)-docker-$(GOOS)-$(GOARCH)/compose
	mkdir -p $(DIST_DIR)/docker/$(PKG_NAME)-$(VERSION)-docker-$(GOOS)-$(GOARCH)/config/scripts
	mkdir -p $(DIST_DIR)/docker/$(PKG_NAME)-$(VERSION)-docker-$(GOOS)-$(GOARCH)/docs
	$(MAKE) docker-image GOOS=$(GOOS) GOARCH=$(GOARCH) DOCKER_PLATFORM=linux/$(GOARCH) IMAGE_TAG=$(VERSION)-$(GOOS)-$(GOARCH)
	$(DOCKER) save $(IMAGE_NAME):$(VERSION)-$(GOOS)-$(GOARCH) -o $(DIST_DIR)/docker/$(PKG_NAME)-$(VERSION)-docker-$(GOOS)-$(GOARCH)/images/$(IMAGE_NAME)-$(VERSION)-$(GOOS)-$(GOARCH).tar
	cp deploy/docker-compose.yml $(DIST_DIR)/docker/$(PKG_NAME)-$(VERSION)-docker-$(GOOS)-$(GOARCH)/compose/docker-compose.yml
	cp deploy/Caddyfile.compose $(DIST_DIR)/docker/$(PKG_NAME)-$(VERSION)-docker-$(GOOS)-$(GOARCH)/compose/Caddyfile.compose
	cp deploy/compose.env.example $(DIST_DIR)/docker/$(PKG_NAME)-$(VERSION)-docker-$(GOOS)-$(GOARCH)/compose/.env.example
	sed 's/^SMART_LLMROUTER_VERSION=.*/SMART_LLMROUTER_VERSION=$(VERSION)-$(GOOS)-$(GOARCH)/' deploy/compose.env.example > $(DIST_DIR)/docker/$(PKG_NAME)-$(VERSION)-docker-$(GOOS)-$(GOARCH)/compose/.env
	cp config.example.yaml $(DIST_DIR)/docker/$(PKG_NAME)-$(VERSION)-docker-$(GOOS)-$(GOARCH)/config/config.example.yaml
	cp env.example.json $(DIST_DIR)/docker/$(PKG_NAME)-$(VERSION)-docker-$(GOOS)-$(GOARCH)/config/env.example.json
	cp scripts/router.ts $(DIST_DIR)/docker/$(PKG_NAME)-$(VERSION)-docker-$(GOOS)-$(GOARCH)/config/scripts/router.ts
	cp README.md $(DIST_DIR)/docker/$(PKG_NAME)-$(VERSION)-docker-$(GOOS)-$(GOARCH)/docs/README.md
	cp docs/*.md $(DIST_DIR)/docker/$(PKG_NAME)-$(VERSION)-docker-$(GOOS)-$(GOARCH)/docs/
	find $(DIST_DIR)/docker/$(PKG_NAME)-$(VERSION)-docker-$(GOOS)-$(GOARCH) -type d -exec chmod 0755 {} \;
	find $(DIST_DIR)/docker/$(PKG_NAME)-$(VERSION)-docker-$(GOOS)-$(GOARCH) -type f -exec chmod 0644 {} \;
	tar --owner=0 --group=0 --numeric-owner -C $(DIST_DIR)/docker -czf $(DIST_DIR)/$(PKG_NAME)-$(VERSION)-docker-$(GOOS)-$(GOARCH).tar.gz $(PKG_NAME)-$(VERSION)-docker-$(GOOS)-$(GOARCH)

package-docker-all:
	$(MAKE) package-docker GOOS=linux GOARCH=amd64
	$(MAKE) package-docker GOOS=linux GOARCH=arm64

e2e-mock:
	$(MAKE) -C examples/cli-e2e-c clean test

e2e-live-c: build
	bash scripts/live_cli_c_e2e.sh

e2e-live-full: build
	bash scripts/live_full_e2e.sh

e2e-compose-live:
	bash scripts/compose_live_e2e.sh

clean:
	rm -rf router router-token router-token-gen router-usage-report examples/cli-e2e-c/cli-e2e $(DIST_DIR)
	rm -rf $(DOCS_SITE_DIR)/build $(DOCS_SITE_DIR)/.docusaurus
	find $(DOCS_EMBED_DIR) -mindepth 1 ! -name .keep -exec rm -rf {} +
