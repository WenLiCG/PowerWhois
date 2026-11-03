.PHONY: setup backend frontend test all

setup:
	cd backend && go mod download
	cd frontend && npm ci

backend:
	mkdir -p ../dist/backend
	cd backend && go build -trimpath -o ../../dist/backend/typonamer .
	cd backend && go build -trimpath -o ../../dist/backend/powerwhois ./cmd/powerwhois
	cd backend && go build -trimpath -o ../../dist/backend/powerwhois-mcp ./cmd/powerwhois-mcp

frontend:
	cd frontend && npm run build

test:
	cd backend && go test -race ./...
	cd frontend && npm test && npm run lint

all: backend frontend
