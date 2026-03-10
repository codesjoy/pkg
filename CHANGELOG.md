<a name="unreleased"></a>
## [unreleased](https://github.com/codesjoy/pkg/compare/07c71a40111ab0f737994797388d5335dd0b6b9b...unreleased) (2026-03-10)
### Features
- **aipsql:** split planner parts and add offset pagination
- **aipsql:** add offset page token helpers
- **tools-google-aip:** add resource-level parent helpers
- **tools-google-aip:** add google aip protoc plugin
- **utils:** add xsync concurrency primitives
- **utils:** add map utilities with stdlib tests and docs
- **utils:** add xnet package and docs
- **xgorm:** add sharding and dbresolver support
- **xredis:** add distributed lock with optional redlock
- **xredis:** add native redis client with observability hooks
### Refactors
- **aipsql:** simplify query planner output
- **aipsql:** simplify parser and planner helper flow
- **snowflake:** simplify worker update field handling
- **tools-google-aip:** focus plugin on resource names
- **tools-modelgen:** simplify generator and resolve flow
- **tools-reason:** simplify reason generation flow
- **utils:** simplify xnet and xsync helper flow
- **xerror:** simplify error message composition
- **xgorm:** simplify setup and metrics orchestration
- **xjwt:** simplify ec curve byte-size mapping
- **xkafka:** simplify config validation and runtime flow
- **xredis:** simplify config normalization and options
### BREAKING CHANGE
QueryPlan no longer exposes intermediate fragments.
 Callers must use plan.NextPageToken(rows) after executing the current
 page.

<a name="2026-02"></a>
## [2026-02](https://github.com/codesjoy/pkg/compare/430b0d00236e2a14c87ed42a56ffcb50dcc5a739...07c71a40111ab0f737994797388d5335dd0b6b9b) (2026-02-27)
### Features
- **aipsql:** add aip sql planner parser and filters
- **snowflake:** add distributed id generator module
- **tools:** add protoc-gen-codesjoy-reason with buf example
- **tools-modelgen:** add modelgen tool with tests and example
- **utils:** add shared utility packages and tests
- **xerror:** add framework-agnostic domain error module
- **xgorm:** add enhanced gorm utilities module
- **xjwt:** add jwt key and jwk utility module
- **xkafka:** add runnable examples module
- **xkafka:** add otel trace middleware and docs
- **xkafka:** add producer sync batch async support
- **xkafka:** add consumer runtimes and middleware core
