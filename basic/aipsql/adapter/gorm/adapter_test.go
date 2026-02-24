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

package aipsqlgorm

import (
	"database/sql"
	"testing"

	aip "github.com/codesjoy/pkg/basic/aipsql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNamedArgs(t *testing.T) {
	params := []aip.QueryParameter{
		{Name: "p_0", Value: "active"},
		{Name: "p_1", Value: 42},
		{Name: "p_2", Value: nil},
	}

	args := NamedArgs(params)
	require.Len(t, args, 3)

	first, ok := args[0].(sql.NamedArg)
	require.True(t, ok)
	assert.Equal(t, "p_0", first.Name)
	assert.Equal(t, "active", first.Value)

	second, ok := args[1].(sql.NamedArg)
	require.True(t, ok)
	assert.Equal(t, "p_1", second.Name)
	assert.Equal(t, 42, second.Value)

	third, ok := args[2].(sql.NamedArg)
	require.True(t, ok)
	assert.Equal(t, "p_2", third.Name)
	assert.Nil(t, third.Value)
}

func TestNamedArgsEmpty(t *testing.T) {
	assert.Empty(t, NamedArgs(nil))
	assert.Empty(t, NamedArgs([]aip.QueryParameter{}))
}
