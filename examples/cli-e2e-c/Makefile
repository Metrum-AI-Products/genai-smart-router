CC ?= cc
CFLAGS ?= -std=c11 -Wall -Wextra -Werror -O2
BIN := cli-e2e

.PHONY: all test test-mock live live-claude live-codex clean

all: $(BIN)

$(BIN): cli_e2e.c
	$(CC) $(CFLAGS) -o $@ $<

test: test-mock

test-mock: $(BIN)
	CLAUDE_BIN="./mock-bin/claude" CODEX_BIN="./mock-bin/codex" ./$(BIN) --tool both --expect "hello world"

live: $(BIN)
	./$(BIN) --tool both --expect "hello world"

live-claude: $(BIN)
	./$(BIN) --tool claude --expect "hello world"

live-codex: $(BIN)
	./$(BIN) --tool codex --expect "hello world"

clean:
	rm -f $(BIN)
