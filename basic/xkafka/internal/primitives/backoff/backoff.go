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

package backoff

import (
	"context"
	"math"
	"time"
)

// Exponential returns exponential backoff for attempt >= 1.
// 计算指数退避时长：每次尝试乘以 multiplier，直到达到 max 或溢出。
func Exponential(initial, max time.Duration, multiplier float64, attempt int) time.Duration {
	if attempt <= 1 {
		return initial
	}

	// 逐步乘法计算退避时长
	delay := float64(initial)
	for i := 1; i < attempt; i++ {
		delay *= multiplier
		// 提前截断：超过上限直接返回 max
		if delay >= float64(max) {
			return max
		}
	}
	// 溢出保护：超过 int64 范围时返回 max
	if delay >= float64(math.MaxInt64) {
		return max
	}

	out := time.Duration(delay)
	// 最终上限截断
	if out > max {
		return max
	}
	return out
}

// Wait sleeps for backoff duration or returns on context cancellation.
// 等待指定时长，支持 context 取消提前返回。
func Wait(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer func() {
		// 清理未触发的 timer
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
	}()

	select {
	case <-ctx.Done():
		// context 取消，提前返回
		return ctx.Err()
	case <-timer.C:
		// 等待完成
		return nil
	}
}
