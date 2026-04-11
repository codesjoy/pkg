# xevent/outbox/debezium/gorm

`xevent/outbox/debezium/gorm` provides the GORM-backed append-only store for
`xevent/outbox/debezium`.

Use this module when Kafka is the only target and application code wants to:

- append one Debezium-friendly outbox row inside the same transaction as
  business writes
- keep storage simple and append-only
- run retention separately from publish

## Installation

```bash
go get github.com/codesjoy/pkg/basic/xevent/outbox/debezium/gorm
```

## Package

```go
import (
	"github.com/codesjoy/pkg/basic/xevent/outbox/debezium"
	debeziumgorm "github.com/codesjoy/pkg/basic/xevent/outbox/debezium/gorm"
)
```

## What This Package Provides

- `GORMStore`: GORM-backed append-only `debezium.Store`
- `NewGORMStore`: creates one configured store
- `DeleteBefore`: conservative retention helper for rows older than a cutoff

## Database Support

- PostgreSQL: supported as a GORM store and documented below as the primary
  Debezium / Kafka Connect example
- MySQL: supported as a GORM store for append-only outbox rows and retention
  tasks; this README does not add a MySQL Debezium connector template

The storage model is the same on both databases: one immutable row per outbound
event, with the final Kafka topic already resolved before insert.

## Schema

You can use GORM migrations, but the following SQL is suitable when schema
changes are managed explicitly.

### PostgreSQL

```sql
CREATE TABLE xevent_debezium_outbox_records (
  id VARCHAR(36) PRIMARY KEY,
  topic VARCHAR(255) NOT NULL,
  partition_key VARCHAR(255) NOT NULL DEFAULT '',
  event_type VARCHAR(255) NOT NULL,
  event_id VARCHAR(255) NOT NULL DEFAULT '',
  payload BYTEA NOT NULL,
  created_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX idx_xevent_debezium_outbox_topic
  ON xevent_debezium_outbox_records (topic);

CREATE INDEX idx_xevent_debezium_outbox_created_at
  ON xevent_debezium_outbox_records (created_at);
```

### MySQL

```sql
CREATE TABLE xevent_debezium_outbox_records (
  id VARCHAR(36) NOT NULL,
  topic VARCHAR(255) NOT NULL,
  partition_key VARCHAR(255) NOT NULL DEFAULT '',
  event_type VARCHAR(255) NOT NULL,
  event_id VARCHAR(255) NOT NULL DEFAULT '',
  payload LONGBLOB NOT NULL,
  created_at DATETIME(6) NOT NULL,
  PRIMARY KEY (id),
  KEY idx_xevent_debezium_outbox_topic (topic),
  KEY idx_xevent_debezium_outbox_created_at (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
```

## Transactional Append Example

```go
store, err := debeziumgorm.NewGORMStore(debeziumgorm.GORMStoreConfig{
	DB:                 db,
	SessionFromContext: gormtx.DB,
})
if err != nil {
	panic(err)
}

txErr := db.Transaction(func(tx *gorm.DB) error {
	ctx := gormtx.WithDB(ctx, tx)

	if err := tx.Create(&order).Error; err != nil {
		return err
	}

	_, err = debezium.AppendEvent(ctx, store, &OrderCreated{
		ID:      "evt_1",
		OrderID: order.ID,
	}, debezium.AppendOptions{Topic: "orders"})
	return err
})
if txErr != nil {
	return txErr
}
```

## Retention

`DeleteBefore` is a retention helper for old rows. It works with both MySQL and
PostgreSQL, but it is not part of the publish path.

```go
deleted, err := store.DeleteBefore(context.Background(), time.Now().Add(-24*time.Hour), 1000)
if err != nil {
	panic(err)
}
_ = deleted
```

## Test Coverage

- `go test -tags=integration ./testing/integration -v` runs the lightweight
  PostgreSQL database integration suite for append, rollback, and retention.
- `go test -tags=e2e ./testing/e2e -v` runs the full PostgreSQL + Kafka +
  Kafka Connect / Debezium pipeline and verifies emitted Kafka key, payload,
  and headers.

## Debezium Notes

- PostgreSQL is the documented connector path in this repository; the template
  below is aligned with the current `xevent/kafka` header and payload shape
- MySQL is documented here as a supported storage backend only; if you run
  Debezium on MySQL, you should derive connector settings from your platform
  conventions
- the outbox row already contains the final Kafka `topic`; there is no runtime
  fallback topic in this path

## Debezium / Kafka Connect Template

The following PostgreSQL connector template keeps the emitted Kafka message
shape aligned with `xevent/kafka` as closely as Debezium allows:

```json
{
  "name": "xevent-debezium-outbox",
  "config": {
    "connector.class": "io.debezium.connector.postgresql.PostgresConnector",
    "plugin.name": "pgoutput",
    "database.hostname": "postgres",
    "database.port": "5432",
    "database.user": "postgres",
    "database.password": "postgres",
    "database.dbname": "app",
    "database.sslmode": "disable",
    "topic.prefix": "app-db",
    "schema.include.list": "public",
    "table.include.list": "public.xevent_debezium_outbox_records",
    "publication.autocreate.mode": "filtered",
    "tombstones.on.delete": "false",
    "transforms": "outbox",
    "transforms.outbox.type": "io.debezium.transforms.outbox.EventRouter",
    "transforms.outbox.table.op.invalid.behavior": "fatal",
    "transforms.outbox.table.field.event.id": "id",
    "transforms.outbox.route.by.field": "topic",
    "transforms.outbox.route.topic.replacement": "${routedByValue}",
    "transforms.outbox.table.field.event.key": "partition_key",
    "transforms.outbox.table.field.event.payload": "payload",
    "transforms.outbox.table.fields.additional.placement": "event_type:header:x-event-type,event_id:header:x-event-id",
    "value.converter": "io.debezium.converters.BinaryDataConverter",
    "value.converter.delegate.converter.type": "org.apache.kafka.connect.json.JsonConverter",
    "value.converter.delegate.converter.type.schemas.enable": "false"
  }
}
```

The same template is committed at
[`examples/postgres-connector.json`](./examples/postgres-connector.json).

## Verify

Automated:

```bash
go test -tags=integration ./testing/integration -v
go test -tags=e2e ./testing/e2e -v
```

Manual:

1. Confirm a row lands in `xevent_debezium_outbox_records`.
2. Confirm the row already contains the final `topic`.
3. Start the PostgreSQL connector with an outbox-only table include list.
4. Consume from the target Kafka topic and verify:
   - Kafka key equals `partition_key`
   - Kafka value equals the raw `payload` bytes
   - headers include `id`, `x-event-type`, and `x-event-id`
