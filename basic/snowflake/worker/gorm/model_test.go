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

package gorm

import (
	"fmt"
	"strings"
	"testing"

	"github.com/codesjoy/pkg/basic/snowflake/worker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupModelTestDB(t *testing.T) *gorm.DB {
	dbName := strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared&_busy_timeout=5000", dbName)
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	err = db.AutoMigrate(&SnowflakeWorker{})
	require.NoError(t, err)

	return db
}

func TestSnowflakeWorkerData_getReleasedWorkerInfo(t *testing.T) {
	db := setupModelTestDB(t)

	business := "test-business-released"
	flag := "test-flag"

	data := &snowflakeWorkerData{
		maxWorkerID: (1 << 6) - 1,
		db:          db,
	}

	// Create a released worker
	worker := &SnowflakeWorker{
		WorkerID: 1,
		Business: business,
		Flag:     "old-flag",
		Status:   statusUnused,
	}
	err := db.Create(worker).Error
	require.NoError(t, err)

	// Get released worker info
	info, err := data.getReleasedWorkerInfo(business, flag)
	assert.NoError(t, err)
	assert.NotNil(t, info)
	assert.Equal(t, int64(1), info.WorkerID)

	// Verify status updated to used and flag changed
	var updated SnowflakeWorker
	err = db.Where("worker_id = ? AND business = ?", 1, business).First(&updated).Error
	require.NoError(t, err)
	assert.Equal(t, statusUsed, updated.Status)
	assert.Equal(t, flag, updated.Flag)
}

func TestSnowflakeWorkerData_getReleasedWorkerInfo_NoneAvailable(t *testing.T) {
	db := setupModelTestDB(t)

	business := "test-business-none"
	flag := "test-flag"

	data := &snowflakeWorkerData{
		maxWorkerID: (1 << 6) - 1,
		db:          db,
	}

	// Try to get released worker when none exist
	info, err := data.getReleasedWorkerInfo(business, flag)
	assert.NoError(t, err)
	assert.Nil(t, info)
}

func TestSnowflakeWorkerData_getNewWorker(t *testing.T) {
	db := setupModelTestDB(t)

	business := "test-business-new"
	flag := "test-flag"

	data := &snowflakeWorkerData{
		maxWorkerID: (1 << 6) - 1,
		db:          db,
	}

	// Create new worker
	info, err := data.getNewWorker(business, flag)
	assert.NoError(t, err)
	assert.NotNil(t, info)
	assert.Equal(t, int64(1), info.WorkerID)

	// Verify created in database
	var worker SnowflakeWorker
	err = db.Where("worker_id = ? AND business = ?", 1, business).First(&worker).Error
	require.NoError(t, err)
	assert.Equal(t, int64(1), worker.WorkerID)
	assert.Equal(t, business, worker.Business)
	assert.Equal(t, flag, worker.Flag)
	assert.Equal(t, statusUsed, worker.Status)
}

func TestSnowflakeWorkerData_getNewWorker_Sequential(t *testing.T) {
	db := setupModelTestDB(t)

	business := "test-business-sequential"
	flag := "test-flag"

	data := &snowflakeWorkerData{
		maxWorkerID: (1 << 6) - 1,
		db:          db,
	}

	// Create multiple workers
	for i := 1; i <= 5; i++ {
		info, err := data.getNewWorker(business, flag)
		assert.NoError(t, err)
		assert.Equal(t, int64(i), info.WorkerID)
	}

	// Verify all workers in database
	var count int64
	err := db.Model(&SnowflakeWorker{}).Where("business = ?", business).Count(&count).Error
	require.NoError(t, err)
	assert.Equal(t, int64(5), count)
}

func TestSnowflakeWorkerData_getNewWorker_Conflict(t *testing.T) {
	db := setupModelTestDB(t)

	business := "test-business-conflict"
	flag1 := "test-flag-1"
	flag2 := "test-flag-2"

	data := &snowflakeWorkerData{
		maxWorkerID: (1 << 6) - 1,
		db:          db,
	}

	// Create first worker
	info1, err := data.getNewWorker(business, flag1)
	assert.NoError(t, err)
	assert.Equal(t, int64(1), info1.WorkerID)

	// Manually insert a conflicting worker ID
	conflictWorker := &SnowflakeWorker{
		WorkerID: 2,
		Business: business,
		Flag:     "other-flag",
		Status:   statusUsed,
	}
	err = db.Create(conflictWorker).Error
	require.NoError(t, err)

	// Try to create next worker - should skip to ID 3 due to conflict
	info2, err := data.getNewWorker(business, flag2)
	assert.NoError(t, err)
	assert.Equal(t, int64(3), info2.WorkerID)
}

func TestSnowflakeWorkerData_getNewWorker_Exhausted(t *testing.T) {
	db := setupModelTestDB(t)

	business := "test-business-exhausted"
	flag := "test-flag"
	maxWorkerID := int64(3) // Small limit for testing

	data := &snowflakeWorkerData{
		maxWorkerID: maxWorkerID,
		db:          db,
	}

	// Create workers up to the limit
	for i := int64(1); i <= maxWorkerID; i++ {
		info, err := data.getNewWorker(business, flag)
		assert.NoError(t, err)
		assert.Equal(t, i, info.WorkerID)
	}

	// Try to create one more - should fail
	_, err := data.getNewWorker(business, flag)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no worker id is available")
}

func TestSnowflakeWorkerData_releaseWorkerID(t *testing.T) {
	db := setupModelTestDB(t)

	business := "test-business-release"
	flag := "test-flag"
	workerID := int64(1)

	data := &snowflakeWorkerData{
		maxWorkerID: (1 << 6) - 1,
		db:          db,
	}

	// Create a worker
	worker := &SnowflakeWorker{
		WorkerID: workerID,
		Business: business,
		Flag:     flag,
		Status:   statusUsed,
	}
	err := db.Create(worker).Error
	require.NoError(t, err)

	// Release worker
	err = data.releaseWorkerID(workerID, business, flag)
	assert.NoError(t, err)

	// Verify status updated
	var released SnowflakeWorker
	err = db.Where("worker_id = ? AND business = ? AND flag = ?", workerID, business, flag).
		First(&released).
		Error
	require.NoError(t, err)
	assert.Equal(t, statusUnused, released.Status)
}

func TestSnowflakeWorkerData_releaseWorkerID_NotFound(t *testing.T) {
	db := setupModelTestDB(t)

	business := "test-business-notfound"
	flag := "test-flag"
	workerID := int64(999)

	data := &snowflakeWorkerData{
		maxWorkerID: (1 << 6) - 1,
		db:          db,
	}

	// Release non-existent worker - should succeed without error
	err := data.releaseWorkerID(workerID, business, flag)
	assert.NoError(t, err)
}

func TestSnowflakeWorkerData_updateOverLastTime(t *testing.T) {
	db := setupModelTestDB(t)

	business := "test-business-over"
	flag := "test-flag"
	workerID := int64(1)

	data := &snowflakeWorkerData{
		maxWorkerID: (1 << 6) - 1,
		db:          db,
	}

	// Create a worker
	worker := &SnowflakeWorker{
		WorkerID:     workerID,
		Business:     business,
		Flag:         flag,
		Status:       statusUsed,
		OverLastTime: 100,
	}
	err := db.Create(worker).Error
	require.NoError(t, err)

	// Update over last time
	newOverLastTime := int64(200)
	err = data.updateOverLastTime(workerID, business, flag, newOverLastTime)
	assert.NoError(t, err)

	// Verify updated
	var updated SnowflakeWorker
	err = db.Where("worker_id = ? AND business = ? AND flag = ?", workerID, business, flag).
		First(&updated).
		Error
	require.NoError(t, err)
	assert.Equal(t, newOverLastTime, updated.OverLastTime)
}

func TestSnowflakeWorkerData_updateOverLastTime_NotFound(t *testing.T) {
	db := setupModelTestDB(t)

	business := "test-business-over-notfound"
	flag := "test-flag"
	workerID := int64(999)

	data := &snowflakeWorkerData{
		maxWorkerID: (1 << 6) - 1,
		db:          db,
	}

	// Update non-existent worker
	err := data.updateOverLastTime(workerID, business, flag, 200)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "worker id not exist")
}

func TestSnowflakeWorkerData_updateBackLastTime(t *testing.T) {
	db := setupModelTestDB(t)

	business := "test-business-back"
	flag := "test-flag"
	workerID := int64(1)

	data := &snowflakeWorkerData{
		maxWorkerID: (1 << 6) - 1,
		db:          db,
	}

	// Create a worker
	worker := &SnowflakeWorker{
		WorkerID:     workerID,
		Business:     business,
		Flag:         flag,
		Status:       statusUsed,
		BackLastTime: 100,
	}
	err := db.Create(worker).Error
	require.NoError(t, err)

	// Update back last time
	newBackLastTime := int64(300)
	err = data.updateBackLastTime(workerID, business, flag, newBackLastTime)
	assert.NoError(t, err)

	// Verify updated
	var updated SnowflakeWorker
	err = db.Where("worker_id = ? AND business = ? AND flag = ?", workerID, business, flag).
		First(&updated).
		Error
	require.NoError(t, err)
	assert.Equal(t, newBackLastTime, updated.BackLastTime)
}

func TestSnowflakeWorkerData_updateBackLastTime_NotFound(t *testing.T) {
	db := setupModelTestDB(t)

	business := "test-business-back-notfound"
	flag := "test-flag"
	workerID := int64(999)

	data := &snowflakeWorkerData{
		maxWorkerID: (1 << 6) - 1,
		db:          db,
	}

	// Update non-existent worker
	err := data.updateBackLastTime(workerID, business, flag, 300)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "worker id not exist")
}

func TestSnowflakeWorker_TableStructure(t *testing.T) {
	db := setupModelTestDB(t)

	// Verify table exists and has correct structure
	worker := &SnowflakeWorker{
		WorkerID:     1,
		Business:     "test",
		Flag:         "flag",
		Status:       statusUsed,
		OverLastTime: 100,
		BackLastTime: 200,
	}

	err := db.Create(worker).Error
	require.NoError(t, err)

	// Query back and verify all fields
	var result SnowflakeWorker
	err = db.First(&result, worker.ID).Error
	require.NoError(t, err)

	assert.Equal(t, worker.WorkerID, result.WorkerID)
	assert.Equal(t, worker.Business, result.Business)
	assert.Equal(t, worker.Flag, result.Flag)
	assert.Equal(t, worker.Status, result.Status)
	assert.Equal(t, worker.OverLastTime, result.OverLastTime)
	assert.Equal(t, worker.BackLastTime, result.BackLastTime)
}

func TestSnowflakeWorker_UniqueIndex(t *testing.T) {
	db := setupModelTestDB(t)

	// Create first worker
	worker1 := &SnowflakeWorker{
		WorkerID: 1,
		Business: "test-business",
		Flag:     "flag1",
		Status:   statusUsed,
	}
	err := db.Create(worker1).Error
	require.NoError(t, err)

	// Try to create duplicate worker ID + business
	worker2 := &SnowflakeWorker{
		WorkerID: 1,
		Business: "test-business",
		Flag:     "flag2",
		Status:   statusUsed,
	}
	err = db.Create(worker2).Error
	assert.Error(t, err)

	// Same worker ID but different business should work
	worker3 := &SnowflakeWorker{
		WorkerID: 1,
		Business: "other-business",
		Flag:     "flag3",
		Status:   statusUsed,
	}
	err = db.Create(worker3).Error
	assert.NoError(t, err)
}

func TestWorker_Info_Properties(t *testing.T) {
	info := &worker.Info{
		WorkerID:     123,
		OverLastTime: 456,
		BackLastTime: 789,
	}

	assert.Equal(t, int64(123), info.WorkerID)
	assert.Equal(t, int64(456), info.OverLastTime)
	assert.Equal(t, int64(789), info.BackLastTime)
}
