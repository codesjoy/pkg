# ==============================================================================
# Makefile helper functions for common variables
#

# Use bash as shell
SHELL := bash

# Project metadata
PROJECT_NAME ?= codesjoy/pkg
ROOT_DIR     := $(shell pwd)

# Module discovery (dynamic) - find all go.mod files (excluding vendor)
GO_MODULE_DIRS := $(shell cd "$(ROOT_DIR)" && find . -name "go.mod" -not -path "*/vendor/*" -exec dirname {} \; | sed 's|^\./||' | sort)

# Module configuration
# MODULES has highest priority when explicitly provided by caller.
# MODULES_DIR is kept for legacy single-module shorthand targets (e.g. lint.xjwt).
MODULES_DIR      ?= basic
MODULES          ?= $(GO_MODULE_DIRS)
MODULE_INCLUDE   ?=
MODULE_EXCLUDE   ?=

ifneq ($(strip $(MODULE_INCLUDE)),)
MODULES := $(filter $(MODULE_INCLUDE),$(MODULES))
endif

ifneq ($(strip $(MODULE_EXCLUDE)),)
MODULES := $(filter-out $(MODULE_EXCLUDE),$(MODULES))
endif

# Go tools
GO              := go
GOLANGCI_LINT   := golangci-lint
GOFUMPT         := gofumpt
GOIMPORTS       := goimports
GOLINES         := golines
FIND            := find
XARGS           := xargs

# Build directories
OUTPUT_DIR    := $(ROOT_DIR)/_output
COVERAGE_DIR  := $(OUTPUT_DIR)/coverage

# Coverage threshold (default 60%)
COVERAGE      := 60

# Platform support
PLATFORMS ?= linux_amd64 linux_arm64 darwin_amd64 darwin_arm64
GOOS         := $(shell go env GOOS)
GOARCH       := $(shell go env GOARCH)
PLATFORM     := $(GOOS)_$(GOARCH)

# Tool categorization (defaults, can be overridden)
BLOCKER_TOOLS   ?=
CRITICAL_TOOLS  ?= $(GOLANGCI_LINT) $(GOFUMPT) $(GOIMPORTS) $(GOLINES)
TRIVIAL_TOOLS   ?= git-chglog addlicense go-junit-report

# Common commands
MKDIR          := mkdir -p
RM             := rm -rf

# ==============================================================================
# Logging Helpers
# ==============================================================================
# Optimized logging using wrapper scripts to avoid repeated shell sourcing.
#
# Available levels: LOG_DEBUG, LOG_INFO, LOG_WARN, LOG_ERROR, LOG_SUCCESS
# Controlled by LOG_LEVEL environment variable (0-3, default: 1)
#
# Usage:
#   @$(LOG_INFO) "Message text"
#   @$(LOG_ERROR) "Error message"
# ==============================================================================

# Logging helpers (optimized - use wrapper scripts to avoid repeated sourcing)
LOG_INFO    = $(ROOT_DIR)/scripts/bin/log-info
LOG_DEBUG   = $(ROOT_DIR)/scripts/bin/log-debug
LOG_WARN    = $(ROOT_DIR)/scripts/bin/log-warn
LOG_ERROR   = $(ROOT_DIR)/scripts/bin/log-error
LOG_SUCCESS = $(ROOT_DIR)/scripts/bin/log-success

# ==============================================================================
# Module Iteration Helpers
# ==============================================================================
# Macros for running commands across multiple modules.
#
# Usage:
#   $(call iterate-modules-log,command)  - Run with per-module logging
#   $(call run-in-modules,command)       - Run without logging
#
# Example:
#   $(call iterate-modules-log,$(GO) test ./...)
# ==============================================================================

# Helper to run command in each module (legacy, without logging)
define run-in-modules
@for module in $(MODULES); do \
    cd "$(ROOT_DIR)/$$module" && $(1) || exit 1; \
done
endef

# Optimized module iteration with logging (sources logger once per loop)
define iterate-modules-log
@source $(ROOT_DIR)/scripts/lib/logger.sh >/dev/null 2>&1; \
for module in $(MODULES); do \
    log::info "$$module"; \
    cd "$(ROOT_DIR)/$$module" && $(1) || exit 1; \
done
endef

# Resolve module path for single-module targets.
# Supports explicit module paths (e.g. basic/xjwt) and legacy shorthand with MODULES_DIR (e.g. xjwt).
define resolve-module-path
module_path="$(1)"; \
if [ -d "$(ROOT_DIR)/$$module_path" ] && [ -f "$(ROOT_DIR)/$$module_path/go.mod" ]; then \
	true; \
elif [ -d "$(ROOT_DIR)/$(MODULES_DIR)/$$module_path" ] && [ -f "$(ROOT_DIR)/$(MODULES_DIR)/$$module_path/go.mod" ]; then \
	module_path="$(MODULES_DIR)/$$module_path"; \
else \
	$(LOG_ERROR) "Module '$$module_path' not found. Use a valid module path (e.g. utils, basic/xjwt)."; \
	exit 1; \
fi
endef

# Efficient find with exclusions (avoids piped grep)
# Usage: $(call find-go-files,directory,exclude_patterns...)
# Example: $(call find-go-files,.,vendor _output)
define find-go-files
$(FIND) $(1) -name "*.go" $(foreach pattern,$(2),-not -path "*/$(pattern)/*")
endef

# ==============================================================================
# Validation Helpers
# ==============================================================================

# Require a tool to be installed
# Usage: $(call require-tool,golangci-lint)
define require-tool
@if ! command -v $(1) >/dev/null 2>&1; then \
    $$(LOG_ERROR) "Required tool '$(1)' not found. Run 'make tools' to install."; \
    exit 1; \
fi
endef

# Require a file to exist
# Usage: $(call require-file,$(CONFIG_FILE))
define require-file
@if [ ! -f "$(1)" ]; then \
    $$(LOG_ERROR) "Required file '$(1)' not found."; \
    exit 1; \
fi
endef
