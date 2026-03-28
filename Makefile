.PHONY: run build test fmt vet clean

APP_NAME=cache-proxy-server
MAIN_FILE=cmd/server/main.go
BIN=bin/$(APP_NAME)

run:
	go run $(MAIN_FILE)

build:
	@mkdir -p bin
	go build -o $(BIN) $(MAIN_FILE)

fmt:
	go fmt ./...

vet:
	go vet ./...

clean:
	rm -rf bin
