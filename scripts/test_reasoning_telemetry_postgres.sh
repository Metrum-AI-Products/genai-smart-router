#!/usr/bin/env bash
set -euo pipefail

# This runner owns a fresh Docker database and does not print its DSN or
# credentials. The Go test refuses to run against a database without the same
# explicit disposable-test guard.
container_id=""
cleanup() {
  if [[ -n "$container_id" ]]; then
    docker rm -f "$container_id" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

container_id="$(docker run --rm -d \
  --label smart-llmrouter.test=issue-571 \
  -e POSTGRES_DB=smart_router_issue_571 \
  -e POSTGRES_PASSWORD=postgres \
  -p 127.0.0.1::5432 \
  postgres:18-bookworm)"
for _ in $(seq 1 60); do
  if docker exec "$container_id" pg_isready -U postgres -d smart_router_issue_571 >/dev/null 2>&1; then
    port="$(docker port "$container_id" 5432/tcp | sed -n 's/.*:\([0-9][0-9]*\)$/\1/p')"
    if [[ -n "$port" ]]; then
      SMART_ROUTER_POSTGRES_TEST_DSN="postgres://postgres:postgres@127.0.0.1:${port}/smart_router_issue_571?sslmode=disable" \
      SMART_ROUTER_POSTGRES_TEST_ALLOW=issue-571 \
      go test ./internal/router -run '^TestReasoningTelemetryPostgresParity$' -count=1
      exit 0
    fi
  fi
  sleep 1
done
echo 'disposable PostgreSQL did not become ready' >&2
exit 1
