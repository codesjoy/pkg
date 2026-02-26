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

package producer

import (
	"context"
	"sync"

	"github.com/codesjoy/pkg/basic/xkafka/middleware/produce"
)

// Future carries one async produce result.
type Future struct {
	done chan struct{}

	once sync.Once
	res  *produce.Result
	err  error
}

// NewFuture creates a pending future.
func NewFuture() *Future {
	return &Future{done: make(chan struct{})}
}

// Resolve closes future with one result.
func (f *Future) Resolve(res *produce.Result, err error) {
	if f == nil {
		return
	}
	f.once.Do(func() {
		f.res = res
		f.err = err
		close(f.done)
	})
}

// Await waits for future completion or context cancellation.
func (f *Future) Await(ctx context.Context) (*produce.Result, error) {
	if f == nil {
		return nil, context.Canceled
	}
	if ctx == nil {
		ctx = context.Background()
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-f.done:
		return f.res, f.err
	}
}

// Done returns closed channel when future resolves.
func (f *Future) Done() <-chan struct{} {
	if f == nil {
		closed := make(chan struct{})
		close(closed)
		return closed
	}
	return f.done
}
