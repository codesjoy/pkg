# ==============================================================================
# Git Hooks Management
# ==============================================================================
# Manages git hooks installation and verification.
#
# This module provides automated git hooks management to ensure code quality
# and consistency across all development environments.
#
# Targets:
#   - githooks.install:   Install git hooks to .git/hooks/
#   - githooks.verify:    Verify git hooks are installed
#   - githooks.clean:     Remove installed git hooks
#
# Usage:
#   make githooks.install   - Install all git hooks from githooks/ directory
#   make githooks.verify    - Check if hooks are properly installed
#   make githooks.clean     - Remove installed hooks
#   make tools              - Includes githooks.install automatically
#
# Integration:
#   Githooks are automatically installed when running 'make tools'
#   Hooks are sourced from $(GIT_HOOKS_DIR) (default: githooks/)
#   Hooks are installed to $(GIT_HOOKS_TARGET) (default: .git/hooks/)
# ==============================================================================

# Git hooks directories
GIT_HOOKS_DIR   := $(ROOT_DIR)/githooks
GIT_HOOKS_TARGET:= $(ROOT_DIR)/.git/hooks

# ==============================================================================
# PHONY Targets
# ==============================================================================
.PHONY: githooks.install githooks.verify githooks.clean

## githooks.install: Install git hooks to .git/hooks/
githooks.install:
	@$(LOG_INFO) "Installing git hooks"
	@if [ ! -d "$(GIT_HOOKS_TARGET)" ]; then \
		$(LOG_ERROR) ".git/hooks directory not found. Not a git repository?"; \
		exit 1; \
	fi
	@$(MKDIR) $(GIT_HOOKS_TARGET)
	@for hook in $(GIT_HOOKS_DIR)/*; do \
		if [ -f "$$hook" ]; then \
			hook_name=$$(basename "$$hook"); \
			$(LOG_INFO) "Installing $$hook_name"; \
			cp "$$hook" $(GIT_HOOKS_TARGET)/$$hook_name; \
			chmod +x $(GIT_HOOKS_TARGET)/$$hook_name; \
		fi; \
	done
	@$(LOG_SUCCESS) "Git hooks installed successfully"
	@echo ""
	@echo "Installed hooks:"
	@for hook in $(GIT_HOOKS_DIR)/*; do \
		if [ -f "$$hook" ]; then \
			hook_name=$$(basename "$$hook"); \
			echo "  ✓ $$hook_name"; \
		fi; \
	done

## githooks.verify: Verify git hooks are installed
githooks.verify:
	@$(LOG_INFO) "Verifying git hooks"
	@if [ ! -d "$(GIT_HOOKS_TARGET)" ]; then \
		$(LOG_ERROR) ".git/hooks directory not found. Not a git repository?"; \
		exit 1; \
	fi
	@missing=0; \
	for hook in $(GIT_HOOKS_DIR)/*; do \
		if [ -f "$$hook" ]; then \
			hook_name=$$(basename "$$hook"); \
			if [ ! -f "$(GIT_HOOKS_TARGET)/$$hook_name" ]; then \
				$(LOG_WARN) "Hook $$hook_name not installed"; \
				missing=1; \
			fi; \
		fi; \
	done; \
	if [ $$missing -eq 0 ]; then \
		$(LOG_SUCCESS) "All git hooks verified"; \
	else \
		exit 1; \
	fi

## githooks.clean: Remove installed git hooks
githooks.clean:
	@$(LOG_INFO) "Removing git hooks"
	@if [ ! -d "$(GIT_HOOKS_TARGET)" ]; then \
		$(LOG_WARN) ".git/hooks directory not found"; \
		exit 1; \
	fi
	@for hook in $(GIT_HOOKS_DIR)/*; do \
		if [ -f "$$hook" ]; then \
			hook_name=$$(basename "$$hook"); \
			if [ -f "$(GIT_HOOKS_TARGET)/$$hook_name" ]; then \
				$(LOG_INFO) "Removing $$hook_name"; \
				rm -f $(GIT_HOOKS_TARGET)/$$hook_name; \
			fi; \
		fi; \
	done
	@$(LOG_SUCCESS) "Git hooks removed"
