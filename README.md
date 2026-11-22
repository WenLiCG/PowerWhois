English | [中文](./README.zh.md)

# PowerWhois

## Web Version: [link](https://whois.1ka.net)

## screenshot
![screenshot1](./img/screen1.jpg)
![screenshot2](./img/screen2.jpg)
![screenshot3](./img/screen3.jpg)
![screenshot4](./img/screen4.jpg)
![screenshot5](./img/screen5.jpg)


## Features
1. Supports interactive and bulk domain checks with pause, resume and CSV export.
2. Clicking on the web query results will pop up the original WHOIS information.
3. Supports RDAP, WHOIS and DNS. Registry coverage varies, and DNS provides delegation evidence.
4. Supports proxy settings and allows setting proxies for different suffixes.
5. Added Mix mode, which allows freely setting direct connection, proxy, or DNS scanning for different suffixes.
6. Supports typo generation.
7. Supports custom WHOIS APIs, allowing you to add your commonly used registrar APIs to check domain availability.
8. Supports domain registration and custom register APIs, allowing you to add your commonly used registrar APIs for convenient domain registration.
9. Backend settings can control threads, failed retries, and various other settings.
10. Supports API mode. Set `POWERWHOIS_API_KEYS` to enable `GET /api/v1/check?domain=example.com` for single-domain checks, for example from WHMCS. The browser's WebSocket protocol is documented too. [See documentation](https://github.com/WenLiCG/PowerWhois/blob/main/API%20documentation.md)

## Directory Structure

```
.
├── deploy  (Deployment related)
│   ├── backend (Backend program)
│   ├── docker-compose.yml
│   ├── frontend    (Frontend program)
│   ├── nginx       (Nginx configuration files)
│   └── redis       (Redis related)
├── README.md   (Documentation)
└── src (Source code directory)
    ├── backend     (Backend source code)
    ├── frontend    (Frontend source code)
    └── Makefile
```

# Deployment

Download the `PowerWhois-linux` artifact from GitHub Actions for the commit you want, or build it below. Choose Linux `amd64` or `arm64`, verify the archive with its `.sha256` file, then extract it.

On a fresh install, set `POWERWHOIS_ADMIN_PASSWORD` and `POWERWHOIS_JWT_SECRET` in the environment or a private Compose `.env` file. Make `deploy/backend` and its configuration writable by the application user, UID/GID 1000 by default. Use `POWERWHOIS_UID` and `POWERWHOIS_GID` to match the deployment account. Set your hostname and certificate paths in `deploy/nginx/typonamer.conf`, include it in the host's Nginx configuration, and run `nginx -t`. Nginx terminates TLS. Compose publishes Caddy on host loopback only.

Run `docker compose up -d --build` from `deploy`. For upgrades, back up the running release, configuration and Redis data first. Replace the executable and SPA as a pair, and retain the existing runtime configuration and Redis data. Each installation uses one process for its legacy single bulk task. Verify migration separately if you used the unreleased multi-job branch.

Anonymous sessions can use direct WHOIS, RDAP, DNS and Mixed mode. Paid proxy and custom provider access requires an administrator. DNS without delegation is reported as unknown. A registrar confirms whether the domain can be purchased. Registrations with an uncertain outcome remain pending for review by operation ID, listed under **Manage → 待核对注册**.

Anonymous visitors are limited per public IP: 500 domains per minute by default (setting `publicDomainsPerMinute`), 16 WebSocket connections, and half of the web lookup concurrency. Loopback and private addresses are exempt, so the backend must see real client addresses. It reads `X-Forwarded-For` from the right and stops at the first address that is not a trusted proxy, so entries a client forges are ignored. `POWERWHOIS_TRUSTED_PROXIES` lists the trusted proxies: Compose trusts private ranges (only Caddy reaches the backend), and when unset only loopback is trusted, which suits a native install behind Nginx on the same host. IPv6 clients are limited per /64. Behind a CDN, configure Nginx's `real_ip` module so `$remote_addr` is the visitor rather than the CDN edge. Do not publish Caddy's port beyond loopback: anything that can reach Caddy or the backend from a private address can set the client address. When an upstream answers 429 or a WHOIS throttling message, PowerWhois pauses that upstream (honouring `Retry-After`, at most two minutes). Lookup timeouts count only the exchange with the upstream, not queuing. Within each WHOIS, RDAP or DNS upstream, anonymous lookups may hold only part of the shared slots (2 of 3, or 3 of 4), so administrators, bulk jobs and API clients always have capacity without raising the load on the registry.

Anonymous visitors share 64 WebSocket connections. A signed-in administrator's browser first requests a one-time ticket over HTTPS and connects with it into 8 reserved connections, so a flood of anonymous connections cannot lock the administrator out. Logging out revokes administrator tokens in every browser, closes reserved connections, and stops a running registration batch from submitting domains it has not sent yet; requests already sent still have their outcomes collected and stored.

Other environment variables: `POWERWHOIS_API_KEYS` (comma-separated keys of at least 24 characters; the integration API answers 404 when unset), `POWERWHOIS_LOG_STDOUT=true` (also log to stdout; set in Compose). The backend serves `/healthz` for health checks; Caddy does not expose it.

# Development

Use Go 1.27.4, Node 22 and Yarn 1.22.22. The release build runs on Linux.

```bash
make -C src setup
(cd src/backend && go test -race ./... && go vet ./...)
(cd src/frontend && yarn lint && yarn test)
make -C src package GOARCH=amd64
```

The package includes its source revision and a checksum. `make -C src all` builds directly into `deploy` for development. For native execution, set `POWERWHOIS_LISTEN` and `POWERWHOIS_CONFIG` to override the loopback listener and executable-relative configuration path.

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## 为爱发电
[爱发电](https://afdian.com/a/Powerwhois)
