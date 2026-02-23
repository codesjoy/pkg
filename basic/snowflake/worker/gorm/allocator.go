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

// Package gorm implements the worker  allocator
package gorm

import (
	"errors"
	"fmt"
	"sync"

	"github.com/codesjoy/pkg/basic/snowflake/worker"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Config the worker allocator config
type Config struct {
	WorkerIDBitLength int8   `default:"6"`
	Business          string `default:"default"`
	DBName            string `default:"default"`
	db                *gorm.DB
}

// WithDB set config db field
func (cfg *Config) WithDB(db *gorm.DB) {
	cfg.db = db
}

func (cfg *Config) check() error {
	if cfg.db == nil {
		return errors.New("db not set")
	}
	return nil
}

// WorkerIDAllocator gorm worker id allocator
type WorkerIDAllocator struct {
	sync.RWMutex
	info              *worker.Info
	business          string
	workerIDBitLength byte
	flag              string
	data              *snowflakeWorkerData
}

// NewWorker new worker
func NewWorker(cfg *Config) (worker.Worker, error) {
	if err := cfg.check(); err != nil {
		return nil, err
	}
	w := &WorkerIDAllocator{
		workerIDBitLength: byte(cfg.WorkerIDBitLength),
		flag:              uuid.Must(uuid.NewUUID()).String(),
		business:          cfg.Business,
		data: &snowflakeWorkerData{
			maxWorkerID: (1 << cfg.WorkerIDBitLength) - 1,
			db:          cfg.db,
		},
	}
	return w, nil
}

// GetWorkerInfo get worker info
func (w *WorkerIDAllocator) GetWorkerInfo() (*worker.Info, error) {
	// Fast path: read lock for cached info
	w.RLock()
	if w.info != nil {
		info := w.info
		w.RUnlock()
		return info, nil
	}
	w.RUnlock()

	// Slow path: write lock to allocate new worker
	w.Lock()
	defer w.Unlock()

	// Double-check after acquiring write lock
	if w.info != nil {
		return w.info, nil
	}

	workerInfo, err := w.data.getReleasedWorkerInfo(w.business, w.flag)
	if err != nil {
		return nil, err
	}
	if workerInfo != nil {
		w.info = workerInfo
		return workerInfo, nil
	}
	var i int
	for i < 3 {
		workerInfo, err = w.data.getNewWorker(w.business, w.flag)
		if err == nil {
			w.info = workerInfo
			return workerInfo, nil
		}

		if !errors.Is(err, errWorkerIDExist) {
			return nil, err
		}
		i++
	}
	return nil, fmt.Errorf("failed to allocate worker after 3 retries: %w", err)
}

// WorkerIDBitLength get worker id bit length
func (w *WorkerIDAllocator) WorkerIDBitLength() byte {
	w.RLock()
	defer w.RUnlock()
	return w.workerIDBitLength
}

// ReleaseWorkerID release worker id
func (w *WorkerIDAllocator) ReleaseWorkerID() error {
	w.Lock()
	defer w.Unlock()
	if w.info == nil {
		return nil
	}
	if err := w.data.releaseWorkerID(w.info.WorkerID, w.business, w.flag); err != nil {
		return err
	}
	w.info = nil
	return nil
}

// UpdateOverLastTime update over last time
func (w *WorkerIDAllocator) UpdateOverLastTime(overLastTime int64) error {
	w.Lock()
	defer w.Unlock()
	if w.info == nil {
		return errWorkerIDNotExist
	}
	if err := w.data.updateOverLastTime(w.info.WorkerID, w.business, w.flag, overLastTime); err != nil {
		return err
	}
	w.info.OverLastTime = overLastTime
	return nil
}

// UpdateBackLastTime update back last time
func (w *WorkerIDAllocator) UpdateBackLastTime(backLastTime int64) error {
	w.Lock()
	defer w.Unlock()
	if w.info == nil {
		return errWorkerIDNotExist
	}
	if err := w.data.updateBackLastTime(w.info.WorkerID, w.business, w.flag, backLastTime); err != nil {
		return err
	}
	w.info.BackLastTime = backLastTime
	return nil
}
