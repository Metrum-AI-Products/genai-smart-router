# Content-Based Routing With Go External Classifier Support

## Summary

Add content-aware routing to the TypeScript routing layer, including a safe way for `scripts/router.ts` to call an external routing service implemented in Go. The service receives a sanitized routing payload: prompt/messages/metadata, safe caller identity, and safe target config, but never provider API keys, raw router tokens, or token hashes.

## Key Changes

- Extend the TypeScript routing context with a synchronous helper:
  - `ctx.callRoutingService(name)` sends the standard sanitized routing payload.
  - `ctx.callRoutingService(name, payload)` sends caller-provided JSON merged with the standard routing context.
  - The helper returns parsed JSON so scripts can return `{ targetIndex }`, `{ target: { provider, model } }`, `fallbackIndexes`, and `classLabel`.
- Add YAML configuration for external routing services:
  - service name
  - URL
  - timeout
  - optional auth header
  - optional auth token env var
  - max response size
- Populate normalized request metadata into `IRRequest.Metadata` for OpenAI Chat, OpenAI Responses, and Anthropic-style requests.
- Keep all provider/model/routing config in YAML/env/script files, with no hardcoded provider config in Go source.
- Preserve weighted routing as the default fallback behavior.

## Go Classifier Example

- Add a small Go example service, not FastAPI:
  - HTTP endpoint: `POST /route`
  - Accepts the sanitized router payload.
  - Applies simple rules such as:
    - code/build/debug prompts route to `big-coder` when available
    - low-latency metadata routes to `fast`
    - otherwise return no decision or a weighted/default target
  - Returns:

    ```json
    {
      "targetIndex": 0,
      "classLabel": "code-request",
      "fallbackIndexes": [1, 2]
    }
    ```

- Include a Makefile target or documented command to build/run the Go classifier locally for demos and tests.

## Routing Script Examples

- Update the sample TypeScript router to demonstrate:
  - direct prompt-content routing using `ctx.text`
  - metadata routing using `ctx.request.metadata`
  - caller-token regex routing
  - external Go classifier routing:

    ```ts
    try {
      const decision = ctx.callRoutingService("go-classifier");
      if (decision?.targetIndex !== undefined || decision?.target) return decision;
    } catch (_) {
      // fallback to local weighted routing
    }
    ```

- Keep the local weighted target selection as the final fallback.

## Safety And Failure Behavior

- External service payload includes:
  - `group`, `text`, normalized request fields, request metadata
  - safe caller fields: id, user, project, environment, token id, allowed model groups
  - safe target fields: provider, internal model, provider model ref, dialect, base URL, weight, tier, cost, key env name, key configured boolean
- External service payload excludes:
  - provider API key values
  - raw router API tokens
  - token hashes
  - secret file paths
- If the Go classifier times out, returns non-2xx, invalid JSON, or an invalid target, the helper throws a script exception. The sample TypeScript catches it and falls back to local routing.
- Add configurable script timeout so HTTP-backed routing has enough time while still bounding latency.

## Tests

- Unit test content-based routing using `ctx.text`.
- Unit test metadata extraction from OpenAI Chat, OpenAI Responses, and Anthropic request bodies.
- Unit test `ctx.callRoutingService` with `httptest`:
  - verifies sanitized payload contents
  - verifies no secrets are sent
  - verifies returned `targetIndex` routes correctly
- Unit test external-service failure fallback through TypeScript `try/catch`.
- Unit test timeout and non-2xx behavior.
- Add a lightweight test for the Go classifier's rule behavior.
- Run `go test ./...`.

## Docs

- Update README with:
  - content-based routing overview
  - metadata-based routing example
  - external Go routing service config
  - build/run example for the Go classifier
  - timeout and failure behavior
  - warning that external classifiers receive prompts and metadata, so they must be trusted/internal
- Update `docs/solution-brief.md` with a high-level diagram showing:
  - caller API
  - router auth/model-policy layer
  - TypeScript routing layer
  - optional Go routing classifier
  - provider dispatch layer
- Explain that deployments do not require source distribution: packaged binaries, docs, config, scripts, and optional classifier binary are sufficient.

## Assumptions

- Use the configured-helper approach, not generic `fetch`.
- Send sanitized routing payload only, not raw provider request bodies.
- Implement the external classifier example in Go.
- Treat external classifier services as trusted internal services because prompt text and metadata may be sensitive.
