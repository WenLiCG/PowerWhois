# PowerWhois review fixes — handoff (2026-11-22)

独立验证补记见第 9 节。第 1–8 节保留作者在源候选 `de0fad6` 上的原文，不把作者自述改成验证结论。

## 1. Status — read this first

| Item | State |
|---|---|
| Base | `main` at `6662f8ae55447140152119a90b0cf9399159bb21` |
| Branch | `pw-cli-impl-20261122`, **local only**: `b093223` (first candidate) plus one commit fixing verification findings F1–F4 (section 2a). The new commit is the candidate |
| Independent verification | `b093223` was rejected (NO-GO) with findings F1–F4 and handoff corrections. **The new candidate has not been independently verified** |
| Push and pull request | **Not done**, and not to be done until the new candidate is verified. The authoring machine has no GitHub credentials |
| Merge into `main` | **Not done.** Requires independent verification of the new candidate and a passing `Verify PowerWhois` check |

Local results on the new candidate tree (Linux, Go 1.27.4, Node 22.23.5, Yarn 1.22.22; Redis 7.0.15 from the verifier's isolated, extracted Debian package via a scoped `PATH`, not installed). Every command's own exit code was recorded, not a pipeline's:

| Check | Exit code and result |
|---|---|
| `gofmt -l .` | 0, no files listed |
| `go vet ./...` | 0, no output |
| `go test -race -count=1 -v ./...` | **0**. 42 top-level tests PASS, 0 FAIL, **0 SKIP** (Redis present), 0 `DATA RACE` warnings |
| `yarn lint` | 0 |
| `yarn test` | 0, 4 tests, 4 pass, 0 skipped |
| `yarn build` | 0 |
| The verifier's probes (their overlay, unchanged) | 1 overall: `TestIndependentLogoutStopsUnsubmittedRegistrations` and `TestIndependentProxyAndAPIKeyBoundaries` PASS. Three still fail **by design**, because they encode assumptions the fixes change (section 2a lists each and the regression test that replaces it). `pending-identity.mjs` exits 1 because its regex no longer finds the filter, which moved to `src/utils/pendingRegistrations.js` |
| Caddyfile | Unchanged since `b093223`; earlier `caddy validate` (v2.11.9) valid, and Caddy's `X-Forwarded-For` forwarding checked against an echo upstream |
| `docker-compose.yml` | Unchanged since `b093223`. **`docker compose config` and a real container start have still not been run** (no Docker on the author's or the verifier's machine) |
| Browser | **The new ticket flow in `websocketHandler.js` has not been exercised in a real browser.** The server side is covered by real WebSocket tests |

The first candidate `b093223` passed 35 top-level Go tests with zero skips on the verifier's machine; its four blockers are fixed below.

## 2a. Fixes for the independent verification of `b093223`

Each finding was reproduced on `b093223` with the verifier's probes before it was fixed. For each new regression test, the fix was temporarily removed and the test was confirmed to fail.

### F1 — anonymous clients could fill every WebSocket slot before the administrator authenticated
- Cause: authentication happened in a message after admission, and every connection competed for the same 64 slots. Four public addresses (16 each), or one exempt private client, locked the administrator out.
- Fix (`api/admission.go`, new):
  - `POST /api/admin/ws-ticket` (signed in) returns a one-time ticket, valid for 30 s.
  - `/app/ws?ticket=…` redeems it during the upgrade and admits the connection into a separate pool of 8 reserved connections, with administrator access already granted (`adminAuthResult: true` is sent at once).
  - Anonymous connections still share 64 slots with the per-address cap.
  - HTTP is not limited by the WebSocket pools, so the ticket request works while those slots are full.
  - Tickets are single-use, bound to the session generation, cleared by logout, capped at 256 outstanding, and 32 random bytes each. An invalid or used ticket falls back to an ordinary connection.
  - When the session expires or is revoked, the server closes reserved connections so their slots return.
- Frontend (`src/utils/websocketHandler.js`): when signed in, each connection first requests a fresh ticket. The module reconnects itself instead of using VueUse `autoReconnect`, which would reuse the spent ticket and could open two sockets. Without a token, or if the ticket request fails, it connects as before and still sends `adminAuth`.
- **Found while fixing F1 (also present on `main`):** `Close()` on a hijacked fasthttp connection is a no-op until the handler returns (`fasthttp` `hijackConn.Close`; Fiber does not set `KeepHijackedConns`). So the shutdown hook and the write-error paths in `websocket.go` never closed anything; graceful shutdown waited for each idle client's next message. `closeConn` now also expires the read deadline, which ends the read loop.
- Tests (`api/admission_test.go`):
  - `TestTicketAdmitsAdministratorWhilePublicPoolIsFull` runs both of the verifier's scenarios over real connections: the 65th anonymous connection is refused, the ticketed administrator is admitted and answers ping, and a replayed ticket is refused.
  - `TestTicketsRequireAValidSessionAndExpireOnLogout` covers four things: an anonymous ticket request gets 401; logout closes reserved connections, including one whose client never reads (only a server-side close frees that slot); all 8 slots are usable again afterwards; and the cap is 8.
  - `TestShutdownClosesIdleSessions`: drain finishes with an idle session open.
- **Replaced probe:** `TestIndependentAdminConnectionReservation` dials the administrator without any credential. Such a connection cannot be told apart from an anonymous one, so it stays in the public pool by design; the ticketed test above covers the same saturation scenarios.

### F2 — a running registration batch kept submitting after logout
- Cause: logout only changed the session generation; the batch's context depended on the WebSocket and the token's expiry time, and `Register` never checked revocation.
- Fix:
  - `register.Register` takes a `mayStart` check, consulted before the domain is claimed (nothing is recorded) and again immediately before the request is sent. If the claim was made, a definite `failed` record "管理员会话已结束，此域名未提交" (session ended, domain not submitted) is stored, which releases the domain.
  - The WebSocket batch passes `sessionStillValid(s.isAdmin)`.
  - `Logout` holds `submissionGate` (write lock) while changing the generation; the check takes the read lock. Once logout returns 204, no domain that had not passed the check can be sent.
  - A request that passed the check just before logout may reach the registrar a moment later. That is the "already submitted" case: its outcome is collected and stored with `WithoutCancel`, as before.
- Tests:
  - `TestLogoutStopsUnsubmittedRegistrations` (real WebSocket, mock registrar, isolated Redis): the in-flight request finishes and its success is stored; the second domain never reaches the registrar and is never claimed.
  - `register.TestRevokedSessionSubmitsNothing` covers both checkpoints and confirms the domain is not left locked.
  - The verifier's own probe now passes.

### F3 — the separate web pools did not reserve any real upstream capacity
- Cause: `web:public` and `web:admin` were outer pools; inside them, everyone shared the same WHOIS (3), RDAP (4) and DNS (4) slots, which anonymous work could fill.
- Fix (`network/limits.go`):
  - Anonymous web checks run with `network.WithPublic(ctx)`. On every shared upstream key, `Acquire` lets public work hold at most `PublicLimit(limit)` slots: 2 of 3, 3 of 4, keeping `max(1, limit/4)` reserved.
  - Privileged work (administrator web checks, Typo, bulk, API) may use every slot.
  - The total per upstream is unchanged, so registry load is not multiplied. Cooldowns stay shared.
- Tests:
  - `TestAnonymousWorkCannotTakeReservedUpstreamSlots`: public work stops at its share, the administrator gets the reserved slots, the total never exceeds the limit, and freed public slots are reusable.
  - `TestAnonymousLookupsUsePublicUpstreamShare`: over a real WebSocket, anonymous checks carry the public marker and administrator checks do not.
- **Replaced probe:** `TestIndependentUpstreamAdminReservation` fills the upstream with unmarked contexts, which the server treats as privileged work. The two tests above cover the scenario with real anonymous marking.

### F4 — the pending-registration page treated a batch ID as a row ID
- Fix: `src/utils/pendingRegistrations.js` identifies rows by operation ID and domain together. The table's `row-key` uses it, and reconciling removes only that row.
- Test: `tests/pendingRegistrations.test.mjs`, where reconciling one domain of a two-domain batch keeps the other.

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
  - Anonymous checks use a separate `web:public` pool with half of `WebCheckConcurrencyLimit`. The administrator uses `web:admin`. These outer pools only limit how many lookups each kind of session runs; the actual upstream reservation is the public share described under F3.
  - Administrator connections: see F1.
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
- It allows administrator lookup modes. **Pool accounting:** API requests go through their own outer pool `web:api`, sized to `WebCheckConcurrencyLimit`. That is a separate pool with the same number, not a share of one web total, so the API can run that many lookups alongside the web pools. All of them then share the real per-upstream slots (3 per WHOIS server, 4 per RDAP server and per TLD for DNS). API work counts as privileged, so it can also use the slots reserved from anonymous visitors.
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
  - `api/apis.go`, `api/auth.go`, `api/routes.go`, `api/websocket.go`; new `api/admission.go`, `api/admission_test.go`, `api/clientip.go`, `api/clientip_test.go`, `api/limits.go`, `api/session_test.go`
  - `config/config.go`, `constant/constant.go`, `log/logger.go`, `main.go`
  - `lookup/customize/customize.go`, `lookup/dnslib/dnsresolver.go`, `lookup/lookuper/lookuper.go`
  - `lookup/network/http.go`, `lookup/network/limits.go`; new `lookup/network/network_test.go`
  - `lookup/rdaplib/rdap_query.go`, `lookup/whoislib/whois_query.go`; new `lookup/whoislib/whois_ratelimit_test.go`
  - `register/register.go`, `register/register_test.go`
  - `scheduler/bulk_check.go`, `scheduler/export.go`, `scheduler/bulk_check_test.go`
  - `typo/typo.go`, `utils/format_tools.go`; new `utils/format_tools_test.go`
  - `security_regression_test.go` (now expects 401); new `proxy_regression_test.go`
- Frontend: `.env`, `src/pages/IndexPage.vue`, `src/components/AdminLogin.vue`, `src/components/AdminSetting.vue`, `src/utils/websocketHandler.js`; new `src/components/PendingRegistrations.vue`, `src/utils/pendingRegistrations.js`, `tests/pendingRegistrations.test.mjs`.
- Deploy: `deploy/caddy/Caddyfile`, `deploy/docker-compose.yml`, `deploy/backend/config.yaml`.
- Docs: `README.md`, `README.zh.md`, `API documentation.md`, `HANDOFF.md`, this file.

## 4. Behaviour changes and compatibility risks

1. **Login failures return 401 instead of 400.** External scripts that check for 400 must be updated.
2. **Logout revokes every administrator token** (single admin account). Logging out in one browser signs out all the others, closes reserved WebSocket connections, and stops running registration batches from sending domains not yet submitted. Those domains are reported as `error` "管理员会话已结束，此域名未提交" (session ended, domain not submitted) and can be submitted again after signing in.
   - **Administrator WebSocket tickets.** The ticket is in the connection URL, so Nginx access logs may record it. It is single-use and expires in 30 s, so a logged value is useless once the connection is made. Eight reserved connections are shared by all administrator browsers; a ninth falls back to the public pool.
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
10. **Anonymous lookups get at most 2 of 3 WHOIS or 3 of 4 RDAP/DNS slots per upstream.** Under heavy anonymous load that is one slot fewer than before for anonymous visitors on each upstream.

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

The new candidate has not been verified by anyone other than the author. Before merging, a reviewer should, on Linux with Go 1.27.4, Node 22.23.5, Yarn 1.22.22, **`redis-server`** and Docker, check out the candidate commit and run each command separately, saving raw output and recording each command's own exit code. Do not pipe a test command into `grep`: the pipeline's status is `grep`'s, and a build failure need not print `--- FAIL`.

```bash
set -o pipefail
mkdir -p /tmp/pw-verify
gofmt -l src/backend | tee /tmp/pw-verify/gofmt.log; echo "gofmt exit=$? files=$(wc -l < /tmp/pw-verify/gofmt.log)"   # files must be 0
(cd src/backend && go vet ./...) 2>&1 | tee /tmp/pw-verify/vet.log; echo "vet exit=$?"
(cd src/backend && go test -race -count=1 -v ./...) > /tmp/pw-verify/go-test.log 2>&1; echo "go test exit=$?"
grep -c '^--- PASS' /tmp/pw-verify/go-test.log; grep -c -- '--- SKIP' /tmp/pw-verify/go-test.log; grep -c 'DATA RACE' /tmp/pw-verify/go-test.log   # expect 42, 0, 0
(cd src/frontend && yarn install --frozen-lockfile && yarn lint && yarn test && yarn build) > /tmp/pw-verify/frontend.log 2>&1; echo "frontend exit=$?"
(cd deploy && POWERWHOIS_ADMIN_PASSWORD=x POWERWHOIS_JWT_SECRET=y docker compose config >/dev/null); echo "compose exit=$?"
docker run --rm -v "$PWD/deploy/caddy/Caddyfile:/etc/caddy/Caddyfile:ro" caddy:2.11.9-alpine caddy validate --config /etc/caddy/Caddyfile; echo "caddy exit=$?"
```

Then:
1. Bring up the Compose stack once and confirm that the backend turns healthy and Caddy starts (item 7 in section 7). That is the first time the `wget` healthcheck and `depends_on: service_healthy` run for real.
2. In a real browser, signed in: the Network tab shows `POST /api/admin/ws-ticket` followed by `/app/ws?ticket=…`. After restarting the backend, the page reconnects with a new ticket, one socket at a time. While anonymous connections fill the public pool (for example 64 scripted connections from loopback), the administrator still connects (items 9 and 10 in section 7).
3. Review the diff, especially `api/admission.go`, `api/clientip.go`, `api/auth.go` (generation, logout, `submissionGate`), `network/limits.go` (public share) and `register/register.go` (`mayStart`, `Pending`).

Merge only when that verification and the required `Verify PowerWhois` check on the PR both pass, for example with `gh pr merge --merge`. The Redis tests must show `PASS` in the CI log, not `SKIP`.

To publish the branch from an authenticated machine:

```bash
git push -u origin pw-cli-impl-20261122
gh pr create --base main --head pw-cli-impl-20261122 \
  --title "Review fixes: rate limits, proxy trust, upstream backoff, bulk recheck, sessions" \
  --body-file docs/handoff/2026-11-22-review-fixes.md
```

For reference, the first candidate commit (`b093223`) was created with:

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
9. With 64 anonymous connections open (scripted, from several addresses), a signed-in administrator page still connects, and the bulk pause and cancel buttons work.
10. Start a registration batch of several domains against a mock or sandbox registrar that answers slowly, and log out while the first request is in flight. Only that domain reaches the registrar; its outcome appears under Manage → 待核对注册 or as a stored result after signing in again. The others report "管理员会话已结束，此域名未提交" (session ended, domain not submitted).
11. In 待核对注册, with two uncertain domains from one batch, recording one leaves the other listed.

## 8. Rollback

Redeploy the previous release package and frontend as a pair, after these steps.

**1. Back up Redis** (for example `redis-cli BGSAVE` and copy the RDB and AOF files) before changing anything.

**2. Do not delete `bulkCheckErrorResult:kept`.** This list exists only while a re-check is requeueing or was interrupted mid-requeue. It holds complete `Unknown` and `Reserved` results that have been moved out of `bulkCheckErrorResult`. Older releases do not know the key: they neither count nor export it, and their Clear leaves it in place. Deleting it, or rolling back while it exists, loses those results. With this release still running:

```bash
redis-cli EXISTS bulkCheckErrorResult:kept     # 0: nothing to do
```

If it returns 1, use either option:
- **Finish the requeue:** press 继续任务 (resume) or let the task run. The requeue step returns the list to `bulkCheckErrorResult` (`restoreKept`), after which the key no longer exists.
- **Or export first:** download the CSV with this release (its export includes the kept rows), keep it with the backup, and then use the manual recovery below.

If you have already rolled back and the key exists, return the rows to the list the older release reads. Every element is a complete result row, and a domain is never in both lists:

```bash
redis-cli EVAL "local r = redis.call('LRANGE', KEYS[1], 0, -1) for _, v in ipairs(r) do redis.call('RPUSH', KEYS[2], v) end redis.call('DEL', KEYS[1]) return #r" 2 bulkCheckErrorResult:kept bulkCheckErrorResult
```

**3. Rotate `POWERWHOIS_JWT_SECRET` when rolling back.** Older releases ignore the `gen` claim and `powerwhois:auth:generation`. Every unexpired token signed with the current secret would become valid again, including tokens revoked by logout. A new secret invalidates them all; administrators then sign in again.

**4. Other data stays compatible.** The bulk keys are unchanged. Registration records written by this release, including the new `failed` "此域名未提交" (domain not submitted) records, use the existing format.

## 9. Independent verification (Cursor, 2026-11-22)

This section was written by the handoff reviewer, not by the author of `de0fad6`. No product logic was rewritten. The two commits above were applied with `git am` onto `main` `6662f8ae55447140152119a90b0cf9399159bb21` (confirmed with `git fetch origin main` immediately before apply; local `main` and `origin/main` were that commit). Author remains WenLiCG; both messages keep `Co-Authored-By: Claude Opus 5.5`. Committer is Cursor Agent, so the SHAs differ from the source candidate:

| Source candidate | Applied commit | Tree |
|---|---|---|
| `b0932238c7c358dd9c52d4e4767986aaafbd012f` | `a972b8974d74b92ed852160196d22faa88e2dd94` | `dee5cb71dc63032201f57639a1139a1a73be6dd0` |
| `de0fad6913136724e963e6bf135f1abd42f47ea2` | `ef816e382e5ccdaa881f40bc6f931fe252d2701e` | `b3bfc248b5862a4fdfe85443572e5350021e1470` |

`git apply --check` succeeded before `git am`. The verification files added with this section have parent `ef816e382e5ccdaa881f40bc6f931fe252d2701e`. The pull request body records the exact head; this file does not predict that SHA.

Role: independent verification and release preparation. Not merged. Not deployed. No production configuration, domain purchase, or external message.

The previous probes (`independent_api_test.go`, `pending-identity.mjs`) were not scored. They dial an administrator with no credential, fill upstream slots without the anonymous marker, and match a filter expression that moved to `pendingRegistrations.js`. Those mismatches are not a pass or a product failure. The checks below use the ticket, `WithPublic`, and `withoutPendingRow`.

### F1–F4 and the related regressions

| Item | Result |
|---|---|
| F1 public saturation | Pass. Four public addresses, 16 connections each, and a separate direct loopback source with no `X-Forwarded-For`, each filled 64 public sockets. The 65th anonymous socket was refused. HTTP login still returned a token. All 8 ticket connections received `adminAuthResult: true` and answered ping. A 9th ticket was not admitted. |
| Ticket reuse, expiry, logout | Pass as refusal of administrator admission. Reuse, a real 31-second wait past the 30-second TTL, and logout did not produce `adminAuthResult: true`. With room in the public pool the socket stays open as an ordinary connection; it does not enter the reserved pool. `expiresIn` was 30. |
| Frontend reconnect | Pass in Chrome against the isolated Compose stack on `127.0.0.1:8080`. Anonymous load opened one socket with no ticket. After login the page reloaded, `POST /api/admin/ws-ticket` ran, the plain socket closed, and exactly one ticket socket remained. Restarting the backend container closed that socket; the page requested a new ticket and opened one ticket socket. |
| F2 logout vs registration | Pass against a local mock registrar only. Logout returned 204 while the first request was blocked. That request then completed, its success was stored in Redis, and its `Idempotency-Key` was the operation id plus domain. `example.net` was not requested and was not claimed. |
| F3 upstream share | Pass for the executed paths. Anonymous `lookuper.Query` to a local provider stopped at 3 of 4 while an API-key request took the remaining slot; the provider never saw a 5th concurrent call. The WebSocket path marked `anonymous.com` public and `admin.com` not public. WHOIS, RDAP and DNS pass that same context into `Acquire` before dialing (`whois_query.go`, `rdap_query.go` `fetch`, `dnsresolver.go` `NsCheck`); this run did not open a registry connection to prove those three dials. Bulk calls `lookuper.Query` without `WithPublic` (`bulk_check.go`). Public waiters did not block the reserved slot, a cancelled waiter did not leak a slot, and a race run stayed within the limit. |
| F4 pending rows | Pass. The helper keeps the other domain. The built page, in Chrome, listed `example.com` and `example.net` under one operation id; recording failure for `example.com` left `example.net` on screen and still `outcome_unknown` in Redis. `example.com` became `failed` and lost its pending field. No registrar was contacted. |
| Right-to-left XFF and IPv6 /64 | Pass in the existing unit tests (`TestResolveClientIPResistsForgedForwardedFor`, `TestLimitKeyGroupsIPv6By64`, `TestLoginLimiterUsesForwardedClientBehindTrustedProxy`). No live IPv6 client was connected. |
| API key limit | Pass. A key in the query string without the header was 401. Five invalid keys were 401. 600 valid-key requests with an invalid domain were 400. Request 601 was 429. A further invalid key stayed 401. |
| Idempotency and funds | The existing registration CAS test passed, and the F2 mock observed a single idempotency key. No real registration or payment. |

### Commands and environment

Linux amd64, UTC. Go 1.27.4 was the official linux-amd64 tarball. Test Redis was Ubuntu `redis-server` 7.0.15, used only through `testredis` Unix sockets; no system Redis was left running. Frontend commands used the environment Node v22.23.4 and Yarn 1.22.22. The documented CI Node is 22.23.5; this Node is still in the package's `^22` range. Caddy was the official v2.11.9 linux-amd64 binary. Docker was Ubuntu `docker.io` 29.1.3 and Compose 2.40.3, installed for this environment. The first dockerd had been started without a bridge; after a normal restart, `iptables-legacy` still had `FORWARD` policy DROP and dropped bridge traffic that nftables had accepted. Setting that legacy policy to ACCEPT was an environment fix. Container-to-container TCP and `127.0.0.1` published ports then worked. That change is not in the repository.

| Check | When (UTC) | Exit | Result |
|---|---|---|---|
| `gofmt -l src/backend` on the applied candidate, before the extra tests | 2026-11-22 15:39:22–15:40:21 | 0 | no files |
| `go vet ./...`, `go build ./...` | same window | 0, 0 | no diagnostics |
| `go test -race -count=1 -v ./...` on the applied candidate | same window | 0 | 42 top-level PASS, 0 FAIL, 0 SKIP, 0 `DATA RACE` |
| Adapted `TestIndependentVerify*` with `-race -count=1 -v` | log mtime 2026-11-22 15:46:09 | 0 | all new tests PASS, including the 31-second ticket expiry |
| `go test -race -count=1 ./...` after those tests were added | 15:46:24–15:47:02 | 0 | 11 packages `ok`, 0 SKIP, 0 `DATA RACE`. This run had no `-v`, so it did not reprint the PASS count |
| `yarn install --frozen-lockfile`, `yarn lint`, `yarn test`, `yarn build` | 15:46:26–15:46:38 | 0 | lint empty; tests 5 pass, 0 fail, 0 skipped. Node printed `MODULE_TYPELESS_PACKAGE_JSON` while loading two files; the test runner still exited 0 |
| `caddy validate --config deploy/caddy/Caddyfile` | 15:39:24 | 0 | `Valid configuration`, Caddy v2.11.9 |
| `docker compose --env-file … config` in a temporary copy of `deploy` | after image pull | 0 | rendered config binds `127.0.0.1:8080`, waits on `service_healthy`, uses `redis:7.4.12-alpine` and `caddy:2.11.9-alpine` |
| `docker compose -p pwverify up -d --build` | immediately after | 0 | redis healthy, typonamer healthy, caddy up. `http://127.0.0.1:8080/` returned the SPA. Inside the backend container, `/healthz` returned `ok`. Through Caddy, `/healthz` is the SPA HTML because the Caddyfile does not route it |
| Browser flow on that stack | after health | — | login 200 and ticket `expiresIn` 30 through Caddy; Chrome results in the table above. Stack then removed with `docker compose down -v`. Temporary passwords were not printed and the env file was deleted |

No test was skipped. No command in this table was retried after a product failure. The Docker network setup above was an environment retry before the stack was considered. `docker compose config` was not used as a substitute for the running stack.

### Compatibility, rollback, and what remains

This verification commit does not change runtime configuration. The patch's compatibility notes in sections 4 and 8 still apply: do not delete `bulkCheckErrorResult:kept`; export or finish a requeue first; rotate `POWERWHOIS_JWT_SECRET` on rollback so revoked tokens cannot become valid again; the API outer pool is its own `web:api` budget of `WebCheckConcurrencyLimit`, then shares the real upstream slots.

Not done here, and not claimed:

- Merge, deploy, or a production login.
- A browser session that itself holds 64 anonymous sockets. The saturation cases ran as Go WebSocket clients.
- A live IPv6 client, or a WHOIS/RDAP/DNS dial that proves the public marker inside those three functions. The marker was proved at `lookuper.Query` and on the WebSocket call into it.
- The follow-ups already listed in section 5, including enforcing CSP.
- GitHub `Verify PowerWhois` at the time this file was written. The pull request is where that check is recorded after it runs.

Next owner: the parent coordinator compares the pull request head with this history and reads the GitHub check before any merge. This verification does not merge the pull request.
