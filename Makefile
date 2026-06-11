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

CMDS := gonemaster gonemaster-server gonemaster-client gonemaster-nagios gonemaster-mcp
CMD ?= all

.PHONY: help build build-all test install ui-build ui-install ui-dev ui-test \
	ui-public-build ui-public-install ui-public-dev ui-public-test \
	ui-analysis-build ui-analysis-install ui-analysis-dev ui-analysis-test clean \
	build-gonemaster build-gonemaster-badkeys-embed build-gonemaster-server build-gonemaster-server-noui \
	build-gonemaster-server-badkeys-embed build-gonemaster-server-noui-badkeys-embed build-gonemaster-client \
	build-gonemaster-nagios build-gonemaster-mcp install-gonemaster install-gonemaster-badkeys-embed install-gonemaster-server install-gonemaster-client \
	install-gonemaster-nagios install-gonemaster-mcp ui-check test-go test-integration vet race \
	spec-export-implemented spec-export-tags spec-export spec-validate spec-validate-scan spec-check \
	spec-export-testcase-descriptions spec-check-testcase-descriptions \
	spec-generate-tags spec-check-tags spec-export-log-args spec-check-coherency spec-check-i18n-placeholders \
	architecture-check badkeys-update badkeys-update-embed man man-gz clean-man \
	package-binaries package-deb package-rpm packages clean-packages

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
	@echo "  build-gonemaster-mcp          Build the MCP (Model Context Protocol) bridge"
	@echo "  spec-export        Refresh generated specification inventories (JSON)"
	@echo "  spec-export-testcase-descriptions  Refresh the site testcase-description data file (TOML)"
	@echo "  spec-check-testcase-descriptions   Check the site testcase-description data file is up to date"
	@echo "  spec-export-log-args  Refresh generated log argument inventory (JSON + markdown)"
	@echo "  spec-validate      Validate canonical testcase specs against implementation metadata"
	@echo "  spec-validate-scan Validate specs + scan append*Log literals for metadata omissions"
	@echo "  spec-generate-tags Regenerate per-module tag catalog markdown files"
	@echo "  spec-check-tags    Check tag catalog files are up to date (drift detection)"
	@echo "  spec-check-coherency Run log-args coherency guardrail checks"
	@echo "  spec-check-i18n-placeholders  Verify placeholder parity and reject non-allowlisted legacy placeholders"
	@echo "  spec-check         Run spec-validate + spec-check-tags + coherency + i18n placeholder + testcase-description checks"
	@echo "  architecture-check Verify docs/architecture.md against cmd/, build tags, drivers, and the Last reviewed date"
	@echo "  docs             Build the documentation site (writes site/public/)"
	@echo "  docs-serve       Serve the documentation site locally"
	@echo "  man              Generate man pages from docs/man/*.md"
	@echo "  badkeys-update     Download badkeys blocklist to share/badkeys/"
	@echo "  badkeys-update-embed  Download and gzip-compress blocklist for embedded builds"
	@echo "  man-gz           Gzip-compress man pages for packaging"
	@echo "  package-binaries Cross-build all binaries for linux/amd64 + linux/arm64"
	@echo "  package-deb      Build .deb packages into dist/packages/"
	@echo "  package-rpm      Build .rpm packages into dist/packages/"
	@echo "  packages         Build both .deb and .rpm packages"
	@echo "  clean-packages   Remove dist/ build output"
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

build-gonemaster-mcp: $(BIN_DIR)
	$(GO) build -ldflags "-X main.version=$(VERSION)" -o $(BIN_DIR)/gonemaster-mcp ./cmd/gonemaster-mcp

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

test: ui-test ui-public-test ui-analysis-test test-go spec-check ui-csp-check

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

install-gonemaster-mcp:
	$(GO) install -ldflags "-X main.version=$(VERSION)" ./cmd/gonemaster-mcp

vet:
	$(GO) vet ./...

race:
	$(GO) test -race ./...

spec-export-implemented:
	GOOS= GOARCH= $(GO) run ./tools/specifications/export-implemented > docs/specifications/implemented-testcases.json

spec-export-tags:
	GOOS= GOARCH= $(GO) run ./tools/specifications/export-tags > docs/specifications/possible-tags-by-testcase.json

spec-export-testcase-descriptions:
	GOOS= GOARCH= $(GO) run ./tools/specifications/export-testcase-descriptions

spec-check-testcase-descriptions:
	GOOS= GOARCH= $(GO) run ./tools/specifications/export-testcase-descriptions --check

spec-export: spec-export-implemented spec-export-tags spec-export-testcase-descriptions

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

spec-check: spec-validate spec-check-tags spec-check-coherency spec-check-i18n-placeholders spec-check-testcase-descriptions

architecture-check:
	GOOS= GOARCH= $(GO) run ./tools/architecture-check

badkeys-update:
	GOOS= GOARCH= $(GO) run ./tools/badkeys-update --output share/badkeys

badkeys-update-embed: badkeys-update
	gzip -9 -k -f share/badkeys/blocklist.dat

.PHONY: docs docs-serve
docs:
	hugo --source site/ --minify

docs-serve:
	hugo server --source site/ --bind 0.0.0.0

MAN_SRCS := $(wildcard docs/man/*.md)
MAN_OUT  := $(patsubst docs/man/%.md,man/man1/%,$(MAN_SRCS))

man: $(MAN_OUT)

man/man1/%: docs/man/%.md
	@mkdir -p man/man1
	GOOS= GOARCH= $(GO) run github.com/cpuguy83/go-md2man/v2@latest -in $< -out $@

MAN_GZ := $(patsubst docs/man/%.md,man/man1/%.gz,$(MAN_SRCS))

man-gz: $(MAN_GZ)

man/man1/%.gz: man/man1/%
	gzip -9 -k -f $<

clean-man:
	@rm -rf man

# Packaging targets: produce .deb and .rpm for each binary, plus a noarch
# data package for the badkeys blocklist. See plans/packaging.md.
VERSION := $(shell awk '/^var Version = /{gsub(/"/,"",$$4); print $$4}' engine/engine.go)
PACKAGE_ARCHES ?= amd64 arm64
DIST_DIR := dist
PKG_DIR := $(DIST_DIR)/packages
NFPM := $(GO) run github.com/goreleaser/nfpm/v2/cmd/nfpm@latest
PER_ARCH_PKGS := gonemaster gonemaster-server gonemaster-server-nogui gonemaster-client gonemaster-nagios gonemaster-mcp

# Refresh blocklist data only if missing; an explicit `make badkeys-update`
# is the way to pull a new snapshot.
share/badkeys/blocklist.dat share/badkeys/badkeysdata.json:
	$(MAKE) badkeys-update

package-binaries: ui-build
	@for arch in $(PACKAGE_ARCHES); do \
		echo "Building binaries for linux/$$arch..."; \
		mkdir -p $(DIST_DIR)/linux_$$arch; \
		GOOS=linux GOARCH=$$arch CGO_ENABLED=0 $(GO) build -trimpath -ldflags='-s -w' \
			-o $(DIST_DIR)/linux_$$arch/gonemaster ./cmd/gonemaster || exit 1; \
		GOOS=linux GOARCH=$$arch CGO_ENABLED=0 $(GO) build -trimpath -ldflags='-s -w' \
			-o $(DIST_DIR)/linux_$$arch/gonemaster-server ./cmd/gonemaster-server || exit 1; \
		GOOS=linux GOARCH=$$arch CGO_ENABLED=0 $(GO) build -trimpath -ldflags='-s -w' \
			-tags nogui \
			-o $(DIST_DIR)/linux_$$arch/gonemaster-server-nogui ./cmd/gonemaster-server || exit 1; \
		GOOS=linux GOARCH=$$arch CGO_ENABLED=0 $(GO) build -trimpath -ldflags='-s -w' \
			-o $(DIST_DIR)/linux_$$arch/gonemaster-client ./cmd/gonemaster-client || exit 1; \
		GOOS=linux GOARCH=$$arch CGO_ENABLED=0 $(GO) build -trimpath -ldflags='-s -w' \
			-o $(DIST_DIR)/linux_$$arch/gonemaster-nagios ./cmd/gonemaster-nagios || exit 1; \
		GOOS=linux GOARCH=$$arch CGO_ENABLED=0 $(GO) build -trimpath -ldflags="-s -w -X main.version=$(VERSION)" \
			-o $(DIST_DIR)/linux_$$arch/gonemaster-mcp ./cmd/gonemaster-mcp || exit 1; \
	done

# nfpm expands env vars in scalar metadata fields (name, arch, version, ...)
# but not inside contents.src paths, so we sed-substitute the whole YAML to
# a per-arch temp file and feed that to nfpm.
NFPM_CONF := $(DIST_DIR)/nfpm

package-deb: package-binaries man-gz share/badkeys/blocklist.dat share/badkeys/badkeysdata.json
	@mkdir -p $(PKG_DIR) $(NFPM_CONF)
	@for arch in $(PACKAGE_ARCHES); do \
		for pkg in $(PER_ARCH_PKGS); do \
			echo "Building $$pkg $$arch deb..."; \
			sed -e "s|\$${ARCH}|$$arch|g" -e "s|\$${VERSION}|$(VERSION)|g" \
				packaging/nfpm/$$pkg.yaml > $(NFPM_CONF)/$$pkg-$$arch.yaml || exit 1; \
			$(NFPM) pkg --config $(NFPM_CONF)/$$pkg-$$arch.yaml \
				--packager deb --target $(PKG_DIR)/ || exit 1; \
		done; \
	done
	@echo "Building gonemaster-badkeys-data deb..."
	@sed -e "s|\$${VERSION}|$(VERSION)|g" \
		packaging/nfpm/gonemaster-badkeys-data.yaml > $(NFPM_CONF)/gonemaster-badkeys-data.yaml
	@$(NFPM) pkg --config $(NFPM_CONF)/gonemaster-badkeys-data.yaml \
		--packager deb --target $(PKG_DIR)/

package-rpm: package-binaries man-gz share/badkeys/blocklist.dat share/badkeys/badkeysdata.json
	@mkdir -p $(PKG_DIR) $(NFPM_CONF)
	@for arch in $(PACKAGE_ARCHES); do \
		for pkg in $(PER_ARCH_PKGS); do \
			echo "Building $$pkg $$arch rpm..."; \
			sed -e "s|\$${ARCH}|$$arch|g" -e "s|\$${VERSION}|$(VERSION)|g" \
				packaging/nfpm/$$pkg.yaml > $(NFPM_CONF)/$$pkg-$$arch.yaml || exit 1; \
			$(NFPM) pkg --config $(NFPM_CONF)/$$pkg-$$arch.yaml \
				--packager rpm --target $(PKG_DIR)/ || exit 1; \
		done; \
	done
	@echo "Building gonemaster-badkeys-data rpm..."
	@sed -e "s|\$${VERSION}|$(VERSION)|g" \
		packaging/nfpm/gonemaster-badkeys-data.yaml > $(NFPM_CONF)/gonemaster-badkeys-data.yaml
	@$(NFPM) pkg --config $(NFPM_CONF)/gonemaster-badkeys-data.yaml \
		--packager rpm --target $(PKG_DIR)/

packages: package-deb package-rpm
	@echo ""
	@echo "Built packages in $(PKG_DIR):"
	@ls -1 $(PKG_DIR)

clean-packages:
	@rm -rf $(DIST_DIR)

clean: clean-man clean-packages
	@rm -rf $(BIN_DIR) $(UI_BUILD_DIR) $(UI_DIR)/node_modules
