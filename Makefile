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
CLAUDE_SKILLS_DIR ?= $(HOME)/.claude/skills
SKILLS_SRC := $(CURDIR)/skills

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
	@for src in $(SKILLS_SRC)/*/; do \
		name=$$(basename $$src); \
		target=$(CLAUDE_SKILLS_DIR)/$$name; \
		if [ -L $$target ]; then \
			rm -f $$target; \
		elif [ -e $$target ]; then \
			echo "skip: $$target already exists (not a symlink)"; \
			continue; \
		fi; \
		ln -s $$src $$target; \
		echo "Linked $$target -> $$src"; \
	done

uninstall-skills:
	@for src in $(SKILLS_SRC)/*/; do \
		name=$$(basename $$src); \
		target=$(CLAUDE_SKILLS_DIR)/$$name; \
		if [ -L $$target ]; then \
			rm -f $$target; \
			echo "Removed $$target"; \
		fi; \
	done
