# transaction

`transaction` provides callback-first transaction orchestration primitives and
after-commit hook registration.

The package is intentionally small:

- `transaction.Runner` defines the transaction boundary contract
- `transaction.AfterCommit` registers follow-up work for the current transaction
- adapter modules provide concrete integrations for GORM and MongoDB

## Modules

- Core package: `github.com/codesjoy/pkg/basic/transaction`
- GORM adapter: `github.com/codesjoy/pkg/basic/transaction/gorm`
- MongoDB adapter: `github.com/codesjoy/pkg/basic/transaction/mongo`

Adapter-specific guides:

- [GORM adapter](./gorm/README.md)
- [MongoDB adapter](./mongo/README.md)

## Installation

```bash
go get github.com/codesjoy/pkg/basic/transaction
```

Install an adapter module separately when needed:

```bash
go get github.com/codesjoy/pkg/basic/transaction/gorm
go get github.com/codesjoy/pkg/basic/transaction/mongo
```

## Core Model

`Runner` exposes one method:

```go
type Runner interface {
	Within(context.Context, func(context.Context) error) error
}
```

`Within` executes the callback inside the current transaction scope.

- If the incoming context already carries an active transaction for that runner,
  the callback reuses it.
- Otherwise the adapter starts a new transaction and passes a derived context
  into the callback.

The propagation model is `REQUIRED` only. This package does not implement other
propagation levels such as `REQUIRES_NEW`, `MANDATORY`, or `SUPPORTS`.

## After-Commit Hooks

Use `transaction.AfterCommit(ctx, hook)` inside a transaction callback to defer
follow-up work until the outermost transaction commits successfully.

Typical uses include:

- publishing a message after the write is durable
- waking an outbox relay
- updating external caches only after commit

Example with a generic runner:

```go
func CreateOrder(
	ctx context.Context,
	runner transaction.Runner,
	insert func(context.Context) error,
	publish func(context.Context) error,
) error {
	return runner.Within(ctx, func(txCtx context.Context) error {
		if err := insert(txCtx); err != nil {
			return err
		}

		return transaction.AfterCommit(txCtx, func(hookCtx context.Context) error {
			return publish(hookCtx)
		})
	})
}
```

## Transaction Semantics

The shared semantics across adapters are:

- nested `Within` calls reuse the active transaction
- hooks run only after the outermost transaction commits successfully
- hooks run in registration order
- hooks run at most once for one transaction scope
- rollback skips all registered hooks
- hook failures happen after commit and therefore do not roll back committed data

When one or more hooks fail, the runner returns an error joined with
`transaction.ErrAfterCommitFailed`.

## DDD Usage Pattern

In a DDD-style application, the default transaction boundary should live in the
application service or use case layer.

That keeps the commit point visible where one business operation is orchestrated
across aggregates, domain services, repositories, and post-commit side effects.

Recommended layering rule:

- application service opens the transaction with `runner.Within(...)`
- domain service receives and propagates `ctx`
- repositories consume the current transaction from `ctx`
- `transaction.AfterCommit(...)` is registered in orchestration code, not hidden
  inside generic infrastructure helpers

Example:

```go
type OrderRepo interface {
	Save(context.Context, *Order) error
}

type StockRepo interface {
	Reserve(context.Context, []OrderItem) error
}

type OrderDomainService struct{}

func (s *OrderDomainService) BuildOrder(_ context.Context, cmd PlaceOrderCmd) (*Order, error) {
	return NewOrder(cmd.Items), nil
}

type ApplicationService struct {
	tx     transaction.Runner
	orders OrderRepo
	stock  StockRepo
	domain *OrderDomainService
}

func (s *ApplicationService) PlaceOrder(ctx context.Context, cmd PlaceOrderCmd) error {
	return s.tx.Within(ctx, func(txCtx context.Context) error {
		order, err := s.domain.BuildOrder(txCtx, cmd)
		if err != nil {
			return err
		}
		if err := s.orders.Save(txCtx, order); err != nil {
			return err
		}
		if err := s.stock.Reserve(txCtx, order.Items()); err != nil {
			return err
		}
		return transaction.AfterCommit(txCtx, func(context.Context) error {
			return nil
		})
	})
}
```

### Repository Implementation Rule

Repository interfaces should stay `ctx`-based. A repository implementation
should use the current transaction already carried by `ctx` instead of deciding
its own transaction boundary.

For GORM repositories, use `runner.DB(ctx)` to get the active transactional DB
or fall back to the base DB:

```go
type OrderRepo struct {
	tx *gormtx.Runner
}

func (r *OrderRepo) Save(ctx context.Context, order *Order) error {
	return r.tx.DB(ctx).WithContext(ctx).Save(toModel(order)).Error
}
```

For Mongo repositories, pass `ctx` directly to driver calls so the current
session is reused automatically:

```go
type OrderRepo struct {
	coll *mongo.Collection
}

func (r *OrderRepo) Save(ctx context.Context, order *Order) error {
	_, err := r.coll.InsertOne(ctx, toDocument(order))
	return err
}
```

### When Domain Services May Open Transactions

A domain service may call `Within(...)` when it is itself a standalone
business-operation entrypoint that can be invoked both directly and from a
higher orchestration layer.

That is safe because nested `Within(...)` calls reuse the active transaction
instead of starting a second one.

This pattern is not intended for entities, value objects, or low-level domain
logic that should stay persistence-agnostic.

### Avoid This Pattern

- do not let every repository method open its own transaction
- do not hide commit boundaries inside repository implementations
- do not bury `AfterCommit(...)` in generic repository helpers
- do not mix transaction control into entity or value object methods

## Errors

- `ErrAfterCommitOutsideTransaction`: `AfterCommit` was called without an active
  transaction scope
- `ErrAfterCommitClosed`: the transaction scope has already completed, so new
  hooks can no longer be registered
- `ErrNilHook`: the provided hook was nil
- `ErrAfterCommitFailed`: the transaction committed, but one or more hooks
  returned errors

## When To Use It

Use `transaction` when application code wants:

- one consistent transaction entry point across service layers
- nested calls that automatically reuse the current transaction
- post-commit side effects that should not run on rollback

This package is not a replacement for the transaction implementations provided
by GORM or the MongoDB driver. It standardizes orchestration and hook timing on
top of those primitives.
