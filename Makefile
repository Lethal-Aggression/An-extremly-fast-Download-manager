# TurboDL - Multi-Interface Hardware Speed Aggregator
BINARY_NAME=turbodl
WIN_BINARY_NAME=turbodl.exe

LDFLAGS=-s -w

.PHONY: all build linux windows clean run

all: build

build: linux windows

linux:
	@echo "[*] Building native Linux binary..."
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o $(BINARY_NAME) ./cmd/turbodl
	@echo "[OK] Created $(BINARY_NAME)"

windows:
	@echo "[*] Building native Windows binary (turbodl.exe)..."
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o $(WIN_BINARY_NAME) ./cmd/turbodl
	@echo "[OK] Created $(WIN_BINARY_NAME)"

run: linux
	./$(BINARY_NAME)

clean:
	rm -f $(BINARY_NAME) $(WIN_BINARY_NAME) turbodl_config.json
