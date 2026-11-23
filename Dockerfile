FROM node:24-bookworm-slim AS frontend
WORKDIR /src
COPY src/frontend/package*.json ./
RUN npm ci
COPY src/frontend/ ./
RUN npm run build

FROM golang:1.27.4-bookworm AS backend-build
WORKDIR /src
COPY src/backend/go.mod src/backend/go.sum ./
RUN go mod download
COPY src/backend/ ./
RUN CGO_ENABLED=0 go build -trimpath -o /out/typonamer . && CGO_ENABLED=0 go build -trimpath -o /out/powerwhois ./cmd/powerwhois && CGO_ENABLED=0 go build -trimpath -o /out/powerwhois-mcp ./cmd/powerwhois-mcp

FROM debian:bookworm-slim AS backend
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/* && useradd --uid 10001 --create-home --home-dir /var/lib/powerwhois powerwhois
COPY --from=backend-build /out/ /usr/local/bin/
WORKDIR /var/lib/powerwhois
USER 10001:10001
ENV HOME=/var/lib/powerwhois POWERWHOIS_CONFIG=/etc/powerwhois/config.yaml POWERWHOIS_LOG_DIR=/var/lib/powerwhois/logs
ENTRYPOINT ["/usr/local/bin/typonamer"]

FROM caddy:2 AS gateway
COPY --from=frontend /src/dist/spa/ /typonamer/
COPY deploy/caddy/Caddyfile /etc/caddy/Caddyfile
