# PowerWhois review fixes — handoff (2026-11-22)

## 1. Status — read this first

| Item | State |
|---|---|
| Base | `main` at `6662f8ae55447140152119a90b0cf9399159bb21` |
| Branch | `pw-cli-impl-20261122`, one commit on top of the base, **committed locally only** |
| Push and pull request | **Not done.** The authoring machine had no GitHub credentials (`git push` could not authenticate; `gh` not logged in). Push the branch and open a PR against `main` from an authenticated machine |
| Merge into `main` | **Not done.** Wait for independent verification of the PR head and a passing `Verify PowerWhois` check |

Local results on the committed tree (Linux, Go 1.27.4, Node 22.23.5, Yarn 1.22.22):

| Check | Result |
|---|---|
| `gofmt -l .` | Clean |
| `go build ./...`, `go vet ./...` | Pass |
| `go test -race -count=1 ./...` | Pass. **No `redis-server` on this machine, so six Redis tests skipped**: `TestBulkPauseResumeAndClear`, `TestCommitIsAtomicAndRecoveryCanFinishWithNoPending`, `TestRecheckRequeuesOnlyErrors` (new), `TestRegistrationRecordsOneRequestAndReconcilesAmbiguity`, `TestPendingListsOnlyUnresolvedOperations` (new), `TestBrowserProtocolAndBulkDownload`. CI installs Redis and runs them; check the CI log for `PASS`, not `SKIP` |
| `yarn install --frozen-lockfile`, `yarn lint`, `yarn test`, `yarn build` | Pass. Node 24 fails the `engines` check; use Node 22 as CI does |
| Caddyfile | `caddy validate` with Caddy v2.11.9 (built from the official module): valid |
| Caddy forwarding | Checked against a header-echo upstream: from a trusted peer, Caddy keeps client-written `X-Forwarded-For` entries and appends the peer (`10.0.0.5, 203.0.113.7, 127.0.0.1`); from an untrusted peer it replaces the header. Security headers are served |
| `docker-compose.yml` | YAML parses and the changed fields are as intended. **`docker compose config` and a container start were not run** (no Docker on this machine) |

Earlier, a separate controller run of `go vet` and `go test -race -count=1 ./...` also passed on the tree before the proxy-trust fix in 1.1.

## 2. What changed, by review finding

Numbers refer to the review of `main@6662f8a`.

### 1.1 Rate limits shared by every client behind the proxies
- `deploy/caddy/Caddyfile`: `servers { trusted_proxies static private_ranges }`, so Caddy keeps the client address Nginx puts in `X-Forwarded-For` (Nginx overwrites that header with `$remote_addr`, so clients cannot spoof it).
- `deploy/docker-compose.yml`: `POWERWHOIS_TRUSTED_PROXIES` defaults to `10.0.0.0/8 172.16.0.0/12 192.168.0.0/16 fc00::/7`. Only Caddy can reach the backend on the Compose network.
- `src/backend/api/clientip.go` (new) replaces Fiber's proxy handling. **Fiber's `c.IP()` returns the leftmost `X-Forwarded-For` entry**, which the client writes. Caddy passes such entries through from trusted peers, as checked above. With the first version of this change, any front proxy that appends instead of overwriting (`$proxy_add_x_forwarded_for`, most CDNs) let a client:
  - send `X-Forwarded-For: 10.0.0.1`, be treated as private, and skip every per-IP limit and the connection cap;
  - rotate forged public addresses to get a fresh login-limiter bucket each time, which allows unlimited password guessing. The regression test reproduced this (HTTP 401 instead of 429) before the fix.
- `ClientIP` reads `X-Forwarded-For` from the right and returns the first address that is not a trusted proxy, so client-written entries on the left are ignored whether front proxies overwrite or append. It reads every `X-Forwarded-For` header line, unmaps IPv4-mapped addresses, and stops at malformed entries.
- `limitKey` groups IPv6 clients by /64, so rotating addresses inside one allocation does not reset limits. It is used by the login limiter, the WebSocket upgrade limiter, the per-IP connection cap and the anonymous budget.
- `POWERWHOIS_TRUSTED_PROXIES` accepts addresses or CIDR ranges separated by spaces or commas. **When unset, loopback is trusted**, so a native install behind Nginx on the same host works without configuration. An invalid entry stops startup.
- Effect: the login limiter (10 failures per minute) and the WebSocket upgrade limiter count each real client separately, and forged entries cannot change the result.
- Tests:
  - `proxy_regression_test.go` locks out one forwarded client, checks that forged leftmost entries (public and private) are still locked out, and that another client is unaffected.
  - `api/clientip_test.go` covers the resolver, parsing and IPv6 keys.

### 1.2 Anonymous traffic could crowd out the administrator
- `src/backend/api/limits.go` (new):
  - At most 16 WebSocket sessions per public IP (`connectionsPerIP`).
  - A per-IP token bucket for anonymous web lookups.
  - Loopback, private and link-local addresses are exempt (see the risk in section 4).
- `src/backend/api/websocket.go`:
  - Anonymous web checks are charged to the bucket. The new setting is `PublicDomainsPerMinute`, default 500, range 1–100000; 0 or missing means the default.
  - Anonymous checks use a separate `web:public` pool with half of `WebCheckConcurrencyLimit`. The administrator uses `web:admin`.
- The new setting is in `config/config.go`, the admin settings page (`AdminSetting.vue`) and `deploy/backend/config.yaml` (800 there, to match that file's `WebCheckDomainLimit: 800`).

### 1.3 Time spent queuing counted as upstream timeout
- WHOIS (`whoislib/whois_query.go`), RDAP (`rdaplib/rdap_query.go`, new `fetch` helper), DNS (`dnslib/dnsresolver.go`, `NsCheck` now takes a timeout) and custom providers (`customize/customize.go`, `CustomizeLookup` now takes a timeout) start their timer after acquiring the per-upstream slot.
- `lookuper.Query` no longer wraps each attempt in a timeout.
- RDAP applies the timeout per server, and a timeout on one server moves on to the next.
- `utils/format_tools.go`: a plain `context.DeadlineExceeded` is now reported as "查询超时" (query timed out) instead of "Whois查询超时" (WHOIS query timed out). DNS reports its own timeout message.
- Interrupted DNS lookups (parent context canceled) are reported as interrupted, not as DNS timeouts.

### 1.4 No backoff after rate limiting
- `network/limits.go`: `Cooldown(key, d)` pauses new `Acquire` calls for an upstream, capped at two minutes. `CoolingDown(key)` reports the time left.
- `network/http.go`: `ReadResponseHeader` exposes response headers, and `RetryAfter` parses `Retry-After` in seconds or HTTP-date form. Missing or invalid values give 15 s.
- RDAP and custom-provider 429 responses start a cooldown.
- WHOIS replies that could not be parsed but match common throttling text (`RateLimitedResponse`) become `ErrorRateLimited` with a 30 s cooldown. This only applies when no registration data was found, so terms-of-use text cannot misclassify a registered domain.
- `lookuper.Query` now retries `ErrorRateLimited` like a timeout. Retries still follow `RetryOnTimeout` and `RetryMax`.
- WHOIS and RDAP slot keys get a `|proxy` suffix for proxied traffic, because direct and proxied requests leave from different addresses and are throttled separately.

### 1.5 Bulk re-check re-queried Unknown and Reserved rows
- `scheduler/bulk_check.go`: `moveErrors` sends rows with status `Error` (or no status, from older data) back to pending. `Unknown` and `Reserved` rows move to `bulkCheckErrorResult:kept`, and `restoreKept` renames that list back after the page loop. A restart during requeueing repeats both steps from Redis.
- `Info` counts the kept list. `Clear` and `prepare` delete it, and `Export` reads it, in case a re-check is canceled mid-requeue.
- The Redis key names and the single-task model are unchanged.
- Test: `TestRecheckRequeuesOnlyErrors`.

### 1.6 Bulk actions could freeze a WebSocket session
- `BulkCheck.lock(ctx)` waits for the mutex only until the context ends, then returns `ErrBulkState` (HTTP 409). It is used by `Upload`, `Start`, `Resume`, `Recheck`, `Stop`, `Clear` and `Export`.
- WebSocket bulk actions now run off the read loop, one at a time per connection (`bulkBusy`). A second action while one is running gets `bulkCheckError` "上一个批量操作仍在处理" (the previous bulk action is still running).

### 1.7 Uncertain registrations lost after a reload
- `register.Pending` lists operations that still hold a domain lock (`submitting` or `outcome_unknown`), oldest first, at most 500.
- New route `GET /api/admin/registrations`.
- New page `PendingRegistrations.vue` under Manage → 待核对注册 (pending registrations), with buttons to record success or failure through the existing `PUT /api/admin/registration/{operationId}`.
- Test: `TestPendingListsOnlyUnresolvedOperations`.

### 1.8 Sessions could not be revoked
- `api/auth.go`:
  - Tokens carry a `gen` claim. `POST /api/admin/logout` increments `powerwhois:auth:generation` in Redis, which revokes every token from earlier generations. `InitAuth` loads the generation at startup.
  - Tokens issued before this release have no `gen` claim and are treated as generation 0, so they keep working until the first logout.
- Open WebSocket admin sessions check the generation on every action and on the one-second status tick, then receive `adminAuthResult: false`.
- The frontend's logout calls the endpoint, then clears the local token even if the call fails.
- Caddy now sends `X-Content-Type-Options`, `X-Frame-Options`, `Referrer-Policy` and `Permissions-Policy`, plus a `Content-Security-Policy-Report-Only` header (see section 5).
- Test: `TestLogoutRevokesIssuedTokens`.

### 1.9 The README promised an API mode that did not exist
- New `GET /api/v1/check?domain=&queryType=&raw=`, authenticated by `X-API-Key` or `Authorization: Bearer`. Keys come from `POWERWHOIS_API_KEYS` (comma separated; keys shorter than 24 characters are ignored).
- The endpoint answers 404 when no keys are configured, and allows 600 requests per minute per key.
- It allows administrator lookup modes and shares `WebCheckConcurrencyLimit` through the `web:api` pool.
- Documented in `API documentation.md` and both READMEs.
- Test: `TestIntegrationAPIRequiresConfiguredKey`.

### 1.10 Container operations
- Setting `POWERWHOIS_LOG_STDOUT=true` (enabled in Compose) also writes the application log to stdout, so `docker compose logs` shows it.
- `/healthz` on the backend pings Redis. Caddy does not route it.
- Compose has a backend healthcheck (busybox `wget`), and Caddy waits for it with `condition: service_healthy`.

### 1.11 Smaller fixes
- A failed login returns **401** instead of 400. The frontend accepts both.
- Domain status mapping: `pendingDelete` > `redemptionPeriod` > `registrarHold` (`Expired`) > `serverHold`/`clientHold` (new `Hold`) > `Active`.
- `src/frontend/.env` points at `http://127.0.0.1:8080` instead of a private LAN address. The file is kept because `quasar.config.js` loads it.
- **Correction to the review:** the review said typo generation order was nondeterministic. That was wrong: only one map key matches per position, so the order was already stable. The map scans were still replaced with direct lookups, which give the same output with less work (`typo/typo.go`).

## 3. Files changed

- Backend:
  - `api/apis.go`, `api/auth.go`, `api/routes.go`, `api/websocket.go`; new `api/clientip.go`, `api/clientip_test.go`, `api/limits.go`, `api/session_test.go`
  - `config/config.go`, `constant/constant.go`, `log/logger.go`, `main.go`
  - `lookup/customize/customize.go`, `lookup/dnslib/dnsresolver.go`, `lookup/lookuper/lookuper.go`
  - `lookup/network/http.go`, `lookup/network/limits.go`; new `lookup/network/network_test.go`
  - `lookup/rdaplib/rdap_query.go`, `lookup/whoislib/whois_query.go`; new `lookup/whoislib/whois_ratelimit_test.go`
  - `register/register.go`, `register/register_test.go`
  - `scheduler/bulk_check.go`, `scheduler/export.go`, `scheduler/bulk_check_test.go`
  - `typo/typo.go`, `utils/format_tools.go`; new `utils/format_tools_test.go`
  - `security_regression_test.go` (now expects 401); new `proxy_regression_test.go`
- Frontend: `.env`, `src/pages/IndexPage.vue`, `src/components/AdminLogin.vue`, `src/components/AdminSetting.vue`; new `src/components/PendingRegistrations.vue`.
- Deploy: `deploy/caddy/Caddyfile`, `deploy/docker-compose.yml`, `deploy/backend/config.yaml`.
- Docs: `README.md`, `README.zh.md`, `API documentation.md`, `HANDOFF.md`, this file.

## 4. Behaviour changes and compatibility risks

1. **Login failures return 401 instead of 400.** External scripts that check for 400 must be updated.
2. **Logout revokes every administrator token** (single admin account). Logging out in one browser signs out all the others.
3. **Per-IP limits and the private-address exemption.**
   - Loopback, private and link-local client addresses are exempt from anonymous limits and the per-IP connection cap, on purpose. If a proxy is misconfigured and every visitor appears as one private address, limits switch off (the old behaviour) instead of all visitors, including the administrator before signing in, sharing one bucket of 16 connections.
   - With the right-to-left resolution, a remote client can only be classified as private if every hop to the right of it is a trusted proxy, which a client cannot forge. Remaining ways in, all requiring deployment changes:
     - Anything that can reach Caddy or the backend from a trusted (private) address can set any client address: host processes and other containers. The shipped Compose file publishes Caddy on `127.0.0.1` only. Do not publish it more widely while `trusted_proxies private_ranges` is set.
     - Behind a CDN, `$remote_addr` in Nginx is the CDN edge, so each edge address becomes one shared bucket. Configure Nginx's `real_ip` module with the CDN's ranges.
     - LAN users are never limited.
   - When `POWERWHOIS_TRUSTED_PROXIES` is unset, loopback is trusted; an invalid value stops startup.
   - IPv6 clients are limited per /64. A client with a larger allocation (for example a /48) can still spread over many /64 blocks; the anonymous bucket map is pruned of entries idle for over a minute once it passes 4096 entries.
4. **Total time per lookup can be longer.** Queuing is no longer cut off by `WhoisTimeout`, and rate-limited lookups wait for the cooldown and retry. Overall bounds are still the session or bulk context and the HTTP 2-minute timeout on `/api/v1/check`. Bulk throughput on a throttling registry should improve, because requests stop failing in bulk.
5. **WHOIS/RDAP concurrency through a proxy** is now a separate pool (3 for WHOIS, 4 for RDAP per server), on top of the direct pool.
6. **Bulk "re-check errors"** no longer re-queries Unknown and Reserved rows. The error counter still includes them, as before.
7. **WebSocket bulk actions are asynchronous.** Errors still arrive as `bulkCheckError`. Clients that sent several bulk actions back-to-back now get a busy error for the extra ones.
8. **Caddy now waits for the backend healthcheck.** If `wget` is missing from a custom image, Caddy will never start; the default `alpine:3.24.4` image includes busybox `wget`.
9. New Redis keys: `powerwhois:auth:generation` and, temporarily, `bulkCheckErrorResult:kept`.

## 5. Not done (follow-ups)

- **CSP is report-only.** Load the production site, check the browser console for CSP reports (Quasar inline styles, fonts and the WebSocket are allowed in the policy), then rename the header to `Content-Security-Policy`.
- **Not changed:**
  - The `AuthExpireDays` maximum stays 365. Lowering it would stop existing configurations from starting. Consider 30 once operators are told.
  - The read-only binary mount and splitting config and logs out of `deploy/backend` need executable-relative paths changed.
- Performance items from the review:
  - Caching DNS glue records and randomising nameserver order.
  - Caching RDAP bootstrap failures.
  - Configurable per-registry limits, which are still hard-coded at 3–4.
- UX items from the review:
  - Upload statistics (invalid and duplicate lines).
  - A cancel button for web checks.
  - Virtual scrolling for large result tables.
  - Showing internationalised domain names in Unicode.
  - i18n.
- Testing and process:
  - WHOIS fixture tests for each TLD.
  - `yarn audit` in CI, and pinning GitHub Actions to commit SHAs.
  - Tagged releases.
  - Redis TTLs for registration records and raw uploads.
- The admin settings form still caps web concurrency at 10 and bulk at 100, while the backend allows 64 and 128. This is unchanged.

## 6. Independent verification before merge

The PR head has not been verified by anyone other than the author. Before merging, a reviewer should, on Linux with Go 1.27.4, Node 22.23.5, Yarn 1.22.22, **`redis-server`** and Docker:

```bash
git fetch origin && git switch --detach origin/pw-cli-impl-20261122
gofmt -l src/backend                      # must print nothing
(cd src/backend && go vet ./... && go test -race -count=1 -v ./... 2>&1 | grep -E '^--- (SKIP|FAIL)')   # must print nothing
(cd src/frontend && yarn install --frozen-lockfile && yarn lint && yarn test && yarn build)
(cd deploy && POWERWHOIS_ADMIN_PASSWORD=x POWERWHOIS_JWT_SECRET=y docker compose config >/dev/null)
docker run --rm -v "$PWD/deploy/caddy/Caddyfile:/etc/caddy/Caddyfile:ro" caddy:2.11.9-alpine caddy validate --config /etc/caddy/Caddyfile
```

Then bring up the Compose stack once and confirm that the backend turns healthy and Caddy starts (item 7 in section 7). That is the first time the `wget` healthcheck and `depends_on: service_healthy` run for real.

Also review the diff, especially `api/clientip.go`, `api/auth.go` (token generation and logout) and `register/register.go` (`Pending`). Merge only when that review and the required `Verify PowerWhois` check on the PR both pass, for example with `gh pr merge --merge`. The Redis tests must show `PASS` in the CI log, not `SKIP`.

To publish the branch from an authenticated machine:

```bash
git push -u origin pw-cli-impl-20261122
gh pr create --base main --head pw-cli-impl-20261122 \
  --title "Review fixes: rate limits, proxy trust, upstream backoff, bulk recheck, sessions" \
  --body-file docs/handoff/2026-11-22-review-fixes.md
```

For reference, the commit was created with:

```bash
git add -A
git commit -F - <<'EOF'
Fix shared rate limits, upstream backoff, bulk recheck and session revocation

- Resolve client addresses right to left through trusted proxies, so
  forged X-Forwarded-For entries cannot bypass limits; key IPv6 by /64.
- Limit anonymous lookups and WebSocket sessions per public IP.
- Start lookup timeouts after the upstream slot is held.
- Pause rate-limited upstreams and retry after Retry-After.
- Recheck only Error rows in bulk tasks.
- Keep the WebSocket read loop free during bulk actions.
- List pending registrations for review.
- Revoke admin tokens on logout and return 401 for bad credentials.
- Add the API-key check endpoint, /healthz and stdout logging.

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
EOF
```

## 7. Acceptance checks after deployment

1. From two different public IPs, ten failed logins from one IP do not block the other (expect 401, then 429 only for the first IP). Sending `X-Forwarded-For: 10.0.0.1` or a different public address from the locked-out IP still gets 429.
2. An anonymous visitor running repeated 500-domain checks is refused with "匿名查询过于频繁" (anonymous queries too frequent) within the minute. A logged-in administrator is not affected.
3. Log in in two browsers, log out in one: the other drops to the login page within about a second.
4. Run a bulk DNS job containing undelegated names, then "重查错误域名" (re-check errors): only `Error` rows are queried again (check the remaining count).
5. Submit a registration against a sandbox or mock registrar that returns no marker, reload the page, then open Manage → 待核对注册 and record the outcome.
6. With `POWERWHOIS_API_KEYS` set: `curl -H "X-API-Key: …" "https://<host>/api/v1/check?domain=example.com"` returns JSON. Without the key it returns 401; with no keys configured it returns 404.
7. `docker compose ps` shows the backend as healthy, and `docker compose logs typonamer` shows application log lines.
8. While a registry is throttling, the log and results show "上游请求过多" (too many upstream requests) on retries rather than a wall of timeouts.

## 8. Rollback

Redeploy the previous release package and frontend as a pair. Data stays compatible:
- The bulk keys are unchanged.
- `powerwhois:auth:generation` is ignored by older releases, but tokens issued by this release keep working on them until they expire.
- If a re-check was interrupted, `bulkCheckErrorResult:kept` may be left behind; delete it, or run Clear, after rolling back.
