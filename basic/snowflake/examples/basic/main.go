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

	"github.com/codesjoy/pkg/basic/snowflake"
	"github.com/codesjoy/pkg/basic/snowflake/worker/static"
)

func main() {
	// Example 1: Basic usage with default configuration
	fmt.Println("=== Example 1: Basic Usage ===")
	basicExample()

	// Example 2: Custom configuration
	fmt.Println("\n=== Example 2: Custom Configuration ===")
	customConfigExample()

	// Example 3: Using different base times
	fmt.Println("\n=== Example 3: Custom Base Time ===")
	customBaseTimeExample()

	// Example 4: Builder pattern
	fmt.Println("\n=== Example 4: Builder Pattern ===")
	builderPatternExample()
}

func basicExample() {
	// Create a static worker with default settings
	staticWorker, err := static.NewWorker(&static.Config{
		WorkerID:          1,
		WorkerIDBitLength: 6,
	})
	if err != nil {
		panic(err)
	}

	// Create Snowflake generator with default configuration
	cfg := snowflake.NewConfig().WithWorker(staticWorker)

	sf, err := snowflake.NewSnowflake(cfg)
	if err != nil {
		panic(err)
	}

	// Generate 10 IDs
	for i := 0; i < 10; i++ {
		id := sf.FetchID()
		fmt.Printf("Generated ID %d: %d\n", i+1, id)
	}

	// Release worker when done
	if err := sf.ReleaseWorkerID(); err != nil {
		fmt.Printf("Error releasing worker: %v\n", err)
	}
}

func customConfigExample() {
	// Create a static worker
	staticWorker, err := static.NewWorker(&static.Config{
		WorkerID:          1,
		WorkerIDBitLength: 10, // Larger worker ID bit length
	})
	if err != nil {
		panic(err)
	}

	// Create Snowflake generator with custom configuration
	cfg := &snowflake.Config{
		BaseTime:         snowflake.BaseTime2020(),
		SeqBitLength:     12, // 12 bits for sequence (max 4095)
		MinSeqNumber:     10, // Start sequence from 10
		TopOverCostCount: 5000,
	}
	cfg.WithWorker(staticWorker)

	sf, err := snowflake.NewSnowflake(cfg)
	if err != nil {
		panic(err)
	}

	// Generate IDs
	for i := 0; i < 5; i++ {
		id := sf.FetchID()
		fmt.Printf("Generated ID: %d (Worker ID: %d)\n", id, sf.WorkerID())
	}
}

func customBaseTimeExample() {
	// Create a static worker
	staticWorker, err := static.NewWorker(&static.Config{
		WorkerID:          1,
		WorkerIDBitLength: 6,
	})
	if err != nil {
		panic(err)
	}

	// Use different base time helpers
	baseTimes := []struct {
		name string
		time int64
	}{
		{"Base 2020", snowflake.BaseTime2020()},
		{"Base 2024", snowflake.BaseTime2024()},
	}

	for _, bt := range baseTimes {
		cfg := snowflake.NewConfig().WithWorker(staticWorker).WithBaseTime(bt.time)

		sf, err := snowflake.NewSnowflake(cfg)
		if err != nil {
			panic(err)
		}

		id := sf.FetchID()
		fmt.Printf("%s: Generated ID: %d\n", bt.name, id)
	}
}

func builderPatternExample() {
	// Create a static worker
	staticWorker, err := static.NewWorker(&static.Config{
		WorkerID:          1,
		WorkerIDBitLength: 6,
	})
	if err != nil {
		panic(err)
	}

	// Use fluent builder pattern for configuration
	cfg := snowflake.NewConfig().
		WithWorker(staticWorker).
		WithBaseTime(snowflake.BaseTime2024()).
		WithSeqBitLength(12).
		WithMinSeqNumber(20).
		WithTopOverCostCount(3000)

	sf, err := snowflake.NewSnowflake(cfg)
	if err != nil {
		panic(err)
	}

	// Generate IDs
	for i := 0; i < 5; i++ {
		id := sf.FetchID()
		fmt.Printf("Generated ID: %d\n", id)
	}
}
