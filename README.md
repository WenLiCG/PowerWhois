[English](README.md) | [中文](README.zh.md)

# PowerWhois

Domain research through the existing Web UI, a noninteractive CLI, a documented HTTP API, and an optional MCP adapter. One query core and one durable batch store; no AI vendor dependency.

- DNS, WHOIS/RDAP, Mixed, SOCKS proxy, custom query APIs, typo generation.
- Durable batches with IDs, pause/resume/cancel, retry, progress and paged exports.
- Accurate distinctions between registered, not_found, unknown, timeout and error. DNS absence is not availability.
- Existing Web administration and explicitly confirmed manual registration remain available. Agent query credentials cannot register, delete data or change settings.
- Loopback binding by default; use SSH forwarding or an authenticated TLS gateway remotely.

## Documentation

[Review and feature checklist](docs/REVIEW.md) · [CLI/API/MCP usage and permissions](docs/USAGE.md) · [Deploy, upgrade and rollback](docs/DEPLOYMENT.md) · [Test report](docs/TEST-REPORT.md) · [OpenAPI](src/backend/api/openapi.json)

Build requirements: Go 1.27.3, Node.js 24, npm, Redis. In `src`, run `make setup`, `make test`, `make all`. The Docker Compose configuration builds from the repository's source.

**The old tracked `deploy/backend/typonamer` and `deploy/frontend` files are legacy artifacts, not this release.** Follow the deployment guide and provide private random credentials before starting.

![Batch workspace](docs/screenshots/batch-desktop.jpg)

Licensed under the original [MIT license](LICENSE).
