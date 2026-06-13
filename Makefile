.PHONY: test build e2e-mock e2e-live-c clean

test:
	go test ./...

build:
	go build -o router ./cmd/router
	go build -o router-token-gen ./cmd/router-token-gen

e2e-mock:
	$(MAKE) -C examples/cli-e2e-c clean test

e2e-live-c: build
	bash scripts/live_cli_c_e2e.sh

clean:
	rm -f router router-token router-token-gen examples/cli-e2e-c/cli-e2e
