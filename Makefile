VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
DIST_DIR ?= dist
PKG_NAME ?= smart-llmrouter
GOOS ?= linux
GOARCH ?= $(shell go env GOARCH)

.PHONY: test build build-all package package-all e2e-mock e2e-live-c clean

test:
	go test ./...

build:
	go build -o router ./cmd/router
	go build -o router-token-gen ./cmd/router-token-gen

build-all:
	mkdir -p $(DIST_DIR)/build/linux-amd64 $(DIST_DIR)/build/linux-arm64
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o $(DIST_DIR)/build/linux-amd64/router ./cmd/router
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o $(DIST_DIR)/build/linux-amd64/router-token-gen ./cmd/router-token-gen
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o $(DIST_DIR)/build/linux-arm64/router ./cmd/router
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o $(DIST_DIR)/build/linux-arm64/router-token-gen ./cmd/router-token-gen

package:
	rm -rf $(DIST_DIR)/pkg/$(PKG_NAME)-$(VERSION)-$(GOOS)-$(GOARCH)
	mkdir -p $(DIST_DIR)/pkg/$(PKG_NAME)-$(VERSION)-$(GOOS)-$(GOARCH)/bin
	mkdir -p $(DIST_DIR)/pkg/$(PKG_NAME)-$(VERSION)-$(GOOS)-$(GOARCH)/config/scripts
	mkdir -p $(DIST_DIR)/pkg/$(PKG_NAME)-$(VERSION)-$(GOOS)-$(GOARCH)/docs
	mkdir -p $(DIST_DIR)/pkg/$(PKG_NAME)-$(VERSION)-$(GOOS)-$(GOARCH)/caddy
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build -o $(DIST_DIR)/pkg/$(PKG_NAME)-$(VERSION)-$(GOOS)-$(GOARCH)/bin/router ./cmd/router
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build -o $(DIST_DIR)/pkg/$(PKG_NAME)-$(VERSION)-$(GOOS)-$(GOARCH)/bin/router-token-gen ./cmd/router-token-gen
	cp config.example.yaml $(DIST_DIR)/pkg/$(PKG_NAME)-$(VERSION)-$(GOOS)-$(GOARCH)/config/config.example.yaml
	cp env.example.json $(DIST_DIR)/pkg/$(PKG_NAME)-$(VERSION)-$(GOOS)-$(GOARCH)/config/env.example.json
	cp scripts/router.ts $(DIST_DIR)/pkg/$(PKG_NAME)-$(VERSION)-$(GOOS)-$(GOARCH)/config/scripts/router.ts
	cp README.md $(DIST_DIR)/pkg/$(PKG_NAME)-$(VERSION)-$(GOOS)-$(GOARCH)/docs/README.md
	cp docs/DEPLOYMENT.md $(DIST_DIR)/pkg/$(PKG_NAME)-$(VERSION)-$(GOOS)-$(GOARCH)/docs/DEPLOYMENT.md
	cp deploy/Caddyfile $(DIST_DIR)/pkg/$(PKG_NAME)-$(VERSION)-$(GOOS)-$(GOARCH)/caddy/Caddyfile
	find $(DIST_DIR)/pkg/$(PKG_NAME)-$(VERSION)-$(GOOS)-$(GOARCH) -type d -exec chmod 0755 {} \;
	find $(DIST_DIR)/pkg/$(PKG_NAME)-$(VERSION)-$(GOOS)-$(GOARCH) -type f -exec chmod 0644 {} \;
	chmod 0755 $(DIST_DIR)/pkg/$(PKG_NAME)-$(VERSION)-$(GOOS)-$(GOARCH)/bin/router $(DIST_DIR)/pkg/$(PKG_NAME)-$(VERSION)-$(GOOS)-$(GOARCH)/bin/router-token-gen
	tar --owner=0 --group=0 --numeric-owner -C $(DIST_DIR)/pkg -czf $(DIST_DIR)/$(PKG_NAME)-$(VERSION)-$(GOOS)-$(GOARCH).tar.gz $(PKG_NAME)-$(VERSION)-$(GOOS)-$(GOARCH)

package-all:
	$(MAKE) package GOOS=linux GOARCH=amd64
	$(MAKE) package GOOS=linux GOARCH=arm64

e2e-mock:
	$(MAKE) -C examples/cli-e2e-c clean test

e2e-live-c: build
	bash scripts/live_cli_c_e2e.sh

clean:
	rm -rf router router-token router-token-gen examples/cli-e2e-c/cli-e2e $(DIST_DIR)
