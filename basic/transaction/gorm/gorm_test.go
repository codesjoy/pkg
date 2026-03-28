// Copyright 2022 The codesjoy Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package gormtx

import (
	"context"
	"errors"
	"testing"

	"github.com/codesjoy/pkg/basic/transaction"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func testNilContext() context.Context {
	return nil
}

func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open() error = %v", err)
	}
	return db
}

func TestRequiredStartsTransactionWhenMissing(t *testing.T) {
	runner := New(openTestDB(t))

	err := runner.Within(context.Background(), func(ctx context.Context) error {
		if runner.DB(ctx) == nil {
			t.Fatal("Runner.DB() = nil, want active tx")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Within() error = %v", err)
	}
}

func TestWithinReusesExistingTransaction(t *testing.T) {
	db := openTestDB(t)
	runner := New(db)

	err := db.Transaction(func(tx *gorm.DB) error {
		txCtx := WithDB(context.Background(), tx)
		return runner.Within(txCtx, func(ctx context.Context) error {
			if got := runner.DB(ctx); got != tx {
				t.Fatal("Within() did not reuse existing tx")
			}
			return nil
		})
	})
	if err != nil {
		t.Fatalf("db.Transaction() error = %v", err)
	}
}

func TestWithinRunsCommitHooksAfterCommit(t *testing.T) {
	runner := New(openTestDB(t))
	called := false

	err := runner.Within(context.Background(), func(ctx context.Context) error {
		if err := transaction.AfterCommit(ctx, func(hookCtx context.Context) error {
			called = true
			if got := runner.DB(hookCtx); got == nil {
				t.Fatal("Runner.DB(hookCtx) = nil, want base db")
			}
			return nil
		}); err != nil {
			t.Fatalf("AfterCommit() error = %v", err)
		}
		if called {
			t.Fatal("commit hook ran before commit")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Within() error = %v", err)
	}
	if !called {
		t.Fatal("commit hook did not run after commit")
	}
}

func TestWithinSkipsCommitHooksOnRollback(t *testing.T) {
	runner := New(openTestDB(t))
	called := false
	rollbackErr := errors.New("rollback")

	err := runner.Within(context.Background(), func(ctx context.Context) error {
		if err := transaction.AfterCommit(ctx, func(context.Context) error {
			called = true
			return nil
		}); err != nil {
			t.Fatalf("AfterCommit() error = %v", err)
		}
		return rollbackErr
	})
	if !errors.Is(err, rollbackErr) {
		t.Fatalf("Within() error = %v, want rollbackErr", err)
	}
	if called {
		t.Fatal("commit hook ran on rollback")
	}
}

func TestWithinNestedRunsHooksOnceAfterOuterCommit(t *testing.T) {
	runner := New(openTestDB(t))
	order := make([]string, 0, 2)

	err := runner.Within(context.Background(), func(ctx context.Context) error {
		if err := transaction.AfterCommit(ctx, func(context.Context) error {
			order = append(order, "outer")
			return nil
		}); err != nil {
			t.Fatalf("AfterCommit() error = %v", err)
		}
		return runner.Within(ctx, func(inner context.Context) error {
			if got := runner.DB(inner); got == nil {
				t.Fatal("Runner.DB() = nil, want active tx")
			}
			if err := transaction.AfterCommit(inner, func(context.Context) error {
				order = append(order, "inner")
				return nil
			}); err != nil {
				t.Fatalf("AfterCommit() error = %v", err)
			}
			return nil
		})
	})
	if err != nil {
		t.Fatalf("Within() error = %v", err)
	}
	if len(order) != 2 || order[0] != "outer" || order[1] != "inner" {
		t.Fatalf("hook order = %v, want [outer inner]", order)
	}
}

func TestWithDBAndDBUseDefaultSlot(t *testing.T) {
	db := openTestDB(t)
	ctx := WithDB(context.Background(), db)

	if got := DB(ctx); got != db {
		t.Fatal("DB() did not return injected db")
	}
}

func TestRunnerDBFallsBackToBaseDB(t *testing.T) {
	db := openTestDB(t)
	runner := New(db)

	if got := runner.DB(context.Background()); got != db {
		t.Fatal("Runner.DB() did not fall back to base db")
	}
}

func TestWithinReturnsInvalidDBWhenBaseDBIsNil(t *testing.T) {
	runner := New(nil)

	err := runner.Within(context.Background(), func(context.Context) error { return nil })
	if !errors.Is(err, gorm.ErrInvalidDB) {
		t.Fatalf("Within() error = %v, want gorm.ErrInvalidDB", err)
	}
}

func TestWithinAllowsNilContextAndNilFunc(t *testing.T) {
	runner := New(openTestDB(t))

	if err := runner.Within(testNilContext(), nil); err != nil {
		t.Fatalf("Within(nil, nil) error = %v", err)
	}
}

func TestWithinReturnsAfterCommitErrorsAfterCommit(t *testing.T) {
	type widget struct {
		ID   uint `gorm:"primaryKey"`
		Name string
	}

	db := openTestDB(t)
	if err := db.AutoMigrate(&widget{}); err != nil {
		t.Fatalf("AutoMigrate() error = %v", err)
	}

	runner := New(db)
	hookErr := errors.New("hook failed")

	err := runner.Within(context.Background(), func(ctx context.Context) error {
		if err := runner.DB(ctx).Create(&widget{Name: "saved"}).Error; err != nil {
			return err
		}
		return transaction.AfterCommit(ctx, func(context.Context) error {
			return hookErr
		})
	})
	if !errors.Is(err, transaction.ErrAfterCommitFailed) {
		t.Fatalf("Within() error = %v, want ErrAfterCommitFailed", err)
	}
	if !errors.Is(err, hookErr) {
		t.Fatalf("Within() error = %v, want joined hookErr", err)
	}

	var count int64
	if db.Model(&widget{}).Where("name = ?", "saved").Count(&count).Error != nil {
		t.Fatal("Count() failed")
	}
	if count != 1 {
		t.Fatalf("committed rows = %d, want 1", count)
	}
}
