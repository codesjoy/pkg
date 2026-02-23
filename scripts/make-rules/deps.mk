# ==============================================================================
# Dependency Management
# ==============================================================================
# Manages Go modules and workspace dependencies.
#
# Targets:
#   - go.tidy:          Run 'go mod tidy' on all modules
#   - go.mod.download:  Download dependencies for all modules
#   - go.mod.verify:    Verify dependencies integrity
#   - go.work.sync:     Sync go.work with all discovered go.mod modules
#   - go.work.init:     Initialize go.work if not exists
#   - go.clean:         Remove build artifacts and caches
#
# Usage:
#   make tidy           - Tidy all modules
#   make sync           - Initialize/sync go workspace
#   make clean          - Clean all artifacts
# ==============================================================================

# ==============================================================================
# PHONY Targets
# ==============================================================================
.PHONY: go.tidy go.mod.download go.mod.verify \
        go.work.verify go.work.sync go.work.init \
        go.clean

## go.tidy: Tidy go.mod for all modules
go.tidy:
	@$(LOG_INFO) "Tidying go.mod for all modules"
	@source $(ROOT_DIR)/scripts/lib/logger.sh >/dev/null 2>&1; \
	for go_mod in $$(find "$(ROOT_DIR)" -name "go.mod" -type f 2>/dev/null | sort); do \
		mod_dir=$$(dirname "$$go_mod"); \
		mod_rel=$${mod_dir#$(ROOT_DIR)/}; \
		log::info "$$mod_rel"; \
		cd "$$mod_dir" && $(GO) mod tidy || exit 1; \
	done
	@$(LOG_SUCCESS) "All modules tidied"

## go.mod.download: Download dependencies for all modules
go.mod.download:
	@$(LOG_INFO) "Downloading dependencies"
	@source $(ROOT_DIR)/scripts/lib/logger.sh >/dev/null 2>&1; \
	for go_mod in $$(find "$(ROOT_DIR)" -name "go.mod" -type f 2>/dev/null | sort); do \
		mod_dir=$$(dirname "$$go_mod"); \
		mod_rel=$${mod_dir#$(ROOT_DIR)/}; \
		log::info "$$mod_rel"; \
		cd "$$mod_dir" && $(GO) mod download || exit 1; \
	done
	@$(LOG_SUCCESS) "Dependencies downloaded"

## go.mod.verify: Verify dependencies
go.mod.verify:
	@$(LOG_INFO) "Verifying dependencies"
	@source $(ROOT_DIR)/scripts/lib/logger.sh >/dev/null 2>&1; \
	for go_mod in $$(find "$(ROOT_DIR)" -name "go.mod" -type f 2>/dev/null | sort); do \
		mod_dir=$$(dirname "$$go_mod"); \
		mod_rel=$${mod_dir#$(ROOT_DIR)/}; \
		log::info "$$mod_rel"; \
		cd "$$mod_dir" && $(GO) mod verify || exit 1; \
	done
	@$(LOG_SUCCESS) "Dependencies verified"

## go.work.verify: Verify go.work exists
go.work.verify:
	@$(LOG_INFO) "Verifying go.work"
	@if [ ! -f "$(ROOT_DIR)/go.work" ]; then \
		$(LOG_WARN) "go.work not found. Run 'make sync' to initialize."; \
		exit 1; \
	fi
	@$(LOG_SUCCESS) "go.work verified"

## go.work.sync: Sync go workspace with all modules
go.work.sync: go.work.init
	@$(LOG_INFO) "Syncing go workspace"
	@cd $(ROOT_DIR); \
	go_version=$$($(GO) version | awk '{print $$3}' | sed 's/go//'); \
	echo "go $$go_version" > go.work; \
	echo "" >> go.work; \
	echo "use (" >> go.work; \
	for go_mod in $$(find . -name "go.mod" -type f 2>/dev/null | sort); do \
		mod_dir=$$(dirname "$$go_mod"); \
		mod_rel=$${mod_dir#./}; \
		echo "	./$$mod_rel" >> go.work; \
	done; \
	echo ")" >> go.work
	@$(LOG_SUCCESS) "Go workspace synced"

## go.work.init: Initialize go workspace
go.work.init:
	@if [ ! -f "$(ROOT_DIR)/go.work" ]; then \
		$(LOG_INFO) "Initializing go.work"; \
		cd $(ROOT_DIR) && bash scripts/gowork.sh; \
	fi

## go.clean: Clean build artifacts and caches
go.clean:
	@$(LOG_INFO) "Cleaning build artifacts"
	@$(RM) $(OUTPUT_DIR)
	@for go_mod in $$(find "$(ROOT_DIR)" -name "go.mod" -type f 2>/dev/null | sort); do \
		mod_dir=$$(dirname "$$go_mod"); \
		cd "$$mod_dir" && $(GO) clean -cache -testcache; \
	done
	@$(LOG_SUCCESS) "Cleaned build artifacts"
