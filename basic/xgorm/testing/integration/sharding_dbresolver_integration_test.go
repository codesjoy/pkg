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
	"fmt"
	"testing"

	"github.com/codesjoy/pkg/basic/xgorm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	postgresdriver "gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/plugin/dbresolver"
	"gorm.io/sharding"
)

type resolverUser struct {
	ID   int64 `gorm:"primaryKey"`
	Name string
}

func (resolverUser) TableName() string {
	return "xgorm_integration_resolver_users"
}

type shardedOrder struct {
	ID      int64 `gorm:"primaryKey"`
	UserID  int64
	Product string
}

func (shardedOrder) TableName() string {
	return "orders"
}

func TestPostgresIntegration_DBResolverRouting(t *testing.T) {
	ctx, cancel := integrationContext(t)
	defer cancel()

	sourceDB := mustSourceDB(t).WithContext(ctx)
	replicaDB := mustReplicaDB(t).WithContext(ctx)
	resetResolverTables(t, sourceDB, replicaDB)

	require.NoError(t, sourceDB.Create(&resolverUser{ID: 1, Name: "source-value"}).Error)
	require.NoError(t, replicaDB.Create(&resolverUser{ID: 1, Name: "replica-value"}).Error)

	routedDB := mustRoutedDB(t, xgorm.WithDBResolver(newResolverConfig(t))).WithContext(ctx)

	var readUser resolverUser
	require.NoError(t, routedDB.Model(&resolverUser{}).First(&readUser, 1).Error)
	assert.Equal(t, "replica-value", readUser.Name)

	require.NoError(
		t,
		routedDB.Model(&resolverUser{}).Where("id = ?", 1).Update("name", "source-updated").Error,
	)

	var sourceUser resolverUser
	var replicaUser resolverUser
	require.NoError(t, sourceDB.Model(&resolverUser{}).First(&sourceUser, 1).Error)
	require.NoError(t, replicaDB.Model(&resolverUser{}).First(&replicaUser, 1).Error)
	assert.Equal(t, "source-updated", sourceUser.Name)
	assert.Equal(t, "replica-value", replicaUser.Name)
}

func TestPostgresIntegration_ShardingRouting(t *testing.T) {
	ctx, cancel := integrationContext(t)
	defer cancel()

	sourceDB := mustSourceDB(t).WithContext(ctx)
	resetShardingTables(t, sourceDB, nil, defaultShardNum)

	shardingDB := mustRoutedDB(
		t,
		xgorm.WithSharding(
			sharding.Config{
				ShardingKey:         "user_id",
				NumberOfShards:      uint(defaultShardNum),
				PrimaryKeyGenerator: sharding.PKSnowflake,
			},
			&shardedOrder{},
		),
	).WithContext(ctx)

	orders := []shardedOrder{
		{UserID: 1, Product: "u1"},
		{UserID: 2, Product: "u2"},
		{UserID: 5, Product: "u5"},
	}
	for i := range orders {
		require.NoError(t, shardingDB.Create(&orders[i]).Error)
	}

	var got shardedOrder
	require.NoError(
		t,
		shardingDB.Model(&shardedOrder{}).Where("user_id = ?", int64(5)).First(&got).Error,
	)
	assert.Equal(t, "u5", got.Product)

	require.NoError(
		t,
		shardingDB.Model(&shardedOrder{}).Where("user_id = ?", int64(5)).Update("product", "u5-updated").Error,
	)

	var updated shardedOrder
	require.NoError(
		t,
		shardingDB.Model(&shardedOrder{}).Where("user_id = ?", int64(5)).First(&updated).Error,
	)
	assert.Equal(t, "u5-updated", updated.Product)

	expectedTable := shardTableName(5, defaultShardNum)
	var shardCount int64
	require.NoError(
		t,
		sourceDB.Table(expectedTable).Where("user_id = ?", int64(5)).Count(&shardCount).Error,
	)
	assert.Equal(t, int64(1), shardCount)
}

func TestPostgresIntegration_ShardingAndDBResolverCombined(t *testing.T) {
	ctx, cancel := integrationContext(t)
	defer cancel()

	sourceDB := mustSourceDB(t).WithContext(ctx)
	replicaDB := mustReplicaDB(t).WithContext(ctx)
	resetShardingTables(t, sourceDB, replicaDB, defaultShardNum)

	combinedDB := mustRoutedDB(
		t,
		xgorm.WithSharding(
			sharding.Config{
				ShardingKey:         "user_id",
				NumberOfShards:      uint(defaultShardNum),
				PrimaryKeyGenerator: sharding.PKSnowflake,
			},
			&shardedOrder{},
		),
		xgorm.WithDBResolver(newResolverConfig(t)),
	).WithContext(ctx)

	const (
		targetUserID = int64(1)
		targetID     = int64(1)
	)
	targetTable := shardTableName(targetUserID, defaultShardNum)
	insertSQL := fmt.Sprintf("INSERT INTO %s(id, user_id, product) VALUES(?, ?, ?)", targetTable)
	require.NoError(t, sourceDB.Exec(insertSQL, targetID, targetUserID, "source-product").Error)
	require.NoError(t, replicaDB.Exec(insertSQL, targetID, targetUserID, "replica-product").Error)

	var readOrder shardedOrder
	require.NoError(
		t,
		combinedDB.Model(&shardedOrder{}).Where("user_id = ?", targetUserID).First(&readOrder).Error,
	)
	assert.Equal(t, "replica-product", readOrder.Product)

	require.NoError(
		t,
		combinedDB.Model(&shardedOrder{}).
			Where("user_id = ?", targetUserID).
			Update("product", "source-updated").Error,
	)

	var sourceOrder shardedOrder
	var replicaOrder shardedOrder
	require.NoError(t, sourceDB.Table(targetTable).Where("id = ?", targetID).First(&sourceOrder).Error)
	require.NoError(t, replicaDB.Table(targetTable).Where("id = ?", targetID).First(&replicaOrder).Error)
	assert.Equal(t, "source-updated", sourceOrder.Product)
	assert.Equal(t, "replica-product", replicaOrder.Product)
}

func newResolverConfig(t *testing.T) dbresolver.Config {
	t.Helper()
	require.NotNil(t, sourceHarness)
	require.NotNil(t, replicaHarness)
	require.NotEmpty(t, sourceHarness.dsn)
	require.NotEmpty(t, replicaHarness.dsn)
	return dbresolver.Config{
		Sources:  []gorm.Dialector{postgresdriver.Open(sourceHarness.dsn)},
		Replicas: []gorm.Dialector{postgresdriver.Open(replicaHarness.dsn)},
	}
}

func shardTableName(userID int64, shardCount int) string {
	return fmt.Sprintf("orders_%d", int(userID%int64(shardCount)))
}
