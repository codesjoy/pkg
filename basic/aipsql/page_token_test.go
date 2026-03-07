// Copyright 2026 The codesjoy Authors.
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

package aipsql

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOffsetPageTokenCodec(t *testing.T) {
	t.Run("round trips positive offset", func(t *testing.T) {
		encoded := EncodeOffsetPageToken(123)
		assert.Equal(t, "123", encoded)

		decoded, err := DecodeOffsetPageToken(encoded)
		require.NoError(t, err)
		assert.Equal(t, 123, decoded)
	})

	t.Run("negative offset token falls back to zero", func(t *testing.T) {
		decoded, err := DecodeOffsetPageToken("-10")
		require.NoError(t, err)
		assert.Equal(t, 0, decoded)
	})

	t.Run("trims surrounding whitespace", func(t *testing.T) {
		decoded, err := DecodeOffsetPageToken(" 42 ")
		require.NoError(t, err)
		assert.Equal(t, 42, decoded)
	})

	t.Run("rejects invalid token", func(t *testing.T) {
		_, err := DecodeOffsetPageToken("abc")
		require.Error(t, err)
	})
}
