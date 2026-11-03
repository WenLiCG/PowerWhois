# PowerWhois review and implementation plan

Baseline: `bf1258b4dc7e067038cc6728f6868d4561fbfabb` (upstream main).

## Scope and smallest complete change

Keep Go/Fiber, Vue/Quasar, Redis, the existing domain parsers and typo generators. Preserve Web DNS, WHOIS/RDAP, proxy, Mixed, custom query APIs, uploads, progress, pause/resume/cancel, retry failures, CSV, raw responses, login, settings, logs and manual registration. No registrar transactions will run during verification.

1. Fix shared domain validation, result classification, cancellation, unsafe logging/TLS, configuration writes and authentication boundaries.
2. Put durable query tasks behind a single Redis-backed service. Existing Web operations and versioned HTTP routes use it; retain a compatibility path for original stored batch data. Results are committed before work is acknowledged. Explicitly document restart and duplicate-submission semantics.
3. Add a noninteractive CLI that calls the service, plus an optional official-SDK MCP adapter over the same API. Neither adapter gets administrator capabilities. Publish OpenAPI schemas and examples.
4. Refine the existing UI: truthful statuses, accessible result details, search/filter/pagination, task identity/history and connection/error recovery. Keep all existing operation paths.
5. Verify with deterministic network mocks, real isolated Redis, concurrency/race checks, restart and interruption scenarios, frontend production build and browser inspection. Deliver a review, feature checklist, test results and deployment/upgrade/rollback instructions.

## Confirmed baseline defects

- DNS transport failure/no delegation becomes Free in both schedulers.
- RDAP certificate verification is disabled; sparse positive RDAP objects become not found.
- Login, WebSocket authentication and config debugging leak credentials.
- Web concurrency reads the domain-count limit; server does not enforce the input cap.
- Global WebSocket client/config/custom limiter maps race; cancellation can block on channel sends or race with a replacement task.
- Clearing a batch uses FLUSHDB; result-write failure still removes pending work.
- Repeated start/resume can create duplicate workers; batches have no stable identity.
- Domain cleaning silently removes invalid characters and may query another name.
- YAML is built by interpolation and overwritten in place; special characters can corrupt it.
- Frontend disconnects and duplicate messages can leave counters or progress stuck.

## Operational boundary

Single tenant, one active backend worker service per Redis namespace. Remote access requires an authenticated TLS reverse proxy. No public deployment, real paid registration or account-specific ChatGPT/Grok connection is implied by local tests. Official product documentation and available local clients determine each integration example.
