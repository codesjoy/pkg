# ==============================================================================
# pkg Makefile

.DEFAULT_GOAL := help

# Include common.mk first (convention)
include scripts/make-rules/common.mk

# Include modular make-rules
include scripts/make-rules/deps.mk
include scripts/make-rules/golang.mk
include scripts/make-rules/copyright.mk
include scripts/make-rules/precommit.mk
include scripts/make-rules/tools.mk


# ==============================================================================
# User-facing targets (forward to make-rule targets)

# ==============================================================================
# PHONY Targets
# ==============================================================================
.PHONY: all build install tidy download \
        fmt fmt.check lint fix \
        test test.race test.bench coverage \
        clean copyright tools sync help help.targets \
        hooks.install hooks.verify hooks.run hooks.run-all hooks.clean

## all: Run format, lint, and test
all: fmt lint test
	@$(LOG_SUCCESS) "All checks passed"

## build: Build (disabled for library package)
build: go.build

## install: Install (disabled for library package)
install: go.install

## tidy: Tidy go.mod for all modules
tidy: go.tidy

## download: Download dependencies
download: go.mod.download

## fmt: Format code using all formatters
fmt: go.fmt

## fmt.check: Check if code is formatted (CI gate)
fmt.check: go.fmt.check

## lint: Run linters
lint: go.lint

## fix: Run linters with auto-fix
fix: go.fix

## test: Run tests for all modules
test: go.test

## test.race: Run tests with race detector
test.race: go.test.race

## test.bench: Run benchmarks
test.bench: go.test.bench

## coverage: Run tests with coverage quality gate
coverage: go.test.coverage

## clean: Clean build artifacts
clean: go.clean

## copyright: Add copyright headers
copyright: copyright.add

## tools: Install all required tools
tools: tools.install

## sync: Sync go workspace
sync: go.work.sync

## help: Show this help message
help:
	@echo ""
	@echo "codesjoy/pkg Makefile"
	@echo ""
	@echo "Usage: make [target] [options]"
	@echo ""
	@echo "Options:"
	@echo "  LOG_LEVEL=0       Show all messages (debug, info, warn, error)"
	@echo "  LOG_LEVEL=1       Show info, warn, error (default)"
	@echo "  LOG_LEVEL=2       Show warn, error only"
	@echo "  LOG_LEVEL=3       Show error only"
	@echo "  COVERAGE=60       Set coverage threshold (default: 60%)"
	@echo "  EXCLUDE_TESTS=    Pattern to exclude from tests (e.g., \"vendor|test\")"
	@echo "  INCLUDE_EXAMPLES=1 Include example/examples modules in test & coverage"
	@echo "  MODULES=...       Explicit module list (e.g., \"utils basic/xjwt\")"
	@echo "  MODULE_INCLUDE=... Filter modules to include (space-separated)"
	@echo "  MODULE_EXCLUDE=... Filter modules to exclude (space-separated)"
	@echo "  MODULES_DIR=basic Legacy shorthand base for go.*.<module> targets"
	@echo ""
	@echo "Examples:"
	@echo "  make help                     Show this help message"
	@echo "  make tidy                     Tidy dependencies"
	@echo "  make fmt                      Format code"
	@echo "  make lint                     Run linters"
	@echo "  make test                     Run tests"
	@echo "  make coverage                 Run tests with coverage"
	@echo "  make test INCLUDE_EXAMPLES=1  Run tests including example modules"
	@echo "  make coverage INCLUDE_EXAMPLES=1 Run coverage including example modules"
	@echo "  make tools                    Install all required tools and pre-commit hooks"
	@echo "  make hooks.install            Install pre-commit hooks manually"
	@echo "  make hooks.run                Run hooks on staged files"
	@echo "  make hooks.run-all            Run hooks on all files"
	@echo "  make copyright                Add copyright headers"
	@echo "  make sync                     Sync go workspace"
	@echo "  make MODULES=\"utils\" lint    Lint only utils module"
	@echo "  make MODULE_EXCLUDE=\"basic/snowflake/examples\" test"
	@echo ""
	@echo "Module-specific targets:"
	@echo "  make go.test.xjwt             Test xjwt (legacy shorthand)"
	@echo "  make MODULES=\"basic/xjwt\" test Test xjwt with explicit module list"
	@echo "  make go.lint.utils            Lint utils module"
	@echo ""
	@echo "Available targets:"
	@$(MAKE) help.targets
	@echo ""

## help.targets: Show all targets with descriptions
help.targets:
	@grep -E '^## ' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  %-30s %s\n", $$2, $$3}'
