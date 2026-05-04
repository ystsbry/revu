.PHONY: build test run tidy fmt vet clean install uninstall

BIN := bin/revu
PKG := ./cmd/revu

# Override with `make install PREFIX=$HOME/.local` to avoid sudo, or
# `make install DESTDIR=/tmp/staging PREFIX=/usr/local` for packaging.
PREFIX ?= /usr/local
INSTALL_DIR := $(DESTDIR)$(PREFIX)/bin

build:
	@mkdir -p bin
	go build -o $(BIN) $(PKG)

test:
	go test ./...

run: build
	@$(BIN) $(ARGS)

tidy:
	go mod tidy

fmt:
	gofmt -w .

vet:
	go vet ./...

clean:
	rm -rf bin/

install: build
	install -d $(INSTALL_DIR)
	install -m 0755 $(BIN) $(INSTALL_DIR)/revu
	@echo "Installed revu to $(INSTALL_DIR)/revu"

uninstall:
	rm -f $(INSTALL_DIR)/revu
	@echo "Removed $(INSTALL_DIR)/revu"
