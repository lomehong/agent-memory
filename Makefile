.PHONY: build run test clean docker-build docker-up docker-down

BUILD_CMD = export PATH=$$PATH:/usr/local/go/bin && go build -o bin/agent-memory ./cmd/server
BUILD_MIGRATE = export PATH=$$PATH:/usr/local/go/bin && go build -o bin/migrate ./cmd/migrate

build:
	$(BUILD_CMD)

run: build
	./bin/agent-memory -config config.yaml

test:
	export PATH=$$PATH:/usr/local/go/bin && go test -v -count=1 ./...

test-coverage:
	export PATH=$$PATH:/usr/local/go/bin && go test -v -coverprofile=coverage.out ./...
	export PATH=$$PATH:/usr/local/go/bin && go tool cover -html=coverage.out -o coverage.html

lint:
	export PATH=$$PATH:/usr/local/go/bin && go vet ./...

clean:
	rm -rf bin/ coverage.out coverage.html data/*.db

migrate:
	$(BUILD_MIGRATE)
	./bin/migrate -config config.yaml

docker-build:
	docker build -t agent-memory:latest .

docker-up:
	docker compose up -d

docker-down:
	docker compose down

docker-logs:
	docker compose logs -f agent-memory
