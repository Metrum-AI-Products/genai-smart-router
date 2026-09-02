#!/usr/bin/env bash
# Copyright 2026 Metrum AI, Inc.
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

BASE_URL=""
MODEL_ID=""
API_KEY_ENV=""
DIALECT="openai-chat"
OUTPUT="table"
RECEIPT_IMAGE_URL=""
DELAY_SECONDS="1"
TEXT_MAX_TOKENS="256"
TOOL_MAX_TOKENS="512"
IMAGE_MAX_TOKENS="512"
CAP_MAX_TOKENS="1"
TIMEOUT_SECONDS="60"

RESULT_IDS=()
RESULT_NAMES=()
RESULT_STATES=()
RESULT_DETAILS=()
SCRIPT_ERROR=0

usage() {
  cat <<'EOF'
Usage:
  scripts/probe-model-capabilities.sh \
    --base-url https://api.provider.example/v1 \
    --model provider-model-id \
    --api-key-env PROVIDER_API_KEY \
    --dialect openai-chat|openai-responses|anthropic \
    [--output table|yaml] \
    [--receipt-image-url https://example.test/receipt.png]

Options:
  --delay SECONDS             Delay between provider requests. Default: 1
  --text-max-tokens N         Acceptance budget for text/reasoning probes. Default: 256
  --tool-max-tokens N         Acceptance budget for tool probes. Default: 512
  --image-max-tokens N        Acceptance budget for image probes. Default: 512
  --cap-max-tokens N          Tiny cap probe budget. Default: 1
  --timeout-seconds N         curl timeout per request. Default: 60
  -h, --help                  Show this help.

The script reads the API key from --api-key-env and never prints it.
Provider 4xx capability rejections are recorded as failed probe results.
Network errors, malformed required arguments, and missing tools are script errors.
EOF
}

die() {
  printf 'error: %s\n' "$*" >&2
  exit 2
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --base-url) BASE_URL="${2:-}"; shift 2 ;;
    --model) MODEL_ID="${2:-}"; shift 2 ;;
    --api-key-env) API_KEY_ENV="${2:-}"; shift 2 ;;
    --dialect) DIALECT="${2:-}"; shift 2 ;;
    --output) OUTPUT="${2:-}"; shift 2 ;;
    --receipt-image-url) RECEIPT_IMAGE_URL="${2:-}"; shift 2 ;;
    --delay) DELAY_SECONDS="${2:-}"; shift 2 ;;
    --text-max-tokens) TEXT_MAX_TOKENS="${2:-}"; shift 2 ;;
    --tool-max-tokens) TOOL_MAX_TOKENS="${2:-}"; shift 2 ;;
    --image-max-tokens) IMAGE_MAX_TOKENS="${2:-}"; shift 2 ;;
    --cap-max-tokens) CAP_MAX_TOKENS="${2:-}"; shift 2 ;;
    --timeout-seconds) TIMEOUT_SECONDS="${2:-}"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) die "unknown argument: $1" ;;
  esac
done

[[ -n "$BASE_URL" ]] || die "--base-url is required"
[[ -n "$MODEL_ID" ]] || die "--model is required"
[[ -n "$API_KEY_ENV" ]] || die "--api-key-env is required"
[[ "$DIALECT" == "openai-chat" || "$DIALECT" == "openai-responses" || "$DIALECT" == "anthropic" ]] || die "unsupported --dialect: $DIALECT"
[[ "$OUTPUT" == "table" || "$OUTPUT" == "yaml" ]] || die "unsupported --output: $OUTPUT"
command -v curl >/dev/null 2>&1 || die "curl is required"
command -v python3 >/dev/null 2>&1 || die "python3 is required for safe JSON construction and response inspection"
API_KEY="${!API_KEY_ENV:-}"
[[ -n "$API_KEY" ]] || die "environment variable $API_KEY_ENV is not set"

BASE_URL="${BASE_URL%/}"
TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

json_get() {
  local file="$1"
  local expr="$2"
  if command -v python3 >/dev/null 2>&1; then
    python3 - "$file" "$expr" <<'PY'
import json
import sys
path, expr = sys.argv[1], sys.argv[2]
try:
    with open(path, "r", encoding="utf-8") as f:
        data = json.load(f)
except Exception:
    print("")
    raise SystemExit(0)
value = data
for part in expr.split("."):
    if not part:
        continue
    if isinstance(value, list):
        try:
            value = value[int(part)]
        except Exception:
            print("")
            raise SystemExit(0)
    elif isinstance(value, dict):
        value = value.get(part, "")
    else:
        print("")
        raise SystemExit(0)
if isinstance(value, (dict, list)):
    print(json.dumps(value, separators=(",", ":")))
else:
    print("" if value is None else value)
PY
  else
    jq -r "$expr // empty" "$file" 2>/dev/null || true
  fi
}

json_has_nonempty() {
  local file="$1"
  local expr="$2"
  if [[ "$expr" == "__responses_tool_call__" ]]; then
    python3 - "$file" <<'PY'
import json
import sys
try:
    with open(sys.argv[1], "r", encoding="utf-8") as f:
        data = json.load(f)
except Exception:
    raise SystemExit(1)
for item in data.get("output") or []:
    if isinstance(item, dict) and item.get("type") in {"function_call", "tool_call"}:
        raise SystemExit(0)
raise SystemExit(1)
PY
    return $?
  fi
  if [[ "$expr" == "__anthropic_tool_use__" ]]; then
    python3 - "$file" <<'PY'
import json
import sys
try:
    with open(sys.argv[1], "r", encoding="utf-8") as f:
        data = json.load(f)
except Exception:
    raise SystemExit(1)
for item in data.get("content") or []:
    if isinstance(item, dict) and item.get("type") == "tool_use":
        raise SystemExit(0)
raise SystemExit(1)
PY
    return $?
  fi
  local value
  value="$(json_get "$file" "$expr")"
  [[ -n "$value" && "$value" != "[]" && "$value" != "{}" ]]
}

yaml_escape() {
  local value="${1//$'\n'/ }"
  value="${value//\"/\\\"}"
  printf '"%s"' "$value"
}

add_result() {
  RESULT_IDS+=("$1")
  RESULT_NAMES+=("$2")
  RESULT_STATES+=("$3")
  RESULT_DETAILS+=("$4")
}

sleep_between() {
  if [[ "$DELAY_SECONDS" != "0" && "$DELAY_SECONDS" != "0.0" ]]; then
    sleep "$DELAY_SECONDS"
  fi
}

post_json() {
  local endpoint="$1"
  local payload="$2"
  local out="$3"
  local code
  code="$(curl -sS --max-time "$TIMEOUT_SECONDS" \
    -o "$out" \
    -w '%{http_code}' \
    -H "Authorization: Bearer ${API_KEY}" \
    -H "Content-Type: application/json" \
    -H "Accept: application/json" \
    -H "User-Agent: smart-llmrouter-capability-probe" \
    -X POST "$endpoint" \
    --data-binary "$payload")" || return 99
  printf '%s' "$code"
}

text_payload() {
  case "$DIALECT" in
    openai-chat)
      printf '{"model":%s,"messages":[{"role":"user","content":"Reply OK only."}],"max_tokens":%s,"stream":false}' "$(printf '%s' "$MODEL_ID" | python3 -c 'import json,sys; print(json.dumps(sys.stdin.read()))')" "$TEXT_MAX_TOKENS"
      ;;
    openai-responses)
      printf '{"model":%s,"input":"Reply OK only.","max_output_tokens":%s,"store":false}' "$(printf '%s' "$MODEL_ID" | python3 -c 'import json,sys; print(json.dumps(sys.stdin.read()))')" "$TEXT_MAX_TOKENS"
      ;;
    anthropic)
      printf '{"model":%s,"messages":[{"role":"user","content":"Reply OK only."}],"max_tokens":%s}' "$(printf '%s' "$MODEL_ID" | python3 -c 'import json,sys; print(json.dumps(sys.stdin.read()))')" "$TEXT_MAX_TOKENS"
      ;;
  esac
}

stream_payload() {
  case "$DIALECT" in
    openai-chat)
      printf '{"model":%s,"messages":[{"role":"user","content":"Reply OK only."}],"max_tokens":%s,"stream":true}' "$(printf '%s' "$MODEL_ID" | python3 -c 'import json,sys; print(json.dumps(sys.stdin.read()))')" "$TEXT_MAX_TOKENS"
      ;;
    openai-responses)
      printf '{"model":%s,"input":"Reply OK only.","max_output_tokens":%s,"stream":true,"store":false}' "$(printf '%s' "$MODEL_ID" | python3 -c 'import json,sys; print(json.dumps(sys.stdin.read()))')" "$TEXT_MAX_TOKENS"
      ;;
    anthropic)
      printf '{"model":%s,"messages":[{"role":"user","content":"Reply OK only."}],"max_tokens":%s,"stream":true}' "$(printf '%s' "$MODEL_ID" | python3 -c 'import json,sys; print(json.dumps(sys.stdin.read()))')" "$TEXT_MAX_TOKENS"
      ;;
  esac
}

cap_payload() {
  case "$DIALECT" in
    openai-chat)
      printf '{"model":%s,"messages":[{"role":"user","content":"Reply with one word only."}],"max_tokens":%s,"stream":false}' "$(printf '%s' "$MODEL_ID" | python3 -c 'import json,sys; print(json.dumps(sys.stdin.read()))')" "$CAP_MAX_TOKENS"
      ;;
    openai-responses)
      printf '{"model":%s,"input":"Reply with one word only.","max_output_tokens":%s,"store":false}' "$(printf '%s' "$MODEL_ID" | python3 -c 'import json,sys; print(json.dumps(sys.stdin.read()))')" "$CAP_MAX_TOKENS"
      ;;
    anthropic)
      printf '{"model":%s,"messages":[{"role":"user","content":"Reply with one word only."}],"max_tokens":%s}' "$(printf '%s' "$MODEL_ID" | python3 -c 'import json,sys; print(json.dumps(sys.stdin.read()))')" "$CAP_MAX_TOKENS"
      ;;
  esac
}

tool_payload() {
  local choice="$1"
  case "$DIALECT" in
    openai-chat)
      if [[ "$choice" == "forced" ]]; then
        printf '{"model":%s,"messages":[{"role":"user","content":"Use the weather tool for San Francisco, CA."}],"max_tokens":%s,"stream":false,"tools":[{"type":"function","function":{"name":"get_weather","description":"Get weather for a city","parameters":{"type":"object","properties":{"location":{"type":"string"}},"required":["location"],"additionalProperties":false}}}],"tool_choice":{"type":"function","function":{"name":"get_weather"}}}' "$(printf '%s' "$MODEL_ID" | python3 -c 'import json,sys; print(json.dumps(sys.stdin.read()))')" "$TOOL_MAX_TOKENS"
      else
        printf '{"model":%s,"messages":[{"role":"user","content":"Use the weather tool for San Francisco, CA."}],"max_tokens":%s,"stream":false,"tools":[{"type":"function","function":{"name":"get_weather","description":"Get weather for a city","parameters":{"type":"object","properties":{"location":{"type":"string"}},"required":["location"],"additionalProperties":false}}}],"tool_choice":"auto"}' "$(printf '%s' "$MODEL_ID" | python3 -c 'import json,sys; print(json.dumps(sys.stdin.read()))')" "$TOOL_MAX_TOKENS"
      fi
      ;;
    openai-responses)
      printf '{"model":%s,"input":"Use the weather tool for San Francisco, CA.","max_output_tokens":%s,"store":false,"tools":[{"type":"function","name":"get_weather","description":"Get weather for a city","parameters":{"type":"object","properties":{"location":{"type":"string"}},"required":["location"],"additionalProperties":false}}]}' "$(printf '%s' "$MODEL_ID" | python3 -c 'import json,sys; print(json.dumps(sys.stdin.read()))')" "$TOOL_MAX_TOKENS"
      ;;
    anthropic)
      printf '{"model":%s,"messages":[{"role":"user","content":"Use the weather tool for San Francisco, CA."}],"max_tokens":%s,"tools":[{"name":"get_weather","description":"Get weather for a city","input_schema":{"type":"object","properties":{"location":{"type":"string"}},"required":["location"]}}],"tool_choice":{"type":"auto"}}' "$(printf '%s' "$MODEL_ID" | python3 -c 'import json,sys; print(json.dumps(sys.stdin.read()))')" "$TOOL_MAX_TOKENS"
      ;;
  esac
}

structured_payload() {
  case "$DIALECT" in
    openai-chat)
      printf '{"model":%s,"messages":[{"role":"user","content":"Return JSON with ok=true."}],"max_tokens":%s,"stream":false,"response_format":{"type":"json_schema","json_schema":{"name":"ok_result","strict":true,"schema":{"type":"object","properties":{"ok":{"type":"boolean"}},"required":["ok"],"additionalProperties":false}}}}' "$(printf '%s' "$MODEL_ID" | python3 -c 'import json,sys; print(json.dumps(sys.stdin.read()))')" "$TEXT_MAX_TOKENS"
      ;;
    openai-responses)
      printf '{"model":%s,"input":"Return JSON with ok=true.","max_output_tokens":%s,"store":false,"text":{"format":{"type":"json_schema","name":"ok_result","strict":true,"schema":{"type":"object","properties":{"ok":{"type":"boolean"}},"required":["ok"],"additionalProperties":false}}}}' "$(printf '%s' "$MODEL_ID" | python3 -c 'import json,sys; print(json.dumps(sys.stdin.read()))')" "$TEXT_MAX_TOKENS"
      ;;
    anthropic)
      printf '{"model":%s,"messages":[{"role":"user","content":"Return only JSON: {\"ok\":true}"}],"max_tokens":%s}' "$(printf '%s' "$MODEL_ID" | python3 -c 'import json,sys; print(json.dumps(sys.stdin.read()))')" "$TEXT_MAX_TOKENS"
      ;;
  esac
}

tools_structured_payload() {
  case "$DIALECT" in
    openai-chat)
      printf '{"model":%s,"messages":[{"role":"user","content":"Use the weather tool if needed, then return JSON with ok=true."}],"max_tokens":%s,"stream":false,"tools":[{"type":"function","function":{"name":"get_weather","description":"Get weather for a city","parameters":{"type":"object","properties":{"location":{"type":"string"}},"required":["location"],"additionalProperties":false}}}],"tool_choice":"auto","response_format":{"type":"json_schema","json_schema":{"name":"ok_result","strict":true,"schema":{"type":"object","properties":{"ok":{"type":"boolean"}},"required":["ok"],"additionalProperties":false}}}}' "$(printf '%s' "$MODEL_ID" | python3 -c 'import json,sys; print(json.dumps(sys.stdin.read()))')" "$TOOL_MAX_TOKENS"
      ;;
    openai-responses)
      printf '{"model":%s,"input":"Use the weather tool if needed, then return JSON with ok=true.","max_output_tokens":%s,"store":false,"tools":[{"type":"function","name":"get_weather","description":"Get weather for a city","parameters":{"type":"object","properties":{"location":{"type":"string"}},"required":["location"],"additionalProperties":false}}],"text":{"format":{"type":"json_schema","name":"ok_result","strict":true,"schema":{"type":"object","properties":{"ok":{"type":"boolean"}},"required":["ok"],"additionalProperties":false}}}}' "$(printf '%s' "$MODEL_ID" | python3 -c 'import json,sys; print(json.dumps(sys.stdin.read()))')" "$TOOL_MAX_TOKENS"
      ;;
    anthropic)
      printf '{"model":%s,"messages":[{"role":"user","content":"Use the weather tool if needed, then return JSON with ok=true."}],"max_tokens":%s,"tools":[{"name":"get_weather","description":"Get weather for a city","input_schema":{"type":"object","properties":{"location":{"type":"string"}},"required":["location"]}}],"tool_choice":{"type":"auto"}}' "$(printf '%s' "$MODEL_ID" | python3 -c 'import json,sys; print(json.dumps(sys.stdin.read()))')" "$TOOL_MAX_TOKENS"
      ;;
  esac
}

image_payload() {
  local url_json
  url_json="$(printf '%s' "$RECEIPT_IMAGE_URL" | python3 -c 'import json,sys; print(json.dumps(sys.stdin.read()))')"
  case "$DIALECT" in
    openai-chat)
      printf '{"model":%s,"messages":[{"role":"user","content":[{"type":"text","text":"Read the receipt image. Reply with only the merchant name."},{"type":"image_url","image_url":{"url":%s}}]}],"max_tokens":%s,"stream":false}' "$(printf '%s' "$MODEL_ID" | python3 -c 'import json,sys; print(json.dumps(sys.stdin.read()))')" "$url_json" "$IMAGE_MAX_TOKENS"
      ;;
    openai-responses)
      printf '{"model":%s,"input":[{"role":"user","content":[{"type":"input_text","text":"Read the receipt image. Reply with only the merchant name."},{"type":"input_image","image_url":%s}]}],"max_output_tokens":%s,"store":false}' "$(printf '%s' "$MODEL_ID" | python3 -c 'import json,sys; print(json.dumps(sys.stdin.read()))')" "$url_json" "$IMAGE_MAX_TOKENS"
      ;;
    anthropic)
      printf '{"model":%s,"messages":[{"role":"user","content":[{"type":"text","text":"Read the receipt image. Reply with only the merchant name."},{"type":"image","source":{"type":"url","url":%s}}]}],"max_tokens":%s}' "$(printf '%s' "$MODEL_ID" | python3 -c 'import json,sys; print(json.dumps(sys.stdin.read()))')" "$url_json" "$IMAGE_MAX_TOKENS"
      ;;
  esac
}

reasoning_payload() {
  case "$DIALECT" in
    openai-chat)
      printf '{"model":%s,"messages":[{"role":"user","content":"Solve 19+23 and reply with the number."}],"reasoning_effort":"low","max_tokens":%s,"stream":false}' "$(printf '%s' "$MODEL_ID" | python3 -c 'import json,sys; print(json.dumps(sys.stdin.read()))')" "$TEXT_MAX_TOKENS"
      ;;
    openai-responses)
      printf '{"model":%s,"input":"Solve 19+23 and reply with the number.","reasoning":{"effort":"low"},"max_output_tokens":%s,"store":false}' "$(printf '%s' "$MODEL_ID" | python3 -c 'import json,sys; print(json.dumps(sys.stdin.read()))')" "$TEXT_MAX_TOKENS"
      ;;
    anthropic)
      printf '{"model":%s,"messages":[{"role":"user","content":"Solve 19+23 and reply with the number."}],"thinking":{"type":"enabled","budget_tokens":1024},"max_tokens":2048}' "$(printf '%s' "$MODEL_ID" | python3 -c 'import json,sys; print(json.dumps(sys.stdin.read()))')"
      ;;
  esac
}

endpoint_for() {
  case "$DIALECT" in
    openai-chat) printf '%s/chat/completions' "$BASE_URL" ;;
    openai-responses) printf '%s/responses' "$BASE_URL" ;;
    anthropic) printf '%s/v1/messages' "$BASE_URL" ;;
  esac
}

probe() {
  local id="$1" name="$2" payload="$3" pass_expr="$4"
  local out="$TMPDIR/${id}.json"
  local code detail state
  code="$(post_json "$(endpoint_for)" "$payload" "$out")" || {
    SCRIPT_ERROR=1
    add_result "$id" "$name" "error" "network or curl failure"
    return
  }
  if [[ "$code" =~ ^2 ]]; then
    if [[ -z "$pass_expr" || "$(json_has_nonempty "$out" "$pass_expr"; echo $?)" == "0" ]]; then
      state="pass"
      detail="HTTP ${code}"
    else
      state="fail"
      detail="HTTP ${code}, response did not match expected shape"
    fi
  else
    state="fail"
    detail="HTTP ${code}: $(head -c 180 "$out" | tr '\n' ' ')"
  fi
  add_result "$id" "$name" "$state" "$detail"
  sleep_between
}

probe_streaming() {
  local out="$TMPDIR/streaming.txt"
  local code
  code="$(curl -sS --max-time "$TIMEOUT_SECONDS" \
    -o "$out" \
    -w '%{http_code}' \
    -H "Authorization: Bearer ${API_KEY}" \
    -H "Content-Type: application/json" \
    -H "Accept: text/event-stream" \
    -H "User-Agent: smart-llmrouter-capability-probe" \
    -X POST "$(endpoint_for)" \
    --data-binary "$(stream_payload)")" || {
      SCRIPT_ERROR=1
      add_result "streaming" "Streaming text" "error" "network or curl failure"
      return
    }
  if [[ "$code" =~ ^2 ]] && grep -Eq '(^data:|event:|type)' "$out"; then
    add_result "streaming" "Streaming text" "pass" "HTTP ${code}, stream-like response"
  elif [[ "$code" =~ ^2 ]]; then
    add_result "streaming" "Streaming text" "fail" "HTTP ${code}, response did not look like SSE/stream JSON"
  else
    add_result "streaming" "Streaming text" "fail" "HTTP ${code}: $(head -c 180 "$out" | tr '\n' ' ')"
  fi
  sleep_between
}

case "$DIALECT" in
  openai-chat) text_expr="choices.0.message.content"; tool_expr="choices.0.message.tool_calls" ;;
  openai-responses) text_expr="output.0.content.0.text"; tool_expr="__responses_tool_call__" ;;
  anthropic) text_expr="content.0.text"; tool_expr="__anthropic_tool_use__" ;;
esac

probe "text" "Basic text connectivity" "$(text_payload)" "$text_expr"
probe_streaming
probe "max-tokens-cap" "Max-token cap honoring" "$(cap_payload)" ""
probe "auto-tools" "Auto tool choice" "$(tool_payload auto)" "$tool_expr"
if [[ "$DIALECT" == "openai-chat" ]]; then
  probe "forced-tools" "Forced tool choice" "$(tool_payload forced)" "choices.0.message.tool_calls"
else
  add_result "forced-tools" "Forced tool choice" "skip" "not applicable for this generic dialect probe"
fi
probe "structured-outputs" "Structured outputs" "$(structured_payload)" ""
probe "tools-plus-structured" "Tools plus structured outputs" "$(tools_structured_payload)" ""
if [[ -n "$RECEIPT_IMAGE_URL" ]]; then
  probe "image-input" "Image input accepted" "$(image_payload)" ""
  add_result "image-ocr" "Image OCR quality" "info" "inspect image-input response for workload-specific answer"
else
  add_result "image-input" "Image input accepted" "skip" "not tested; --receipt-image-url not provided"
  add_result "image-ocr" "Image OCR quality" "skip" "not tested; --receipt-image-url not provided"
fi
probe "reasoning-effort" "Reasoning effort or thinking budget" "$(reasoning_payload)" ""

has_pass() {
  local wanted="$1" i
  for i in "${!RESULT_IDS[@]}"; do
    [[ "${RESULT_IDS[$i]}" == "$wanted" && "${RESULT_STATES[$i]}" == "pass" ]] && return 0
  done
  return 1
}

print_table() {
  printf '%-24s %-8s %s\n' "test_id" "state" "detail"
  printf '%-24s %-8s %s\n' "-------" "-----" "------"
  local i
  for i in "${!RESULT_IDS[@]}"; do
    printf '%-24s %-8s %s\n' "${RESULT_IDS[$i]}" "${RESULT_STATES[$i]}" "${RESULT_DETAILS[$i]}"
  done
}

print_yaml() {
  printf 'provider:\n'
  printf '  base_url: %s\n' "$(yaml_escape "$BASE_URL")"
  printf '  model: %s\n' "$(yaml_escape "$MODEL_ID")"
  printf '  dialect: %s\n' "$(yaml_escape "$DIALECT")"
  printf '  tested_at: %s\n\n' "$(date -u +%F)"
  printf 'results:\n'
  local i
  for i in "${!RESULT_IDS[@]}"; do
    printf '  - test_id: %s\n' "$(yaml_escape "${RESULT_IDS[$i]}")"
    printf '    name: %s\n' "$(yaml_escape "${RESULT_NAMES[$i]}")"
    printf '    state: %s\n' "$(yaml_escape "${RESULT_STATES[$i]}")"
    printf '    passed: %s\n' "$([[ "${RESULT_STATES[$i]}" == "pass" ]] && echo true || echo false)"
    printf '    detail: %s\n' "$(yaml_escape "${RESULT_DETAILS[$i]}")"
  done
  printf '\nrecommended_config:\n'
  printf '  input_modalities:\n'
  printf '    - text\n'
  if has_pass image-input; then
    printf '    - image\n'
  fi
  printf '  output_modalities:\n'
  printf '    - text\n'
  if has_pass auto-tools || has_pass forced-tools || has_pass structured-outputs; then
    printf '  tool_support:\n'
    case "$DIALECT" in
      openai-chat)
        printf '    openai_chat:\n'
        has_pass auto-tools && printf '      - tools\n'
        has_pass forced-tools && printf '      - tool_choice\n'
        has_pass structured-outputs && printf '      - structured_outputs\n'
        ;;
      openai-responses)
        printf '    openai_responses:\n'
        has_pass auto-tools && printf '      - function\n'
        has_pass structured-outputs && printf '      - structured_outputs\n'
        ;;
      anthropic)
        printf '    anthropic_messages:\n'
        has_pass auto-tools && printf '      - client_tools\n'
        ;;
    esac
  fi
  if has_pass reasoning-effort; then
    printf '  reasoning:\n'
    printf '    supported: true\n'
    if [[ "$DIALECT" == "anthropic" ]]; then
      printf '    mode: opt_in\n'
      printf '    control: token_budget\n'
    else
      printf '    mode: opt_in\n'
      printf '    control: effort_enum\n'
    fi
  fi
  printf '  pricing_notes: >\n'
  printf '    Direct %s smokes ran on %s for %s. Copy only the fields whose\n' "$DIALECT" "$(date -u +%F)" "$MODEL_ID"
  printf '    tests passed. Failed, skipped, or informational tests are intentionally\n'
  printf '    omitted from active capability metadata until direct and router-level\n'
  printf '    validation pass for the exact provider, model, account, dialect, and route.\n'
}

if [[ "$OUTPUT" == "yaml" ]]; then
  print_yaml
else
  print_table
fi

if [[ "$SCRIPT_ERROR" == "1" ]]; then
  exit 1
fi
