# PATHS
THIS         := github.com/primandproper/docent
SCRIPTS_DIR  := scripts
FRONTEND_DIR := frontend
WAILS        := $(SCRIPTS_DIR)/wails.sh

# CONTAINER VERSIONS
SHELLCHECK_IMAGE := koalaman/shellcheck:stable

# COMMANDS
CONTAINER_RUNNER := docker

## non-PHONY folders/files

$(FRONTEND_DIR)/node_modules:
	cd $(FRONTEND_DIR) && pnpm install --frozen-lockfile

# The Go binary embeds frontend/dist, so nothing in Go compiles until it exists.
$(FRONTEND_DIR)/dist: $(FRONTEND_DIR)/node_modules
	cd $(FRONTEND_DIR) && pnpm run build

## PREREQUISITES

.PHONY: setup
setup: $(FRONTEND_DIR)/node_modules
	go mod download
	cd $(FRONTEND_DIR) && pnpm exec playwright install webkit

## FORMATTING

.PHONY: format_imports
format_imports:
	$(SCRIPTS_DIR)/format_imports.sh $(THIS) $(CURDIR)

# fieldalignment type-checks, and main.go embeds frontend/dist, so it needs the
# frontend built. Without it the script's retry loop never succeeds.
.PHONY: format_go_fieldalignment
format_go_fieldalignment: $(FRONTEND_DIR)/dist
	@$(SCRIPTS_DIR)/format_go_fieldalignment.sh

.PHONY: format_go_tag_alignment
format_go_tag_alignment:
	@$(SCRIPTS_DIR)/format_go_tag_alignment.sh

.PHONY: goimports
goimports:
	$(SCRIPTS_DIR)/goimports.sh

.PHONY: format_golang
format_golang: $(FRONTEND_DIR)/dist goimports format_imports format_go_fieldalignment format_go_tag_alignment
	@$(SCRIPTS_DIR)/format_golang.sh $(CURDIR)

.PHONY: format_frontend
format_frontend: $(FRONTEND_DIR)/node_modules
	$(SCRIPTS_DIR)/format_frontend.sh

.PHONY: format
format: format_golang format_frontend

.PHONY: fmt
fmt: format

## LINTING

.PHONY: golang_lint
golang_lint: $(FRONTEND_DIR)/dist
	@$(SCRIPTS_DIR)/golang_lint.sh

.PHONY: shellcheck
shellcheck:
	@$(SCRIPTS_DIR)/shellcheck.sh $(CONTAINER_RUNNER) $(SHELLCHECK_IMAGE) $(SCRIPTS_DIR)

.PHONY: lint_frontend
lint_frontend: $(FRONTEND_DIR)/node_modules
	$(SCRIPTS_DIR)/lint_frontend.sh

.PHONY: lint
lint: golang_lint shellcheck lint_frontend

## GENERATED FILES

# bindings regenerates the TypeScript bindings under frontend/bindings from the
# Go services. Commit the output: the frontend type-checks against it.
.PHONY: bindings
bindings:
	$(WAILS) task common:generate:bindings

# skill regenerates the agent skill and the AGENTS.md snippet from the
# authoring guide in internal/guide. Commit the output.
.PHONY: skill
skill:
	go run ./cmd/tools/skill skill

## TESTING

.PHONY: test_golang
test_golang: $(FRONTEND_DIR)/dist
	$(SCRIPTS_DIR)/test.sh

.PHONY: test_frontend
test_frontend: $(FRONTEND_DIR)/node_modules
	$(SCRIPTS_DIR)/test_frontend.sh

# test_e2e runs the production frontend bundle in WebKit against answers
# recorded from the real Go service. Pass Playwright flags with ARGS.
.PHONY: test_e2e
test_e2e: $(FRONTEND_DIR)/node_modules
	$(SCRIPTS_DIR)/test_e2e.sh $(ARGS)

.PHONY: test
test: test_golang test_frontend test_e2e

## EXECUTION

# build produces the app bundle under bin/.
.PHONY: build
build: $(FRONTEND_DIR)/node_modules
	$(WAILS) build

# dev runs the app with hot reload for both Go and the frontend.
.PHONY: dev
dev: $(FRONTEND_DIR)/node_modules
	$(WAILS) task dev
