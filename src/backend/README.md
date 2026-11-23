# PowerWhois backend

Go/Fiber + Redis. See [deployment](../../docs/DEPLOYMENT.md), [API/CLI/MCP](../../docs/USAGE.md) and [tests](../../docs/TEST-REPORT.md).

Build the service with `go build .`, CLI with `go build ./cmd/powerwhois`, MCP with `go build ./cmd/powerwhois-mcp`. Tests: `go test -race ./...` (Linux/WSL with redis-server).
