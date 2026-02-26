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

package offset

import "sync"

// Tracker tracks observed/done offsets and advances only contiguous ranges.
type Tracker struct {
	mu sync.Mutex

	initialized bool
	nextOffset  int64
	doneOffsets map[int64]struct{}
}

// NewTracker creates a tracker for one ordered stream.
func NewTracker() *Tracker {
	return &Tracker{doneOffsets: make(map[int64]struct{})}
}

// Observe records one consumed offset.
func (t *Tracker) Observe(offset int64) {
	if t == nil {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.initialized || offset < t.nextOffset {
		t.nextOffset = offset
		t.initialized = true
	}
}

// MarkDone marks one processed offset and returns next commit/save offset when advanced.
func (t *Tracker) MarkDone(offset int64) (int64, bool) {
	if t == nil {
		return 0, false
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.initialized {
		t.initialized = true
		t.nextOffset = offset
	}

	t.doneOffsets[offset] = struct{}{}

	advanced := false
	for {
		if _, ok := t.doneOffsets[t.nextOffset]; !ok {
			break
		}
		delete(t.doneOffsets, t.nextOffset)
		t.nextOffset++
		advanced = true
	}

	if !advanced {
		return 0, false
	}
	return t.nextOffset, true
}
