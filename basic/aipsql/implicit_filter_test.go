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

// ============================================================================
// Single Column Tests
// ============================================================================

func buildSingleImplicitColumnTable(
	fieldPath string,
	databaseName string,
	matchModes ...MatchMode,
) *Table {
	column := NewColumn().
		WithFieldPath(fieldPath).
		WithDatabaseName(databaseName).
		FilterableImplicitly()
	if len(matchModes) > 0 {
		column = column.WithMatchModes(matchModes...)
	}
	return NewTable().WithColumns(column.Build()).Build()
}

func runImplicitFilterQuery(
	t *testing.T,
	table *Table,
	filterText string,
	options WhereClauseOptions,
) (string, []QueryParameter) {
	t.Helper()

	filter, err := ParseFilter(filterText)
	require.NoError(t, err)

	sql, params, err := table.WhereClauseWithOptions(filter, "p_", options)
	require.NoError(t, err)
	return sql, params
}

func TestSingleColumnImplicitFilter_MatchModes(t *testing.T) {
	testCases := []struct {
		name          string
		filter        string
		fieldPath     string
		databaseName  string
		matchModes    []MatchMode
		options       WhereClauseOptions
		expectedSQL   string
		expectedValue string
	}{
		{
			name:          "single exact mode",
			filter:        `"test"`,
			fieldPath:     "name",
			databaseName:  "db_name",
			matchModes:    []MatchMode{MatchModeExact},
			options:       WhereClauseOptions{Dialect: SQLDialectGeneric},
			expectedSQL:   "db_name = @p_0",
			expectedValue: "test",
		},
		{
			name:          "single prefix mode",
			filter:        `"hello"`,
			fieldPath:     "title",
			databaseName:  "db_title",
			matchModes:    []MatchMode{MatchModePrefix},
			options:       WhereClauseOptions{Dialect: SQLDialectGeneric},
			expectedSQL:   "db_title LIKE @p_0",
			expectedValue: "hello%",
		},
		{
			name:          "single contains mode",
			filter:        `"world"`,
			fieldPath:     "description",
			databaseName:  "db_description",
			matchModes:    []MatchMode{MatchModeContains},
			options:       WhereClauseOptions{Dialect: SQLDialectGeneric},
			expectedSQL:   "db_description LIKE @p_0",
			expectedValue: "%world%",
		},
		{
			name:          "single fulltext mode postgres",
			filter:        `"machine learning"`,
			fieldPath:     "content",
			databaseName:  "db_content",
			matchModes:    []MatchMode{MatchModeFullText},
			options:       WhereClauseOptions{Dialect: SQLDialectPostgres},
			expectedSQL:   "to_tsvector('simple', db_content) @@ websearch_to_tsquery('simple', @p_0)",
			expectedValue: "machine learning",
		},
		{
			name:          "single fulltext mode mysql",
			filter:        `"machine learning"`,
			fieldPath:     "content",
			databaseName:  "db_content",
			matchModes:    []MatchMode{MatchModeFullText},
			options:       WhereClauseOptions{Dialect: SQLDialectMySQL},
			expectedSQL:   "MATCH(db_content) AGAINST (@p_0 IN BOOLEAN MODE)",
			expectedValue: "machine learning",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			table := buildSingleImplicitColumnTable(tc.fieldPath, tc.databaseName, tc.matchModes...)
			sql, params := runImplicitFilterQuery(t, table, tc.filter, tc.options)

			assert.Equal(t, tc.expectedSQL, sql)
			assert.NotContains(t, sql, " OR ", "Should not use OR for single column")
			require.Len(t, params, 1)
			assert.Equal(t, "p_0", params[0].Name)
			assert.Equal(t, tc.expectedValue, params[0].Value)
		})
	}
}

func TestSingleColumnImplicitFilter_FallbackAndStrictMode(t *testing.T) {
	t.Run("fallback mode in non-strict mode", func(t *testing.T) {
		table := buildSingleImplicitColumnTable("name", "db_name")
		sql, params := runImplicitFilterQuery(t, table, `"test"`, WhereClauseOptions{
			Dialect:    SQLDialectGeneric,
			StrictMode: false,
		})

		assert.Equal(t, "db_name LIKE @p_0", sql)
		assert.NotContains(t, sql, " OR ", "Should not use OR for single column")
		require.Len(t, params, 1)
		assert.Equal(t, "p_0", params[0].Name)
		assert.Equal(t, "%test%", params[0].Value)
	})

	t.Run("strict mode returns error for unsupported modes", func(t *testing.T) {
		table := buildSingleImplicitColumnTable("content", "db_content", MatchModeFullText)
		filter, err := ParseFilter(`"test"`)
		require.NoError(t, err)

		_, _, err = table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
			Dialect:    SQLDialectGeneric,
			StrictMode: true,
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no supported match mode")
	})
}

// TestSingleColumnImplicitFilter_MatchModePriority tests that when implicit filter
// contains only one column with multiple match modes, it uses the first supported mode
func TestSingleColumnImplicitFilter_MatchModePriority(t *testing.T) {
	testCases := []struct {
		name          string
		options       WhereClauseOptions
		matchModes    []MatchMode
		fieldPath     string
		databaseName  string
		filter        string
		expectedSQL   string
		expectedValue string
	}{
		{
			name:          "fulltext first with generic fallback to prefix",
			options:       WhereClauseOptions{Dialect: SQLDialectGeneric},
			matchModes:    []MatchMode{MatchModeFullText, MatchModePrefix},
			fieldPath:     "content",
			databaseName:  "db_content",
			filter:        `"test"`,
			expectedSQL:   "db_content LIKE @p_0",
			expectedValue: "test%",
		},
		{
			name:          "fulltext first with postgres uses fulltext",
			options:       WhereClauseOptions{Dialect: SQLDialectPostgres},
			matchModes:    []MatchMode{MatchModeFullText, MatchModePrefix},
			fieldPath:     "content",
			databaseName:  "db_content",
			filter:        `"test"`,
			expectedSQL:   "to_tsvector('simple', db_content) @@ websearch_to_tsquery('simple', @p_0)",
			expectedValue: "test",
		},
		{
			name:          "exact first then prefix uses exact",
			options:       WhereClauseOptions{Dialect: SQLDialectGeneric},
			matchModes:    []MatchMode{MatchModeExact, MatchModePrefix},
			fieldPath:     "name",
			databaseName:  "db_name",
			filter:        `"test"`,
			expectedSQL:   "db_name = @p_0",
			expectedValue: "test",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			table := buildSingleImplicitColumnTable(tc.fieldPath, tc.databaseName, tc.matchModes...)
			sql, params := runImplicitFilterQuery(t, table, tc.filter, tc.options)

			assert.Equal(t, tc.expectedSQL, sql)
			require.Len(t, params, 1)
			assert.Equal(t, tc.expectedValue, params[0].Value)
		})
	}
}

// TestSingleColumnImplicitFilter_SpecialCharacters tests that single column
// implicit filter properly handles special characters
func TestSingleColumnImplicitFilter_SpecialCharacters(t *testing.T) {
	t.Run("LIKE special characters with prefix mode", func(t *testing.T) {
		table := NewTable().WithColumns(
			NewColumn().
				WithFieldPath("name").
				WithDatabaseName("db_name").
				FilterableImplicitly().
				WithMatchModes(MatchModePrefix).
				Build(),
		).Build()

		// Note: In the filter string, \v is an escape sequence that becomes a vertical tab character
		// To test actual backslash, we need to use double backslash in the filter string
		filter, err := ParseFilter(`"test%_value"`)
		require.NoError(t, err)

		sql, params, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
			Dialect: SQLDialectGeneric,
		})
		require.NoError(t, err)

		// Should escape special characters
		// Input: test%_value
		// After QuoteLike: test\%\_value (% -> \%, _ -> \_)
		// After adding suffix: test\%\_value%
		assert.Equal(t, "db_name LIKE @p_0", sql)
		assert.Equal(t, `test\%\_value%`, params[0].Value, "Should escape LIKE special characters")
	})

	t.Run("SQL injection attempt with exact mode", func(t *testing.T) {
		table := NewTable().WithColumns(
			NewColumn().
				WithFieldPath("name").
				WithDatabaseName("db_name").
				FilterableImplicitly().
				WithMatchModes(MatchModeExact).
				Build(),
		).Build()

		filter, err := ParseFilter(`"'; DROP TABLE users; --"`)
		require.NoError(t, err)

		sql, params, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
			Dialect: SQLDialectGeneric,
		})
		require.NoError(t, err)

		// Should use parameterized query
		assert.Equal(t, "db_name = @p_0", sql)
		assert.NotContains(t, sql, "DROP TABLE", "SQL injection should be prevented")
		assert.Equal(
			t,
			"'; DROP TABLE users; --",
			params[0].Value,
			"Malicious input should be in parameter",
		)
	})
}

// TestSingleColumnImplicitFilter_EmptyString tests that single column
// implicit filter handles empty string correctly
func TestSingleColumnImplicitFilter_EmptyString(t *testing.T) {
	testCases := []struct {
		name          string
		matchMode     MatchMode
		expectedSQL   string
		expectedValue string
	}{
		{
			name:          "exact mode",
			matchMode:     MatchModeExact,
			expectedSQL:   "db_name = @p_0",
			expectedValue: "",
		},
		{
			name:          "prefix mode",
			matchMode:     MatchModePrefix,
			expectedSQL:   "db_name LIKE @p_0",
			expectedValue: "%",
		},
		{
			name:          "contains mode",
			matchMode:     MatchModeContains,
			expectedSQL:   "db_name LIKE @p_0",
			expectedValue: "%%",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			table := NewTable().WithColumns(
				NewColumn().
					WithFieldPath("name").
					WithDatabaseName("db_name").
					FilterableImplicitly().
					WithMatchModes(tc.matchMode).
					Build(),
			).Build()

			filter, err := ParseFilter(`""`)
			require.NoError(t, err)

			sql, params, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
				Dialect: SQLDialectGeneric,
			})
			require.NoError(t, err)

			assert.Equal(t, tc.expectedSQL, sql)
			assert.Len(t, params, 1)
			assert.Equal(t, tc.expectedValue, params[0].Value)
		})
	}
}

// TestSingleColumnImplicitFilter_ComparisonWithMultiColumn tests that single
// column implicit filter generates different SQL than multi-column implicit filter
func TestSingleColumnImplicitFilter_ComparisonWithMultiColumn(t *testing.T) {
	// Single column table
	singleColTable := NewTable().WithColumns(
		NewColumn().
			WithFieldPath("name").
			WithDatabaseName("db_name").
			FilterableImplicitly().
			WithMatchModes(MatchModePrefix).
			Build(),
	).Build()

	// Multi column table
	multiColTable := NewTable().WithColumns(
		NewColumn().
			WithFieldPath("name").
			WithDatabaseName("db_name").
			FilterableImplicitly().
			WithMatchModes(MatchModePrefix).
			Build(),
		NewColumn().
			WithFieldPath("title").
			WithDatabaseName("db_title").
			FilterableImplicitly().
			WithMatchModes(MatchModePrefix).
			Build(),
	).Build()

	filter, err := ParseFilter(`"test"`)
	require.NoError(t, err)

	// Single column: should NOT use OR
	sqlSingle, paramsSingle, err := singleColTable.WhereClauseWithOptions(
		filter,
		"p_",
		WhereClauseOptions{
			Dialect: SQLDialectGeneric,
		},
	)
	require.NoError(t, err)
	assert.Equal(t, "db_name LIKE @p_0", sqlSingle)
	assert.NotContains(t, sqlSingle, " OR ")
	assert.Len(t, paramsSingle, 1)

	// Multi column: should use OR
	sqlMulti, paramsMulti, err := multiColTable.WhereClauseWithOptions(
		filter,
		"p_",
		WhereClauseOptions{
			Dialect: SQLDialectGeneric,
		},
	)
	require.NoError(t, err)
	assert.Equal(t, "(db_name LIKE @p_0 OR db_title LIKE @p_1)", sqlMulti)
	assert.Contains(t, sqlMulti, " OR ")
	assert.Len(t, paramsMulti, 2)
}

// ============================================================================
// Multi Column Tests
// ============================================================================

// TestMultiColumnImplicitFilter_SameMatchMode tests that when implicit filter
// contains multiple columns with the same match mode, it generates OR-connected clauses
func TestMultiColumnImplicitFilter_SameMatchMode(t *testing.T) {
	t.Run("all exact mode", func(t *testing.T) {
		table := NewTable().WithColumns(
			NewColumn().
				WithFieldPath("name").
				WithDatabaseName("db_name").
				FilterableImplicitly().
				WithMatchModes(MatchModeExact).
				Build(),
			NewColumn().
				WithFieldPath("title").
				WithDatabaseName("db_title").
				FilterableImplicitly().
				WithMatchModes(MatchModeExact).
				Build(),
		).Build()

		filter, err := ParseFilter(`"test"`)
		require.NoError(t, err)

		sql, params, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
			Dialect: SQLDialectGeneric,
		})
		require.NoError(t, err)

		// Should generate OR-connected clauses
		assert.Equal(t, "(db_name = @p_0 OR db_title = @p_1)", sql)
		assert.Contains(t, sql, " OR ", "Should use OR for multiple columns")

		// Verify parameters
		assert.Len(t, params, 2)
		assert.Equal(t, "p_0", params[0].Name)
		assert.Equal(t, "test", params[0].Value)
		assert.Equal(t, "p_1", params[1].Name)
		assert.Equal(t, "test", params[1].Value)
	})

	t.Run("all prefix mode", func(t *testing.T) {
		table := NewTable().WithColumns(
			NewColumn().
				WithFieldPath("name").
				WithDatabaseName("db_name").
				FilterableImplicitly().
				WithMatchModes(MatchModePrefix).
				Build(),
			NewColumn().
				WithFieldPath("title").
				WithDatabaseName("db_title").
				FilterableImplicitly().
				WithMatchModes(MatchModePrefix).
				Build(),
		).Build()

		filter, err := ParseFilter(`"hello"`)
		require.NoError(t, err)

		sql, params, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
			Dialect: SQLDialectGeneric,
		})
		require.NoError(t, err)

		// Should generate OR-connected clauses
		assert.Equal(t, "(db_name LIKE @p_0 OR db_title LIKE @p_1)", sql)

		// Verify parameters
		assert.Len(t, params, 2)
		assert.Equal(t, "hello%", params[0].Value)
		assert.Equal(t, "hello%", params[1].Value)
	})

	t.Run("all contains mode", func(t *testing.T) {
		table := NewTable().WithColumns(
			NewColumn().
				WithFieldPath("name").
				WithDatabaseName("db_name").
				FilterableImplicitly().
				WithMatchModes(MatchModeContains).
				Build(),
			NewColumn().
				WithFieldPath("description").
				WithDatabaseName("db_description").
				FilterableImplicitly().
				WithMatchModes(MatchModeContains).
				Build(),
		).Build()

		filter, err := ParseFilter(`"world"`)
		require.NoError(t, err)

		sql, params, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
			Dialect: SQLDialectGeneric,
		})
		require.NoError(t, err)

		// Should generate OR-connected clauses
		assert.Equal(t, "(db_name LIKE @p_0 OR db_description LIKE @p_1)", sql)

		// Verify parameters
		assert.Len(t, params, 2)
		assert.Equal(t, "%world%", params[0].Value)
		assert.Equal(t, "%world%", params[1].Value)
	})
}

// TestMultiColumnImplicitFilter_DifferentMatchModes tests that when implicit filter
// contains multiple columns with different match modes, each column uses its configured mode
func TestMultiColumnImplicitFilter_DifferentMatchModes(t *testing.T) {
	t.Run("exact and prefix", func(t *testing.T) {
		table := NewTable().WithColumns(
			NewColumn().
				WithFieldPath("name").
				WithDatabaseName("db_name").
				FilterableImplicitly().
				WithMatchModes(MatchModeExact).
				Build(),
			NewColumn().
				WithFieldPath("title").
				WithDatabaseName("db_title").
				FilterableImplicitly().
				WithMatchModes(MatchModePrefix).
				Build(),
		).Build()

		filter, err := ParseFilter(`"test"`)
		require.NoError(t, err)

		sql, params, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
			Dialect: SQLDialectGeneric,
		})
		require.NoError(t, err)

		// Should generate OR-connected clauses with different match modes
		assert.Equal(t, "(db_name = @p_0 OR db_title LIKE @p_1)", sql)

		// Verify parameters - different values based on match mode
		assert.Len(t, params, 2)
		assert.Equal(t, "test", params[0].Value, "Exact mode should not add wildcards")
		assert.Equal(t, "test%", params[1].Value, "Prefix mode should add % suffix")
	})

	t.Run("prefix and contains", func(t *testing.T) {
		table := NewTable().WithColumns(
			NewColumn().
				WithFieldPath("title").
				WithDatabaseName("db_title").
				FilterableImplicitly().
				WithMatchModes(MatchModePrefix).
				Build(),
			NewColumn().
				WithFieldPath("description").
				WithDatabaseName("db_description").
				FilterableImplicitly().
				WithMatchModes(MatchModeContains).
				Build(),
		).Build()

		filter, err := ParseFilter(`"search"`)
		require.NoError(t, err)

		sql, params, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
			Dialect: SQLDialectGeneric,
		})
		require.NoError(t, err)

		// Should generate OR-connected clauses with different match modes
		assert.Equal(t, "(db_title LIKE @p_0 OR db_description LIKE @p_1)", sql)

		// Verify parameters
		assert.Len(t, params, 2)
		assert.Equal(t, "search%", params[0].Value, "Prefix mode")
		assert.Equal(t, "%search%", params[1].Value, "Contains mode")
	})

	t.Run("exact, prefix, and contains", func(t *testing.T) {
		table := NewTable().WithColumns(
			NewColumn().
				WithFieldPath("id").
				WithDatabaseName("db_id").
				FilterableImplicitly().
				WithMatchModes(MatchModeExact).
				Build(),
			NewColumn().
				WithFieldPath("name").
				WithDatabaseName("db_name").
				FilterableImplicitly().
				WithMatchModes(MatchModePrefix).
				Build(),
			NewColumn().
				WithFieldPath("description").
				WithDatabaseName("db_description").
				FilterableImplicitly().
				WithMatchModes(MatchModeContains).
				Build(),
		).Build()

		filter, err := ParseFilter(`"value"`)
		require.NoError(t, err)

		sql, params, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
			Dialect: SQLDialectGeneric,
		})
		require.NoError(t, err)

		// Should generate OR-connected clauses with three different match modes
		assert.Equal(t, "(db_id = @p_0 OR db_name LIKE @p_1 OR db_description LIKE @p_2)", sql)

		// Verify parameters
		assert.Len(t, params, 3)
		assert.Equal(t, "value", params[0].Value, "Exact mode")
		assert.Equal(t, "value%", params[1].Value, "Prefix mode")
		assert.Equal(t, "%value%", params[2].Value, "Contains mode")
	})
}

// TestMultiColumnImplicitFilter_WithFullText tests multi-column implicit filter
// with fulltext mode for supported dialects
func TestMultiColumnImplicitFilter_WithFullText(t *testing.T) {
	t.Run("postgres fulltext with prefix", func(t *testing.T) {
		table := NewTable().WithColumns(
			NewColumn().
				WithFieldPath("title").
				WithDatabaseName("db_title").
				FilterableImplicitly().
				WithMatchModes(MatchModePrefix).
				Build(),
			NewColumn().
				WithFieldPath("content").
				WithDatabaseName("db_content").
				FilterableImplicitly().
				WithMatchModes(MatchModeFullText).
				Build(),
		).Build()

		filter, err := ParseFilter(`"machine learning"`)
		require.NoError(t, err)

		sql, params, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
			Dialect: SQLDialectPostgres,
		})
		require.NoError(t, err)

		// Should generate OR-connected clauses with different match modes
		assert.Contains(t, sql, "db_title LIKE @p_0")
		assert.Contains(t, sql, "to_tsvector('simple', db_content)")
		assert.Contains(t, sql, "websearch_to_tsquery('simple', @p_1)")
		assert.Contains(t, sql, " OR ")

		// Verify parameters
		assert.Len(t, params, 2)
		assert.Equal(t, "machine learning%", params[0].Value, "Prefix mode")
		assert.Equal(t, "machine learning", params[1].Value, "Fulltext mode")
	})

	t.Run("mysql fulltext with exact", func(t *testing.T) {
		table := NewTable().WithColumns(
			NewColumn().
				WithFieldPath("name").
				WithDatabaseName("db_name").
				FilterableImplicitly().
				WithMatchModes(MatchModeExact).
				Build(),
			NewColumn().
				WithFieldPath("content").
				WithDatabaseName("db_content").
				FilterableImplicitly().
				WithMatchModes(MatchModeFullText).
				Build(),
		).Build()

		filter, err := ParseFilter(`"test"`)
		require.NoError(t, err)

		sql, params, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
			Dialect: SQLDialectMySQL,
		})
		require.NoError(t, err)

		// Should generate OR-connected clauses
		assert.Contains(t, sql, "db_name = @p_0")
		assert.Contains(t, sql, "MATCH(db_content)")
		assert.Contains(t, sql, "AGAINST (@p_1 IN BOOLEAN MODE)")
		assert.Contains(t, sql, " OR ")

		// Verify parameters
		assert.Len(t, params, 2)
		assert.Equal(t, "test", params[0].Value, "Exact mode")
		assert.Equal(t, "test", params[1].Value, "Fulltext mode")
	})

	t.Run("postgres fulltext for all implicit columns", func(t *testing.T) {
		table := NewTable().WithColumns(
			NewColumn().
				WithFieldPath("title").
				WithDatabaseName("db_title").
				FilterableImplicitly().
				WithMatchModes(MatchModeFullText).
				Build(),
			NewColumn().
				WithFieldPath("content").
				WithDatabaseName("db_content").
				FilterableImplicitly().
				WithMatchModes(MatchModeFullText).
				Build(),
		).Build()

		sql, params := runImplicitFilterQuery(t, table, `"hello"`, WhereClauseOptions{
			Dialect: SQLDialectPostgres,
		})

		assert.Contains(t, sql, "to_tsvector('simple', db_title)")
		assert.Contains(t, sql, "to_tsvector('simple', db_content)")
		assert.Contains(t, sql, " OR ")
		assert.Len(t, params, 2)
		assert.Equal(t, "hello", params[0].Value)
		assert.Equal(t, "hello", params[1].Value)
	})

	t.Run("mysql fulltext for all implicit columns", func(t *testing.T) {
		table := NewTable().WithColumns(
			NewColumn().
				WithFieldPath("title").
				WithDatabaseName("db_title").
				FilterableImplicitly().
				WithMatchModes(MatchModeFullText).
				Build(),
			NewColumn().
				WithFieldPath("content").
				WithDatabaseName("db_content").
				FilterableImplicitly().
				WithMatchModes(MatchModeFullText).
				Build(),
		).Build()

		sql, params := runImplicitFilterQuery(t, table, `"hello"`, WhereClauseOptions{
			Dialect: SQLDialectMySQL,
		})

		assert.Contains(t, sql, "MATCH(db_title)")
		assert.Contains(t, sql, "MATCH(db_content)")
		assert.Contains(t, sql, " OR ")
		assert.Len(t, params, 2)
		assert.Equal(t, "hello", params[0].Value)
		assert.Equal(t, "hello", params[1].Value)
	})

	t.Run("fulltext fallback to prefix for generic dialect", func(t *testing.T) {
		table := NewTable().WithColumns(
			NewColumn().
				WithFieldPath("title").
				WithDatabaseName("db_title").
				FilterableImplicitly().
				WithMatchModes(MatchModeExact).
				Build(),
			NewColumn().
				WithFieldPath("content").
				WithDatabaseName("db_content").
				FilterableImplicitly().
				WithMatchModes(MatchModeFullText, MatchModePrefix). // Fulltext with prefix fallback
				Build(),
		).Build()

		filter, err := ParseFilter(`"test"`)
		require.NoError(t, err)

		sql, params, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
			Dialect: SQLDialectGeneric, // Generic doesn't support fulltext
		})
		require.NoError(t, err)

		// Should use prefix mode as fallback for content column
		assert.Equal(t, "(db_title = @p_0 OR db_content LIKE @p_1)", sql)

		// Verify parameters
		assert.Len(t, params, 2)
		assert.Equal(t, "test", params[0].Value, "Exact mode")
		assert.Equal(t, "test%", params[1].Value, "Prefix mode (fallback from fulltext)")
	})
}

// TestMultiColumnImplicitFilter_ThreeOrMoreColumns tests implicit filter with 3+ columns
func TestMultiColumnImplicitFilter_ThreeOrMoreColumns(t *testing.T) {
	table := NewTable().WithColumns(
		NewColumn().
			WithFieldPath("name").
			WithDatabaseName("db_name").
			FilterableImplicitly().
			WithMatchModes(MatchModeExact).
			Build(),
		NewColumn().
			WithFieldPath("title").
			WithDatabaseName("db_title").
			FilterableImplicitly().
			WithMatchModes(MatchModePrefix).
			Build(),
		NewColumn().
			WithFieldPath("summary").
			WithDatabaseName("db_summary").
			FilterableImplicitly().
			WithMatchModes(MatchModePrefix).
			Build(),
		NewColumn().
			WithFieldPath("description").
			WithDatabaseName("db_description").
			FilterableImplicitly().
			WithMatchModes(MatchModeContains).
			Build(),
	).Build()

	filter, err := ParseFilter(`"search term"`)
	require.NoError(t, err)

	sql, params, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
		Dialect: SQLDialectGeneric,
	})
	require.NoError(t, err)

	// Should generate OR-connected clauses for all columns
	expected := "(db_name = @p_0 OR db_title LIKE @p_1 OR db_summary LIKE @p_2 OR db_description LIKE @p_3)"
	assert.Equal(t, expected, sql)

	// Verify parameters
	assert.Len(t, params, 4)
	assert.Equal(t, "search term", params[0].Value, "Exact mode")
	assert.Equal(t, "search term%", params[1].Value, "Prefix mode")
	assert.Equal(t, "search term%", params[2].Value, "Prefix mode")
	assert.Equal(t, "%search term%", params[3].Value, "Contains mode")
}

// TestMultiColumnImplicitFilter_SpecialCharacters tests that multi-column
// implicit filter properly handles special characters
func TestMultiColumnImplicitFilter_SpecialCharacters(t *testing.T) {
	table := NewTable().WithColumns(
		NewColumn().
			WithFieldPath("name").
			WithDatabaseName("db_name").
			FilterableImplicitly().
			WithMatchModes(MatchModePrefix).
			Build(),
		NewColumn().
			WithFieldPath("description").
			WithDatabaseName("db_description").
			FilterableImplicitly().
			WithMatchModes(MatchModeContains).
			Build(),
	).Build()

	filter, err := ParseFilter(`"test%_value"`)
	require.NoError(t, err)

	sql, params, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
		Dialect: SQLDialectGeneric,
	})
	require.NoError(t, err)

	// Should escape special characters in both columns
	assert.Equal(t, "(db_name LIKE @p_0 OR db_description LIKE @p_1)", sql)

	// Verify parameters - special characters should be escaped
	assert.Len(t, params, 2)
	assert.Equal(t, `test\%\_value%`, params[0].Value, "Prefix mode with escaped chars")
	assert.Equal(t, `%test\%\_value%`, params[1].Value, "Contains mode with escaped chars")
}

// TestMultiColumnImplicitFilter_EmptyString tests multi-column implicit filter
// with empty string input
func TestMultiColumnImplicitFilter_EmptyString(t *testing.T) {
	table := NewTable().WithColumns(
		NewColumn().
			WithFieldPath("name").
			WithDatabaseName("db_name").
			FilterableImplicitly().
			WithMatchModes(MatchModeExact).
			Build(),
		NewColumn().
			WithFieldPath("title").
			WithDatabaseName("db_title").
			FilterableImplicitly().
			WithMatchModes(MatchModePrefix).
			Build(),
	).Build()

	filter, err := ParseFilter(`""`)
	require.NoError(t, err)

	sql, params, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
		Dialect: SQLDialectGeneric,
	})
	require.NoError(t, err)

	// Should generate OR-connected clauses
	assert.Equal(t, "(db_name = @p_0 OR db_title LIKE @p_1)", sql)

	// Verify parameters
	assert.Len(t, params, 2)
	assert.Equal(t, "", params[0].Value, "Exact mode with empty string")
	assert.Equal(t, "%", params[1].Value, "Prefix mode with empty string")
}

// TestMultiColumnImplicitFilter_MixedFallback tests multi-column implicit filter
// where some columns have no match mode configured
func TestMultiColumnImplicitFilter_MixedFallback(t *testing.T) {
	t.Run("one configured, one fallback", func(t *testing.T) {
		table := NewTable().WithColumns(
			NewColumn().
				WithFieldPath("name").
				WithDatabaseName("db_name").
				FilterableImplicitly().
				WithMatchModes(MatchModeExact).
				Build(),
			NewColumn().
				WithFieldPath("description").
				WithDatabaseName("db_description").
				FilterableImplicitly().
				// No match modes configured - should fallback to contains
				Build(),
		).Build()

		filter, err := ParseFilter(`"test"`)
		require.NoError(t, err)

		sql, params, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
			Dialect:    SQLDialectGeneric,
			StrictMode: false, // Allow fallback
		})
		require.NoError(t, err)

		// Should generate OR-connected clauses
		assert.Equal(t, "(db_name = @p_0 OR db_description LIKE @p_1)", sql)

		// Verify parameters
		assert.Len(t, params, 2)
		assert.Equal(t, "test", params[0].Value, "Exact mode")
		assert.Equal(t, "%test%", params[1].Value, "Fallback to contains mode")
	})
}

// TestMultiColumnImplicitFilter_StrictModePartialFailure tests that when one column
// has no supported match mode in strict mode, the entire query fails
func TestMultiColumnImplicitFilter_StrictModePartialFailure(t *testing.T) {
	table := NewTable().WithColumns(
		NewColumn().
			WithFieldPath("name").
			WithDatabaseName("db_name").
			FilterableImplicitly().
			WithMatchModes(MatchModeExact).
			Build(),
		NewColumn().
			WithFieldPath("content").
			WithDatabaseName("db_content").
			FilterableImplicitly().
			WithMatchModes(MatchModeFullText). // Only fulltext, not supported by generic
			Build(),
	).Build()

	filter, err := ParseFilter(`"test"`)
	require.NoError(t, err)

	_, _, err = table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
		Dialect:    SQLDialectGeneric, // Generic doesn't support fulltext
		StrictMode: true,              // Strict mode prevents fallback
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no supported match mode")
}

// TestMultiColumnImplicitFilter_ParameterNaming tests that parameters are
// correctly named and unique across multiple columns
func TestMultiColumnImplicitFilter_ParameterNaming(t *testing.T) {
	table := NewTable().WithColumns(
		NewColumn().
			WithFieldPath("col1").
			WithDatabaseName("db_col1").
			FilterableImplicitly().
			WithMatchModes(MatchModeExact).
			Build(),
		NewColumn().
			WithFieldPath("col2").
			WithDatabaseName("db_col2").
			FilterableImplicitly().
			WithMatchModes(MatchModePrefix).
			Build(),
		NewColumn().
			WithFieldPath("col3").
			WithDatabaseName("db_col3").
			FilterableImplicitly().
			WithMatchModes(MatchModeContains).
			Build(),
	).Build()

	filter, err := ParseFilter(`"test"`)
	require.NoError(t, err)

	sql, params, err := table.WhereClauseWithOptions(filter, "param_", WhereClauseOptions{
		Dialect: SQLDialectGeneric,
	})
	require.NoError(t, err)

	// Verify SQL uses correct parameter names
	assert.Contains(t, sql, "@param_0")
	assert.Contains(t, sql, "@param_1")
	assert.Contains(t, sql, "@param_2")

	// Verify parameters have unique names
	assert.Len(t, params, 3)
	assert.Equal(t, "param_0", params[0].Name)
	assert.Equal(t, "param_1", params[1].Name)
	assert.Equal(t, "param_2", params[2].Name)

	// Verify all parameters have the same base value (with different transformations)
	assert.Equal(t, "test", params[0].Value)
	assert.Equal(t, "test%", params[1].Value)
	assert.Equal(t, "%test%", params[2].Value)
}

// TestMultiColumnImplicitFilter_SQLInjectionPrevention tests that multi-column
// implicit filter prevents SQL injection
func TestMultiColumnImplicitFilter_SQLInjectionPrevention(t *testing.T) {
	table := NewTable().WithColumns(
		NewColumn().
			WithFieldPath("name").
			WithDatabaseName("db_name").
			FilterableImplicitly().
			WithMatchModes(MatchModeExact).
			Build(),
		NewColumn().
			WithFieldPath("title").
			WithDatabaseName("db_title").
			FilterableImplicitly().
			WithMatchModes(MatchModePrefix).
			Build(),
	).Build()

	maliciousInput := `"'; DROP TABLE users; --"`
	filter, err := ParseFilter(maliciousInput)
	require.NoError(t, err)

	sql, params, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
		Dialect: SQLDialectGeneric,
	})
	require.NoError(t, err)

	// SQL should use parameterized queries
	assert.Equal(t, "(db_name = @p_0 OR db_title LIKE @p_1)", sql)
	assert.NotContains(t, sql, "DROP TABLE", "SQL injection should be prevented")

	// Malicious input should be in parameters
	assert.Len(t, params, 2)
	assert.Equal(t, "'; DROP TABLE users; --", params[0].Value)
	assert.Equal(t, "'; DROP TABLE users; --%", params[1].Value)
}

// ============================================================================
// Property Tests
// ============================================================================

// **Validates: Requirements 2.2, 2.3**
// Feature: aip-sql-execution-optimization, Property 4: 隐式过滤多列 OR 连接
//
// For any implicit filter configuration containing multiple columns, the Filter_Generator
// should generate a matching clause for each column and connect them using OR operator.
// Each column should use its configured match mode.
func TestProperty_ImplicitFilterMultiColumnORConnection(t *testing.T) {
	// Test scenario 1: Multiple columns with same match mode should generate OR-connected clauses
	t.Run("multiple columns same mode generates OR clauses", func(t *testing.T) {
		property := func(searchValue string) bool {
			// Skip empty strings for this test
			if searchValue == "" {
				return true
			}

			// Create table with 3 columns all using prefix mode
			table := NewTable().WithColumns(
				NewColumn().
					WithFieldPath("name").
					WithDatabaseName("db_name").
					FilterableImplicitly().
					WithMatchModes(MatchModePrefix).
					Build(),
				NewColumn().
					WithFieldPath("title").
					WithDatabaseName("db_title").
					FilterableImplicitly().
					WithMatchModes(MatchModePrefix).
					Build(),
				NewColumn().
					WithFieldPath("summary").
					WithDatabaseName("db_summary").
					FilterableImplicitly().
					WithMatchModes(MatchModePrefix).
					Build(),
			).Build()

			filter, err := ParseFilter(fmt.Sprintf("%q", searchValue))
			if err != nil {
				return true
			}

			sql, params, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
				Dialect: SQLDialectGeneric,
			})
			if err != nil {
				return true
			}

			// Verify OR is used to connect multiple columns
			orCount := strings.Count(sql, " OR ")
			if orCount != 2 { // 3 columns = 2 OR operators
				return false
			}

			// Verify all three columns are present in SQL
			if !strings.Contains(sql, "db_name") {
				return false
			}
			if !strings.Contains(sql, "db_title") {
				return false
			}
			if !strings.Contains(sql, "db_summary") {
				return false
			}

			// Verify we have 3 parameters (one for each column)
			if len(params) != 3 {
				return false
			}

			// Verify all parameters have the same base value with prefix wildcard
			expectedValue := QuoteLike(searchValue) + "%"
			for _, param := range params {
				if param.Value != expectedValue {
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

	// Test scenario 2: Multiple columns with different match modes should each use their configured mode
	t.Run("multiple columns different modes use respective modes", func(t *testing.T) {
		property := func(searchValue string) bool {
			// Skip empty strings for this test
			if searchValue == "" {
				return true
			}

			// Create table with columns using different match modes
			table := NewTable().WithColumns(
				NewColumn().
					WithFieldPath("id").
					WithDatabaseName("db_id").
					FilterableImplicitly().
					WithMatchModes(MatchModeExact).
					Build(),
				NewColumn().
					WithFieldPath("name").
					WithDatabaseName("db_name").
					FilterableImplicitly().
					WithMatchModes(MatchModePrefix).
					Build(),
				NewColumn().
					WithFieldPath("description").
					WithDatabaseName("db_description").
					FilterableImplicitly().
					WithMatchModes(MatchModeContains).
					Build(),
			).Build()

			filter, err := ParseFilter(fmt.Sprintf("%q", searchValue))
			if err != nil {
				return true
			}

			sql, params, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
				Dialect: SQLDialectGeneric,
			})
			if err != nil {
				return true
			}

			// Verify OR is used to connect columns
			orCount := strings.Count(sql, " OR ")
			if orCount != 2 { // 3 columns = 2 OR operators
				return false
			}

			// Verify exact mode: db_id = @param (no LIKE)
			if !strings.Contains(sql, "db_id = ") {
				return false
			}

			// Verify prefix mode: db_name LIKE @param
			if !strings.Contains(sql, "db_name LIKE ") {
				return false
			}

			// Verify contains mode: db_description LIKE @param
			if !strings.Contains(sql, "db_description LIKE ") {
				return false
			}

			// Verify we have 3 parameters
			if len(params) != 3 {
				return false
			}

			// Verify parameter values match their respective modes
			quotedValue := QuoteLike(searchValue)
			expectedValues := []string{
				searchValue,             // exact mode - no wildcards
				quotedValue + "%",       // prefix mode - suffix wildcard
				"%" + quotedValue + "%", // contains mode - both wildcards
			}

			for i, param := range params {
				if param.Value != expectedValues[i] {
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

	// Test scenario 3: Verify OR clause is properly parenthesized
	t.Run("OR clause is properly parenthesized", func(t *testing.T) {
		table := NewTable().WithColumns(
			NewColumn().
				WithFieldPath("col1").
				WithDatabaseName("db_col1").
				FilterableImplicitly().
				WithMatchModes(MatchModeExact).
				Build(),
			NewColumn().
				WithFieldPath("col2").
				WithDatabaseName("db_col2").
				FilterableImplicitly().
				WithMatchModes(MatchModeExact).
				Build(),
		).Build()

		filter, err := ParseFilter(`"test"`)
		require.NoError(t, err)

		sql, _, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
			Dialect: SQLDialectGeneric,
		})
		require.NoError(t, err)

		// Verify the OR clause is wrapped in parentheses
		assert.True(t, strings.HasPrefix(sql, "("), "SQL should start with (")
		assert.True(t, strings.HasSuffix(sql, ")"), "SQL should end with )")
		assert.Contains(t, sql, " OR ", "SQL should contain OR operator")
	})

	// Test scenario 4: Multiple columns with fulltext mode in different dialects
	t.Run("multiple columns with fulltext in postgres", func(t *testing.T) {
		table := NewTable().WithColumns(
			NewColumn().
				WithFieldPath("title").
				WithDatabaseName("db_title").
				FilterableImplicitly().
				WithMatchModes(MatchModePrefix).
				Build(),
			NewColumn().
				WithFieldPath("content").
				WithDatabaseName("db_content").
				FilterableImplicitly().
				WithMatchModes(MatchModeFullText).
				Build(),
		).Build()

		filter, err := ParseFilter(`"machine learning"`)
		require.NoError(t, err)

		sql, params, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
			Dialect: SQLDialectPostgres,
		})
		require.NoError(t, err)

		// Verify OR connection
		assert.Contains(t, sql, " OR ")

		// Verify prefix mode for title
		assert.Contains(t, sql, "db_title LIKE ")

		// Verify fulltext mode for content
		assert.Contains(t, sql, "to_tsvector('simple', db_content)")
		assert.Contains(t, sql, "websearch_to_tsquery('simple', @p_")

		// Verify parameters
		assert.Len(t, params, 2)
		assert.Equal(t, "machine learning%", params[0].Value) // prefix mode
		assert.Equal(t, "machine learning", params[1].Value)  // fulltext mode
	})

	// Test scenario 5: Verify parameter naming is unique across columns
	t.Run("parameter naming is unique across columns", func(t *testing.T) {
		property := func(searchValue string) bool {
			if searchValue == "" {
				return true
			}

			table := NewTable().WithColumns(
				NewColumn().
					WithFieldPath("col1").
					WithDatabaseName("db_col1").
					FilterableImplicitly().
					WithMatchModes(MatchModeExact).
					Build(),
				NewColumn().
					WithFieldPath("col2").
					WithDatabaseName("db_col2").
					FilterableImplicitly().
					WithMatchModes(MatchModeExact).
					Build(),
				NewColumn().
					WithFieldPath("col3").
					WithDatabaseName("db_col3").
					FilterableImplicitly().
					WithMatchModes(MatchModeExact).
					Build(),
			).Build()

			filter, err := ParseFilter(fmt.Sprintf("%q", searchValue))
			if err != nil {
				return true
			}

			sql, params, err := table.WhereClauseWithOptions(filter, "param_", WhereClauseOptions{
				Dialect: SQLDialectGeneric,
			})
			if err != nil {
				return true
			}

			// Verify we have 3 unique parameters
			if len(params) != 3 {
				return false
			}

			// Verify parameter names are unique
			paramNames := make(map[string]bool)
			for _, param := range params {
				if paramNames[param.Name] {
					return false // Duplicate parameter name
				}
				paramNames[param.Name] = true

				// Verify parameter is referenced in SQL
				if !strings.Contains(sql, "@"+param.Name) {
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

	// Test scenario 6: Four or more columns should still use OR correctly
	t.Run("four or more columns use OR correctly", func(t *testing.T) {
		table := NewTable().WithColumns(
			NewColumn().
				WithFieldPath("col1").
				WithDatabaseName("db_col1").
				FilterableImplicitly().
				WithMatchModes(MatchModeExact).
				Build(),
			NewColumn().
				WithFieldPath("col2").
				WithDatabaseName("db_col2").
				FilterableImplicitly().
				WithMatchModes(MatchModePrefix).
				Build(),
			NewColumn().
				WithFieldPath("col3").
				WithDatabaseName("db_col3").
				FilterableImplicitly().
				WithMatchModes(MatchModePrefix).
				Build(),
			NewColumn().
				WithFieldPath("col4").
				WithDatabaseName("db_col4").
				FilterableImplicitly().
				WithMatchModes(MatchModeContains).
				Build(),
		).Build()

		filter, err := ParseFilter(`"test value"`)
		require.NoError(t, err)

		sql, params, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
			Dialect: SQLDialectGeneric,
		})
		require.NoError(t, err)

		// Verify OR count: 4 columns = 3 OR operators
		orCount := strings.Count(sql, " OR ")
		assert.Equal(t, 3, orCount, "Should have 3 OR operators for 4 columns")

		// Verify all columns are present
		assert.Contains(t, sql, "db_col1")
		assert.Contains(t, sql, "db_col2")
		assert.Contains(t, sql, "db_col3")
		assert.Contains(t, sql, "db_col4")

		// Verify we have 4 parameters
		assert.Len(t, params, 4)
	})
}

// **Validates: Requirements 2.4**
// Feature: aip-sql-execution-optimization, Property 5: 隐式过滤优先使用索引友好模式
//
// For any implicit filter column configuration, when columns are configured with exact or prefix
// Match_Mode, the Filter_Generator should prioritize these index-friendly modes over contains mode.
func TestProperty_ImplicitFilterPrioritizesIndexFriendlyModes(t *testing.T) {
	// Test scenario 1: Columns with exact or prefix mode should use those modes, not contains
	t.Run("exact and prefix modes are used over contains", func(t *testing.T) {
		property := func(searchValue string) bool {
			if searchValue == "" {
				return true
			}

			// Create table with columns configured with index-friendly modes
			table := NewTable().WithColumns(
				NewColumn().
					WithFieldPath("id").
					WithDatabaseName("db_id").
					FilterableImplicitly().
					WithMatchModes(MatchModeExact). // Index-friendly
					Build(),
				NewColumn().
					WithFieldPath("name").
					WithDatabaseName("db_name").
					FilterableImplicitly().
					WithMatchModes(MatchModePrefix). // Index-friendly
					Build(),
				NewColumn().
					WithFieldPath("description").
					WithDatabaseName("db_description").
					FilterableImplicitly().
					WithMatchModes(MatchModeContains). // Not index-friendly
					Build(),
			).Build()

			filter, err := ParseFilter(fmt.Sprintf("%q", searchValue))
			if err != nil {
				return true
			}

			sql, params, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
				Dialect: SQLDialectGeneric,
			})
			if err != nil {
				return true
			}

			// Verify exact mode is used for db_id (= operator, not LIKE)
			if !strings.Contains(sql, "db_id = ") {
				return false
			}

			// Verify prefix mode is used for db_name (LIKE with suffix wildcard only)
			if !strings.Contains(sql, "db_name LIKE ") {
				return false
			}

			// Verify contains mode is used for db_description (LIKE with both wildcards)
			if !strings.Contains(sql, "db_description LIKE ") {
				return false
			}

			// Verify parameter values
			if len(params) != 3 {
				return false
			}

			quotedValue := QuoteLike(searchValue)

			// db_id should use exact match (no wildcards)
			if params[0].Value != searchValue {
				return false
			}

			// db_name should use prefix match (suffix wildcard only)
			if params[1].Value != quotedValue+"%" {
				return false
			}

			// db_description should use contains match (both wildcards)
			if params[2].Value != "%"+quotedValue+"%" {
				return false
			}

			return true
		}

		config := &quick.Config{MaxCount: 100}
		if err := quick.Check(property, config); err != nil {
			t.Error(err)
		}
	})

	// Test scenario 2: When column has multiple match modes, first supported index-friendly mode is used
	t.Run("first supported index-friendly mode is prioritized", func(t *testing.T) {
		testCases := []struct {
			name          string
			matchModes    []MatchMode
			dialect       SQLDialect
			expectedMode  MatchMode
			expectedSQL   string
			expectedValue func(string) string
		}{
			{
				name:          "exact first, prefix second - uses exact",
				matchModes:    []MatchMode{MatchModeExact, MatchModePrefix, MatchModeContains},
				dialect:       SQLDialectGeneric,
				expectedMode:  MatchModeExact,
				expectedSQL:   " = ",
				expectedValue: func(v string) string { return v },
			},
			{
				name:          "prefix first, exact second - uses prefix",
				matchModes:    []MatchMode{MatchModePrefix, MatchModeExact, MatchModeContains},
				dialect:       SQLDialectGeneric,
				expectedMode:  MatchModePrefix,
				expectedSQL:   " LIKE ",
				expectedValue: func(v string) string { return QuoteLike(v) + "%" },
			},
			{
				name:          "fulltext first, prefix second, generic dialect - uses prefix",
				matchModes:    []MatchMode{MatchModeFullText, MatchModePrefix, MatchModeContains},
				dialect:       SQLDialectGeneric,
				expectedMode:  MatchModePrefix,
				expectedSQL:   " LIKE ",
				expectedValue: func(v string) string { return QuoteLike(v) + "%" },
			},
			{
				name:          "fulltext first, prefix second, postgres dialect - uses fulltext",
				matchModes:    []MatchMode{MatchModeFullText, MatchModePrefix, MatchModeContains},
				dialect:       SQLDialectPostgres,
				expectedMode:  MatchModeFullText,
				expectedSQL:   "to_tsvector",
				expectedValue: func(v string) string { return v },
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				table := NewTable().WithColumns(
					NewColumn().
						WithFieldPath("test_field").
						WithDatabaseName("test_column").
						FilterableImplicitly().
						WithMatchModes(tc.matchModes...).
						Build(),
				).Build()

				filter, err := ParseFilter(`"test value"`)
				require.NoError(t, err)

				sql, params, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
					Dialect: tc.dialect,
				})
				require.NoError(t, err)

				// Verify expected SQL pattern
				assert.Contains(
					t,
					sql,
					tc.expectedSQL,
					"SQL should contain expected pattern for %s",
					tc.name,
				)

				// Verify parameter value
				assert.Len(t, params, 1)
				expectedValue := tc.expectedValue("test value")
				assert.Equal(
					t,
					expectedValue,
					params[0].Value,
					"Parameter value mismatch for %s",
					tc.name,
				)
			})
		}
	})

	// Test scenario 3: Index-friendly modes (exact, prefix) should not add unnecessary wildcards
	t.Run("index-friendly modes avoid unnecessary wildcards", func(t *testing.T) {
		property := func(searchValue string) bool {
			if searchValue == "" {
				return true
			}

			// Create table with only index-friendly modes
			table := NewTable().WithColumns(
				NewColumn().
					WithFieldPath("exact_col").
					WithDatabaseName("db_exact").
					FilterableImplicitly().
					WithMatchModes(MatchModeExact).
					Build(),
				NewColumn().
					WithFieldPath("prefix_col").
					WithDatabaseName("db_prefix").
					FilterableImplicitly().
					WithMatchModes(MatchModePrefix).
					Build(),
			).Build()

			filter, err := ParseFilter(fmt.Sprintf("%q", searchValue))
			if err != nil {
				return true
			}

			_, params, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
				Dialect: SQLDialectGeneric,
			})
			if err != nil {
				return true
			}

			// Verify exact mode: no wildcards at all
			if len(params) < 1 {
				return false
			}
			exactValue, ok := params[0].Value.(string)
			if !ok {
				return false
			}
			// Exact mode should not have any wildcards
			if exactValue != searchValue {
				return false
			}

			// Verify prefix mode: only suffix wildcard
			if len(params) < 2 {
				return false
			}
			prefixValue, ok := params[1].Value.(string)
			if !ok {
				return false
			}
			// Prefix mode should have suffix wildcard only
			expectedPrefix := QuoteLike(searchValue) + "%"
			if prefixValue != expectedPrefix {
				return false
			}
			// Prefix mode should NOT have prefix wildcard
			if strings.HasPrefix(prefixValue, "%") {
				return false
			}

			return true
		}

		config := &quick.Config{MaxCount: 100}
		if err := quick.Check(property, config); err != nil {
			t.Error(err)
		}
	})

	// Test scenario 4: Verify exact and prefix modes can utilize B-tree indexes
	t.Run("exact and prefix modes are B-tree index friendly", func(t *testing.T) {
		table := NewTable().WithColumns(
			NewColumn().
				WithFieldPath("indexed_exact").
				WithDatabaseName("db_indexed_exact").
				FilterableImplicitly().
				WithMatchModes(MatchModeExact).
				Build(),
			NewColumn().
				WithFieldPath("indexed_prefix").
				WithDatabaseName("db_indexed_prefix").
				FilterableImplicitly().
				WithMatchModes(MatchModePrefix).
				Build(),
			NewColumn().
				WithFieldPath("non_indexed_contains").
				WithDatabaseName("db_non_indexed").
				FilterableImplicitly().
				WithMatchModes(MatchModeContains).
				Build(),
		).Build()

		filter, err := ParseFilter(`"search"`)
		require.NoError(t, err)

		sql, params, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
			Dialect: SQLDialectGeneric,
		})
		require.NoError(t, err)

		// Exact mode: uses = operator (B-tree index friendly)
		assert.Contains(t, sql, "db_indexed_exact = ")
		assert.Equal(t, "search", params[0].Value)

		// Prefix mode: uses LIKE with suffix wildcard only (B-tree index friendly)
		assert.Contains(t, sql, "db_indexed_prefix LIKE ")
		assert.Equal(t, "search%", params[1].Value)
		assert.False(
			t,
			strings.HasPrefix(params[1].Value.(string), "%"),
			"Prefix mode should not have leading wildcard",
		)

		// Contains mode: uses LIKE with both wildcards (NOT B-tree index friendly)
		assert.Contains(t, sql, "db_non_indexed LIKE ")
		assert.Equal(t, "%search%", params[2].Value)
		assert.True(
			t,
			strings.HasPrefix(params[2].Value.(string), "%"),
			"Contains mode should have leading wildcard",
		)
	})

	// Test scenario 5: Verify that when all columns use index-friendly modes, no contains mode is used
	t.Run("all index-friendly modes, no contains fallback", func(t *testing.T) {
		property := func(searchValue string) bool {
			if searchValue == "" {
				return true
			}

			// Create table with only index-friendly modes
			table := NewTable().WithColumns(
				NewColumn().
					WithFieldPath("col1").
					WithDatabaseName("db_col1").
					FilterableImplicitly().
					WithMatchModes(MatchModeExact).
					Build(),
				NewColumn().
					WithFieldPath("col2").
					WithDatabaseName("db_col2").
					FilterableImplicitly().
					WithMatchModes(MatchModePrefix).
					Build(),
				NewColumn().
					WithFieldPath("col3").
					WithDatabaseName("db_col3").
					FilterableImplicitly().
					WithMatchModes(MatchModeExact).
					Build(),
			).Build()

			filter, err := ParseFilter(fmt.Sprintf("%q", searchValue))
			if err != nil {
				return true
			}

			_, params, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
				Dialect: SQLDialectGeneric,
			})
			if err != nil {
				return true
			}

			// Verify no parameter has both prefix and suffix wildcards (contains mode pattern)
			quotedValue := QuoteLike(searchValue)
			containsPattern := "%" + quotedValue + "%"

			for _, param := range params {
				if param.Value == containsPattern {
					return false // Found contains mode, which should not be used
				}
			}

			return true
		}

		config := &quick.Config{MaxCount: 100}
		if err := quick.Check(property, config); err != nil {
			t.Error(err)
		}
	})

	// Test scenario 6: Mixed configuration - verify each column uses its configured mode
	t.Run("mixed configuration uses respective modes", func(t *testing.T) {
		table := NewTable().WithColumns(
			NewColumn().
				WithFieldPath("exact_field").
				WithDatabaseName("db_exact").
				FilterableImplicitly().
				WithMatchModes(MatchModeExact).
				Build(),
			NewColumn().
				WithFieldPath("prefix_field").
				WithDatabaseName("db_prefix").
				FilterableImplicitly().
				WithMatchModes(MatchModePrefix).
				Build(),
			NewColumn().
				WithFieldPath("contains_field").
				WithDatabaseName("db_contains").
				FilterableImplicitly().
				WithMatchModes(MatchModeContains).
				Build(),
		).Build()

		filter, err := ParseFilter(`"test%_value"`)
		require.NoError(t, err)

		sql, params, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
			Dialect: SQLDialectGeneric,
		})
		require.NoError(t, err)

		// Verify SQL structure
		assert.Contains(t, sql, "db_exact = ")
		assert.Contains(t, sql, "db_prefix LIKE ")
		assert.Contains(t, sql, "db_contains LIKE ")
		assert.Contains(t, sql, " OR ")

		// Verify parameters use correct modes
		assert.Len(t, params, 3)

		// Exact mode: no wildcards, special chars preserved
		assert.Equal(t, "test%_value", params[0].Value)

		// Prefix mode: suffix wildcard only, special chars escaped
		assert.Equal(t, `test\%\_value%`, params[1].Value)

		// Contains mode: both wildcards, special chars escaped
		assert.Equal(t, `%test\%\_value%`, params[2].Value)
	})
}
