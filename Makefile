.PHONY: build run test lint docker-build clean mod-tidy

APP_NAME   := worker
BUILD_DIR  := bin
DOCKER_TAG := tetracore-worker

build:
	@echo "Building $(APP_NAME)..."
	go build -o $(BUILD_DIR)/$(APP_NAME) ./cmd/worker

run:
	go run ./cmd/worker

test:
	go test ./... -v -race -count=1

lint:
	golangci-lint run ./...

docker-build:
	docker build -t $(DOCKER_TAG) .

clean:
	rm -rf $(BUILD_DIR)
	go clean -testcache

mod-tidy:
	go mod tidy
