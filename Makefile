SHELL := /bin/sh

GO ?= go
NPM ?= npm
BIN_DIR ?= bin
UI_DIR := ui

CMDS := gonemaster gonemaster-server
CMD ?= all

.PHONY: help build build-all test install ui-build ui-install ui-dev clean \
	build-gonemaster build-gonemaster-server install-gonemaster install-gonemaster-server

help:
	@echo "Targets:"
	@echo "  build            Build all commands (override CMD=gonemaster-server)"
	@echo "  test             Run Go tests"
	@echo "  install          Install all commands (override CMD=gonemaster-server)"
	@echo "  ui-build         Build the embedded UI"
	@echo "  ui-dev           Run the UI dev server"
	@echo "  clean            Remove build artifacts"

$(BIN_DIR):
	@mkdir -p $(BIN_DIR)

ui-install:
	$(NPM) --prefix $(UI_DIR) install

ui-build: ui-install
	$(NPM) --prefix $(UI_DIR) run build

ui-dev: ui-install
	$(NPM) --prefix $(UI_DIR) run dev

build: $(BIN_DIR)
	@if [ "$(CMD)" = "all" ]; then \
		for cmd in $(CMDS); do \
			$(MAKE) build-$$cmd; \
		done; \
	else \
		$(MAKE) build-$(CMD); \
	fi

build-gonemaster: $(BIN_DIR)
	$(GO) build -o $(BIN_DIR)/gonemaster ./cmd/gonemaster

build-gonemaster-server: $(BIN_DIR) ui-build
	$(GO) build -o $(BIN_DIR)/gonemaster-server ./cmd/gonemaster-server

test:
	$(GO) test ./...

install:
	@if [ "$(CMD)" = "all" ]; then \
		for cmd in $(CMDS); do \
			$(MAKE) install-$$cmd; \
		done; \
	else \
		$(MAKE) install-$(CMD); \
	fi

install-gonemaster:
	$(GO) install ./cmd/gonemaster

install-gonemaster-server: ui-build
	$(GO) install ./cmd/gonemaster-server

clean:
	@rm -rf $(BIN_DIR)
