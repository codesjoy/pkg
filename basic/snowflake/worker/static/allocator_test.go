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

package static

import (
	"errors"
	"testing"

	"github.com/codesjoy/pkg/basic/snowflake/worker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewWorker(t *testing.T) {
	tests := []struct {
		name        string
		cfg         *Config
		wantErr     bool
		errContains string
	}{
		{
			name: "valid worker config",
			cfg: &Config{
				WorkerID:          1,
				WorkerIDBitLength: 6,
			},
			wantErr: false,
		},
		{
			name: "valid worker with max ID",
			cfg: &Config{
				WorkerID:          63, // 2^6 - 1
				WorkerIDBitLength: 6,
			},
			wantErr: false,
		},
		{
			name: "worker ID exceeds maximum",
			cfg: &Config{
				WorkerID:          64, // Exceeds 2^6 - 1
				WorkerIDBitLength: 6,
			},
			wantErr:     true,
			errContains: "exceeds maximum value",
		},
		{
			name: "worker ID zero",
			cfg: &Config{
				WorkerID:          0,
				WorkerIDBitLength: 6,
			},
			wantErr: false,
		},
		{
			name: "different bit length",
			cfg: &Config{
				WorkerID:          255, // 2^8 - 1
				WorkerIDBitLength: 8,
			},
			wantErr: false,
		},
		{
			name: "worker ID exceeds max for different bit length",
			cfg: &Config{
				WorkerID:          256, // Exceeds 2^8 - 1
				WorkerIDBitLength: 8,
			},
			wantErr:     true,
			errContains: "exceeds maximum value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewWorker(tt.cfg)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
				assert.Nil(t, got)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, got)
			}
		})
	}
}

func TestWorker_GetWorkerInfo(t *testing.T) {
	tests := []struct {
		name   string
		cfg    *Config
		wantID int64
	}{
		{
			name: "worker ID 1",
			cfg: &Config{
				WorkerID:          1,
				WorkerIDBitLength: 6,
			},
			wantID: 1,
		},
		{
			name: "worker ID 100",
			cfg: &Config{
				WorkerID:          100,
				WorkerIDBitLength: 10,
			},
			wantID: 100,
		},
		{
			name: "worker ID 0",
			cfg: &Config{
				WorkerID:          0,
				WorkerIDBitLength: 6,
			},
			wantID: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w, err := NewWorker(tt.cfg)
			require.NoError(t, err)

			info, err := w.GetWorkerInfo()
			assert.NoError(t, err)
			assert.NotNil(t, info)
			assert.Equal(t, tt.wantID, info.WorkerID)
			assert.Equal(t, int64(0), info.OverLastTime)
			assert.Equal(t, int64(0), info.BackLastTime)
		})
	}
}

func TestWorker_WorkerIDBitLength(t *testing.T) {
	tests := []struct {
		name          string
		cfg           *Config
		wantBitLength byte
	}{
		{
			name: "6 bit length",
			cfg: &Config{
				WorkerID:          1,
				WorkerIDBitLength: 6,
			},
			wantBitLength: 6,
		},
		{
			name: "10 bit length",
			cfg: &Config{
				WorkerID:          1,
				WorkerIDBitLength: 10,
			},
			wantBitLength: 10,
		},
		{
			name: "1 bit length",
			cfg: &Config{
				WorkerID:          1,
				WorkerIDBitLength: 1,
			},
			wantBitLength: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w, err := NewWorker(tt.cfg)
			require.NoError(t, err)
			assert.Equal(t, tt.wantBitLength, w.WorkerIDBitLength())
		})
	}
}

func TestWorker_ReleaseWorkerID(t *testing.T) {
	w, err := NewWorker(&Config{
		WorkerID:          1,
		WorkerIDBitLength: 6,
	})
	require.NoError(t, err)

	err = w.ReleaseWorkerID()
	assert.NoError(t, err)

	// Release should be idempotent
	err = w.ReleaseWorkerID()
	assert.NoError(t, err)
}

func TestWorker_UpdateOverLastTime(t *testing.T) {
	w, err := NewWorker(&Config{
		WorkerID:          1,
		WorkerIDBitLength: 6,
	})
	require.NoError(t, err)

	err = w.UpdateOverLastTime(12345)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, worker.ErrUpdateOverLastTimeUnsupported))
	assert.Contains(t, err.Error(), "does not support updating over last time")
	assert.Contains(t, err.Error(), "worker ID: 1")
	assert.Contains(t, err.Error(), "requested: 12345")
	assert.Contains(t, err.Error(), "use GORM worker allocator")
}

func TestWorker_UpdateBackLastTime(t *testing.T) {
	w, err := NewWorker(&Config{
		WorkerID:          100,
		WorkerIDBitLength: 8,
	})
	require.NoError(t, err)

	err = w.UpdateBackLastTime(67890)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, worker.ErrUpdateBackLastTimeUnsupported))
	assert.Contains(t, err.Error(), "does not support updating back last time")
	assert.Contains(t, err.Error(), "worker ID: 100")
	assert.Contains(t, err.Error(), "requested: 67890")
	assert.Contains(t, err.Error(), "use GORM worker allocator")
}

func TestWorker_InterfaceCompliance(t *testing.T) {
	var _ worker.Worker = (*Worker)(nil)

	w, err := NewWorker(&Config{
		WorkerID:          1,
		WorkerIDBitLength: 6,
	})
	require.NoError(t, err)

	// Verify all interface methods are implemented
	assert.Implements(t, (*worker.Worker)(nil), w)
}

func TestWorker_MultipleInstances(t *testing.T) {
	// Create multiple worker instances
	workers := make([]worker.Worker, 5)
	for i := 0; i < 5; i++ {
		w, err := NewWorker(&Config{
			WorkerID:          int64(i + 1),
			WorkerIDBitLength: 6,
		})
		require.NoError(t, err)
		workers[i] = w
	}

	// Verify each worker has correct ID
	for i, w := range workers {
		info, err := w.GetWorkerInfo()
		require.NoError(t, err)
		assert.Equal(t, int64(i+1), info.WorkerID)
	}
}

func TestWorker_DefaultConfigValues(t *testing.T) {
	// Test with default values mentioned in struct tags
	w, err := NewWorker(&Config{
		WorkerID:          1,
		WorkerIDBitLength: 6,
	})
	require.NoError(t, err)

	info, err := w.GetWorkerInfo()
	require.NoError(t, err)
	assert.Equal(t, int64(1), info.WorkerID)
	assert.Equal(t, byte(6), w.WorkerIDBitLength())
}
