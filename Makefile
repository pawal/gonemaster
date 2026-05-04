SHELL := /bin/sh

GO ?= go
NPM ?= npm
NODE ?= node
BIN_DIR ?= bin
UI_DIR := ui
UI_BUILD_DIR := $(UI_DIR)/dist
UI_PUBLIC_DIR := ui-public
UI_ANALYSIS_DIR := analysis-ui
NODE_MIN ?= 20
NPM_MIN ?= 9

CMDS := gonemaster gonemaster-server gonemaster-client gonemaster-nagios
CMD ?= all

.PHONY: help build build-all test install ui-build ui-install ui-dev ui-test \
	ui-public-build ui-public-install ui-public-dev ui-public-test \
	ui-analysis-build ui-analysis-install ui-analysis-dev ui-analysis-test clean \
	build-gonemaster build-gonemaster-badkeys-embed build-gonemaster-server build-gonemaster-server-noui \
	build-gonemaster-server-badkeys-embed build-gonemaster-server-noui-badkeys-embed build-gonemaster-client \
	build-gonemaster-nagios install-gonemaster install-gonemaster-badkeys-embed install-gonemaster-server install-gonemaster-client \
	install-gonemaster-nagios ui-check test-go test-integration vet race \
	spec-export-implemented spec-export-tags spec-export spec-validate spec-validate-scan spec-check \
	spec-generate-tags spec-check-tags spec-export-log-args spec-check-coherency spec-check-i18n-placeholders \
	badkeys-update badkeys-update-embed man clean-man

help:
	@echo "Targets:"
	@echo "  build            Build all commands (override CMD=gonemaster-server)"
	@echo "  test             Run Go tests, UI tests, and spec checks"
	@echo "  test-go          Run Go tests (no UI)"
	@echo "  test-integration Run Go tests against SQLite + PostgreSQL + MariaDB (requires Docker)"
	@echo "  vet              Run go vet"
	@echo "  race             Run Go tests with -race"
	@echo "  install          Install all commands (override CMD=gonemaster-server)"
	@echo "  ui-check              Verify node/npm availability"
	@echo "  ui-install            Install admin UI dependencies"
	@echo "  ui-build              Build admin, public, and analysis embedded UIs"
	@echo "  ui-dev                Run the admin UI dev server"
	@echo "  ui-test               Run admin UI tests"
	@echo "  ui-public-install     Install public UI dependencies"
	@echo "  ui-public-build       Build the public embedded UI"
	@echo "  ui-public-dev         Run the public UI dev server"
	@echo "  ui-public-test        Run public UI tests"
	@echo "  ui-analysis-install   Install analysis UI dependencies"
	@echo "  ui-analysis-build     Build the analysis embedded UI"
	@echo "  ui-analysis-dev       Run the analysis UI dev server"
	@echo "  ui-analysis-test      Run analysis UI tests"
	@echo "  build-gonemaster-badkeys-embed  Build CLI with embedded badkeys blocklist"
	@echo "  build-gonemaster-server-noui  Build API-only server (no npm/UI embed)"
	@echo "  build-gonemaster-server-badkeys-embed  Build server with embedded badkeys blocklist (with UI)"
	@echo "  build-gonemaster-server-noui-badkeys-embed  Build API-only server with embedded badkeys blocklist"
	@echo "  build-gonemaster-client       Build the HTTP API client"
	@echo "  build-gonemaster-nagios       Build the Nagios plugin"
	@echo "  spec-export        Refresh generated specification inventories (JSON)"
	@echo "  spec-export-log-args  Refresh generated log argument inventory (JSON + markdown)"
	@echo "  spec-validate      Validate canonical testcase specs against implementation metadata"
	@echo "  spec-validate-scan Validate specs + scan append*Log literals for metadata omissions"
	@echo "  spec-generate-tags Regenerate per-module tag catalog markdown files"
	@echo "  spec-check-tags    Check tag catalog files are up to date (drift detection)"
	@echo "  spec-check-coherency Run log-args coherency guardrail checks"
	@echo "  spec-check-i18n-placeholders  Verify placeholder parity and reject non-allowlisted legacy placeholders"
	@echo "  spec-check         Run spec-validate + spec-check-tags + coherency + i18n placeholder checks"
	@echo "  man              Generate man pages from docs/man/*.md"
	@echo "  badkeys-update     Download badkeys blocklist to share/badkeys/"
	@echo "  badkeys-update-embed  Download and gzip-compress blocklist for embedded builds"
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

ui-build: ui-install ui-public-build ui-analysis-build
	$(NPM) --prefix $(UI_DIR) run build
	@mkdir -p server/ui/dist
	@printf '%s\n' \
		'This placeholder keeps the dist directory embeddable when built UI assets are not present.' \
		'Run `make ui-build` before building the default server binary to embed the web UI.' \
		> server/ui/dist/placeholder.txt

ui-dev: ui-install
	$(NPM) --prefix $(UI_DIR) run dev

ui-test: ui-install
	$(NPM) --prefix $(UI_DIR) run test

ui-public-install: ui-check
	$(NPM) --prefix $(UI_PUBLIC_DIR) install

ui-public-build: ui-public-install
	$(NPM) --prefix $(UI_PUBLIC_DIR) run build
	@mkdir -p server/public/dist
	@printf '%s\n' \
		'This placeholder keeps the dist directory embeddable when built UI assets are not present.' \
		'Run `make ui-build` before building the default server binary to embed the public web app.' \
		> server/public/dist/placeholder.txt

ui-public-dev: ui-public-install
	$(NPM) --prefix $(UI_PUBLIC_DIR) run dev

ui-public-test: ui-public-install
	$(NPM) --prefix $(UI_PUBLIC_DIR) run test

ui-analysis-install: ui-check
	$(NPM) --prefix $(UI_ANALYSIS_DIR) install

ui-analysis-build: ui-analysis-install
	$(NPM) --prefix $(UI_ANALYSIS_DIR) run build
	@mkdir -p server/analysisui/dist
	@printf '%s\n' \
		'This placeholder keeps the dist directory embeddable when built UI assets are not present.' \
		'Run `make ui-build` before building the default server binary to embed the analysis dashboard.' \
		> server/analysisui/dist/placeholder.txt

ui-analysis-dev: ui-analysis-install
	$(NPM) --prefix $(UI_ANALYSIS_DIR) run dev

ui-analysis-test: ui-analysis-install
	$(NPM) --prefix $(UI_ANALYSIS_DIR) run test

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

build-gonemaster-badkeys-embed: $(BIN_DIR) badkeys-update-embed
	$(GO) build -tags badkeys_embed -o $(BIN_DIR)/gonemaster ./cmd/gonemaster

build-gonemaster-server: $(BIN_DIR) ui-build
	$(GO) build -o $(BIN_DIR)/gonemaster-server ./cmd/gonemaster-server

build-gonemaster-server-noui: $(BIN_DIR)
	$(GO) build -tags nogui -o $(BIN_DIR)/gonemaster-server ./cmd/gonemaster-server

build-gonemaster-server-badkeys-embed: $(BIN_DIR) ui-build badkeys-update-embed
	$(GO) build -tags badkeys_embed -o $(BIN_DIR)/gonemaster-server ./cmd/gonemaster-server

build-gonemaster-server-noui-badkeys-embed: $(BIN_DIR) badkeys-update-embed
	$(GO) build -tags "nogui badkeys_embed" -o $(BIN_DIR)/gonemaster-server ./cmd/gonemaster-server

build-gonemaster-client: $(BIN_DIR)
	$(GO) build -o $(BIN_DIR)/gonemaster-client ./cmd/gonemaster-client

build-gonemaster-nagios: $(BIN_DIR)
	$(GO) build -o $(BIN_DIR)/gonemaster-nagios ./cmd/gonemaster-nagios

test-go:
	$(GO) test ./...

# test-integration starts PostgreSQL and MariaDB via docker-compose.test.yml,
# runs all server tests against all three backends, then tears the containers
# down. Requires Docker with Compose v2 support.
test-integration:
	docker compose -f docker-compose.test.yml up -d --wait
	TEST_POSTGRES_DSN="postgres://gonemaster:gonemaster@localhost:5432/gonemaster_test?sslmode=disable" \
	TEST_MARIADB_DSN="gonemaster:gonemaster@tcp(localhost:3306)/gonemaster_test" \
	$(GO) test ./server/... -count=1 -timeout 120s; \
	STATUS=$$?; \
	docker compose -f docker-compose.test.yml down; \
	exit $$STATUS

ui-csp-check:
	@echo "Checking admin/public UI for CSP-violating inline styles..."
	@found=0; \
	for f in $$(find ui/src ui-public/src -name '*.svelte' 2>/dev/null); do \
		hits=$$(grep -nE '[[:space:]]style=("[^"]+"|\{)|[[:space:]]style:[a-z-]+=' "$$f" 2>/dev/null \
			| grep -vE '^[[:digit:]]+:[[:space:]]*(//|\*)' || true); \
		if [ -n "$$hits" ]; then \
			echo "$$f:"; \
			echo "$$hits" | sed 's/^/  /'; \
			found=1; \
		fi; \
	done; \
	if [ "$$found" = "1" ]; then \
		echo ""; \
		echo "ERROR: inline style attributes / style: directives found in admin or public UI."; \
		echo "These are blocked at runtime by the strict CSP (style-src 'self')."; \
		echo "Use a CSS class instead. See CLAUDE.md, section 'Web Security: Content-Security-Policy'."; \
		exit 1; \
	fi
	@echo "OK - no inline styles found."

test: ui-test ui-public-test test-go spec-check ui-csp-check

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

install-gonemaster-badkeys-embed: badkeys-update-embed
	$(GO) install -tags badkeys_embed ./cmd/gonemaster

install-gonemaster-server: ui-build
	$(GO) install ./cmd/gonemaster-server

install-gonemaster-client:
	$(GO) install ./cmd/gonemaster-client

install-gonemaster-nagios:
	$(GO) install ./cmd/gonemaster-nagios

vet:
	$(GO) vet ./...

race:
	$(GO) test -race ./...

spec-export-implemented:
	GOOS= GOARCH= $(GO) run ./tools/specifications/export-implemented > docs/specifications/implemented-testcases.json

spec-export-tags:
	GOOS= GOARCH= $(GO) run ./tools/specifications/export-tags > docs/specifications/possible-tags-by-testcase.json

spec-export: spec-export-implemented spec-export-tags

spec-export-log-args:
	GOOS= GOARCH= $(GO) run ./tools/specifications/export-log-args > docs/specifications/log-args-inventory.json

spec-validate:
	GOOS= GOARCH= $(GO) run ./tools/specifications/validate

spec-validate-scan:
	GOOS= GOARCH= $(GO) run ./tools/specifications/validate --scan-append-log

spec-generate-tags:
	GOOS= GOARCH= $(GO) run ./tools/specifications/generate-tag-catalog

spec-check-tags:
	GOOS= GOARCH= $(GO) run ./tools/specifications/generate-tag-catalog --check

spec-check-coherency:
	GOOS= GOARCH= $(GO) run ./tools/specifications/export-log-args --check-coherency --markdown-out '' >/dev/null

spec-check-i18n-placeholders:
	GOOS= GOARCH= $(GO) run ./tools/i18n/check-placeholders

spec-check: spec-validate spec-check-tags spec-check-coherency spec-check-i18n-placeholders

badkeys-update:
	GOOS= GOARCH= $(GO) run ./tools/badkeys-update --output share/badkeys

badkeys-update-embed: badkeys-update
	gzip -9 -k -f share/badkeys/blocklist.dat

MAN_SRCS := $(wildcard docs/man/*.md)
MAN_OUT  := $(patsubst docs/man/%.md,man/man1/%,$(MAN_SRCS))

man: $(MAN_OUT)

man/man1/%: docs/man/%.md
	@mkdir -p man/man1
	GOOS= GOARCH= $(GO) run github.com/cpuguy83/go-md2man/v2@latest -in $< -out $@

clean-man:
	@rm -rf man

clean: clean-man
	@rm -rf $(BIN_DIR) $(UI_BUILD_DIR) $(UI_DIR)/node_modules
