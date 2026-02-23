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
	"log"
	"sync"
	"time"

	"github.com/codesjoy/pkg/basic/snowflake"
	xgorm "github.com/codesjoy/pkg/basic/snowflake/worker/gorm"
	_ "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func main() {
	// Example 1: Basic GORM worker usage
	fmt.Println("=== Example 1: Basic GORM Worker ===")
	basicGORMExample()

	// Example 2: Concurrent ID generation
	fmt.Println("\n=== Example 2: Concurrent Generation ===")
	concurrentExample()

	// Example 3: Worker allocation and release
	fmt.Println("\n=== Example 3: Worker Allocation/Release ===")
	allocationExample()
}

func basicGORMExample() {
	// Setup database connection
	// Replace with your actual database connection string
	// Format: user:password@tcp(host:port)/dbname
	db, err := gorm.Open(mysql.Open("root:password@tcp(127.0.0.1:3306)/snowflake"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	// Auto-migrate the worker table
	if err := db.AutoMigrate(&xgorm.SnowflakeWorker{}); err != nil {
		log.Fatalf("Failed to migrate database: %v", err)
	}

	// Create GORM worker allocator configuration
	cfg := &xgorm.Config{
		WorkerIDBitLength: 6, // Support up to 64 workers
		Business:          "example-app",
	}
	cfg.WithDB(db)

	// Create GORM worker allocator
	worker, err := xgorm.NewWorker(cfg)
	if err != nil {
		log.Fatalf("Failed to create worker: %v", err)
	}

	// Create Snowflake generator
	sfCfg := snowflake.NewConfig().WithWorker(worker)
	sf, err := snowflake.NewSnowflake(sfCfg)
	if err != nil {
		log.Fatalf("Failed to create snowflake: %v", err)
	}
	defer func() {
		_ = sf.ReleaseWorkerID()
	}()

	// Generate IDs
	fmt.Printf("Worker ID: %d\n", sf.WorkerID())
	for i := 0; i < 10; i++ {
		id := sf.FetchID()
		fmt.Printf("Generated ID: %d\n", id)
	}
}

func concurrentExample() {
	// Setup database connection
	db, err := gorm.Open(mysql.Open("root:password@tcp(127.0.0.1:3306)/snowflake"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	// Auto-migrate the worker table
	if err := db.AutoMigrate(&xgorm.SnowflakeWorker{}); err != nil {
		log.Fatalf("Failed to migrate database: %v", err)
	}

	// Create multiple workers simulating different instances
	const numWorkers = 3
	const idsPerWorker = 5

	var wg sync.WaitGroup
	ids := make(chan int64, numWorkers*idsPerWorker)

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerNum int) {
			defer wg.Done()

			// Create worker for this goroutine
			cfg := &xgorm.Config{
				WorkerIDBitLength: 6,
				Business:          "concurrent-example",
			}
			cfg.WithDB(db)

			worker, err := xgorm.NewWorker(cfg)
			if err != nil {
				log.Printf("Worker %d: Failed to create worker: %v", workerNum, err)
				return
			}
			defer func() {
				_ = worker.ReleaseWorkerID()
			}()

			// Create Snowflake generator
			sfCfg := snowflake.NewConfig().WithWorker(worker)
			sf, err := snowflake.NewSnowflake(sfCfg)
			if err != nil {
				log.Printf("Worker %d: Failed to create snowflake: %v", workerNum, err)
				return
			}

			// Generate IDs
			for j := 0; j < idsPerWorker; j++ {
				id := sf.FetchID()
				ids <- id
				fmt.Printf("Worker %d (ID: %d): Generated %d\n", workerNum, sf.WorkerID(), id)
				time.Sleep(10 * time.Millisecond) // Simulate some work
			}
		}(i)
	}

	// Close channel when all goroutines complete
	go func() {
		wg.Wait()
		close(ids)
	}()

	// Collect and verify uniqueness
	idMap := make(map[int64]bool)
	count := 0
	for id := range ids {
		if idMap[id] {
			log.Fatalf("Duplicate ID detected: %d", id)
		}
		idMap[id] = true
		count++
	}

	fmt.Printf("\nGenerated %d unique IDs concurrently\n", count)
}

func allocationExample() {
	// Setup database connection
	db, err := gorm.Open(mysql.Open("root:password@tcp(127.0.0.1:3306)/snowflake"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	// Auto-migrate the worker table
	if err := db.AutoMigrate(&xgorm.SnowflakeWorker{}); err != nil {
		log.Fatalf("Failed to migrate database: %v", err)
	}

	// Allocate a worker
	cfg := &xgorm.Config{
		WorkerIDBitLength: 6,
		Business:          "allocation-example",
	}
	cfg.WithDB(db)

	worker, err := xgorm.NewWorker(cfg)
	if err != nil {
		log.Fatalf("Failed to create worker: %v", err)
	}

	// Create Snowflake generator
	sfCfg := snowflake.NewConfig().WithWorker(worker)
	sf, err := snowflake.NewSnowflake(sfCfg)
	if err != nil {
		log.Fatalf("Failed to create snowflake: %v", err)
	}

	workerID := sf.WorkerID()
	fmt.Printf("Allocated Worker ID: %d\n", workerID)

	// Generate some IDs
	fmt.Println("Generating IDs with allocated worker:")
	for i := 0; i < 5; i++ {
		id := sf.FetchID()
		fmt.Printf("  ID: %d\n", id)
	}

	// Release the worker
	fmt.Println("\nReleasing worker...")
	if err := sf.ReleaseWorkerID(); err != nil {
		log.Printf("Failed to release worker: %v", err)
	}

	// Allocate a new worker (should reuse or get new ID)
	fmt.Println("\nAllocating new worker...")
	worker2, err := xgorm.NewWorker(cfg)
	if err != nil {
		log.Fatalf("Failed to create second worker: %v", err)
	}

	sfCfg2 := snowflake.NewConfig().WithWorker(worker2)
	sf2, err := snowflake.NewSnowflake(sfCfg2)
	if err != nil {
		log.Fatalf("Failed to create second snowflake: %v", err)
	}
	defer func() {
		_ = sf2.ReleaseWorkerID()
	}()

	newWorkerID := sf2.WorkerID()
	fmt.Printf("New Worker ID: %d\n", newWorkerID)

	// Generate IDs with new worker
	fmt.Println("Generating IDs with new worker:")
	for i := 0; i < 5; i++ {
		id := sf2.FetchID()
		fmt.Printf("  ID: %d\n", id)
	}
}
