.PHONY: build clean run test install

BINARY_NAME=mac-profile-sync
BUILD_DIR=build
CMD_PATH=./cmd/mac-profile-sync

# Build the application
build:
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(BINARY_NAME) $(CMD_PATH)

# Build for release (with optimizations)
release:
	@mkdir -p $(BUILD_DIR)
	go build -ldflags="-s -w" -o $(BUILD_DIR)/$(BINARY_NAME) $(CMD_PATH)

# Clean build artifacts
clean:
	rm -rf $(BUILD_DIR)
	go clean

# Run the application
run: build
	./$(BUILD_DIR)/$(BINARY_NAME)

# Run tests
test:
	go test -v ./...

# Install to GOPATH/bin
install:
	go install $(CMD_PATH)

# Download dependencies
deps:
	go mod download
	go mod tidy

# Format code
fmt:
	go fmt ./...

# Lint code (requires golangci-lint)
lint:
	golangci-lint run

# Generate TLS certificates for testing
certs:
	@mkdir -p certs
	openssl req -x509 -newkey rsa:4096 -keyout certs/server.key -out certs/server.crt -days 365 -nodes -subj "/CN=mac-profile-sync"
