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

// Package snowflake is a snowflake id generator.
package snowflake

import (
	"errors"
	"fmt"
	"log/slog"
	"runtime"
	"sync"
	"time"

	"github.com/codesjoy/pkg/basic/snowflake/worker"
)

const (
	// DefaultBaseTime is the default epoch base in milliseconds.
	DefaultBaseTime = 1582136402000 // 2020-02-19 18:20:02 UTC
	// DefaultSeqBitLength is the default bit length of the sequence segment.
	DefaultSeqBitLength = 12
	// DefaultMinSeqNumber is the default minimum sequence number.
	DefaultMinSeqNumber = 5
	// DefaultTopOverCostCount is the default threshold before over-cost handling.
	DefaultTopOverCostCount = 2000
	// DefaultDBTimeout is the default timeout for worker allocation operations.
	DefaultDBTimeout = 20 * time.Second

	// MaxTotalBitLength is the maximum total bit length for worker and sequence bits.
	MaxTotalBitLength = 22 // workerID + sequence bits
	// MaxSeqBitLength is the maximum sequence bit length.
	MaxSeqBitLength = 12
	// MinSeqBitLength is the minimum sequence bit length.
	MinSeqBitLength = 1
	// MaxWorkerIDBitLength is the maximum worker ID bit length.
	MaxWorkerIDBitLength = 10
	// MinWorkerIDBitLength is the minimum worker ID bit length.
	MinWorkerIDBitLength = 1

	// DefaultSleepDuration is the default sleep duration while waiting for next tick.
	DefaultSleepDuration = 1 * time.Millisecond
)

// Config the snowflake config
type Config struct {
	BaseTime         int64
	SeqBitLength     byte
	MaxSeqNumber     int64
	MinSeqNumber     int64
	TopOverCostCount int
	WorkerName       string
	worker           worker.Worker
}

// NewConfig creates a config with recommended defaults.
func NewConfig() *Config {
	return &Config{
		BaseTime:         DefaultBaseTime,
		SeqBitLength:     DefaultSeqBitLength,
		MinSeqNumber:     DefaultMinSeqNumber,
		TopOverCostCount: DefaultTopOverCostCount,
	}
}

// WithWorker set worker
func (c *Config) WithWorker(w worker.Worker) *Config {
	c.worker = w
	return c
}

// WithBaseTime sets the base epoch time for the Snowflake ID generator
func (c *Config) WithBaseTime(t int64) *Config {
	c.BaseTime = t
	return c
}

// WithSeqBitLength sets the sequence bit length (1-12)
func (c *Config) WithSeqBitLength(b byte) *Config {
	c.SeqBitLength = b
	return c
}

// WithMinSeqNumber sets the minimum sequence number
func (c *Config) WithMinSeqNumber(n int64) *Config {
	c.MinSeqNumber = n
	return c
}

// WithTopOverCostCount sets the maximum over-cost count in one term
func (c *Config) WithTopOverCostCount(n int) *Config {
	c.TopOverCostCount = n
	return c
}

// BaseTime2020 returns the epoch timestamp for 2020-02-19 18:20:02 UTC
func BaseTime2020() int64 {
	return DefaultBaseTime
}

// BaseTimeCustom returns a custom base time from a time.Time
func BaseTimeCustom(t time.Time) int64 {
	return t.UnixMilli()
}

// BaseTime2024 returns the epoch timestamp for 2024-01-01 00:00:00 UTC
func BaseTime2024() int64 {
	return 1704067200000 // 2024-01-01 00:00:00 UTC
}

func (c *Config) applyDefaults() {
	if c.BaseTime == 0 {
		c.BaseTime = DefaultBaseTime
	}
	if c.SeqBitLength == 0 {
		c.SeqBitLength = DefaultSeqBitLength
	}
	if c.MinSeqNumber == 0 {
		c.MinSeqNumber = DefaultMinSeqNumber
	}
	if c.TopOverCostCount == 0 {
		c.TopOverCostCount = DefaultTopOverCostCount
	}
}

func (c *Config) check() error {
	// Validate sequence bit length
	if c.SeqBitLength < MinSeqBitLength || c.SeqBitLength > MaxSeqBitLength {
		return fmt.Errorf("sequence bit length must be between %d and %d, got %d",
			MinSeqBitLength, MaxSeqBitLength, c.SeqBitLength)
	}

	// Auto-calculate max sequence number if not set
	if c.MaxSeqNumber == 0 {
		c.MaxSeqNumber = (1 << c.SeqBitLength) - 1
	}

	// Validate min sequence number
	if c.MinSeqNumber < DefaultMinSeqNumber {
		return fmt.Errorf("min seq number must be greater than or equal to %d", DefaultMinSeqNumber)
	}

	// Validate min < max
	if c.MinSeqNumber > c.MaxSeqNumber {
		return fmt.Errorf("min seq number (%d) must be less than max seq number (%d)",
			c.MinSeqNumber, c.MaxSeqNumber)
	}

	// Validate worker is set
	if c.worker == nil {
		return errors.New("worker not set")
	}

	// Validate worker ID bit length
	workerBitLength := c.worker.WorkerIDBitLength()
	if workerBitLength < MinWorkerIDBitLength || workerBitLength > MaxWorkerIDBitLength {
		return fmt.Errorf("worker ID bit length must be between %d and %d, got %d",
			MinWorkerIDBitLength, MaxWorkerIDBitLength, workerBitLength)
	}

	// Validate total bit length
	totalBits := workerBitLength + c.SeqBitLength
	if totalBits > MaxTotalBitLength {
		return fmt.Errorf(
			"worker ID bit length (%d) + sequence bit length (%d) = %d exceeds maximum of %d",
			workerBitLength,
			c.SeqBitLength,
			totalBits,
			MaxTotalBitLength,
		)
	}

	// Validate base time is not in future (allow 1 minute tolerance for clock skew)
	if c.BaseTime > 0 {
		now := time.Now().UnixMilli()
		if c.BaseTime > now+60000 {
			return fmt.Errorf("base time (%d) is in the future (current time: %d)", c.BaseTime, now)
		}
	}

	return nil
}

// Snowflake snowflake
type Snowflake struct {
	sync.Mutex
	baseTime          int64
	workerID          int64
	workerIDBitLength byte
	seqBitLength      byte
	maxSeqNumber      int64
	minSeqNumber      int64
	topOverCostCount  int

	lastTimeTick     int64
	currentSeqNumber int64
	timestampShift   byte

	isOverCost             bool
	overCostCountInOneTerm int
	overCostNoPersist      bool
	overCostWarned         bool

	turnBackTimeTick int64
	minBackTimeTick  int64
	turnBackIndex    int64

	worker worker.Worker
}

// NewSnowflake new snowflake by config
func NewSnowflake(cfg *Config) (*Snowflake, error) {
	if cfg == nil {
		return nil, errors.New("config not set")
	}
	cfg.applyDefaults()
	if err := cfg.check(); err != nil {
		return nil, err
	}
	w := cfg.worker
	workerInfo, err := w.GetWorkerInfo()
	if err != nil {
		return nil, err
	}
	slog.Debug("snowflake generate",
		slog.Int64("worker_id", workerInfo.WorkerID))
	snowflake := &Snowflake{
		baseTime:          cfg.BaseTime,
		workerID:          workerInfo.WorkerID,
		workerIDBitLength: w.WorkerIDBitLength(),
		seqBitLength:      cfg.SeqBitLength,
		maxSeqNumber:      cfg.MaxSeqNumber,
		minSeqNumber:      cfg.MinSeqNumber,
		topOverCostCount:  cfg.TopOverCostCount,
		timestampShift:    w.WorkerIDBitLength() + cfg.SeqBitLength,
		currentSeqNumber:  cfg.MinSeqNumber,
		worker:            w,
	}
	if workerInfo.OverLastTime >= snowflake.getCurrentTimeTick() {
		snowflake.lastTimeTick = workerInfo.OverLastTime
		snowflake.getNextTimeTick()
	}
	return snowflake, nil
}

// FetchID fetch next ID
func (w *Snowflake) FetchID() int64 {
	w.Lock()
	defer w.Unlock()
	if w.isOverCost {
		return w.nextOverCostID()
	}
	return w.nextNormalID()
}

// ReleaseWorkerID release worker id
func (w *Snowflake) ReleaseWorkerID() error {
	return w.worker.ReleaseWorkerID()
}

// WorkerID get snowflake worker id
func (w *Snowflake) WorkerID() int64 {
	return w.workerID
}

func (w *Snowflake) nextNormalID() int64 {
	currentTimeTick := w.getCurrentTimeTick()
	if currentTimeTick < w.lastTimeTick {
		if w.turnBackTimeTick < 1 {
			if !w.beginTurnBackAction() {
				return w.costID(w.lastTimeTick)
			}
		}
		return w.calcTurnBackID(w.turnBackTimeTick)
	}

	// 时间追平时
	if w.turnBackTimeTick > 0 {
		w.endTurnBackAction()
	}

	if currentTimeTick > w.lastTimeTick {
		w.lastTimeTick = currentTimeTick
		w.currentSeqNumber = w.minSeqNumber
		return w.costID(w.lastTimeTick)
	}

	if w.currentSeqNumber > w.maxSeqNumber {
		w.beginOverCostAction()
		return w.costID(w.lastTimeTick)
	}

	return w.costID(w.lastTimeTick)
}

func (w *Snowflake) nextOverCostID() int64 {
	currentTimeTick := w.getCurrentTimeTick()
	if currentTimeTick > w.lastTimeTick {
		w.endOverCostAction(currentTimeTick)
		return w.costID(w.lastTimeTick)
	}
	if w.overCostCountInOneTerm >= w.topOverCostCount {
		tick := w.getNextTimeTick()
		w.endOverCostAction(tick)
		return w.costID(w.lastTimeTick)
	}
	if w.currentSeqNumber > w.maxSeqNumber {
		w.beginOverCostAction()
		return w.costID(w.lastTimeTick)
	}
	return w.costID(w.lastTimeTick)
}

func (w *Snowflake) beginOverCostAction() {
	if w.overCostNoPersist {
		w.lastTimeTick++
		w.currentSeqNumber = w.minSeqNumber
		w.isOverCost = true
		w.overCostCountInOneTerm++
		return
	}

	if err := w.worker.UpdateOverLastTime(w.lastTimeTick + 1); err != nil {
		if errors.Is(err, worker.ErrUpdateOverLastTimeUnsupported) {
			w.overCostNoPersist = true
			if !w.overCostWarned {
				slog.Warn(
					"worker does not support over-cost persistence; falling back to local mode",
					slog.Int64("worker_id", w.workerID),
					slog.String("error", err.Error()),
				)
				w.overCostWarned = true
			}
			w.lastTimeTick++
			w.currentSeqNumber = w.minSeqNumber
			w.isOverCost = true
			w.overCostCountInOneTerm++
			return
		}

		slog.Error("fault to update over last time", "error", err)
		w.endOverCostAction(w.getNextTimeTick())
		return
	}
	w.lastTimeTick++
	w.currentSeqNumber = w.minSeqNumber
	w.isOverCost = true
	w.overCostCountInOneTerm++
}

func (w *Snowflake) endOverCostAction(currentTimeTick int64) {
	w.lastTimeTick = currentTimeTick
	w.currentSeqNumber = w.minSeqNumber
	w.isOverCost = false
	w.overCostCountInOneTerm = 0
}

func (w *Snowflake) beginTurnBackAction() bool {
	w.turnBackIndex++
	w.turnBackTimeTick = w.lastTimeTick - 1
	if w.minBackTimeTick >= w.turnBackTimeTick && w.turnBackIndex >= w.minSeqNumber {
		w.lastTimeTick = w.getNextTimeTick()
		w.endTurnBackAction()
		return false
	}
	if w.turnBackIndex == 1 {
		if err := w.worker.UpdateBackLastTime(w.lastTimeTick); err != nil {
			w.lastTimeTick = w.getNextTimeTick()
			w.endTurnBackAction()
			return false
		}
	}
	return true
}

func (w *Snowflake) endTurnBackAction() {
	w.turnBackTimeTick = 0
	w.turnBackIndex = 0
	info, err := w.worker.GetWorkerInfo()
	if err != nil || info == nil {
		return
	}
	w.minBackTimeTick = info.BackLastTime
}

func (w *Snowflake) getNextTimeTick() int64 {
	for {
		tempTimeTicker := w.getCurrentTimeTick()
		if tempTimeTicker > w.lastTimeTick {
			return tempTimeTicker
		}
		if w.lastTimeTick-tempTimeTicker > 1 {
			time.Sleep(DefaultSleepDuration)
			continue
		}
		runtime.Gosched()
	}
}

func (w *Snowflake) getCurrentTimeTick() int64 {
	return time.Now().UnixMilli() - w.baseTime
}

func (w *Snowflake) costID(useTimeTick int64) int64 {
	result := (useTimeTick << w.timestampShift) + (w.workerID << w.seqBitLength) + w.currentSeqNumber
	w.currentSeqNumber++
	return result
}

func (w *Snowflake) calcTurnBackID(useTimeTick int64) int64 {
	result := (useTimeTick << w.timestampShift) + (w.workerID << w.seqBitLength) + w.turnBackIndex
	w.turnBackTimeTick--
	return result
}
