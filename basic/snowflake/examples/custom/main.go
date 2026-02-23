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

package main

import (
	"fmt"
	"time"

	"github.com/codesjoy/pkg/basic/snowflake"
	"github.com/codesjoy/pkg/basic/snowflake/worker"
	"github.com/codesjoy/pkg/basic/snowflake/worker/static"
)

func main() {
	// Example 1: High-scale deployment configuration
	fmt.Println("=== Example 1: High-Scale Deployment ===")
	highScaleExample()

	// Example 2: Custom base time for extended lifespan
	fmt.Println("\n=== Example 2: Custom Base Time ===")
	customBaseTimeExample()

	// Example 3: Performance tuning configuration
	fmt.Println("\n=== Example 3: Performance Tuning ===")
	performanceTuningExample()

	// Example 4: Different bit allocations
	fmt.Println("\n=== Example 4: Bit Allocation Variants ===")
	bitAllocationExample()

	// Example 5: Custom epoch time
	fmt.Println("\n=== Example 5: Custom Epoch Time ===")
	customEpochExample()
}

// highScaleExample demonstrates configuration for high-scale deployment
// with many workers and moderate throughput per worker
func highScaleExample() {
	// Create worker supporting up to 1024 workers (10 bits)
	worker, err := static.NewWorker(&static.Config{
		WorkerID:          1,
		WorkerIDBitLength: 10,
	})
	if err != nil {
		panic(err)
	}

	// Configure for high-scale scenario:
	// - 10 bits for worker ID (supports up to 1024 workers)
	// - 12 bits for sequence (4096 IDs per millisecond)
	// - Higher TopOverCostCount for better handling of bursts
	cfg := snowflake.NewConfig().
		WithWorker(worker).
		WithSeqBitLength(12).      // Maximum sequence capacity
		WithMinSeqNumber(50).      // Start from higher sequence
		WithTopOverCostCount(5000) // Allow more over-cost operations

	sf, err := snowflake.NewSnowflake(cfg)
	if err != nil {
		panic(err)
	}

	fmt.Printf("Configuration:\n")
	fmt.Printf("  Worker ID: %d\n", sf.WorkerID())
	fmt.Printf("  Worker ID bits: 10 (max 1024 workers)\n")
	fmt.Printf("  Sequence bits: 12 (max 4096 seq/ms)\n")
	fmt.Printf("  Max over-cost count: 5000\n\n")

	// Generate sample IDs
	fmt.Println("Generated IDs:")
	for i := 0; i < 5; i++ {
		fmt.Printf("  ID: %d\n", sf.FetchID())
	}
}

// customBaseTimeExample demonstrates using different base times
// to extend ID lifespan or align with specific epochs
func customBaseTimeExample() {
	worker1, err := static.NewWorker(&static.Config{
		WorkerID:          1,
		WorkerIDBitLength: 6,
	})
	if err != nil {
		panic(err)
	}

	worker2, err := static.NewWorker(&static.Config{
		WorkerID:          1,
		WorkerIDBitLength: 6,
	})
	if err != nil {
		panic(err)
	}

	// Compare different base times
	configs := []struct {
		name     string
		baseTime int64
		worker   worker.Worker
	}{
		{
			name:     "Base 2020",
			baseTime: snowflake.BaseTime2020(),
			worker:   worker1,
		},
		{
			name:     "Base 2024",
			baseTime: snowflake.BaseTime2024(),
			worker:   worker2,
		},
	}

	for _, cfg := range configs {
		sfCfg := snowflake.NewConfig().
			WithWorker(cfg.worker).
			WithBaseTime(cfg.baseTime).
			WithSeqBitLength(12).
			WithMinSeqNumber(100).
			WithTopOverCostCount(10000)

		sf, err := snowflake.NewSnowflake(sfCfg)
		if err != nil {
			panic(err)
		}

		// Parse base time for display in UTC to avoid local timezone confusion.
		baseTimeDate := time.UnixMilli(cfg.baseTime).UTC().Format("2006-01-02 15:04:05 MST")
		id := sf.FetchID()

		fmt.Printf(
			"%s (epoch UTC: %s, %s):\n",
			cfg.name,
			baseTimeDate,
			formatTimestamp(cfg.baseTime),
		)
		fmt.Printf("  Generated ID: %d\n", id)
		fmt.Printf(
			"  Current time tick: ~%d ms since epoch\n\n",
			time.Now().UnixMilli()-cfg.baseTime,
		)
	}
}

// performanceTuningExample demonstrates performance tuning options
func performanceTuningExample() {
	worker, err := static.NewWorker(&static.Config{
		WorkerID:          1,
		WorkerIDBitLength: 6,
	})
	if err != nil {
		panic(err)
	}

	// Configuration tuned for high-throughput scenarios:
	// - Lower MinSeqNumber reduces initial padding
	// - Higher TopOverCostCount allows more burst capacity
	cfg := snowflake.NewConfig().
		WithWorker(worker).
		WithSeqBitLength(12).      // Maximum sequence per ms
		WithMinSeqNumber(5).       // Lower starting point (default)
		WithTopOverCostCount(2000) // Default over-cost limit

	sf, err := snowflake.NewSnowflake(cfg)
	if err != nil {
		panic(err)
	}

	fmt.Printf("Performance Configuration:\n")
	fmt.Printf("  Min Sequence: 5 (low latency start)\n")
	fmt.Printf("  Max Sequence: 4095 (12 bits)\n")
	fmt.Printf("  Over-Cost Limit: 2000\n")
	fmt.Printf("  Capacity per millisecond: ~4090 IDs\n\n")

	// Simulate high-throughput generation
	fmt.Println("Generating 100 IDs (simulating burst):")
	start := time.Now()
	for i := 0; i < 100; i++ {
		sf.FetchID()
	}
	elapsed := time.Since(start)

	fmt.Printf("Completed in %v (%.0f IDs/sec)\n", elapsed, float64(100)/elapsed.Seconds())
}

// bitAllocationExample demonstrates different bit allocation strategies
func bitAllocationExample() {
	scenarios := []struct {
		name              string
		workerIDBitLength byte
		seqBitLength      byte
		description       string
		maxWorkers        int
		maxSeqPerMs       int
	}{
		{
			name:              "Many Workers",
			workerIDBitLength: 10,
			seqBitLength:      12,
			description:       "Maximum workers, maximum throughput",
			maxWorkers:        1024,
			maxSeqPerMs:       4096,
		},
		{
			name:              "Balanced",
			workerIDBitLength: 6,
			seqBitLength:      12,
			description:       "Standard deployment",
			maxWorkers:        64,
			maxSeqPerMs:       4096,
		},
		{
			name:              "Few Workers, High Throughput",
			workerIDBitLength: 4,
			seqBitLength:      12,
			description:       "Small cluster, max per-worker throughput",
			maxWorkers:        16,
			maxSeqPerMs:       4096,
		},
		{
			name:              "Minimal",
			workerIDBitLength: 2,
			seqBitLength:      12,
			description:       "Minimal worker count",
			maxWorkers:        4,
			maxSeqPerMs:       4096,
		},
	}

	for _, scenario := range scenarios {
		worker, err := static.NewWorker(&static.Config{
			WorkerID:          1,
			WorkerIDBitLength: scenario.workerIDBitLength,
		})
		if err != nil {
			panic(err)
		}

		cfg := snowflake.NewConfig().
			WithWorker(worker).
			WithSeqBitLength(scenario.seqBitLength)

		sf, err := snowflake.NewSnowflake(cfg)
		if err != nil {
			panic(err)
		}

		id := sf.FetchID()

		fmt.Printf("%s:\n", scenario.name)
		fmt.Printf(
			"  Worker ID bits: %d (max %d workers)\n",
			scenario.workerIDBitLength,
			scenario.maxWorkers,
		)
		fmt.Printf(
			"  Sequence bits: %d (max %d IDs/ms)\n",
			scenario.seqBitLength,
			scenario.maxSeqPerMs,
		)
		fmt.Printf("  Use case: %s\n", scenario.description)
		fmt.Printf("  Sample ID: %d\n\n", id)
	}
}

// customEpochExample demonstrates creating custom epoch times
func customEpochExample() {
	// Create custom epoch times for different scenarios
	customEpochs := []struct {
		name      string
		timestamp int64
		reason    string
	}{
		{
			name: "Project Start",
			timestamp: snowflake.BaseTimeCustom(
				time.Date(2023, time.January, 1, 0, 0, 0, 0, time.UTC),
			),
			reason: "Align with project launch date",
		},
		{
			name: "Unix Decade",
			timestamp: snowflake.BaseTimeCustom(
				time.Date(2020, time.January, 1, 0, 0, 0, 0, time.UTC),
			),
			reason: "Start of decade",
		},
		{
			name: "Recent",
			timestamp: snowflake.BaseTimeCustom(
				time.Date(2024, time.July, 1, 0, 0, 0, 0, time.UTC),
			),
			reason: "Mid-2024 epoch",
		},
	}

	for _, epoch := range customEpochs {
		worker, err := static.NewWorker(&static.Config{
			WorkerID:          1,
			WorkerIDBitLength: 6,
		})
		if err != nil {
			panic(err)
		}

		cfg := snowflake.NewConfig().
			WithWorker(worker).
			WithBaseTime(epoch.timestamp)

		sf, err := snowflake.NewSnowflake(cfg)
		if err != nil {
			panic(err)
		}

		id := sf.FetchID()
		epochDate := time.UnixMilli(epoch.timestamp).Format("2006-01-02")

		fmt.Printf("%s (epoch: %s):\n", epoch.name, epochDate)
		fmt.Printf("  Reason: %s\n", epoch.reason)
		fmt.Printf("  Sample ID: %d\n", id)
		fmt.Printf("  Time since epoch: ~%.0f days\n\n",
			float64(time.Now().UnixMilli()-epoch.timestamp)/(24*3600*1000))
	}
}

// formatTimestamp converts millisecond timestamp to human-readable format
func formatTimestamp(ms int64) string {
	return fmt.Sprintf("%d ms", ms)
}
