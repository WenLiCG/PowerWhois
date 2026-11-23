[English](README.md) | [中文](README.zh.md)

# PowerWhois

Domain research through the existing Web UI, a noninteractive CLI, a documented HTTP API, and an optional MCP adapter. One query core and one durable batch store; no AI vendor dependency.

- DNS, WHOIS/RDAP, Mixed, SOCKS proxy, custom query APIs, typo generation.
- Durable batches with IDs, pause/resume/cancel, retry, progress and paged exports.
- Accurate distinctions between registered, not_found, unknown, timeout and error. DNS absence is not availability.
- Existing Web administration and explicitly confirmed manual registration remain available. Agent query credentials cannot register, delete data or change settings.
- Loopback binding by default; use SSH forwarding or an authenticated TLS gateway remotely.

## Deploy with Docker Compose

Install Docker Engine with the Compose v2 plugin on a Linux host. For a fresh installation, clone this branch and create a private environment file.

```sh
git clone --branch codex/powerwhois-merge-check https://github.com/WenLiCG/PowerWhois.git
cd PowerWhois/deploy
install -m 600 .env.example .env
${EDITOR:-vi} .env
```

Set all four values in `.env`. Choose an administrator username and a password of at least 12 characters. Generate independent random values of at least 32 characters for `POWERWHOIS_JWT_SECRET` and `POWERWHOIS_QUERY_TOKEN`. Keep `.env` outside version control.

```sh
sudo chown 10001:10001 backend backend/config.yaml
sudo chmod 750 backend
sudo chmod 600 backend/config.yaml
docker compose config --quiet
docker compose up -d --build
docker compose ps
curl -fsS http://127.0.0.1:8080/api/web/setting
```

The Compose stack builds the backend and Web UI from source. UID 10001 needs write access to the configuration directory because the settings page replaces the file atomically. Redis persists task data under `deploy/redis/redisdata`.

Open `http://127.0.0.1:8080` and sign in with the administrator credentials. The port is bound to the host loopback interface. For a remote host, run `ssh -N -L 8080:127.0.0.1:8080 user@server` from your computer and open the same local URL. Use `docker compose logs --tail 100 typonamer caddy redis` to check startup.

## Native deployment

On Linux, install Go 1.27.3, Node.js 24, npm, make and Redis. From the repository root run `make -C src setup` followed by `make -C src all`. The binaries are in `dist/backend/` and the built Web UI is in `src/frontend/dist/spa/`. The deployment guide covers Redis configuration, the private environment file, `POWERWHOIS_WEB_ROOT`, the included systemd unit, and backup and migration steps for existing installations. [Native deployment, upgrades and rollback](docs/DEPLOYMENT.md)

## Documentation

[Review and feature checklist](docs/REVIEW.md) · [CLI/API/MCP usage and permissions](docs/USAGE.md) · [Test report](docs/TEST-REPORT.md) · [OpenAPI](src/backend/api/openapi.json)

![Batch workspace](docs/screenshots/batch-desktop.jpg)

Licensed under the original [MIT license](LICENSE).
