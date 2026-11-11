.PHONY: setup backend frontend all

BACKEND_DEST_DIR=$(abspath $(dir $(lastword $(MAKEFILE_LIST)))/../deploy/backend)
FRONTEND_DEST_DIR=$(abspath $(dir $(lastword $(MAKEFILE_LIST)))/../deploy/frontend)

setup:
	@echo "Setting up backend go modules..."
	cd backend && GOPROXY=https://goproxy.cn go mod download
	@echo "Setup backend go modules finished"

	@echo "Setting up yarn modules..."
	cd frontend && yarn
	@echo "Setup yarn modules finished"

backend:
	@echo "Starting building backend..."
	@echo Backend file destination path is: $(BACKEND_DEST_DIR)
	mkdir -p $(BACKEND_DEST_DIR)
	cd backend && CGO_ENABLED=1 go build -trimpath -buildvcs=true -o typonamer -v
	rm -f $(BACKEND_DEST_DIR)/typonamer
	mv backend/typonamer $(BACKEND_DEST_DIR)/
	chmod +x $(BACKEND_DEST_DIR)/typonamer
	@echo "Building backend finished"

frontend:
	@echo "Starting building frontend..."
	@echo Frontend files destination path is: $(FRONTEND_DEST_DIR)
	mkdir -p $(FRONTEND_DEST_DIR)
	cd frontend && yarn build
	rm -rf $(FRONTEND_DEST_DIR)/* && cp -r frontend/dist/spa/* $(FRONTEND_DEST_DIR)/
	rm -rf frontend/dist
	@echo "Building frontend finished"

all: backend frontend
