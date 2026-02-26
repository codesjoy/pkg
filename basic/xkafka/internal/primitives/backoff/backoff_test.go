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
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestExponential(t *testing.T) {
	t.Parallel()

	initial := time.Millisecond
	max := 10 * time.Millisecond

	require.Equal(t, initial, Exponential(initial, max, 2, 1))
	require.Equal(t, 2*time.Millisecond, Exponential(initial, max, 2, 2))
	require.Equal(t, 4*time.Millisecond, Exponential(initial, max, 2, 3))
	require.Equal(t, 8*time.Millisecond, Exponential(initial, max, 2, 4))
	require.Equal(t, max, Exponential(initial, max, 2, 5))
}

func TestWaitCancel(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	err := Wait(ctx, 50*time.Millisecond)
	require.ErrorIs(t, err, context.DeadlineExceeded)
}
