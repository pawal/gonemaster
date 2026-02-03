SHELL := /bin/sh

GO ?= go
NPM ?= npm
NODE ?= node
BIN_DIR ?= bin
UI_DIR := ui
NODE_MIN ?= 18
NPM_MIN ?= 9

CMDS := gonemaster gonemaster-server
CMD ?= all

.PHONY: help build build-all test install ui-build ui-install ui-dev ui-test clean \
	build-gonemaster build-gonemaster-server build-gonemaster-server-noui \
	install-gonemaster install-gonemaster-server ui-check

help:
	@echo "Targets:"
	@echo "  build            Build all commands (override CMD=gonemaster-server)"
	@echo "  test             Run Go tests"
	@echo "  install          Install all commands (override CMD=gonemaster-server)"
	@echo "  ui-build         Build the embedded UI"
	@echo "  ui-dev           Run the UI dev server"
	@echo "  ui-test          Run UI tests"
	@echo "  build-gonemaster-server-noui  Build API-only server (no npm/UI embed)"
	@echo "  clean            Remove build artifacts"

$(BIN_DIR):
	@mkdir -p $(BIN_DIR)

ui-check:
	@command -v $(NODE) >/dev/null 2>&1 || { \
		echo "Error: node not found. Install Node.js $(NODE_MIN)+ to build the UI."; \
		exit 1; \
	}
	@command -v $(NPM) >/dev/null 2>&1 || { \
		echo "Error: npm not found. Install npm $(NPM_MIN)+ to build the UI."; \
		exit 1; \
	}
	@node_version=`$(NODE) -v 2>/dev/null | sed 's/^v//'`; \
	npm_version=`$(NPM) -v 2>/dev/null`; \
	if [ -n "$$node_version" ]; then \
		major=$${node_version%%.*}; \
		if [ "$$major" -lt "$(NODE_MIN)" ]; then \
			echo "Error: Node.js $$node_version found. Need $(NODE_MIN)+ to build the UI."; \
			exit 1; \
		fi; \
	fi; \
	if [ -n "$$npm_version" ]; then \
		major=$${npm_version%%.*}; \
		if [ "$$major" -lt "$(NPM_MIN)" ]; then \
			echo "Error: npm $$npm_version found. Need $(NPM_MIN)+ to build the UI."; \
			exit 1; \
		fi; \
	fi

ui-install: ui-check
	$(NPM) --prefix $(UI_DIR) install

ui-build: ui-install
	$(NPM) --prefix $(UI_DIR) run build

ui-dev: ui-install
	$(NPM) --prefix $(UI_DIR) run dev

ui-test: ui-install
	$(NPM) --prefix $(UI_DIR) run test

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

build-gonemaster-server-noui: $(BIN_DIR)
	$(GO) build -tags nogui -o $(BIN_DIR)/gonemaster-server ./cmd/gonemaster-server

test: ui-test
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
