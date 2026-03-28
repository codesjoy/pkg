# xevent/outbox/gorm

`xevent/outbox/gorm` provides the optional GORM-backed `outbox.Store`
implementation for `xevent/outbox`.

## Installation

```bash
go get github.com/codesjoy/pkg/basic/xevent/outbox/gorm
```

## Package

```go
import (
    "github.com/codesjoy/pkg/basic/xevent/outbox"
    outboxgorm "github.com/codesjoy/pkg/basic/xevent/outbox/gorm"
)
```

## What This Package Provides

- `GORMStore`: GORM-backed `outbox.Store`
- `NewGORMStore`: creates one configured store
- `GORMStoreDialect`: optional explicit SQL strategy override

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

relay.Wake()
```

## Relay Example

```go
store, err := outboxgorm.NewGORMStore(outboxgorm.GORMStoreConfig{
    DB:                 db,
    SessionFromContext: gormtx.DB,
})
if err != nil {
    panic(err)
}

sender := xevent.SenderFromPublisher(kafkaPublisher)
relay, err := outbox.NewRelay(outbox.RelayConfig{
    Store:        store,
    Sender:       sender,
    PollInterval: time.Second,
    BatchSize:    128,
    ClaimTTL:     30 * time.Second,
    RetryDelay:   time.Second,
    MaxAttempts:  3,
})
if err != nil {
    panic(err)
}
```

## Verify

```bash
go test ./...
go test -tags=integration ./testing/integration -v
```
