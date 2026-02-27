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

// Package static implements the worker  allocator
package static

import (
	"fmt"

	"github.com/codesjoy/pkg/basic/snowflake/worker"
)

// Config  the static worker config
type Config struct {
	WorkerIDBitLength byte  `default:"6"`
	WorkerID          int64 `default:"1"`
}

// Worker static worker struct
type Worker struct {
	info              *worker.Info
	workerIDBitLength byte
}

// NewWorker new static config worker allocator
func NewWorker(cfg *Config) (worker.Worker, error) {
	maxWorkerID := int64((1 << cfg.WorkerIDBitLength) - 1)
	if cfg.WorkerID > maxWorkerID {
		return nil, fmt.Errorf("worker ID %d exceeds maximum value %d for bit length %d",
			cfg.WorkerID, maxWorkerID, cfg.WorkerIDBitLength)
	}
	w := &Worker{
		info: &worker.Info{
			WorkerID: cfg.WorkerID,
		},
		workerIDBitLength: cfg.WorkerIDBitLength,
	}
	return w, nil
}

// GetWorkerInfo get worker info
func (w *Worker) GetWorkerInfo() (*worker.Info, error) {
	return w.info, nil
}

// WorkerIDBitLength get worker id bit length
func (w *Worker) WorkerIDBitLength() byte {
	return w.workerIDBitLength
}

// ReleaseWorkerID release worker id
func (w *Worker) ReleaseWorkerID() error {
	return nil
}

// UpdateOverLastTime update the over last time
// static worker not support over last time
func (w *Worker) UpdateOverLastTime(overLastTime int64) error {
	return w.unsupportedUpdateError(
		worker.ErrUpdateOverLastTimeUnsupported,
		"updating over last time",
		overLastTime,
	)
}

// UpdateBackLastTime update back last time
// static worker not support turn back time
func (w *Worker) UpdateBackLastTime(backLastTime int64) error {
	return w.unsupportedUpdateError(
		worker.ErrUpdateBackLastTimeUnsupported,
		"updating back last time",
		backLastTime,
	)
}

func (w *Worker) unsupportedUpdateError(err error, action string, requested int64) error {
	return fmt.Errorf(
		"%w: static worker (worker ID: %d) does not support %s (requested: %d); "+
			"use GORM worker allocator for time drift handling",
		err,
		w.info.WorkerID,
		action,
		requested,
	)
}
