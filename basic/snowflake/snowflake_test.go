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

package snowflake

import (
	"sync"
	"testing"
	"time"

	"github.com/codesjoy/pkg/basic/snowflake/worker"
	"github.com/codesjoy/pkg/basic/snowflake/worker/static"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockWorker is a mock implementation for testing
type mockWorker struct {
	workerID          int64
	workerIDBitLength byte
	overLastTime      int64
	backLastTime      int64
	updateOverCalled  bool
	updateBackCalled  bool
	updateOverCount   int
	updateBackCount   int
	updateOverErr     error
	updateBackErr     error
}

func (m *mockWorker) GetWorkerInfo() (*worker.Info, error) {
	return &worker.Info{
		WorkerID:     m.workerID,
		OverLastTime: m.overLastTime,
		BackLastTime: m.backLastTime,
	}, nil
}

func (m *mockWorker) WorkerIDBitLength() byte {
	return m.workerIDBitLength
}

func (m *mockWorker) ReleaseWorkerID() error {
	return nil
}

func (m *mockWorker) UpdateOverLastTime(t int64) error {
	m.updateOverCount++
	m.overLastTime = t
	m.updateOverCalled = true
	if m.updateOverErr != nil {
		return m.updateOverErr
	}
	return nil
}

func (m *mockWorker) UpdateBackLastTime(t int64) error {
	m.updateBackCount++
	m.backLastTime = t
	m.updateBackCalled = true
	if m.updateBackErr != nil {
		return m.updateBackErr
	}
	return nil
}

func TestNewSnowflake(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *Config
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid config with static worker",
			cfg: &Config{
				BaseTime:         DefaultBaseTime,
				SeqBitLength:     12,
				MinSeqNumber:     DefaultMinSeqNumber,
				TopOverCostCount: DefaultTopOverCostCount,
			},
			wantErr: false,
		},
		{
			name: "min seq number too small",
			cfg: &Config{
				BaseTime:     DefaultBaseTime,
				SeqBitLength: 12,
				MinSeqNumber: 3, // Less than DefaultMinSeqNumber
			},
			wantErr: true,
			errMsg:  "min seq number must be greater than or equal to",
		},
		{
			name: "min seq number greater than max",
			cfg: &Config{
				BaseTime:     DefaultBaseTime,
				SeqBitLength: 12,
				MaxSeqNumber: 10,
				MinSeqNumber: 20,
			},
			wantErr: true,
			errMsg:  "must be less than max seq number",
		},
		{
			name: "worker not set",
			cfg: &Config{
				BaseTime:     DefaultBaseTime,
				SeqBitLength: 12,
				MinSeqNumber: 20, // Set valid min seq to pass initial validation
			},
			wantErr: true,
			errMsg:  "worker not set",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !tt.wantErr && tt.cfg.MinSeqNumber >= DefaultMinSeqNumber {
				staticWorker, err := static.NewWorker(&static.Config{
					WorkerID:          1,
					WorkerIDBitLength: 6,
				})
				require.NoError(t, err)
				tt.cfg.WithWorker(staticWorker)
			}

			got, err := NewSnowflake(tt.cfg)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				assert.Nil(t, got)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, got)
				assert.Equal(t, int64(1), got.WorkerID())
			}
		})
	}
}

func TestNewSnowflake_NilConfig(t *testing.T) {
	sf, err := NewSnowflake(nil)
	require.Error(t, err)
	assert.Nil(t, sf)
	assert.Contains(t, err.Error(), "config not set")
}

func TestSnowflake_FetchID(t *testing.T) {
	staticWorker, err := static.NewWorker(&static.Config{
		WorkerID:          1,
		WorkerIDBitLength: 6,
	})
	require.NoError(t, err)

	cfg := &Config{
		BaseTime:         DefaultBaseTime,
		SeqBitLength:     12,
		MinSeqNumber:     DefaultMinSeqNumber,
		TopOverCostCount: DefaultTopOverCostCount,
	}
	cfg.WithWorker(staticWorker)

	sf, err := NewSnowflake(cfg)
	require.NoError(t, err)

	// Test sequential ID generation
	ids := make([]int64, 100)
	for i := 0; i < 100; i++ {
		ids[i] = sf.FetchID()
		assert.Greater(t, ids[i], int64(0), "ID should be positive")
		if i > 0 {
			assert.Greater(t, ids[i], ids[i-1], "IDs should be monotonically increasing")
		}
	}

	// Verify uniqueness
	uniqueIDs := make(map[int64]bool)
	for _, id := range ids {
		uniqueIDs[id] = true
	}
	assert.Equal(t, len(ids), len(uniqueIDs), "All IDs should be unique")
}

func TestNewSnowflake_UsesDefaultsForZeroValueConfig(t *testing.T) {
	staticWorker, err := static.NewWorker(&static.Config{
		WorkerID:          1,
		WorkerIDBitLength: 6,
	})
	require.NoError(t, err)

	cfg := &Config{}
	cfg.WithWorker(staticWorker)

	sf, err := NewSnowflake(cfg)
	require.NoError(t, err)
	require.NotNil(t, sf)

	assert.Equal(t, int64(DefaultBaseTime), sf.baseTime)
	assert.Equal(t, byte(DefaultSeqBitLength), sf.seqBitLength)
	assert.Equal(t, int64(DefaultMinSeqNumber), sf.minSeqNumber)
	assert.Equal(t, DefaultTopOverCostCount, sf.topOverCostCount)
	assert.Equal(t, int64((1<<DefaultSeqBitLength)-1), sf.maxSeqNumber)
}

func TestSnowflake_FetchID_Concurrent(t *testing.T) {
	staticWorker, err := static.NewWorker(&static.Config{
		WorkerID:          1,
		WorkerIDBitLength: 6,
	})
	require.NoError(t, err)

	cfg := &Config{
		BaseTime:         DefaultBaseTime,
		SeqBitLength:     12,
		MinSeqNumber:     DefaultMinSeqNumber,
		TopOverCostCount: DefaultTopOverCostCount,
	}
	cfg.WithWorker(staticWorker)

	sf, err := NewSnowflake(cfg)
	require.NoError(t, err)

	// Test concurrent ID generation
	const numGoroutines = 100
	const idsPerGoroutine = 100

	var wg sync.WaitGroup
	ids := make(chan int64, numGoroutines*idsPerGoroutine)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < idsPerGoroutine; j++ {
				ids <- sf.FetchID()
			}
		}()
	}

	wg.Wait()
	close(ids)

	// Verify uniqueness
	uniqueIDs := make(map[int64]bool)
	count := 0
	for id := range ids {
		uniqueIDs[id] = true
		count++
	}

	assert.Equal(t, numGoroutines*idsPerGoroutine, count, "Should generate expected number of IDs")
	assert.Equal(t, count, len(uniqueIDs), "All IDs should be unique")
}

func TestSnowflake_SequenceOverflow(t *testing.T) {
	mockW := &mockWorker{
		workerID:          1,
		workerIDBitLength: 6,
	}

	cfg := &Config{
		BaseTime:         time.Now().UnixMilli() - 1000, // Recent time
		SeqBitLength:     4,                             // Small sequence for testing overflow
		MinSeqNumber:     DefaultMinSeqNumber,
		MaxSeqNumber:     (1 << 4) - 1, // Max 15
		TopOverCostCount: DefaultTopOverCostCount,
	}
	cfg.WithWorker(mockW)

	sf, err := NewSnowflake(cfg)
	require.NoError(t, err)

	// Generate IDs rapidly to trigger sequence overflow
	ids := make([]int64, 1000)
	for i := 0; i < 1000; i++ {
		ids[i] = sf.FetchID()
	}

	// Verify all IDs are unique
	uniqueIDs := make(map[int64]bool)
	for _, id := range ids {
		uniqueIDs[id] = true
	}
	assert.Equal(t, len(ids), len(uniqueIDs), "All IDs should be unique even during overflow")

	// Verify over-cost handling was triggered
	// The mock worker should have UpdateOverLastTime called during overflow
	assert.True(t, mockW.updateOverCalled, "UpdateOverLastTime should be called during overflow")
}

func TestSnowflake_SequenceOverflow_UnsupportedPersistenceFallback(t *testing.T) {
	mockW := &mockWorker{
		workerID:          1,
		workerIDBitLength: 6,
		updateOverErr:     worker.ErrUpdateOverLastTimeUnsupported,
	}

	cfg := &Config{
		BaseTime:         time.Now().UnixMilli() - 1000,
		SeqBitLength:     4, // small sequence for frequent overflow
		MinSeqNumber:     DefaultMinSeqNumber,
		MaxSeqNumber:     (1 << 4) - 1, // max 15
		TopOverCostCount: DefaultTopOverCostCount,
	}
	cfg.WithWorker(mockW)

	sf, err := NewSnowflake(cfg)
	require.NoError(t, err)

	ids := make([]int64, 1000)
	for i := 0; i < len(ids); i++ {
		ids[i] = sf.FetchID()
	}

	uniqueIDs := make(map[int64]bool, len(ids))
	for _, id := range ids {
		uniqueIDs[id] = true
	}

	assert.Equal(t, len(ids), len(uniqueIDs), "fallback path should still produce unique IDs")
	assert.Equal(t, 1, mockW.updateOverCount, "unsupported worker should be detected once")
	assert.True(t, sf.overCostNoPersist, "snowflake should switch to local over-cost mode")
}

func TestSnowflake_WorkerID(t *testing.T) {
	tests := []struct {
		workerID int64
	}{
		{workerID: 1},
		{workerID: 100},
		{workerID: 1000},
	}

	for _, tt := range tests {
		t.Run("worker_id", func(t *testing.T) {
			staticWorker, err := static.NewWorker(&static.Config{
				WorkerID:          tt.workerID,
				WorkerIDBitLength: 10,
			})
			require.NoError(t, err)

			cfg := &Config{
				BaseTime:         DefaultBaseTime,
				SeqBitLength:     12,
				MinSeqNumber:     DefaultMinSeqNumber,
				TopOverCostCount: DefaultTopOverCostCount,
			}
			cfg.WithWorker(staticWorker)

			sf, err := NewSnowflake(cfg)
			require.NoError(t, err)
			assert.Equal(t, tt.workerID, sf.WorkerID())
		})
	}
}

func TestSnowflake_ReleaseWorkerID(t *testing.T) {
	staticWorker, err := static.NewWorker(&static.Config{
		WorkerID:          1,
		WorkerIDBitLength: 6,
	})
	require.NoError(t, err)

	cfg := &Config{
		BaseTime:         DefaultBaseTime,
		SeqBitLength:     12,
		MinSeqNumber:     DefaultMinSeqNumber,
		TopOverCostCount: DefaultTopOverCostCount,
	}
	cfg.WithWorker(staticWorker)

	sf, err := NewSnowflake(cfg)
	require.NoError(t, err)

	err = sf.ReleaseWorkerID()
	assert.NoError(t, err)
}

func TestSnowflake_ConfigBuilder(t *testing.T) {
	staticWorker, err := static.NewWorker(&static.Config{
		WorkerID:          1,
		WorkerIDBitLength: 6,
	})
	require.NoError(t, err)

	cfg := &Config{}
	cfg.WithWorker(staticWorker).
		WithBaseTime(BaseTime2020()).
		WithSeqBitLength(12).
		WithMinSeqNumber(10).
		WithTopOverCostCount(5000)

	assert.Equal(t, staticWorker, cfg.worker)
	assert.Equal(t, BaseTime2020(), cfg.BaseTime)
	assert.Equal(t, byte(12), cfg.SeqBitLength)
	assert.Equal(t, int64(10), cfg.MinSeqNumber)
	assert.Equal(t, 5000, cfg.TopOverCostCount)
}

func TestNewConfig(t *testing.T) {
	cfg := NewConfig()
	require.NotNil(t, cfg)

	assert.Equal(t, int64(DefaultBaseTime), cfg.BaseTime)
	assert.Equal(t, byte(DefaultSeqBitLength), cfg.SeqBitLength)
	assert.Equal(t, int64(DefaultMinSeqNumber), cfg.MinSeqNumber)
	assert.Equal(t, DefaultTopOverCostCount, cfg.TopOverCostCount)
	assert.Equal(t, int64(0), cfg.MaxSeqNumber)
}

func TestBaseTimeHelpers(t *testing.T) {
	t.Run("BaseTime2020", func(t *testing.T) {
		bt := BaseTime2020()
		assert.Equal(t, int64(DefaultBaseTime), bt)
	})

	t.Run("BaseTime2024", func(t *testing.T) {
		bt := BaseTime2024()
		assert.Equal(t, int64(1704067200000), bt)
	})

	t.Run("BaseTimeCustom", func(t *testing.T) {
		customTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
		bt := BaseTimeCustom(customTime)
		assert.Equal(t, customTime.UnixMilli(), bt)
	})
}

func TestSnowflake_IDUniqueness_MultipleInstances(t *testing.T) {
	// Create multiple Snowflake instances with different worker IDs
	instances := make([]*Snowflake, 5)
	for i := 0; i < 5; i++ {
		staticWorker, err := static.NewWorker(&static.Config{
			WorkerID:          int64(i + 1),
			WorkerIDBitLength: 6,
		})
		require.NoError(t, err)

		cfg := &Config{
			BaseTime:         DefaultBaseTime,
			SeqBitLength:     12,
			MinSeqNumber:     DefaultMinSeqNumber,
			TopOverCostCount: DefaultTopOverCostCount,
		}
		cfg.WithWorker(staticWorker)

		sf, err := NewSnowflake(cfg)
		require.NoError(t, err)
		instances[i] = sf
	}

	// Generate IDs from all instances concurrently
	const idsPerInstance = 100
	var wg sync.WaitGroup
	ids := make(chan int64, len(instances)*idsPerInstance)

	for _, sf := range instances {
		wg.Add(1)
		go func(s *Snowflake) {
			defer wg.Done()
			for i := 0; i < idsPerInstance; i++ {
				ids <- s.FetchID()
			}
		}(sf)
	}

	wg.Wait()
	close(ids)

	// Verify uniqueness across all instances
	uniqueIDs := make(map[int64]bool)
	count := 0
	for id := range ids {
		uniqueIDs[id] = true
		count++
	}

	assert.Equal(t, len(instances)*idsPerInstance, count)
	assert.Equal(t, count, len(uniqueIDs), "All IDs from different workers should be unique")
}

func TestConfig_AutoCalculateMaxSeqNumber(t *testing.T) {
	tests := []struct {
		seqBitLength byte
		wantMaxSeq   int64
	}{
		{seqBitLength: 8, wantMaxSeq: (1 << 8) - 1},
		{seqBitLength: 12, wantMaxSeq: (1 << 12) - 1},
		{seqBitLength: 4, wantMaxSeq: (1 << 4) - 1},
	}

	for _, tt := range tests {
		t.Run("auto_calc_max_seq", func(t *testing.T) {
			staticWorker, err := static.NewWorker(&static.Config{
				WorkerID:          1,
				WorkerIDBitLength: 6,
			})
			require.NoError(t, err)

			cfg := &Config{
				BaseTime:         DefaultBaseTime,
				SeqBitLength:     tt.seqBitLength,
				MinSeqNumber:     DefaultMinSeqNumber,
				TopOverCostCount: DefaultTopOverCostCount,
				MaxSeqNumber:     0, // Auto-calculate
			}
			cfg.WithWorker(staticWorker)

			sf, err := NewSnowflake(cfg)
			require.NoError(t, err)
			assert.Equal(t, tt.wantMaxSeq, sf.maxSeqNumber)
		})
	}
}
