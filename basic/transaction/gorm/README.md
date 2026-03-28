# transaction/gorm

`transaction/gorm` provides the GORM adapter for
`github.com/codesjoy/pkg/basic/transaction`.

Use this module when application code wants callback-first transaction handling
with `REQUIRED` propagation and after-commit hooks on top of `*gorm.DB`.

For the shared model and semantics, see the
[core transaction guide](../README.md).
For DDD-oriented transaction boundary guidance, see
[DDD Usage Pattern](../README.md#ddd-usage-pattern).

## Installation

```bash
go get github.com/codesjoy/pkg/basic/transaction/gorm
```

## Quick Start

```go
package main

import (
	"context"

	"github.com/codesjoy/pkg/basic/transaction"
	gormtx "github.com/codesjoy/pkg/basic/transaction/gorm"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type User struct {
	ID   uint `gorm:"primaryKey"`
	Name string
}

func main() {
	db, err := gorm.Open(postgres.Open("postgres://user:pass@127.0.0.1/app?sslmode=disable"), &gorm.Config{})
	if err != nil {
		panic(err)
	}

	runner := gormtx.New(db)

	err = runner.Within(context.Background(), func(ctx context.Context) error {
		if err := runner.DB(ctx).Create(&User{Name: "alice"}).Error; err != nil {
			return err
		}

		return transaction.AfterCommit(ctx, func(hookCtx context.Context) error {
			baseDB := runner.DB(hookCtx)
			_ = baseDB
			return nil
		})
	})
	if err != nil {
		panic(err)
	}
}
```

## Runner Behavior

- `gormtx.New(db)` creates a runner backed by the provided base `*gorm.DB`
- `runner.Within(ctx, fn)` starts a transaction when one is missing
- nested `Within` calls reuse the active transaction already stored in `ctx`
- `runner.DB(ctx)` returns the active transactional `*gorm.DB` when present
- `runner.DB(ctx)` falls back to the base `*gorm.DB` outside a transaction

If the runner was created with a nil base DB and no transaction exists in the
incoming context, `Within` returns `gorm.ErrInvalidDB`.

## After-Commit Context

After-commit hooks run only after the outermost transaction commits
successfully.

For the GORM adapter, the hook context carries the base DB context, not the
transactional `*gorm.DB`. In other words:

- writes in the hook are not part of the committed transaction
- `runner.DB(hookCtx)` resolves to the base DB handle

If a hook fails, `Within` returns an error joined with
`transaction.ErrAfterCommitFailed`, but the transaction has already committed.

## Context Utilities

The adapter exposes helpers for advanced context propagation:

- `gormtx.WithDB(ctx, db)` stores a `*gorm.DB` in the default context slot
- `gormtx.DB(ctx)` reads a `*gorm.DB` from the default context slot
- `gormtx.NewContextSlot()` creates an isolated slot
- `gormtx.WithContextSlot(slot)` configures a runner to use that slot

Use a custom `ContextSlot` when one process needs multiple independent GORM
transaction channels in the same context tree and you want to avoid collisions.

Most applications can use the default slot and only interact with `runner.DB`.
