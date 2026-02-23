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
	"sync"
	"testing"

	"github.com/codesjoy/pkg/basic/snowflake/worker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupTestDB(t *testing.T) *gorm.DB {
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

func TestNewWorker(t *testing.T) {
	db := setupTestDB(t)

	tests := []struct {
		name    string
		cfg     *Config
		wantErr bool
	}{
		{
			name: "valid config with database",
			cfg: &Config{
				WorkerIDBitLength: 6,
				Business:          "test-business",
				DBName:            "test-db",
			},
			wantErr: false,
		},
		{
			name: "database not set",
			cfg: &Config{
				WorkerIDBitLength: 6,
				Business:          "test-business",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !tt.wantErr {
				tt.cfg.WithDB(db)
			}

			got, err := NewWorker(tt.cfg)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, got)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, got)
			}
		})
	}
}

func TestWorkerIDAllocator_GetWorkerInfo(t *testing.T) {
	db := setupTestDB(t)

	cfg := &Config{
		WorkerIDBitLength: 6,
		Business:          "test-business",
		DBName:            "test-db",
	}
	cfg.WithDB(db)

	w, err := NewWorker(cfg)
	require.NoError(t, err)

	// First call should allocate a new worker
	info, err := w.GetWorkerInfo()
	assert.NoError(t, err)
	assert.NotNil(t, info)
	assert.Equal(t, int64(1), info.WorkerID)

	// Second call should return cached info
	info2, err := w.GetWorkerInfo()
	assert.NoError(t, err)
	assert.Equal(t, info.WorkerID, info2.WorkerID)

	// Verify database state
	var worker SnowflakeWorker
	err = db.Where("worker_id = ? AND business = ?", info.WorkerID, "test-business").
		First(&worker).
		Error
	require.NoError(t, err)
	assert.Equal(t, int64(1), worker.WorkerID)
	assert.Equal(t, "test-business", worker.Business)
	assert.Equal(t, statusUsed, worker.Status)
}

func TestWorkerIDAllocator_GetWorkerInfo_ReuseReleased(t *testing.T) {
	db := setupTestDB(t)

	business := "test-business-reuse"

	// Create and release first worker
	cfg1 := &Config{
		WorkerIDBitLength: 6,
		Business:          business,
		DBName:            "test-db",
	}
	cfg1.WithDB(db)

	w1, err := NewWorker(cfg1)
	require.NoError(t, err)

	info1, err := w1.GetWorkerInfo()
	require.NoError(t, err)
	firstWorkerID := info1.WorkerID

	err = w1.ReleaseWorkerID()
	require.NoError(t, err)

	// Create second worker - should reuse the released worker
	cfg2 := &Config{
		WorkerIDBitLength: 6,
		Business:          business,
		DBName:            "test-db",
	}
	cfg2.WithDB(db)

	w2, err := NewWorker(cfg2)
	require.NoError(t, err)

	info2, err := w2.GetWorkerInfo()
	require.NoError(t, err)
	assert.Equal(t, firstWorkerID, info2.WorkerID, "Should reuse released worker ID")
}

func TestWorkerIDAllocator_GetWorkerInfo_SequentialAllocation(t *testing.T) {
	db := setupTestDB(t)

	business := "test-business-sequential"

	// Create multiple workers
	workers := make([]worker.Worker, 3)
	for i := 0; i < 3; i++ {
		cfg := &Config{
			WorkerIDBitLength: 6,
			Business:          business,
			DBName:            "test-db",
		}
		cfg.WithDB(db)

		w, err := NewWorker(cfg)
		require.NoError(t, err)
		workers[i] = w

		info, err := w.GetWorkerInfo()
		require.NoError(t, err)
		assert.Equal(t, int64(i+1), info.WorkerID, "Should allocate sequential worker IDs")
	}

	// Verify all workers are unique
	ids := make(map[int64]bool)
	for _, w := range workers {
		info, err := w.GetWorkerInfo()
		require.NoError(t, err)
		ids[info.WorkerID] = true
	}
	assert.Equal(t, 3, len(ids), "All worker IDs should be unique")
}

func TestWorkerIDAllocator_WorkerIDBitLength(t *testing.T) {
	db := setupTestDB(t)

	tests := []struct {
		name          string
		bitLength     int8
		expectedValue byte
	}{
		{
			name:          "6 bit length",
			bitLength:     6,
			expectedValue: 6,
		},
		{
			name:          "10 bit length",
			bitLength:     10,
			expectedValue: 10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				WorkerIDBitLength: tt.bitLength,
				Business:          "test-business",
				DBName:            "test-db",
			}
			cfg.WithDB(db)

			w, err := NewWorker(cfg)
			require.NoError(t, err)
			assert.Equal(t, tt.expectedValue, w.WorkerIDBitLength())
		})
	}
}

func TestWorkerIDAllocator_ReleaseWorkerID(t *testing.T) {
	db := setupTestDB(t)

	cfg := &Config{
		WorkerIDBitLength: 6,
		Business:          "test-business",
		DBName:            "test-db",
	}
	cfg.WithDB(db)

	w, err := NewWorker(cfg)
	require.NoError(t, err)

	info, err := w.GetWorkerInfo()
	require.NoError(t, err)
	workerID := info.WorkerID

	// Release worker
	err = w.ReleaseWorkerID()
	assert.NoError(t, err)

	// Verify database status changed to unused
	var worker SnowflakeWorker
	err = db.Where("worker_id = ? AND business = ?", workerID, "test-business").First(&worker).Error
	require.NoError(t, err)
	assert.Equal(t, statusUnused, worker.Status)
}

func TestWorkerIDAllocator_ReleaseWorkerID_NotAllocated(t *testing.T) {
	db := setupTestDB(t)

	cfg := &Config{
		WorkerIDBitLength: 6,
		Business:          "test-business",
		DBName:            "test-db",
	}
	cfg.WithDB(db)

	w, err := NewWorker(cfg)
	require.NoError(t, err)

	// Release without getting worker info first
	err = w.ReleaseWorkerID()
	assert.NoError(t, err, "Release should succeed even without allocated worker")
}

func TestWorkerIDAllocator_UpdateOverLastTime(t *testing.T) {
	db := setupTestDB(t)

	cfg := &Config{
		WorkerIDBitLength: 6,
		Business:          "test-business",
		DBName:            "test-db",
	}
	cfg.WithDB(db)

	w, err := NewWorker(cfg)
	require.NoError(t, err)

	info, err := w.GetWorkerInfo()
	require.NoError(t, err)
	workerID := info.WorkerID

	// Update over last time
	newOverLastTime := int64(12345)
	err = w.UpdateOverLastTime(newOverLastTime)
	assert.NoError(t, err)

	// Verify database update
	var worker SnowflakeWorker
	err = db.Where("worker_id = ? AND business = ?", workerID, "test-business").First(&worker).Error
	require.NoError(t, err)
	assert.Equal(t, newOverLastTime, worker.OverLastTime)

	// Verify in-memory update
	info, err = w.GetWorkerInfo()
	require.NoError(t, err)
	assert.Equal(t, newOverLastTime, info.OverLastTime)
}

func TestWorkerIDAllocator_UpdateBackLastTime(t *testing.T) {
	db := setupTestDB(t)

	cfg := &Config{
		WorkerIDBitLength: 6,
		Business:          "test-business",
		DBName:            "test-db",
	}
	cfg.WithDB(db)

	w, err := NewWorker(cfg)
	require.NoError(t, err)

	info, err := w.GetWorkerInfo()
	require.NoError(t, err)
	workerID := info.WorkerID

	// Update back last time
	newBackLastTime := int64(67890)
	err = w.UpdateBackLastTime(newBackLastTime)
	assert.NoError(t, err)

	// Verify database update
	var worker SnowflakeWorker
	err = db.Where("worker_id = ? AND business = ?", workerID, "test-business").First(&worker).Error
	require.NoError(t, err)
	assert.Equal(t, newBackLastTime, worker.BackLastTime)

	// Verify in-memory update
	info, err = w.GetWorkerInfo()
	require.NoError(t, err)
	assert.Equal(t, newBackLastTime, info.BackLastTime)
}

func TestWorkerIDAllocator_UpdateTime_WithMismatchedFlag(t *testing.T) {
	db := setupTestDB(t)

	cfg := &Config{
		WorkerIDBitLength: 6,
		Business:          "test-business-flag",
		DBName:            "test-db",
	}
	cfg.WithDB(db)

	w, err := NewWorker(cfg)
	require.NoError(t, err)

	_, err = w.GetWorkerInfo()
	require.NoError(t, err)

	allocator, ok := w.(*WorkerIDAllocator)
	require.True(t, ok)
	allocator.flag = "mismatched-flag"

	err = w.UpdateOverLastTime(100)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "worker id not exist")

	err = w.UpdateBackLastTime(200)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "worker id not exist")
}

func TestWorkerIDAllocator_UpdateTime_NotAllocated(t *testing.T) {
	db := setupTestDB(t)

	cfg := &Config{
		WorkerIDBitLength: 6,
		Business:          "test-business",
		DBName:            "test-db",
	}
	cfg.WithDB(db)

	w, err := NewWorker(cfg)
	require.NoError(t, err)

	// Try to update without getting worker info first
	err = w.UpdateOverLastTime(12345)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "worker id not exist")

	err = w.UpdateBackLastTime(67890)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "worker id not exist")
}

func TestWorkerIDAllocator_MultipleBusinesses(t *testing.T) {
	db := setupTestDB(t)

	// Create workers for different businesses
	businesses := []string{"business-a", "business-b", "business-c"}
	workers := make([]worker.Worker, len(businesses))

	for i, business := range businesses {
		cfg := &Config{
			WorkerIDBitLength: 6,
			Business:          business,
			DBName:            "test-db",
		}
		cfg.WithDB(db)

		w, err := NewWorker(cfg)
		require.NoError(t, err)
		workers[i] = w

		info, err := w.GetWorkerInfo()
		require.NoError(t, err)
		assert.Equal(t, int64(1), info.WorkerID, "Each business should start from worker ID 1")
	}

	// Verify workers are isolated by business
	for i, w := range workers {
		info, err := w.GetWorkerInfo()
		require.NoError(t, err)
		assert.Equal(t, int64(1), info.WorkerID)

		// Verify in database
		var worker SnowflakeWorker
		err = db.Where("worker_id = ? AND business = ?", info.WorkerID, businesses[i]).
			First(&worker).
			Error
		require.NoError(t, err)
		assert.Equal(t, businesses[i], worker.Business)
	}
}

func TestWorkerIDAllocator_ConcurrentAllocation(t *testing.T) {
	db := setupTestDB(t)

	business := "test-business-concurrent"
	const numGoroutines = 10

	type allocationResult struct {
		worker worker.Worker
		id     int64
		err    error
	}

	results := make(chan allocationResult, numGoroutines)
	var wg sync.WaitGroup
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			cfg := &Config{
				WorkerIDBitLength: 10, // Larger to accommodate more workers
				Business:          business,
				DBName:            "test-db",
			}
			cfg.WithDB(db)

			w, err := NewWorker(cfg)
			if err != nil {
				results <- allocationResult{err: err}
				return
			}

			info, err := w.GetWorkerInfo()
			if err != nil {
				results <- allocationResult{err: err}
				return
			}

			results <- allocationResult{
				worker: w,
				id:     info.WorkerID,
			}
		}()
	}
	wg.Wait()
	close(results)

	workers := make([]worker.Worker, 0, numGoroutines)
	uniqueIDs := make(map[int64]struct{})
	for result := range results {
		require.NoError(t, result.err)
		workers = append(workers, result.worker)
		uniqueIDs[result.id] = struct{}{}
	}

	assert.Equal(t, numGoroutines, len(workers), "all goroutines should allocate one worker")
	assert.Equal(t, numGoroutines, len(uniqueIDs), "concurrent allocations should be unique")

	for _, allocated := range workers {
		require.NoError(t, allocated.ReleaseWorkerID())
	}
}
