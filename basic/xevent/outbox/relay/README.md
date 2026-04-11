# xevent/outbox/relay

`xevent/outbox/relay` provides the application-managed outbox variant: a
transaction-friendly local event table, store contract, and relay that
publishes pending records through any `xevent.Sender`.

## What This Package Provides

- `Record`: one persisted outbox row
- `AppendEvent`: encode an `xevent.Event` and append it to a local outbox table
- `Store`: persistence contract for append/claim/state transitions
- `Relay`: local polling relay with `Run`, `Wake`, and `ProcessOnce`
- `MemoryStore`: in-memory `Store` implementation for tests and local flows

## Optional Adapter Modules

- GORM adapter: `github.com/codesjoy/pkg/basic/xevent/outbox/relay/gorm`

## In-Memory Example

```go
store := outbox.NewMemoryStore()

_, err := outbox.AppendEvent(ctx, store, &OrderCreated{
	ID:      "evt_1",
	OrderID: "o_123",
	UserID:  "u_1",
}, outbox.AppendOptions{})
if err != nil {
	panic(err)
}
```

`AppendOptions.AvailableAt` can be set when one record should not be sent before
a specific time.

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

go func() {
	if err := relay.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		panic(err)
	}
}()
```

`RelayConfig.Owner` identifies one relay instance for claim ownership. When it
is left empty, `NewRelay` generates a UUID automatically. If application code
sets `Owner` explicitly, it must remain globally unique across live relay
instances.

`Wake()` is the entry point for:

- transaction-commit driven immediate scans
- external CDC listeners that observe outbox table changes
- manual nudges from application code

For a GORM-backed store, see `xevent/outbox/relay/gorm`.
