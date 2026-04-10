VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS = -ldflags "-s -w -X main.version=$(VERSION)"
BIN = bin/agent-memory

.PHONY: build build-frontend clean test docker-up docker-logs deploy

# Default: build everything
build: build-frontend
	cd backend && CGO_ENABLED=0 go build $(LDFLAGS) -o ../$(BIN) ./cmd/server/

# Copy frontend assets to Go embed directory
build-frontend:
	@rm -rf backend/cmd/server/web
	@mkdir -p backend/cmd/server/web
	@cp -r frontend/* backend/cmd/server/web/
	@echo "Frontend assets copied to backend/cmd/server/web/"

# Build migration tool
build-migrate: build-frontend
	cd backend && CGO_ENABLED=0 go build -o ../bin/agent-memory-migrate ./cmd/migrate/

# Run locally (requires Qdrant + Ollama)
run: build
	./$(BIN)

# Clean build artifacts
clean:
	rm -rf $(BIN) bin/ backend/cmd/server/web/

# Run tests
test:
	cd backend && go test ./...

# Docker: build and start all services
docker-up: build
	docker compose up -d --build

docker-logs:
	docker compose logs -f

docker-down:
	docker compose down

# Deploy to remote server
DEPLOY_HOST ?= openclaw@192.168.2.131
DEPLOY_DIR ?= /home/openclaw/agent-memory

deploy: build
	scp $(BIN) $(DEPLOY_HOST):$(DEPLOY_DIR)/agent-memory-new
	ssh $(DEPLOY_HOST) "pkill -f agent-memory; sleep 1; cd $(DEPLOY_DIR) && cp agent-memory agent-memory.bak && mv agent-memory-new agent-memory && chmod +x agent-memory && nohup ./agent-memory > /tmp/am.log 2>&1 &"
	@echo "Deployed to $(DEPLOY_HOST):$(DEPLOY_DIR)"
