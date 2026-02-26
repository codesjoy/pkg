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
func Exponential(initial, max time.Duration, multiplier float64, attempt int) time.Duration {
	if attempt <= 1 {
		return initial
	}

	delay := float64(initial)
	for i := 1; i < attempt; i++ {
		delay *= multiplier
		if delay >= float64(max) {
			return max
		}
	}
	if delay >= float64(math.MaxInt64) {
		return max
	}

	out := time.Duration(delay)
	if out > max {
		return max
	}
	return out
}

// Wait sleeps for backoff duration or returns on context cancellation.
func Wait(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer func() {
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
