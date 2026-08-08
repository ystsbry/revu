.PHONY: build test run tidy fmt vet clean install uninstall install-skills uninstall-skills

BIN := bin/revu
PKG := ./cmd/revu

# revu は cgo を使わないので、ARM ホスト上で QEMU 経由の amd64 Go を
# 動かしたときに gcc -m64 で落ちるのを避けるため既定で無効化する。
# 必要なら `make build CGO_ENABLED=1` で上書き可能。
export CGO_ENABLED ?= 0

# Override with `make install PREFIX=$HOME/.local` to avoid sudo, or
# `make install DESTDIR=/tmp/staging PREFIX=/usr/local` for packaging.
PREFIX ?= /usr/local
INSTALL_DIR := $(DESTDIR)$(PREFIX)/bin

# Override with `make install-skills CLAUDE_SKILLS_DIR=/path/to/skills`.
# Installs the plugin as a Claude Code skills-dir plugin: a single symlink
# $(CLAUDE_SKILLS_DIR)/revu -> plugin/, which exposes /revu:pr and /revu:edit.
CLAUDE_SKILLS_DIR ?= $(HOME)/.claude/skills
PLUGIN_SRC := $(CURDIR)/plugin
PLUGIN_LINK := $(CLAUDE_SKILLS_DIR)/revu

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

install-skills:
	@mkdir -p $(CLAUDE_SKILLS_DIR)
	@if [ -L $(PLUGIN_LINK) ]; then \
		rm -f $(PLUGIN_LINK); \
	elif [ -e $(PLUGIN_LINK) ]; then \
		echo "skip: $(PLUGIN_LINK) already exists (not a symlink)"; \
		exit 0; \
	fi; \
	ln -s $(PLUGIN_SRC) $(PLUGIN_LINK); \
	echo "Linked $(PLUGIN_LINK) -> $(PLUGIN_SRC)"

uninstall-skills:
	@if [ -L $(PLUGIN_LINK) ]; then \
		rm -f $(PLUGIN_LINK); \
		echo "Removed $(PLUGIN_LINK)"; \
	fi
