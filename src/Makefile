SHELL := /bin/bash
GOARCH ?= $(shell go env GOARCH)

.PHONY: setup backend bundle frontend all package
all: backend frontend

setup:
	cd backend && go mod download
	cd frontend && yarn install --frozen-lockfile

backend:
	cd backend && CGO_ENABLED=0 GOOS=linux GOARCH=$(GOARCH) go build -trimpath -buildvcs=true -o ../../deploy/backend/typonamer .

bundle:
	cd frontend && yarn build

frontend: bundle
	cp -r frontend/dist/spa/. ../deploy/frontend/

package: bundle
	bash package.sh $(GOARCH)
