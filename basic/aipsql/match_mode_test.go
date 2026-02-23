// Copyright 2022 The codesjoy Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package aipsql

import (
	"fmt"
	"strings"
	"testing"
	"testing/quick"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// Exact Mode Tests
// =============================================================================

// TestExactModeDemo demonstrates Task 2.4: exact mode SQL generation
// This test validates Requirements 1.1, 8.1, 8.2
func TestExactModeDemo(t *testing.T) {
	// Setup: Create a table with a column configured for exact match mode
	table := NewTable().WithColumns(
		NewColumn().
			WithFieldPath("display_name").
			WithDatabaseName("display_name").
			Filterable().
			WithMatchModes(MatchModeExact).
			Build(),
	).Build()

	// Test case from the task description:
	// Input: column "display_name", value "John"
	filter, err := ParseFilter("display_name:John")
	require.NoError(t, err)

	// Generate SQL with exact mode
	sql, params, err := table.WhereClauseWithOptions(filter, "p", WhereClauseOptions{
		Dialect: SQLDialectGeneric,
	})
	require.NoError(t, err)

	// Expected behavior from task description:
	// - Output SQL: "display_name = @p1"
	// - Output parameter: QueryParameter{Name: "@p1", Value: "John"}

	// Verify SQL format: column = @param (exact equality, B-tree index friendly)
	assert.Equal(t, "(display_name = @p0)", sql, "SQL should use exact equality operator")

	// Verify parameterized query (SQL injection prevention)
	require.Len(t, params, 1, "Should have exactly one parameter")
	assert.Equal(t, "p0", params[0].Name, "Parameter name should be p0")
	assert.Equal(
		t,
		"John",
		params[0].Value,
		"Parameter value should be the exact input without wildcards",
	)

	t.Logf("✓ Exact mode generates: %s", sql)
	t.Logf("✓ With parameter: Name=%q, Value=%q", params[0].Name, params[0].Value)
	t.Logf("✓ This is B-tree index friendly (no LIKE, no wildcards)")
}

// =============================================================================
// Contains Mode Tests
// =============================================================================

// TestContainsMode_BasicFunctionality tests that contains mode generates correct SQL
// with "column LIKE @param" format and parameter value as '%value%'
func TestContainsMode_BasicFunctionality(t *testing.T) {
	table := NewTable().WithColumns(
		NewColumn().WithFieldPath("name").
			WithDatabaseName("db_name").
			Filterable().
			WithMatchModes(MatchModeContains).
			Build(),
	).Build()

	tests := []struct {
		name          string
		filter        string
		expectedSQL   string
		expectedValue string
	}{
		{
			name:          "simple contains match",
			filter:        "name:hello",
			expectedSQL:   "(db_name LIKE @p_0)",
			expectedValue: "%hello%",
		},
		{
			name:          "empty string",
			filter:        `name:""`,
			expectedSQL:   "(db_name LIKE @p_0)",
			expectedValue: "%%",
		},
		{
			name:          "single character",
			filter:        "name:a",
			expectedSQL:   "(db_name LIKE @p_0)",
			expectedValue: "%a%",
		},
		{
			name:          "substring in middle",
			filter:        "name:world",
			expectedSQL:   "(db_name LIKE @p_0)",
			expectedValue: "%world%",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter, err := ParseFilter(tt.filter)
			require.NoError(t, err)

			query, params, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
				Dialect: SQLDialectGeneric,
			})
			require.NoError(t, err)
			assert.Equal(t, tt.expectedSQL, query)
			assert.Len(t, params, 1)
			assert.Equal(t, "p_0", params[0].Name)
			assert.Equal(t, tt.expectedValue, params[0].Value)
		})
	}
}

// TestContainsMode_LikeSpecialCharacterEscaping tests that LIKE special characters
// (%, _, \) are properly escaped in contains mode
func TestContainsMode_LikeSpecialCharacterEscaping(t *testing.T) {
	table := NewTable().WithColumns(
		NewColumn().WithFieldPath("name").
			WithDatabaseName("db_name").
			Filterable().
			WithMatchModes(MatchModeContains).
			Build(),
	).Build()

	tests := []struct {
		name          string
		filter        string
		expectedValue string
		description   string
	}{
		{
			name:          "percent sign",
			filter:        "name:test%",
			expectedValue: `%test\%%`,
			description:   "% should be escaped to \\%",
		},
		{
			name:          "underscore",
			filter:        "name:test_",
			expectedValue: `%test\_%`,
			description:   "_ should be escaped to \\_",
		},
		{
			name:          "backslash",
			filter:        `name:test\`,
			expectedValue: `%test\\%`,
			description:   "\\ should be escaped to \\\\",
		},
		{
			name:          "multiple percent signs",
			filter:        "name:%%test%%",
			expectedValue: `%\%\%test\%\%%`,
			description:   "all % should be escaped",
		},
		{
			name:          "multiple underscores",
			filter:        "name:__test__",
			expectedValue: `%\_\_test\_\_%`,
			description:   "all _ should be escaped",
		},
		{
			name:          "mixed special characters",
			filter:        `name:test%_\value`,
			expectedValue: `%test\%\_\\value%`,
			description:   "all special characters should be escaped",
		},
		{
			name:          "backslash before percent",
			filter:        `name:\%`,
			expectedValue: `%\\\%%`,
			description:   "backslash and percent should both be escaped",
		},
		{
			name:          "backslash before underscore",
			filter:        `name:\_`,
			expectedValue: `%\\\_%`,
			description:   "backslash and underscore should both be escaped",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter, err := ParseFilter(tt.filter)
			require.NoError(t, err)

			query, params, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
				Dialect: SQLDialectGeneric,
			})
			require.NoError(t, err)
			assert.Equal(t, "(db_name LIKE @p_0)", query, "SQL format should be correct")
			assert.Len(t, params, 1)
			assert.Equal(t, "p_0", params[0].Name)
			assert.Equal(t, tt.expectedValue, params[0].Value, tt.description)
		})
	}
}

// TestContainsMode_ParameterizedQuery verifies that contains mode uses parameterized queries
// to prevent SQL injection
func TestContainsMode_ParameterizedQuery(t *testing.T) {
	table := NewTable().WithColumns(
		NewColumn().WithFieldPath("name").
			WithDatabaseName("db_name").
			Filterable().
			WithMatchModes(MatchModeContains).
			Build(),
	).Build()

	// Test with SQL injection attempts
	tests := []struct {
		name        string
		filter      string
		description string
	}{
		{
			name:        "SQL injection with quotes",
			filter:      `name:"'; DROP TABLE users; --"`,
			description: "malicious SQL should be safely parameterized",
		},
		{
			name:        "SQL injection with OR",
			filter:      `name:"' OR '1'='1"`,
			description: "OR injection should be safely parameterized",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter, err := ParseFilter(tt.filter)
			require.NoError(t, err)

			query, params, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
				Dialect: SQLDialectGeneric,
			})
			require.NoError(t, err)

			// Verify SQL uses parameterized query
			assert.Equal(t, "(db_name LIKE @p_0)", query, "SQL should use parameterized query")
			assert.Len(t, params, 1)
			assert.Equal(t, "p_0", params[0].Name)

			// Verify the malicious input is in the parameter value, not in SQL
			assert.NotContains(t, query, "DROP TABLE")
			assert.NotContains(t, query, "OR '1'='1'")
		})
	}
}

// TestContainsMode_MultipleDialects verifies contains mode works across all SQL dialects
func TestContainsMode_MultipleDialects(t *testing.T) {
	table := NewTable().WithColumns(
		NewColumn().WithFieldPath("name").
			WithDatabaseName("db_name").
			Filterable().
			WithMatchModes(MatchModeContains).
			Build(),
	).Build()

	dialects := []SQLDialect{
		SQLDialectGeneric,
		SQLDialectPostgres,
		SQLDialectMySQL,
	}

	for _, dialect := range dialects {
		t.Run(string(dialect), func(t *testing.T) {
			filter, err := ParseFilter("name:test")
			require.NoError(t, err)

			query, params, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
				Dialect: dialect,
			})
			require.NoError(t, err)
			assert.Equal(t, "(db_name LIKE @p_0)", query)
			assert.Equal(t, []QueryParameter{
				{Name: "p_0", Value: "%test%"},
			}, params)
		})
	}
}

// TestContainsMode_FallbackBehavior tests that contains mode is used as fallback
// when no match mode is configured or when configured modes are not supported
func TestContainsMode_FallbackBehavior(t *testing.T) {
	t.Run("fallback when no match mode configured", func(t *testing.T) {
		table := NewTable().WithColumns(
			NewColumn().WithFieldPath("name").
				WithDatabaseName("db_name").
				Filterable().
				// No match modes configured
				Build(),
		).Build()

		filter, err := ParseFilter("name:test")
		require.NoError(t, err)

		query, params, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
			Dialect:    SQLDialectGeneric,
			StrictMode: false, // Non-strict mode allows fallback
		})
		require.NoError(t, err)
		assert.Equal(t, "(db_name LIKE @p_0)", query)
		assert.Equal(t, "%test%", params[0].Value, "Should fallback to contains mode")
	})

	t.Run("fallback when fulltext not supported in generic dialect", func(t *testing.T) {
		table := NewTable().WithColumns(
			NewColumn().WithFieldPath("name").
				WithDatabaseName("db_name").
				Filterable().
				WithMatchModes(MatchModeFullText). // Only fulltext configured
				Build(),
		).Build()

		filter, err := ParseFilter("name:test")
		require.NoError(t, err)

		query, params, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
			Dialect:    SQLDialectGeneric, // Generic doesn't support fulltext
			StrictMode: false,             // Non-strict mode allows fallback
		})
		require.NoError(t, err)
		assert.Equal(t, "(db_name LIKE @p_0)", query)
		assert.Equal(t, "%test%", params[0].Value, "Should fallback to contains mode")
	})

	t.Run("error in strict mode when no supported match mode", func(t *testing.T) {
		table := NewTable().WithColumns(
			NewColumn().WithFieldPath("name").
				WithDatabaseName("db_name").
				Filterable().
				WithMatchModes(MatchModeFullText). // Only fulltext configured
				Build(),
		).Build()

		filter, err := ParseFilter("name:test")
		require.NoError(t, err)

		_, _, err = table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
			Dialect:    SQLDialectGeneric, // Generic doesn't support fulltext
			StrictMode: true,              // Strict mode prevents fallback
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no supported match mode")
	})
}

// =============================================================================
// Prefix Mode Tests
// =============================================================================

// TestPrefixMode_BasicFunctionality tests that prefix mode generates correct SQL
// with "column LIKE @param" format and parameter value as 'value%'
func TestPrefixMode_BasicFunctionality(t *testing.T) {
	table := NewTable().WithColumns(
		NewColumn().WithFieldPath("name").
			WithDatabaseName("db_name").
			Filterable().
			WithMatchModes(MatchModePrefix).
			Build(),
	).Build()

	tests := []struct {
		name          string
		filter        string
		expectedSQL   string
		expectedValue string
	}{
		{
			name:          "simple prefix match",
			filter:        "name:hello",
			expectedSQL:   "(db_name LIKE @p_0)",
			expectedValue: "hello%",
		},
		{
			name:          "empty string",
			filter:        `name:""`,
			expectedSQL:   "(db_name LIKE @p_0)",
			expectedValue: "%",
		},
		{
			name:          "single character",
			filter:        "name:a",
			expectedSQL:   "(db_name LIKE @p_0)",
			expectedValue: "a%",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter, err := ParseFilter(tt.filter)
			require.NoError(t, err)

			query, params, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
				Dialect: SQLDialectGeneric,
			})
			require.NoError(t, err)
			assert.Equal(t, tt.expectedSQL, query)
			assert.Len(t, params, 1)
			assert.Equal(t, "p_0", params[0].Name)
			assert.Equal(t, tt.expectedValue, params[0].Value)
		})
	}
}

// TestPrefixMode_LikeSpecialCharacterEscaping tests that LIKE special characters
// (%, _, \) are properly escaped in prefix mode
func TestPrefixMode_LikeSpecialCharacterEscaping(t *testing.T) {
	table := NewTable().WithColumns(
		NewColumn().WithFieldPath("name").
			WithDatabaseName("db_name").
			Filterable().
			WithMatchModes(MatchModePrefix).
			Build(),
	).Build()

	tests := []struct {
		name          string
		filter        string
		expectedValue string
		description   string
	}{
		{
			name:          "percent sign",
			filter:        "name:test%",
			expectedValue: `test\%%`,
			description:   "% should be escaped to \\%",
		},
		{
			name:          "underscore",
			filter:        "name:test_",
			expectedValue: `test\_%`,
			description:   "_ should be escaped to \\_",
		},
		{
			name:          "backslash",
			filter:        `name:test\`,
			expectedValue: `test\\%`,
			description:   "\\ should be escaped to \\\\",
		},
		{
			name:          "multiple percent signs",
			filter:        "name:%%test%%",
			expectedValue: `\%\%test\%\%%`,
			description:   "all % should be escaped",
		},
		{
			name:          "multiple underscores",
			filter:        "name:__test__",
			expectedValue: `\_\_test\_\_%`,
			description:   "all _ should be escaped",
		},
		{
			name:          "mixed special characters",
			filter:        `name:test%_\value`,
			expectedValue: `test\%\_\\value%`,
			description:   "all special characters should be escaped",
		},
		{
			name:          "backslash before percent",
			filter:        `name:\%`,
			expectedValue: `\\\%%`,
			description:   "backslash and percent should both be escaped",
		},
		{
			name:          "backslash before underscore",
			filter:        `name:\_`,
			expectedValue: `\\\_%`,
			description:   "backslash and underscore should both be escaped",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter, err := ParseFilter(tt.filter)
			require.NoError(t, err)

			query, params, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
				Dialect: SQLDialectGeneric,
			})
			require.NoError(t, err)
			assert.Equal(t, "(db_name LIKE @p_0)", query, "SQL format should be correct")
			assert.Len(t, params, 1)
			assert.Equal(t, "p_0", params[0].Name)
			assert.Equal(t, tt.expectedValue, params[0].Value, tt.description)
		})
	}
}

// TestPrefixMode_ParameterizedQuery verifies that prefix mode uses parameterized queries
// to prevent SQL injection
func TestPrefixMode_ParameterizedQuery(t *testing.T) {
	table := NewTable().WithColumns(
		NewColumn().WithFieldPath("name").
			WithDatabaseName("db_name").
			Filterable().
			WithMatchModes(MatchModePrefix).
			Build(),
	).Build()

	// Test with SQL injection attempts
	tests := []struct {
		name        string
		filter      string
		description string
	}{
		{
			name:        "SQL injection with quotes",
			filter:      `name:"'; DROP TABLE users; --"`,
			description: "malicious SQL should be safely parameterized",
		},
		{
			name:        "SQL injection with OR",
			filter:      `name:"' OR '1'='1"`,
			description: "OR injection should be safely parameterized",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter, err := ParseFilter(tt.filter)
			require.NoError(t, err)

			query, params, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
				Dialect: SQLDialectGeneric,
			})
			require.NoError(t, err)

			// Verify SQL uses parameterized query
			assert.Equal(t, "(db_name LIKE @p_0)", query, "SQL should use parameterized query")
			assert.Len(t, params, 1)
			assert.Equal(t, "p_0", params[0].Name)

			// Verify the malicious input is in the parameter value, not in SQL
			assert.NotContains(t, query, "DROP TABLE")
			assert.NotContains(t, query, "OR '1'='1'")
		})
	}
}

// TestPrefixMode_MultipleDialects verifies prefix mode works across all SQL dialects
func TestPrefixMode_MultipleDialects(t *testing.T) {
	table := NewTable().WithColumns(
		NewColumn().WithFieldPath("name").
			WithDatabaseName("db_name").
			Filterable().
			WithMatchModes(MatchModePrefix).
			Build(),
	).Build()

	dialects := []SQLDialect{
		SQLDialectGeneric,
		SQLDialectPostgres,
		SQLDialectMySQL,
	}

	for _, dialect := range dialects {
		t.Run(string(dialect), func(t *testing.T) {
			filter, err := ParseFilter("name:test")
			require.NoError(t, err)

			query, params, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
				Dialect: dialect,
			})
			require.NoError(t, err)
			assert.Equal(t, "(db_name LIKE @p_0)", query)
			assert.Equal(t, []QueryParameter{
				{Name: "p_0", Value: "test%"},
			}, params)
		})
	}
}

// =============================================================================
// FullText Mode Tests
// =============================================================================

// TestFullTextMode_BasicFunctionality tests that fulltext mode generates correct SQL
// for PostgreSQL and MySQL dialects
func TestFullTextMode_BasicFunctionality(t *testing.T) {
	t.Run("postgres fulltext", func(t *testing.T) {
		table := NewTable().WithColumns(
			NewColumn().WithFieldPath("content").
				WithDatabaseName("db_content").
				Filterable().
				WithMatchModes(MatchModeFullText).
				Build(),
		).Build()

		filter, err := ParseFilter("content:hello")
		require.NoError(t, err)

		query, params, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
			Dialect: SQLDialectPostgres,
		})
		require.NoError(t, err)
		assert.Contains(t, query, "to_tsvector('simple', db_content)")
		assert.Contains(t, query, "websearch_to_tsquery('simple', @p_0)")
		assert.Len(t, params, 1)
		assert.Equal(t, "p_0", params[0].Name)
		assert.Equal(t, "hello", params[0].Value)
	})

	t.Run("mysql fulltext", func(t *testing.T) {
		table := NewTable().WithColumns(
			NewColumn().WithFieldPath("content").
				WithDatabaseName("db_content").
				Filterable().
				WithMatchModes(MatchModeFullText).
				Build(),
		).Build()

		filter, err := ParseFilter("content:hello")
		require.NoError(t, err)

		query, params, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
			Dialect: SQLDialectMySQL,
		})
		require.NoError(t, err)
		assert.Contains(t, query, "MATCH(db_content)")
		assert.Contains(t, query, "AGAINST (@p_0 IN BOOLEAN MODE)")
		assert.Len(t, params, 1)
		assert.Equal(t, "p_0", params[0].Name)
		assert.Equal(t, "hello", params[0].Value)
	})
}

// TestFullTextMode_GenericDialectFallback tests that fulltext mode falls back to contains
// when using generic dialect in non-strict mode
func TestFullTextMode_GenericDialectFallback(t *testing.T) {
	table := NewTable().WithColumns(
		NewColumn().WithFieldPath("content").
			WithDatabaseName("db_content").
			Filterable().
			WithMatchModes(MatchModeFullText).
			Build(),
	).Build()

	filter, err := ParseFilter("content:hello")
	require.NoError(t, err)

	t.Run("non-strict mode falls back to contains", func(t *testing.T) {
		query, params, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
			Dialect:    SQLDialectGeneric,
			StrictMode: false,
		})
		require.NoError(t, err)
		assert.Contains(t, query, "LIKE")
		assert.Equal(t, "%hello%", params[0].Value)
	})

	t.Run("strict mode returns error", func(t *testing.T) {
		_, _, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
			Dialect:    SQLDialectGeneric,
			StrictMode: true,
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no supported match mode")
	})
}

// TestFullTextMode_ParameterizedQuery verifies that fulltext mode uses parameterized queries
func TestFullTextMode_ParameterizedQuery(t *testing.T) {
	dialects := []SQLDialect{SQLDialectPostgres, SQLDialectMySQL}

	for _, dialect := range dialects {
		t.Run(string(dialect), func(t *testing.T) {
			table := NewTable().WithColumns(
				NewColumn().WithFieldPath("content").
					WithDatabaseName("db_content").
					Filterable().
					WithMatchModes(MatchModeFullText).
					Build(),
			).Build()

			// Test with SQL injection attempts
			filter, err := ParseFilter(`content:"'; DROP TABLE users; --"`)
			require.NoError(t, err)

			query, params, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
				Dialect: dialect,
			})
			require.NoError(t, err)

			// Verify SQL uses parameterized query
			assert.Contains(t, query, "@p_0")
			assert.Len(t, params, 1)

			// Verify the malicious input is in the parameter value, not in SQL
			assert.NotContains(t, query, "DROP TABLE")
			assert.Equal(t, "'; DROP TABLE users; --", params[0].Value)
		})
	}
}

// =============================================================================
// KeyValue Match Mode Tests
// =============================================================================

// TestKeyValueMatchMode_Exact tests that Key-Value columns with exact match mode
// generate UNNEST subqueries with exact value matching (value = @param)
func TestKeyValueMatchMode_Exact(t *testing.T) {
	table := NewTable().WithColumns(
		NewColumn().
			WithFieldPath("labels").
			WithDatabaseName("db_labels").
			KeyValue().
			Filterable().
			WithMatchModes(MatchModeExact).
			Build(),
	).Build()

	filter, err := ParseFilter("labels.env:production")
	require.NoError(t, err)

	result, pars, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{})
	require.NoError(t, err)

	// Verify parameters
	assert.Equal(t, []QueryParameter{
		{Name: "p_0", Value: "env"},
		{Name: "p_1", Value: "production"},
	}, pars)

	// Verify SQL uses exact match (value = @param)
	assert.Equal(t,
		"(EXISTS (SELECT key, value FROM UNNEST(db_labels) WHERE key = @p_0 AND value = @p_1))",
		result,
	)
}

// TestKeyValueMatchMode_Prefix tests that Key-Value columns with prefix match mode
// generate UNNEST subqueries with prefix value matching (value LIKE 'prefix%')
func TestKeyValueMatchMode_Prefix(t *testing.T) {
	table := NewTable().WithColumns(
		NewColumn().
			WithFieldPath("tags").
			WithDatabaseName("db_tags").
			KeyValue().
			Filterable().
			WithMatchModes(MatchModePrefix).
			Build(),
	).Build()

	filter, err := ParseFilter("tags.category:prod")
	require.NoError(t, err)

	result, pars, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{})
	require.NoError(t, err)

	// Verify parameters - prefix mode adds % at the end
	assert.Equal(t, []QueryParameter{
		{Name: "p_0", Value: "category"},
		{Name: "p_1", Value: "prod%"},
	}, pars)

	// Verify SQL uses LIKE for prefix matching
	assert.Equal(t,
		"(EXISTS (SELECT key, value FROM UNNEST(db_tags) WHERE key = @p_0 AND value LIKE @p_1))",
		result,
	)
}

// TestKeyValueMatchMode_Contains tests that Key-Value columns with contains match mode
// generate UNNEST subqueries with substring value matching (value LIKE '%substring%')
func TestKeyValueMatchMode_Contains(t *testing.T) {
	table := NewTable().WithColumns(
		NewColumn().
			WithFieldPath("metadata").
			WithDatabaseName("db_metadata").
			KeyValue().
			Filterable().
			WithMatchModes(MatchModeContains).
			Build(),
	).Build()

	filter, err := ParseFilter("metadata.description:test")
	require.NoError(t, err)

	result, pars, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{})
	require.NoError(t, err)

	// Verify parameters - contains mode adds % at both ends
	assert.Equal(t, []QueryParameter{
		{Name: "p_0", Value: "description"},
		{Name: "p_1", Value: "%test%"},
	}, pars)

	// Verify SQL uses LIKE for substring matching
	assert.Equal(
		t,
		"(EXISTS (SELECT key, value FROM UNNEST(db_metadata) WHERE key = @p_0 AND value LIKE @p_1))",
		result,
	)
}

// TestKeyValueMatchMode_DefaultContains tests that Key-Value columns without explicit
// match mode configuration default to contains mode
func TestKeyValueMatchMode_DefaultContains(t *testing.T) {
	table := NewTable().WithColumns(
		NewColumn().
			WithFieldPath("attributes").
			WithDatabaseName("db_attributes").
			KeyValue().
			Filterable().
			// No WithMatchModes() - should default to contains
			Build(),
	).Build()

	filter, err := ParseFilter("attributes.name:value")
	require.NoError(t, err)

	result, pars, err := table.WhereClause(filter, "p_")
	require.NoError(t, err)

	// Verify default contains behavior
	assert.Equal(t, []QueryParameter{
		{Name: "p_0", Value: "name"},
		{Name: "p_1", Value: "%value%"},
	}, pars)

	assert.Equal(
		t,
		"(EXISTS (SELECT key, value FROM UNNEST(db_attributes) WHERE key = @p_0 AND value LIKE @p_1))",
		result,
	)
}

// TestKeyValueMatchMode_MultipleModesWithFallback tests that Key-Value columns
// with multiple match modes use the first supported mode
func TestKeyValueMatchMode_MultipleModesWithFallback(t *testing.T) {
	table := NewTable().WithColumns(
		NewColumn().
			WithFieldPath("properties").
			WithDatabaseName("db_properties").
			KeyValue().
			Filterable().
			// Prefix is first and supported, so it should be used
			WithMatchModes(MatchModePrefix, MatchModeExact).
			Build(),
	).Build()

	filter, err := ParseFilter("properties.key:val")
	require.NoError(t, err)

	result, pars, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{})
	require.NoError(t, err)

	// Should use prefix mode (first in the list)
	assert.Equal(t, []QueryParameter{
		{Name: "p_0", Value: "key"},
		{Name: "p_1", Value: "val%"},
	}, pars)

	assert.Equal(
		t,
		"(EXISTS (SELECT key, value FROM UNNEST(db_properties) WHERE key = @p_0 AND value LIKE @p_1))",
		result,
	)
}

// TestKeyValueMatchMode_EqualOperatorIgnoresMatchMode tests that the = operator
// on Key-Value columns always uses exact matching regardless of match mode configuration
func TestKeyValueMatchMode_EqualOperatorIgnoresMatchMode(t *testing.T) {
	table := NewTable().WithColumns(
		NewColumn().
			WithFieldPath("labels").
			WithDatabaseName("db_labels").
			KeyValue().
			Filterable().
			WithMatchModes(MatchModePrefix). // Configured as prefix
			Build(),
	).Build()

	filter, err := ParseFilter("labels.env=production")
	require.NoError(t, err)

	result, pars, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{})
	require.NoError(t, err)

	// = operator should always use exact match
	assert.Equal(t, []QueryParameter{
		{Name: "p_0", Value: "env"},
		{Name: "p_1", Value: "production"},
	}, pars)

	assert.Equal(t,
		"(EXISTS (SELECT key, value FROM UNNEST(db_labels) WHERE key = @p_0 AND value = @p_1))",
		result,
	)
}

// TestKeyValueMatchMode_NotEqualOperatorIgnoresMatchMode tests that the != operator
// on Key-Value columns always uses exact matching regardless of match mode configuration
func TestKeyValueMatchMode_NotEqualOperatorIgnoresMatchMode(t *testing.T) {
	table := NewTable().WithColumns(
		NewColumn().
			WithFieldPath("labels").
			WithDatabaseName("db_labels").
			KeyValue().
			Filterable().
			WithMatchModes(MatchModeContains). // Configured as contains
			Build(),
	).Build()

	filter, err := ParseFilter("labels.env!=staging")
	require.NoError(t, err)

	result, pars, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{})
	require.NoError(t, err)

	// != operator should always use exact match
	assert.Equal(t, []QueryParameter{
		{Name: "p_0", Value: "env"},
		{Name: "p_1", Value: "staging"},
	}, pars)

	assert.Equal(t,
		"(EXISTS (SELECT key, value FROM UNNEST(db_labels) WHERE key = @p_0 AND value <> @p_1))",
		result,
	)
}

// TestKeyValueMatchMode_SpecialCharactersEscaped tests that special LIKE characters
// are properly escaped in Key-Value value matching
func TestKeyValueMatchMode_SpecialCharactersEscaped(t *testing.T) {
	table := NewTable().WithColumns(
		NewColumn().
			WithFieldPath("tags").
			WithDatabaseName("db_tags").
			KeyValue().
			Filterable().
			WithMatchModes(MatchModePrefix).
			Build(),
	).Build()

	filter, err := ParseFilter("tags.pattern:test%value_with\\slash")
	require.NoError(t, err)

	result, pars, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{})
	require.NoError(t, err)

	// Special characters should be escaped
	assert.Equal(t, []QueryParameter{
		{Name: "p_0", Value: "pattern"},
		{Name: "p_1", Value: "test\\%value\\_with\\\\slash%"},
	}, pars)

	assert.Equal(t,
		"(EXISTS (SELECT key, value FROM UNNEST(db_tags) WHERE key = @p_0 AND value LIKE @p_1))",
		result,
	)
}

// =============================================================================
// Match Mode Support Tests
// =============================================================================

func TestIsSupported(t *testing.T) {
	tests := []struct {
		name     string
		mode     MatchMode
		dialect  SQLDialect
		expected bool
	}{
		// exact, prefix, contains are always supported
		{
			name:     "exact mode with generic dialect",
			mode:     MatchModeExact,
			dialect:  SQLDialectGeneric,
			expected: true,
		},
		{
			name:     "exact mode with postgres dialect",
			mode:     MatchModeExact,
			dialect:  SQLDialectPostgres,
			expected: true,
		},
		{
			name:     "prefix mode with generic dialect",
			mode:     MatchModePrefix,
			dialect:  SQLDialectGeneric,
			expected: true,
		},
		{
			name:     "prefix mode with mysql dialect",
			mode:     MatchModePrefix,
			dialect:  SQLDialectMySQL,
			expected: true,
		},
		{
			name:     "contains mode with generic dialect",
			mode:     MatchModeContains,
			dialect:  SQLDialectGeneric,
			expected: true,
		},
		{
			name:     "contains mode with postgres dialect",
			mode:     MatchModeContains,
			dialect:  SQLDialectPostgres,
			expected: true,
		},
		// fulltext is only supported for postgres and mysql
		{
			name:     "fulltext mode with postgres dialect",
			mode:     MatchModeFullText,
			dialect:  SQLDialectPostgres,
			expected: true,
		},
		{
			name:     "fulltext mode with mysql dialect",
			mode:     MatchModeFullText,
			dialect:  SQLDialectMySQL,
			expected: true,
		},
		{
			name:     "fulltext mode with generic dialect",
			mode:     MatchModeFullText,
			dialect:  SQLDialectGeneric,
			expected: false,
		},
		// invalid mode
		{
			name:     "invalid mode",
			mode:     MatchMode("invalid"),
			dialect:  SQLDialectGeneric,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isSupported(tt.mode, tt.dialect)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// =============================================================================
// Property Tests for Match Modes
// =============================================================================

// TestProperty_ContainsModeGeneratesCorrectSQL validates contains mode SQL generation
func TestProperty_ContainsModeGeneratesCorrectSQL(t *testing.T) {
	t.Run("contains mode generates LIKE with prefix and suffix wildcard", func(t *testing.T) {
		property := func(inputValue string) bool {
			table := NewTable().WithColumns(
				NewColumn().
					WithFieldPath("test_field").
					WithDatabaseName("test_column").
					Filterable().
					WithMatchModes(MatchModeContains).
					Build(),
			).Build()

			filter, err := ParseFilter(fmt.Sprintf("test_field:%q", inputValue))
			if err != nil {
				return true
			}

			sql, params, err := table.WhereClauseWithOptions(
				filter,
				"p",
				WhereClauseOptions{
					Dialect:    SQLDialectGeneric,
					StrictMode: false,
				},
			)
			if err != nil {
				return true
			}

			if !strings.Contains(sql, " LIKE ") {
				return false
			}

			if strings.Contains(sql, " = ") {
				return false
			}

			if !strings.Contains(sql, "@p") {
				return false
			}

			if len(params) != 1 {
				return false
			}

			paramValue, ok := params[0].Value.(string)
			if !ok {
				return false
			}

			if !strings.HasPrefix(paramValue, "%") {
				return false
			}

			if !strings.HasSuffix(paramValue, "%") {
				return false
			}

			return true
		}

		config := &quick.Config{MaxCount: 100}
		if err := quick.Check(property, config); err != nil {
			t.Error(err)
		}
	})
}

// TestProperty_PrefixModeGeneratesCorrectSQL validates prefix mode SQL generation
func TestProperty_PrefixModeGeneratesCorrectSQL(t *testing.T) {
	t.Run("prefix mode generates LIKE with suffix wildcard", func(t *testing.T) {
		property := func(inputValue string) bool {
			table := NewTable().WithColumns(
				NewColumn().
					WithFieldPath("test_field").
					WithDatabaseName("test_column").
					Filterable().
					WithMatchModes(MatchModePrefix).
					Build(),
			).Build()

			filter, err := ParseFilter(fmt.Sprintf("test_field:%q", inputValue))
			if err != nil {
				return true
			}

			sql, params, err := table.WhereClauseWithOptions(
				filter,
				"p",
				WhereClauseOptions{
					Dialect:    SQLDialectGeneric,
					StrictMode: false,
				},
			)
			if err != nil {
				return true
			}

			if !strings.Contains(sql, " LIKE ") {
				return false
			}

			if strings.Contains(sql, " = ") {
				return false
			}

			if !strings.Contains(sql, "@p") {
				return false
			}

			if len(params) != 1 {
				return false
			}

			paramValue, ok := params[0].Value.(string)
			if !ok {
				return false
			}

			if !strings.HasSuffix(paramValue, "%") {
				return false
			}

			if strings.HasPrefix(paramValue, "%") && !strings.HasPrefix(inputValue, "%") &&
				inputValue != "" {
				return false
			}

			return true
		}

		config := &quick.Config{MaxCount: 100}
		if err := quick.Check(property, config); err != nil {
			t.Error(err)
		}
	})

	t.Run("prefix mode is B-tree index friendly", func(t *testing.T) {
		property := func(inputValue string) bool {
			table := NewTable().WithColumns(
				NewColumn().
					WithFieldPath("indexed_field").
					WithDatabaseName("indexed_column").
					Filterable().
					WithMatchModes(MatchModePrefix).
					Build(),
			).Build()

			filter, err := ParseFilter(fmt.Sprintf("indexed_field:%q", inputValue))
			if err != nil {
				return true
			}

			sql, params, err := table.WhereClauseWithOptions(
				filter,
				"p",
				WhereClauseOptions{
					Dialect:    SQLDialectGeneric,
					StrictMode: false,
				},
			)
			if err != nil {
				return true
			}

			hasLike := strings.Contains(sql, " LIKE ")
			if !hasLike || len(params) != 1 {
				return false
			}

			paramValue, ok := params[0].Value.(string)
			if !ok {
				return false
			}

			if !strings.HasSuffix(paramValue, "%") {
				return false
			}

			if strings.HasPrefix(paramValue, "%") && !strings.HasPrefix(inputValue, "%") &&
				inputValue != "" {
				return false
			}

			return true
		}

		config := &quick.Config{MaxCount: 100}
		if err := quick.Check(property, config); err != nil {
			t.Error(err)
		}
	})

	t.Run("prefix mode across different dialects", func(t *testing.T) {
		dialects := []SQLDialect{SQLDialectGeneric, SQLDialectPostgres, SQLDialectMySQL}

		for _, dialect := range dialects {
			t.Run(string(dialect), func(t *testing.T) {
				table := NewTable().WithColumns(
					NewColumn().
						WithFieldPath("test_field").
						WithDatabaseName("test_column").
						Filterable().
						WithMatchModes(MatchModePrefix).
						Build(),
				).Build()

				filter, err := ParseFilter(`test_field:"test"`)
				assert.NoError(t, err)

				sql, params, err := table.WhereClauseWithOptions(
					filter,
					"p",
					WhereClauseOptions{
						Dialect:    dialect,
						StrictMode: false,
					},
				)

				assert.NoError(t, err, "Should not error for dialect: %s", dialect)
				assert.Contains(
					t,
					sql,
					" LIKE ",
					"SQL should contain LIKE operator for dialect: %s",
					dialect,
				)
				assert.NotContains(
					t,
					sql,
					" = ",
					"SQL should not contain = for dialect: %s",
					dialect,
				)
				assert.Contains(
					t,
					sql,
					"@p",
					"SQL should use parameterized query for dialect: %s",
					dialect,
				)
				assert.Len(
					t,
					params,
					1,
					"Should have exactly one parameter for dialect: %s",
					dialect,
				)
				assert.Equal(
					t,
					"test%",
					params[0].Value,
					"Parameter value should have % suffix for dialect: %s",
					dialect,
				)
			})
		}
	})
}

// TestProperty_LikeSpecialCharacterEscaping validates LIKE special character escaping
func TestProperty_LikeSpecialCharacterEscaping(t *testing.T) {
	t.Run("all special characters are escaped", func(t *testing.T) {
		property := func(inputValue string) bool {
			if !strings.ContainsAny(inputValue, "%_\\") {
				return true
			}

			table := NewTable().WithColumns(
				NewColumn().
					WithFieldPath("test_field").
					WithDatabaseName("test_column").
					Filterable().
					WithMatchModes(MatchModePrefix).
					Build(),
			).Build()

			filter, err := ParseFilter(fmt.Sprintf("test_field:%q", inputValue))
			if err != nil {
				return true
			}

			_, params, err := table.WhereClauseWithOptions(
				filter,
				"p",
				WhereClauseOptions{
					Dialect:    SQLDialectGeneric,
					StrictMode: false,
				},
			)

			if err != nil || len(params) != 1 {
				return true
			}

			paramValue, ok := params[0].Value.(string)
			if !ok {
				return false
			}

			if !strings.HasSuffix(paramValue, "%") {
				return false
			}
			escapedInput := paramValue[:len(paramValue)-1]

			backslashCount := strings.Count(inputValue, "\\")
			percentCount := strings.Count(inputValue, "%")
			underscoreCount := strings.Count(inputValue, "_")

			if backslashCount > 0 {
				if !strings.Contains(escapedInput, "\\\\") {
					return false
				}
			}

			if percentCount > 0 {
				if !strings.Contains(escapedInput, "\\%") {
					return false
				}
			}

			if underscoreCount > 0 {
				if !strings.Contains(escapedInput, "\\_") {
					return false
				}
			}

			return true
		}

		config := &quick.Config{MaxCount: 100}
		if err := quick.Check(property, config); err != nil {
			t.Error(err)
		}
	})
}

// TestProperty_FullTextModeGeneratesCorrectSQL validates fulltext mode SQL generation
func TestProperty_FullTextModeGeneratesCorrectSQL(t *testing.T) {
	t.Run("postgres fulltext generates to_tsvector and websearch_to_tsquery", func(t *testing.T) {
		property := func(inputValue string) bool {
			table := NewTable().WithColumns(
				NewColumn().
					WithFieldPath("test_field").
					WithDatabaseName("test_column").
					Filterable().
					WithMatchModes(MatchModeFullText).
					Build(),
			).Build()

			filter, err := ParseFilter(fmt.Sprintf("test_field:%q", inputValue))
			if err != nil {
				return true
			}

			sql, params, err := table.WhereClauseWithOptions(
				filter,
				"p",
				WhereClauseOptions{
					Dialect:    SQLDialectPostgres,
					StrictMode: false,
				},
			)
			if err != nil {
				return true
			}

			if !strings.Contains(sql, "to_tsvector('simple', test_column)") {
				return false
			}

			if !strings.Contains(sql, "websearch_to_tsquery('simple', @p") {
				return false
			}

			if !strings.Contains(sql, "@@") {
				return false
			}

			if len(params) != 1 {
				return false
			}

			paramValue, ok := params[0].Value.(string)
			if !ok {
				return false
			}

			if paramValue != inputValue {
				return false
			}

			return true
		}

		config := &quick.Config{MaxCount: 100}
		if err := quick.Check(property, config); err != nil {
			t.Error(err)
		}
	})

	t.Run("mysql fulltext generates MATCH AGAINST", func(t *testing.T) {
		property := func(inputValue string) bool {
			table := NewTable().WithColumns(
				NewColumn().
					WithFieldPath("test_field").
					WithDatabaseName("test_column").
					Filterable().
					WithMatchModes(MatchModeFullText).
					Build(),
			).Build()

			filter, err := ParseFilter(fmt.Sprintf("test_field:%q", inputValue))
			if err != nil {
				return true
			}

			sql, params, err := table.WhereClauseWithOptions(
				filter,
				"p",
				WhereClauseOptions{
					Dialect:    SQLDialectMySQL,
					StrictMode: false,
				},
			)
			if err != nil {
				return true
			}

			if !strings.Contains(sql, "MATCH(test_column)") {
				return false
			}

			if !strings.Contains(sql, "AGAINST (@p") {
				return false
			}

			if !strings.Contains(sql, "IN BOOLEAN MODE)") {
				return false
			}

			if len(params) != 1 {
				return false
			}

			paramValue, ok := params[0].Value.(string)
			if !ok {
				return false
			}

			if paramValue != inputValue {
				return false
			}

			return true
		}

		config := &quick.Config{MaxCount: 100}
		if err := quick.Check(property, config); err != nil {
			t.Error(err)
		}
	})
}

// TestProperty_ExactModeGeneratesCorrectSQL validates exact mode SQL generation
func TestProperty_ExactModeGeneratesCorrectSQL(t *testing.T) {
	t.Run("exact mode generates equality operator", func(t *testing.T) {
		property := func(inputValue string) bool {
			table := NewTable().WithColumns(
				NewColumn().
					WithFieldPath("test_field").
					WithDatabaseName("test_column").
					Filterable().
					WithMatchModes(MatchModeExact).
					Build(),
			).Build()

			filter, err := ParseFilter(fmt.Sprintf("test_field:%q", inputValue))
			if err != nil {
				return true
			}

			sql, params, err := table.WhereClauseWithOptions(
				filter,
				"p",
				WhereClauseOptions{
					Dialect:    SQLDialectGeneric,
					StrictMode: false,
				},
			)
			if err != nil {
				return true
			}

			if !strings.Contains(sql, " = ") {
				return false
			}

			if strings.Contains(sql, "LIKE") {
				return false
			}

			if !strings.Contains(sql, "@p") {
				return false
			}

			if len(params) != 1 {
				return false
			}

			paramValue, ok := params[0].Value.(string)
			if !ok {
				return false
			}

			if paramValue != inputValue {
				return false
			}

			return true
		}

		config := &quick.Config{MaxCount: 100}
		if err := quick.Check(property, config); err != nil {
			t.Error(err)
		}
	})

	t.Run("exact mode is B-tree index friendly", func(t *testing.T) {
		property := func(inputValue string) bool {
			table := NewTable().WithColumns(
				NewColumn().
					WithFieldPath("indexed_field").
					WithDatabaseName("indexed_column").
					Filterable().
					WithMatchModes(MatchModeExact).
					Build(),
			).Build()

			filter, err := ParseFilter(fmt.Sprintf("indexed_field:%q", inputValue))
			if err != nil {
				return true
			}

			sql, _, err := table.WhereClauseWithOptions(
				filter,
				"p",
				WhereClauseOptions{
					Dialect:    SQLDialectGeneric,
					StrictMode: false,
				},
			)
			if err != nil {
				return true
			}

			hasEquality := strings.Contains(sql, " = ")
			hasNoLike := !strings.Contains(sql, "LIKE")
			hasNoWildcard := !strings.Contains(sql, "%")

			return hasEquality && hasNoLike && hasNoWildcard
		}

		config := &quick.Config{MaxCount: 100}
		if err := quick.Check(property, config); err != nil {
			t.Error(err)
		}
	})
}

// TestProperty_MatchModeFallbackMechanism validates match mode fallback behavior
func TestProperty_MatchModeFallbackMechanism(t *testing.T) {
	t.Run("first mode supported", func(t *testing.T) {
		property := func() bool {
			column := &Column{
				fieldPath:    NewFieldPath("test_field"),
				databaseName: "test_column",
				columnType:   ColumnTypeString,
				matchModes:   []MatchMode{MatchModePrefix, MatchModeExact, MatchModeContains},
			}

			wc := &whereClause{
				dialect:       SQLDialectGeneric,
				strictMode:    false,
				fallbackMode:  MatchModeContains,
				optimizeMatch: true,
			}

			mode, err := wc.selectMatchMode(column, true)
			return err == nil && mode == MatchModePrefix
		}

		config := &quick.Config{MaxCount: 100}
		if err := quick.Check(property, config); err != nil {
			t.Error(err)
		}
	})

	t.Run("first mode unsupported, second mode supported", func(t *testing.T) {
		property := func() bool {
			column := &Column{
				fieldPath:    NewFieldPath("test_field"),
				databaseName: "test_column",
				columnType:   ColumnTypeString,
				matchModes:   []MatchMode{MatchModeFullText, MatchModePrefix, MatchModeExact},
			}

			wc := &whereClause{
				dialect:       SQLDialectGeneric,
				strictMode:    false,
				fallbackMode:  MatchModeContains,
				optimizeMatch: true,
			}

			mode, err := wc.selectMatchMode(column, true)
			return err == nil && mode == MatchModePrefix
		}

		config := &quick.Config{MaxCount: 100}
		if err := quick.Check(property, config); err != nil {
			t.Error(err)
		}
	})

	t.Run("no supported modes in strict mode", func(t *testing.T) {
		property := func() bool {
			column := &Column{
				fieldPath:    NewFieldPath("test_field"),
				databaseName: "test_column",
				columnType:   ColumnTypeString,
				matchModes:   []MatchMode{MatchModeFullText},
			}

			wc := &whereClause{
				dialect:       SQLDialectGeneric,
				strictMode:    true,
				fallbackMode:  MatchModeContains,
				optimizeMatch: true,
			}

			_, err := wc.selectMatchMode(column, true)
			return err != nil
		}

		config := &quick.Config{MaxCount: 100}
		if err := quick.Check(property, config); err != nil {
			t.Error(err)
		}
	})

	t.Run("no supported modes in non-strict mode", func(t *testing.T) {
		property := func() bool {
			column := &Column{
				fieldPath:    NewFieldPath("test_field"),
				databaseName: "test_column",
				columnType:   ColumnTypeString,
				matchModes:   []MatchMode{MatchModeFullText},
			}

			wc := &whereClause{
				dialect:       SQLDialectGeneric,
				strictMode:    false,
				fallbackMode:  MatchModeContains,
				optimizeMatch: true,
			}

			mode, err := wc.selectMatchMode(column, true)
			return err == nil && mode == MatchModeContains
		}

		config := &quick.Config{MaxCount: 100}
		if err := quick.Check(property, config); err != nil {
			t.Error(err)
		}
	})
}

// TestProperty_StrictModeErrorHandling validates strict mode error handling
func TestProperty_StrictModeErrorHandling(t *testing.T) {
	t.Run("unsupported modes in strict mode returns error", func(t *testing.T) {
		property := func() bool {
			column := &Column{
				fieldPath:    NewFieldPath("test_field"),
				databaseName: "test_column",
				columnType:   ColumnTypeString,
				matchModes:   []MatchMode{MatchModeFullText},
			}

			wc := &whereClause{
				dialect:       SQLDialectGeneric,
				strictMode:    true,
				fallbackMode:  MatchModeContains,
				optimizeMatch: true,
			}

			_, err := wc.selectMatchMode(column, true)
			return err != nil
		}

		config := &quick.Config{MaxCount: 100}
		if err := quick.Check(property, config); err != nil {
			t.Error(err)
		}
	})

	t.Run("unsupported modes in non-strict mode fallback to contains", func(t *testing.T) {
		property := func() bool {
			column := &Column{
				fieldPath:    NewFieldPath("test_field"),
				databaseName: "test_column",
				columnType:   ColumnTypeString,
				matchModes:   []MatchMode{MatchModeFullText},
			}

			wc := &whereClause{
				dialect:       SQLDialectGeneric,
				strictMode:    false,
				fallbackMode:  MatchModeContains,
				optimizeMatch: true,
			}

			mode, err := wc.selectMatchMode(column, true)
			return err == nil && mode == MatchModeContains
		}

		config := &quick.Config{MaxCount: 100}
		if err := quick.Check(property, config); err != nil {
			t.Error(err)
		}
	})
}
