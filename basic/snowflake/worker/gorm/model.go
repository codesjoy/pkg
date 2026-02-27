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
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/codesjoy/pkg/basic/snowflake/worker"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	_ int64 = iota
	statusUnused
	statusUsed
)

var (
	errWorkerIDExist    = errors.New("worker id already existed")
	errWorkerIDNotExist = errors.New("worker id not exist")
)

// SnowflakeWorker define the snowflake worker gorm model
type SnowflakeWorker struct {
	ID           int64  `gorm:"primaryKey"`
	WorkerID     int64  `gorm:"uniqueIndex:idx_workerid_business"`
	Business     string `gorm:"uniqueIndex:idx_workerid_business"`
	Flag         string
	Status       int64
	OverLastTime int64
	BackLastTime int64
}

type snowflakeWorkerData struct {
	maxWorkerID int64
	db          *gorm.DB
}

func (d *snowflakeWorkerData) getReleasedWorkerInfo(business, flag string) (*worker.Info, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	var info *worker.Info
	err := d.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		m := &SnowflakeWorker{}
		err := tx.Where("business = ? and status = ?", business, statusUnused).
			Clauses(clause.Locking{
				Strength: clause.LockingStrengthUpdate,
			}).First(m).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return fmt.Errorf("failed to query released worker: %w", err)
		}
		err = tx.Model(&SnowflakeWorker{}).
			Where("id = ?", m.ID).
			Updates(map[string]interface{}{
				"status": statusUsed,
				"flag":   flag,
			}).Error
		if err != nil {
			return fmt.Errorf("failed to update worker status: %w", err)
		}
		info = &worker.Info{
			WorkerID:     m.WorkerID,
			OverLastTime: m.OverLastTime,
			BackLastTime: m.BackLastTime,
		}
		return nil
	})
	return info, err
}

func (d *snowflakeWorkerData) getNewWorker(business, flag string) (*worker.Info, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	var workerID int64
	err := d.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		m := &SnowflakeWorker{}
		err := tx.Where("business = ?", business).
			Order("worker_id desc").
			First(m).Error
		if err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("failed to query max worker ID: %w", err)
			}
			workerID = 1
		} else {
			workerID = m.WorkerID + 1
			if workerID > d.maxWorkerID {
				return errors.New("no worker id is available")
			}
		}
		return d.createWorker(tx, workerID, business, flag)
	})
	if err != nil {
		return nil, err
	}
	return &worker.Info{
		WorkerID: workerID,
	}, nil
}

func (d *snowflakeWorkerData) createWorker(
	tx *gorm.DB,
	workerID int64,
	business, flag string,
) error {
	err := tx.Create(&SnowflakeWorker{
		WorkerID: workerID,
		Business: business,
		Status:   statusUsed,
		Flag:     flag,
	}).Error
	if err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return errWorkerIDExist
		}
		return fmt.Errorf("failed to create worker: %w", err)
	}
	return nil
}

func (d *snowflakeWorkerData) releaseWorkerID(workerID int64, business, flag string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	result := d.db.WithContext(ctx).Model(&SnowflakeWorker{}).
		Where("worker_id = ? and business = ? and flag = ?", workerID, business, flag).
		Update("status", statusUnused)
	if result.Error != nil {
		return fmt.Errorf("failed to release worker ID %d: %w", workerID, result.Error)
	}
	// Keep release idempotent.
	return nil
}

func (d *snowflakeWorkerData) updateOverLastTime(
	workerID int64,
	business, flag string,
	overLastTime int64,
) error {
	return d.updateTimeColumn(
		workerID,
		business,
		flag,
		"over_last_time",
		overLastTime,
		"failed to update over last time for worker %d: %w",
	)
}

func (d *snowflakeWorkerData) updateBackLastTime(
	workerID int64,
	business, flag string,
	backLastTime int64,
) error {
	return d.updateTimeColumn(
		workerID,
		business,
		flag,
		"back_last_time",
		backLastTime,
		"failed to update back last time for worker %d: %w",
	)
}

func (d *snowflakeWorkerData) updateTimeColumn(
	workerID int64,
	business string,
	flag string,
	column string,
	value int64,
	errFormat string,
) error {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	result := d.db.WithContext(ctx).Model(&SnowflakeWorker{}).
		Where("worker_id = ? and business = ? and flag = ?", workerID, business, flag).
		Update(column, value)
	if result.Error != nil {
		return fmt.Errorf(errFormat, workerID, result.Error)
	}
	if result.RowsAffected == 0 {
		return errWorkerIDNotExist
	}
	return nil
}
