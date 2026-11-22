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
10. Supports API mode. You can use the program as an API, for example, integrate it with your WHMCS. [See documentation](https://github.com/WenLiCG/PowerWhois/blob/main/API%20documentation.md)

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

Anonymous sessions can use direct WHOIS, RDAP, DNS and Mixed mode. Paid proxy and custom provider access requires an administrator. DNS without delegation is reported as unknown. A registrar confirms whether the domain can be purchased. Registrations with an uncertain outcome remain pending for review by operation ID.

# Development

Use Go 1.27.3, Node 22 and Yarn 1.22.22. The release build runs on Linux.

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
