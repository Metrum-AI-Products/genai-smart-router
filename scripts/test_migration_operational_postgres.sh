#!/usr/bin/env bash
# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

# This rehearsal owns a fresh disposable database and intentionally never
# prints its DSN. The Go test repeats this guard before using the connection.
container_id=""
cleanup() {
  if [[ -n "$container_id" ]]; then
    docker rm -f "$container_id" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

container_id="$(docker run --rm -d \
  --label smart-llmrouter.test=issue-507-stage4 \
  -e POSTGRES_DB=smart_router_issue_507_stage4 \
  -e POSTGRES_PASSWORD=postgres \
  -p 127.0.0.1::5432 \
  postgres:18-bookworm)"
for _ in $(seq 1 60); do
  if docker exec "$container_id" pg_isready -U postgres -d smart_router_issue_507_stage4 >/dev/null 2>&1; then
    port="$(docker port "$container_id" 5432/tcp | sed -n 's/.*:\([0-9][0-9]*\)$/\1/p')"
    if [[ -n "$port" ]]; then
      SMART_ROUTER_POSTGRES_TEST_DSN="postgres://postgres:postgres@127.0.0.1:${port}/smart_router_issue_507_stage4?sslmode=disable" \
      SMART_ROUTER_POSTGRES_TEST_ALLOW=issue-507-stage4 \
      go test ./internal/router -run '^TestMigrationOperationalPostgresAdvisoryLockAndTimeouts$' -count=1
      exit 0
    fi
  fi
  sleep 1
done
echo 'disposable PostgreSQL did not become ready' >&2
exit 1
