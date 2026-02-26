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

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTrackerOutOfOrder(t *testing.T) {
	t.Parallel()

	tracker := NewTracker()
	tracker.Observe(10)
	tracker.Observe(11)
	tracker.Observe(12)

	next, advanced := tracker.MarkDone(11)
	require.False(t, advanced)
	require.Equal(t, int64(0), next)

	next, advanced = tracker.MarkDone(10)
	require.True(t, advanced)
	require.Equal(t, int64(12), next)

	next, advanced = tracker.MarkDone(12)
	require.True(t, advanced)
	require.Equal(t, int64(13), next)
}

func TestTrackerSingleMessage(t *testing.T) {
	t.Parallel()

	tracker := NewTracker()
	tracker.Observe(5)

	next, advanced := tracker.MarkDone(5)
	require.True(t, advanced)
	require.Equal(t, int64(6), next)
}
