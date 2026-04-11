# xevent/outbox/relay/gorm

`xevent/outbox/relay/gorm` provides the GORM-backed storage adapter for the
stateful `xevent/outbox/relay` variant.

Use this module when application code wants:

- transactional append into a local outbox table
- claim/send/retry/fail state transitions in the database
- MySQL or PostgreSQL specific claim SQL chosen automatically from the GORM
  dialector

For the relay loop and end-to-end send path, see
[`../README.md`](../README.md).

## Installation

```bash
go get github.com/codesjoy/pkg/basic/xevent/outbox/relay/gorm
```

## Package

```go
import (
	outbox "github.com/codesjoy/pkg/basic/xevent/outbox/relay"
	outboxgorm "github.com/codesjoy/pkg/basic/xevent/outbox/relay/gorm"
)
```

## What This Package Provides

- `GORMStore`: GORM-backed `outbox.Store`
- `NewGORMStore`: creates one configured store
- `GORMStoreDialect`: optional explicit SQL strategy override

## Database Support

- PostgreSQL: uses a PostgreSQL-specific claim path with `LATERAL`, `FOR UPDATE
  SKIP LOCKED`, and `RETURNING`
- MySQL: uses a MySQL-specific claim path with `ROW_NUMBER() OVER (...)` and
  `FOR UPDATE SKIP LOCKED`
- Other dialects: fall back to the `standard` SQL strategy; this path is
  functionally supported but the README does not promise the same lock behavior
  or performance characteristics as the PostgreSQL and MySQL paths

`NewGORMStore` auto-detects the dialect from `db.Name()` when
`GORMStoreConfig.Dialect` is empty. You can override it explicitly with:

- `outboxgorm.GORMStoreDialectPostgres`
- `outboxgorm.GORMStoreDialectMySQL`
- `outboxgorm.GORMStoreDialectStandard`

## Schema

You can let GORM create the table from `outbox.Record`, but many teams prefer a
reviewed SQL migration. The GORM-managed schema below keeps only the minimal
shared runtime index. Teams that want dialect-specific tuning can add the
reviewed PostgreSQL or MySQL indexes shown after the shared schema.

### PostgreSQL

```sql
CREATE TABLE xevent_outbox_records (
  id BIGSERIAL PRIMARY KEY,
  event_type VARCHAR(255) NOT NULL,
  event_id VARCHAR(255),
  partition_key VARCHAR(255) NOT NULL DEFAULT '',
  payload BYTEA NOT NULL,
  topic VARCHAR(255) NOT NULL DEFAULT '',
  available_at TIMESTAMPTZ NOT NULL,
  status VARCHAR(16) NOT NULL,
  attempts INTEGER NOT NULL DEFAULT 0,
  last_error TEXT,
  claim_owner VARCHAR(255),
  claim_until TIMESTAMPTZ,
  sent_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX idx_xevent_outbox_status_partition_available_id
  ON xevent_outbox_records (status, partition_key, available_at, id);
```

### MySQL

```sql
CREATE TABLE xevent_outbox_records (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  event_type VARCHAR(255) NOT NULL,
  event_id VARCHAR(255) NULL,
  partition_key VARCHAR(255) NOT NULL DEFAULT '',
  payload LONGBLOB NOT NULL,
  topic VARCHAR(255) NOT NULL DEFAULT '',
  available_at DATETIME(6) NOT NULL,
  status VARCHAR(16) NOT NULL,
  attempts INT NOT NULL DEFAULT 0,
  last_error LONGTEXT NULL,
  claim_owner VARCHAR(255) NULL,
  claim_until DATETIME(6) NULL,
  sent_at DATETIME(6) NULL,
  created_at DATETIME(6) NOT NULL,
  updated_at DATETIME(6) NOT NULL,
  PRIMARY KEY (id),
  KEY idx_xevent_outbox_status_partition_available_id (status, partition_key, available_at, id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
```

### Reviewed PostgreSQL Migration

```sql
CREATE INDEX idx_xevent_outbox_pending_partition_available_id
  ON xevent_outbox_records (partition_key, available_at, id)
  WHERE status = 'pending';

CREATE INDEX idx_xevent_outbox_sending_partition_available_id
  ON xevent_outbox_records (partition_key, available_at, id)
  WHERE status = 'sending';
```

### Reviewed MySQL Migration

```sql
CREATE INDEX idx_xevent_outbox_status_partition_available_id
  ON xevent_outbox_records (status, partition_key, available_at, id);
```

Indexes such as `event_id`, `sent_at`, or `claim_owner` are not part of the
core relay runtime schema. Add them only when your application has operator or
reporting queries that need them.

## Transactional Append Example

```go
store, err := outboxgorm.NewGORMStore(outboxgorm.GORMStoreConfig{
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

	_, err = outbox.AppendEvent(ctx, store, &OrderCreated{
		ID:      "evt_1",
		OrderID: order.ID,
		UserID:  order.UserID,
	}, outbox.AppendOptions{})
	return err
})
if txErr != nil {
	return txErr
}
```

After commit, start or wake the relay loop as described in
[`../README.md`](../README.md).

## Dialect Notes

- PostgreSQL and MySQL are both covered by dialect-specific unit and integration
  tests in this module
- dialect auto-detection uses `db.Name()`; `postgres`, `postgresql`, and `pgx`
  map to PostgreSQL, while `mysql` and `mariadb` map to MySQL
- choose `standard` only when you intentionally want the portable SQL path or
  are on another dialect

## Verify

```bash
go test ./...
go test -tags=integration ./testing/integration -v
```
