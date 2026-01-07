.PHONY: build clean test install run fmt vet lint all

BINARY_NAME=llm-translate
BINARY_DIR=bin
MAIN_PATH=cmd/llm-translate/main.go
GO=go
GOFLAGS=-v

all: clean fmt vet build

build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BINARY_DIR)
	$(GO) build $(GOFLAGS) -o $(BINARY_DIR)/$(BINARY_NAME) $(MAIN_PATH)

install:
	@echo "Installing $(BINARY_NAME)..."
	$(GO) install $(GOFLAGS) $(MAIN_PATH)

clean:
	@echo "Cleaning..."
	@rm -rf $(BINARY_DIR)
	$(GO) clean

test:
	@echo "Running tests..."
	$(GO) test $(GOFLAGS) ./...

test-coverage:
	@echo "Running tests with coverage..."
	$(GO) test -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report saved to coverage.html"

fmt:
	@echo "Formatting code..."
	$(GO) fmt ./...

vet:
	@echo "Running vet..."
	$(GO) vet ./...

lint:
	@echo "Running golangci-lint..."
	@command -v golangci-lint >/dev/null 2>&1 || { echo "golangci-lint not installed"; exit 1; }
	golangci-lint run

deps:
	@echo "Downloading dependencies..."
	$(GO) mod download
	$(GO) mod tidy

update-deps:
	@echo "Updating dependencies..."
	$(GO) get -u ./...
	$(GO) mod tidy

build-all:
	@echo "Building for all platforms..."
	@mkdir -p $(BINARY_DIR)
	GOOS=linux GOARCH=amd64 $(GO) build $(GOFLAGS) -o $(BINARY_DIR)/$(BINARY_NAME)-linux-amd64 $(MAIN_PATH)
	GOOS=linux GOARCH=arm64 $(GO) build $(GOFLAGS) -o $(BINARY_DIR)/$(BINARY_NAME)-linux-arm64 $(MAIN_PATH)
	GOOS=darwin GOARCH=amd64 $(GO) build $(GOFLAGS) -o $(BINARY_DIR)/$(BINARY_NAME)-darwin-amd64 $(MAIN_PATH)
	GOOS=darwin GOARCH=arm64 $(GO) build $(GOFLAGS) -o $(BINARY_DIR)/$(BINARY_NAME)-darwin-arm64 $(MAIN_PATH)
	GOOS=windows GOARCH=amd64 $(GO) build $(GOFLAGS) -o $(BINARY_DIR)/$(BINARY_NAME)-windows-amd64.exe $(MAIN_PATH)

run:
	@echo "Running $(BINARY_NAME)..."
	$(GO) run $(MAIN_PATH) $(ARGS)

help:
	@echo "Available targets:"
	@echo "  build          - Build the binary"
	@echo "  install        - Install the binary to GOPATH/bin"
	@echo "  clean          - Remove binary and clean build cache"
	@echo "  test           - Run tests"
	@echo "  test-coverage  - Run tests with coverage report"
	@echo "  fmt            - Format code"
	@echo "  vet            - Run go vet"
	@echo "  lint           - Run golangci-lint (must be installed)"
	@echo "  deps           - Download and tidy dependencies"
	@echo "  update-deps    - Update all dependencies"
	@echo "  build-all      - Build for all platforms"
	@echo "  run            - Run the application (use ARGS for arguments)"
	@echo "  all            - Run clean, fmt, vet, and build"
	@echo "  help           - Show this help message"