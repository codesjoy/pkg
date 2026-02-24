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

package aipsql

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWhereClauseBackwardCompatibility verifies that the WhereClause method
// maintains backward compatibility by:
// 1. Keeping the same method signature
// 2. Using default behavior (contains mode) when no MatchModes are configured
// 3. Not applying composite index optimization by default
func TestWhereClauseBackwardCompatibility(t *testing.T) {
	t.Run("uses contains mode by default for has operator", func(t *testing.T) {
		// Create a table without MatchModes configured
		table := NewTable().WithColumns(
			NewColumn().
				WithFieldPath("name").
				WithDatabaseName("name").
				Filterable().
				Build(),
		).Build()

		filter, err := ParseFilter("name:\"test\"")
		require.NoError(t, err)

		// Call the old WhereClause method
		sql, params, err := table.WhereClause(filter, "p_")
		require.NoError(t, err)

		// Should use contains mode (LIKE with %value%)
		assert.Equal(t, "(name LIKE @p_0)", sql)
		assert.Equal(t, []QueryParameter{
			{Name: "p_0", Value: "%test%"},
		}, params)
	})

	t.Run("does not apply composite index optimization by default", func(t *testing.T) {
		// Create a table with composite index
		table := NewTable().WithColumns(
			NewColumn().
				WithFieldPath("status").
				WithDatabaseName("status").
				Filterable().
				Build(),
			NewColumn().
				WithFieldPath("user_id").
				WithDatabaseName("user_id").
				Filterable().
				Build(),
		).Build()

		// Manually add composite index to the table
		table.CompositeIndexes = []CompositeIndex{
			{
				Name:    "idx_status_user",
				Columns: []string{"status", "user_id"},
			},
		}

		// Filter with conditions in non-optimal order
		filter, err := ParseFilter("user_id=123 AND status=\"active\"")
		require.NoError(t, err)

		// Call the old WhereClause method
		sql, params, err := table.WhereClause(filter, "p_")
		require.NoError(t, err)

		// Should NOT reorder conditions (maintains original order)
		// The original order is user_id first, then status
		assert.Contains(t, sql, "user_id")
		assert.Contains(t, sql, "status")
		assert.Equal(t, 2, len(params))
	})

	t.Run("WhereClauseWithOptions can enable optimizations", func(t *testing.T) {
		// Create a table with MatchModes configured
		table := NewTable().WithColumns(
			NewColumn().
				WithFieldPath("name").
				WithDatabaseName("name").
				Filterable().
				WithMatchModes(MatchModePrefix, MatchModeExact).
				Build(),
		).Build()

		filter, err := ParseFilter("name:\"test\"")
		require.NoError(t, err)

		// Call the new WhereClauseWithOptions method with options
		sql, params, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
			Dialect: SQLDialectGeneric,
		})
		require.NoError(t, err)

		// Should use prefix mode (LIKE with value%)
		assert.Equal(t, "(name LIKE @p_0)", sql)
		assert.Equal(t, []QueryParameter{
			{Name: "p_0", Value: "test%"},
		}, params)
	})

	t.Run("WhereClause ignores MatchModes configuration", func(t *testing.T) {
		// Create a table with MatchModes configured
		table := NewTable().WithColumns(
			NewColumn().
				WithFieldPath("name").
				WithDatabaseName("name").
				Filterable().
				WithMatchModes(MatchModePrefix, MatchModeExact).
				Build(),
		).Build()

		filter, err := ParseFilter("name:\"test\"")
		require.NoError(t, err)

		// Call the old WhereClause method
		sql, params, err := table.WhereClause(filter, "p_")
		require.NoError(t, err)

		// Should still use contains mode (ignoring MatchModes)
		assert.Equal(t, "(name LIKE @p_0)", sql)
		assert.Equal(t, []QueryParameter{
			{Name: "p_0", Value: "%test%"},
		}, params)
	})

	t.Run("method signature remains unchanged", func(t *testing.T) {
		table := NewTable().WithColumns(
			NewColumn().
				WithFieldPath("foo").
				WithDatabaseName("foo").
				Filterable().
				Build(),
		).Build()

		filter, err := ParseFilter("foo=\"bar\"")
		require.NoError(t, err)

		// Verify the method signature: (filter, parameterPrefix) -> (string, []QueryParameter, error)
		sql, params, err := table.WhereClause(filter, "p_")

		// Should work as before
		require.NoError(t, err)
		assert.NotEmpty(t, sql)
		assert.NotNil(t, params)
	})
}
