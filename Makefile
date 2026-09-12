.PHONY: all build build-all test test-race vet fmt fmt-check clean

BINARY ?= dorm-gateway
CMD_DIR ?= ./cmd/dorm-gateway
BUILD_DIR ?= ./bin

all: fmt-check vet test build

build:
	@mkdir -p $(BUILD_DIR)
	go build -ldflags="-s -w" -o $(BUILD_DIR)/$(BINARY) $(CMD_DIR)

build-all:
	@mkdir -p $(BUILD_DIR)
	GOOS=darwin GOARCH=amd64 go build -ldflags="-s -w" -o $(BUILD_DIR)/$(BINARY)-darwin-amd64 $(CMD_DIR)
	GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w" -o $(BUILD_DIR)/$(BINARY)-darwin-arm64 $(CMD_DIR)
	GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o $(BUILD_DIR)/$(BINARY)-linux-amd64 $(CMD_DIR)
	GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o $(BUILD_DIR)/$(BINARY)-linux-arm64 $(CMD_DIR)

test:
	go test -v ./...

test-race:
	go test -race -v ./...

vet:
	go vet ./...

fmt:
	gofmt -w -s .

fmt-check:
	@test -z "$$(gofmt -l .)" || (echo "Unformatted files found. Run 'make fmt'." && gofmt -l . && exit 1)

clean:
	rm -rf $(BUILD_DIR) *.out *.test *.prof
