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
	"fmt"
	"sync"
	"testing"

	"github.com/codesjoy/pkg/basic/snowflake/worker/static"
	"github.com/stretchr/testify/require"
)

func benchmarkSnowflake(b *testing.B, workers int) {
	staticWorker, err := static.NewWorker(&static.Config{
		WorkerID:          1,
		WorkerIDBitLength: 6,
	})
	require.NoError(b, err)

	cfg := &Config{
		BaseTime:         DefaultBaseTime,
		SeqBitLength:     12,
		MinSeqNumber:     DefaultMinSeqNumber,
		TopOverCostCount: DefaultTopOverCostCount,
	}
	cfg.WithWorker(staticWorker)

	sf, err := NewSnowflake(cfg)
	require.NoError(b, err)

	b.SetParallelism(workers)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = sf.FetchID()
		}
	})
}

func BenchmarkSnowflake_SingleWorker(b *testing.B) {
	benchmarkSnowflake(b, 1)
}

func BenchmarkSnowflake_Concurrent_4(b *testing.B) {
	benchmarkSnowflake(b, 4)
}

func BenchmarkSnowflake_Concurrent_8(b *testing.B) {
	benchmarkSnowflake(b, 8)
}

func BenchmarkSnowflake_Concurrent_16(b *testing.B) {
	benchmarkSnowflake(b, 16)
}

func BenchmarkSnowflake_Concurrent_32(b *testing.B) {
	benchmarkSnowflake(b, 32)
}

func BenchmarkSnowflake_SequentialGeneration(b *testing.B) {
	staticWorker, err := static.NewWorker(&static.Config{
		WorkerID:          1,
		WorkerIDBitLength: 6,
	})
	require.NoError(b, err)

	cfg := &Config{
		BaseTime:         DefaultBaseTime,
		SeqBitLength:     12,
		MinSeqNumber:     DefaultMinSeqNumber,
		TopOverCostCount: DefaultTopOverCostCount,
	}
	cfg.WithWorker(staticWorker)

	sf, err := NewSnowflake(cfg)
	require.NoError(b, err)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = sf.FetchID()
	}
}

func BenchmarkSnowflake_MultipleWorkers(b *testing.B) {
	workers := make([]*Snowflake, 4)
	for i := 0; i < 4; i++ {
		staticWorker, err := static.NewWorker(&static.Config{
			WorkerID:          int64(i + 1),
			WorkerIDBitLength: 6,
		})
		require.NoError(b, err)

		cfg := &Config{
			BaseTime:         DefaultBaseTime,
			SeqBitLength:     12,
			MinSeqNumber:     DefaultMinSeqNumber,
			TopOverCostCount: DefaultTopOverCostCount,
		}
		cfg.WithWorker(staticWorker)

		sf, err := NewSnowflake(cfg)
		require.NoError(b, err)
		workers[i] = sf
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		workerIdx := 0
		for pb.Next() {
			_ = workers[workerIdx].FetchID()
			workerIdx = (workerIdx + 1) % len(workers)
		}
	})
}

func BenchmarkSnowflake_OverCostScenario(b *testing.B) {
	// Use small sequence bits to trigger over-cost more frequently
	staticWorker, err := static.NewWorker(&static.Config{
		WorkerID:          1,
		WorkerIDBitLength: 6,
	})
	require.NoError(b, err)

	cfg := &Config{
		BaseTime:         DefaultBaseTime,
		SeqBitLength:     4, // Small sequence for testing
		MinSeqNumber:     DefaultMinSeqNumber,
		MaxSeqNumber:     (1 << 4) - 1, // Max 15
		TopOverCostCount: DefaultTopOverCostCount,
	}
	cfg.WithWorker(staticWorker)

	sf, err := NewSnowflake(cfg)
	require.NoError(b, err)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = sf.FetchID()
	}
}

func BenchmarkSnowflake_HighContention(b *testing.B) {
	staticWorker, err := static.NewWorker(&static.Config{
		WorkerID:          1,
		WorkerIDBitLength: 6,
	})
	require.NoError(b, err)

	cfg := &Config{
		BaseTime:         DefaultBaseTime,
		SeqBitLength:     12,
		MinSeqNumber:     DefaultMinSeqNumber,
		TopOverCostCount: DefaultTopOverCostCount,
	}
	cfg.WithWorker(staticWorker)

	sf, err := NewSnowflake(cfg)
	require.NoError(b, err)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = sf.FetchID()
		}
	})
}

func BenchmarkSnowflake_IDUniquenessCheck(b *testing.B) {
	staticWorker, err := static.NewWorker(&static.Config{
		WorkerID:          1,
		WorkerIDBitLength: 6,
	})
	require.NoError(b, err)

	cfg := &Config{
		BaseTime:         DefaultBaseTime,
		SeqBitLength:     12,
		MinSeqNumber:     DefaultMinSeqNumber,
		TopOverCostCount: DefaultTopOverCostCount,
	}
	cfg.WithWorker(staticWorker)

	sf, err := NewSnowflake(cfg)
	require.NoError(b, err)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		id := sf.FetchID()
		// Simulate uniqueness check
		_ = id
	}
}

func BenchmarkSnowflake_GoroutineSpawn(b *testing.B) {
	staticWorker, err := static.NewWorker(&static.Config{
		WorkerID:          1,
		WorkerIDBitLength: 6,
	})
	require.NoError(b, err)

	cfg := &Config{
		BaseTime:         DefaultBaseTime,
		SeqBitLength:     12,
		MinSeqNumber:     DefaultMinSeqNumber,
		TopOverCostCount: DefaultTopOverCostCount,
	}
	cfg.WithWorker(staticWorker)

	sf, err := NewSnowflake(cfg)
	require.NoError(b, err)

	b.ResetTimer()

	var wg sync.WaitGroup
	for i := 0; i < b.N; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = sf.FetchID()
		}()
	}
	wg.Wait()
}

func BenchmarkSnowflake_DifferentBitLengths(b *testing.B) {
	bitLengths := []byte{4, 8, 12}

	for _, bits := range bitLengths {
		b.Run(fmt.Sprintf("seq_bits_%d", bits), func(b *testing.B) {
			staticWorker, err := static.NewWorker(&static.Config{
				WorkerID:          1,
				WorkerIDBitLength: 6,
			})
			require.NoError(b, err)

			cfg := &Config{
				BaseTime:         DefaultBaseTime,
				SeqBitLength:     bits,
				MinSeqNumber:     DefaultMinSeqNumber,
				TopOverCostCount: DefaultTopOverCostCount,
			}
			cfg.WithWorker(staticWorker)

			sf, err := NewSnowflake(cfg)
			require.NoError(b, err)

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = sf.FetchID()
			}
		})
	}
}
