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

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	"github.com/codesjoy/pkg/basic/transaction"
	gormtx "github.com/codesjoy/pkg/basic/transaction/gorm"
	"github.com/codesjoy/pkg/basic/xgorm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type integrationUser struct {
	ID      uint `gorm:"primaryKey"`
	Name    string
	Age     int
	Balance int
}

func (integrationUser) TableName() string {
	return "xgorm_integration_users"
}

func TestPostgresIntegration_WrapPageQueryAndFindOne(t *testing.T) {
	ctx, cancel := integrationContext(t)
	defer cancel()

	db := mustDB(t)
	db = db.WithContext(ctx)
	resetTables(t, db)

	seed := []integrationUser{
		{Name: "alice", Age: 18, Balance: 100},
		{Name: "bob", Age: 25, Balance: 200},
		{Name: "carol", Age: 30, Balance: 300},
		{Name: "dave", Age: 35, Balance: 400},
		{Name: "erin", Age: 40, Balance: 500},
	}
	require.NoError(t, db.Create(&seed).Error)

	var page []integrationUser
	result, err := xgorm.WrapPageQuery(
		db.Model(&integrationUser{}).Where("age >= ?", 25).Order("id ASC"),
		xgorm.PaginationParam{Pagination: true, Current: 2, PageSize: 2},
		&page,
	)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, uint32(4), result.Total)
	assert.Equal(t, uint32(2), result.Current)
	assert.Equal(t, uint32(2), result.PageSize)
	require.Len(t, page, 2)
	assert.Equal(t, "dave", page[0].Name)
	assert.Equal(t, "erin", page[1].Name)

	var onlyCountRows []integrationUser
	countResult, err := xgorm.WrapPageQuery(
		db.Model(&integrationUser{}).Where("age >= ?", 30),
		xgorm.PaginationParam{OnlyCount: true},
		&onlyCountRows,
	)
	require.NoError(t, err)
	require.NotNil(t, countResult)
	assert.Equal(t, uint32(3), countResult.Total)
	assert.Empty(t, onlyCountRows)

	var found integrationUser
	exists, err := xgorm.FindOne(db.Model(&integrationUser{}).Where("name = ?", "alice"), &found)
	require.NoError(t, err)
	assert.True(t, exists)
	assert.Equal(t, "alice", found.Name)

	var missing integrationUser
	exists, err = xgorm.FindOne(db.Model(&integrationUser{}).Where("name = ?", "nobody"), &missing)
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestPostgresIntegration_TransactionWithinAndRollback(t *testing.T) {
	ctx, cancel := integrationContext(t)
	defer cancel()

	db := mustDB(t)
	db = db.WithContext(ctx)
	resetTables(t, db)

	runner := gormtx.New(db)

	require.NoError(
		t,
		runner.Within(ctx, func(txCtx context.Context) error {
			return runner.DB(txCtx).
				Create(&integrationUser{Name: "committed", Age: 30, Balance: 100}).
				Error
		}),
	)

	var committedCount int64
	require.NoError(
		t,
		db.Model(&integrationUser{}).Where("name = ?", "committed").Count(&committedCount).Error,
	)
	assert.Equal(t, int64(1), committedCount)

	err := runner.Within(ctx, func(txCtx context.Context) error {
		tx := runner.DB(txCtx)
		if err := tx.Create(&integrationUser{Name: "rolled-back", Age: 28, Balance: 80}).Error; err != nil {
			return err
		}
		if err := tx.Model(&integrationUser{}).
			Where("name = ?", "committed").
			Update("balance", 999).
			Error; err != nil {
			return err
		}
		return errors.New("force rollback")
	})
	require.EqualError(t, err, "force rollback")

	var rolledBackCount int64
	require.NoError(
		t,
		db.Model(&integrationUser{}).Where("name = ?", "rolled-back").Count(&rolledBackCount).Error,
	)
	assert.Equal(t, int64(0), rolledBackCount)

	var committed integrationUser
	require.NoError(
		t,
		db.Model(&integrationUser{}).Where("name = ?", "committed").First(&committed).Error,
	)
	assert.Equal(t, 100, committed.Balance)
}

func TestPostgresIntegration_TransactionAfterCommit(t *testing.T) {
	ctx, cancel := integrationContext(t)
	defer cancel()

	db := mustDB(t)
	db = db.WithContext(ctx)
	resetTables(t, db)

	runner := gormtx.New(db)
	called := false

	err := runner.Within(ctx, func(txCtx context.Context) error {
		if err := runner.DB(txCtx).
			Create(&integrationUser{Name: "helper-success", Age: 31, Balance: 310}).
			Error; err != nil {
			return err
		}
		return transaction.AfterCommit(txCtx, func(hookCtx context.Context) error {
			called = true
			var count int64
			if err := runner.DB(hookCtx).
				Model(&integrationUser{}).
				Where("name = ?", "helper-success").
				Count(&count).
				Error; err != nil {
				return err
			}
			if count != 1 {
				return errors.New("after-commit hook could not observe committed row")
			}
			return nil
		})
	})
	require.NoError(t, err)
	assert.True(t, called)

	var successCount int64
	require.NoError(
		t,
		db.Model(&integrationUser{}).Where("name = ?", "helper-success").Count(&successCount).Error,
	)
	assert.Equal(t, int64(1), successCount)

	called = false
	err = runner.Within(ctx, func(txCtx context.Context) error {
		tx := runner.DB(txCtx)
		if err := tx.Create(&integrationUser{Name: "helper-rollback", Age: 29, Balance: 290}).Error; err != nil {
			return err
		}
		if err := transaction.AfterCommit(txCtx, func(context.Context) error {
			called = true
			return nil
		}); err != nil {
			return err
		}
		return errors.New("force rollback")
	})
	require.EqualError(t, err, "force rollback")
	assert.False(t, called)

	var rollbackCount int64
	require.NoError(
		t,
		db.Model(&integrationUser{}).
			Where("name = ?", "helper-rollback").
			Count(&rollbackCount).
			Error,
	)
	assert.Equal(t, int64(0), rollbackCount)
}
