<a name="unreleased"></a>
## [unreleased](https://github.com/codesjoy/pkg/compare/07c71a40111ab0f737994797388d5335dd0b6b9b...unreleased) (2026-03-16)
### Features
- **aipsql:** add offset page token helpers
- **aipsql:** split planner parts and add offset pagination
- **proto-ddd:** add shared ddd event proto module
- **proto-reason:** extract shared reason proto module
- **tools-ddd:** add protoc generator for xevent event methods
- **tools-google-aip:** add google aip protoc plugin
- **tools-google-aip:** add resource-level parent helpers
- **tools-google-aip:** add typed name formatting helpers
- **tools-modelgen:** improve inferred naming and timestamp handling
- **utils:** add xsync concurrency primitives
- **utils:** add map utilities with stdlib tests and docs
- **utils:** add xnet package and docs
- **xevent:** add transport-agnostic event contracts and kafka adapter
- **xevent:** add jetstream nats adapter
- **xgorm:** add sharding and dbresolver support
- **xnats:** add nats and jetstream helpers
- **xnats:** add ordered jetstream consumer runtime
- **xredis:** add ordered redis streams mq support
- **xredis:** add distributed lock with optional redlock
- **xredis:** add native redis client with observability hooks
### Refactors
- **aipsql:** simplify query planner output
- **aipsql:** simplify parser and planner helper flow
- **aipsql:** reorganize filter planner and pagination code
- **snowflake:** simplify worker update field handling
- **tools-event:** rename protoc-gen-codesjoy-ddd to protoc-gen-codesjoy-event
- **tools-google-aip:** focus plugin on resource names
- **tools-modelgen:** simplify generator and resolve flow
- **tools-modelgen:** infer database from dsn only
- **tools-reason:** simplify reason generation flow
- **utils:** simplify xnet and xsync helper flow
- **xerror:** simplify error message composition
- **xgorm:** simplify setup and metrics orchestration
- **xgorm:** simplify config and transaction helpers
- **xjwt:** simplify ec curve byte-size mapping
- **xkafka:** simplify package layout
- **xkafka:** simplify config validation and runtime flow
- **xnats:** consolidate shared helpers and tests
- **xredis:** simplify package layout
- **xredis:** simplify config normalization and options
### BREAKING CHANGE
codesjoy-modelgen no longer accepts --dialect; database type is inferred from --dsn
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
