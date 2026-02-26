
<a name="unreleased"></a>
## unreleased (2026-02-26)
### Build System
- add managed changelog workflow for no-tag repositories
- harden make/scripts workflows and workspace checks
- add lightweight logger wrapper scripts
- fix gitlint commit-msg hook configuration
- migrate repository hooks to pre-commit
- add make rules scripts and commit hooks
### Chores
- scope bin ignore rule to repository root
- initialize repository metadata and baseline policies
- **aipsql:** add apache license headers to docs and tests
- **xgorm:** add apache license headers across module files
### Documentation
- add contributor guide in AGENTS.md
- add top level package catalog and guides
### Features
- **aipsql:** add aip sql planner parser and filters
- **snowflake:** add distributed id generator module
- **tools:** add protoc-gen-codesjoy-reason with buf example
- **utils:** add shared utility packages and tests
- **xerror:** add framework-agnostic domain error module
- **xgorm:** add enhanced gorm utilities module
- **xjwt:** add jwt key and jwk utility module
- **xkafka:** add runnable examples module
- **xkafka:** add otel trace middleware and docs
- **xkafka:** add producer sync batch async support
- **xkafka:** add consumer runtimes and middleware core
### Styles
- normalize shell scripts with shfmt
- **aipsql:** collapse nested else-if in validation tests
### Tests
- **aipsql:** consolidate test helpers and integration testkit
- **xkafka:** add docker-backed integration tests
