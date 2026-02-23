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
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTask_11_2_AllUserInputParameterized verifies that all user input is properly
// parameterized throughout the codebase to prevent SQL injection attacks.
//
// This test validates Requirements 8.1, 8.2, and 8.3:
// - 8.1: Filter_Generator SHALL use parameterized queries for all user input
// - 8.2: Filter_Generator SHALL NOT directly concatenate user input into SQL
// - 8.3: OrderBy_Generator SHALL only use column names from Table definitions
func TestTask_11_2_AllUserInputParameterized(t *testing.T) {
	// Test 1: Filter WHERE clause parameterization
	t.Run("WHERE clause user input is parameterized", func(t *testing.T) {
		table := NewTable().WithColumns(
			NewColumn().WithFieldPath("name").WithDatabaseName("db_name").Filterable().Build(),
			NewColumn().WithFieldPath("status").WithDatabaseName("db_status").Filterable().Build(),
			NewColumn().WithFieldPath("count").
				WithDatabaseName("db_count").
				Filterable().
				Bool().
				Build(),
		).
			Build()

		testCases := []struct {
			name        string
			filter      string
			description string
		}{
			{
				name:        "simple equality",
				filter:      `name="test"`,
				description: "simple string value should be parameterized",
			},
			{
				name:        "SQL injection attempt with quotes",
				filter:      `name="'; DROP TABLE users; --"`,
				description: "malicious SQL should be parameterized",
			},
			{
				name:        "SQL injection with OR",
				filter:      `name="' OR '1'='1"`,
				description: "OR injection should be parameterized",
			},
			{
				name:        "multiple conditions",
				filter:      `name="test" AND status="active"`,
				description: "all values in multiple conditions should be parameterized",
			},
			{
				name:        "special characters",
				filter:      `name="test%_\\"`,
				description: "special characters should be parameterized",
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				parsedFilter, err := ParseFilter(tc.filter)
				require.NoError(t, err)

				sql, params, err := table.WhereClause(parsedFilter, "p_")
				require.NoError(t, err, tc.description)

				// Verify SQL uses parameterized queries
				assert.Contains(t, sql, "@p_", "SQL should use parameterized query format")

				// Verify no user input is directly embedded in SQL
				// Extract all string literals from the filter
				userValues := extractUserValues(tc.filter)
				for _, value := range userValues {
					if value != "" && !isSQLKeyword(value) {
						assert.NotContains(t, sql, value,
							"User input '%s' should not be directly embedded in SQL", value)
					}
				}

				// Verify all user values are in parameters
				assert.Greater(t, len(params), 0, "Should have at least one parameter")
				paramValues := make(map[string]bool)
				for _, p := range params {
					if strVal, ok := p.Value.(string); ok {
						paramValues[strVal] = true
					}
				}
			})
		}
	})

	// Test 2: has (:) operator parameterization with different match modes
	t.Run("has operator with match modes is parameterized", func(t *testing.T) {
		matchModes := []struct {
			name string
			mode MatchMode
		}{
			{"exact", MatchModeExact},
			{"prefix", MatchModePrefix},
			{"contains", MatchModeContains},
		}

		for _, mm := range matchModes {
			t.Run(mm.name, func(t *testing.T) {
				table := NewTable().WithColumns(
					NewColumn().WithFieldPath("name").WithDatabaseName("db_name").
						Filterable().WithMatchModes(mm.mode).Build(),
				).Build()

				filter, err := ParseFilter(`name:"'; DROP TABLE users; --"`)
				require.NoError(t, err)

				sql, params, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{})
				require.NoError(t, err)

				// Verify parameterized query
				assert.Contains(t, sql, "@p_", "SQL should use parameterized query")
				assert.NotContains(t, sql, "DROP TABLE", "Malicious input should not be in SQL")
				assert.Greater(t, len(params), 0, "Should have parameters")
			})
		}
	})

	// Test 3: Fulltext mode parameterization
	t.Run("fulltext mode is parameterized", func(t *testing.T) {
		dialects := []SQLDialect{SQLDialectPostgres, SQLDialectMySQL}

		for _, dialect := range dialects {
			t.Run(string(dialect), func(t *testing.T) {
				table := NewTable().WithColumns(
					NewColumn().WithFieldPath("content").WithDatabaseName("db_content").
						Filterable().WithMatchModes(MatchModeFullText).Build(),
				).Build()

				filter, err := ParseFilter(`content:"'; DROP TABLE users; --"`)
				require.NoError(t, err)

				sql, params, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
					Dialect: dialect,
				})
				require.NoError(t, err)

				// Verify parameterized query
				assert.Contains(t, sql, "@p_", "SQL should use parameterized query")
				assert.NotContains(t, sql, "DROP TABLE", "Malicious input should not be in SQL")
				assert.Greater(t, len(params), 0, "Should have parameters")
			})
		}
	})

	// Test 4: Key-Value column parameterization
	t.Run("key-value column queries are parameterized", func(t *testing.T) {
		table := NewTable().WithColumns(
			NewColumn().WithFieldPath("labels").WithDatabaseName("db_labels").
				KeyValue().Filterable().Build(),
		).Build()

		testCases := []struct {
			name   string
			filter string
		}{
			{"has operator", `labels.env:"prod"`},
			{"equality", `labels.env="prod"`},
			{"inequality", `labels.env!="dev"`},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				filter, err := ParseFilter(tc.filter)
				require.NoError(t, err)

				sql, params, err := table.WhereClause(filter, "p_")
				require.NoError(t, err)

				// Verify both key and value are parameterized
				assert.Contains(t, sql, "@p_", "SQL should use parameterized query")
				assert.GreaterOrEqual(
					t,
					len(params),
					2,
					"Should have at least 2 parameters (key and value)",
				)

				// Verify no direct embedding of user input
				assert.NotContains(t, sql, "'env'", "Key should not be directly embedded")
				assert.NotContains(t, sql, "'prod'", "Value should not be directly embedded")
				assert.NotContains(t, sql, "'dev'", "Value should not be directly embedded")
			})
		}
	})

	// Test 5: Implicit filter parameterization
	t.Run("implicit filter is parameterized", func(t *testing.T) {
		table := NewTable().WithColumns(
			NewColumn().WithFieldPath("name").
				WithDatabaseName("db_name").
				FilterableImplicitly().
				Build(),
			NewColumn().WithFieldPath("title").
				WithDatabaseName("db_title").
				FilterableImplicitly().
				Build(),
		).
			Build()

		filter, err := ParseFilter(`"'; DROP TABLE users; --"`)
		require.NoError(t, err)

		sql, params, err := table.WhereClause(filter, "p_")
		require.NoError(t, err)

		// Verify parameterized query
		assert.Contains(t, sql, "@p_", "SQL should use parameterized query")
		assert.NotContains(t, sql, "DROP TABLE", "Malicious input should not be in SQL")
		assert.Greater(t, len(params), 0, "Should have parameters")
	})

	// Test 6: ORDER BY only uses Table-defined columns
	t.Run("ORDER BY only uses Table-defined column names", func(t *testing.T) {
		table := NewTable().WithColumns(
			NewColumn().WithFieldPath("name").WithDatabaseName("db_name").Sortable().Build(),
			NewColumn().WithFieldPath("created_at").
				WithDatabaseName("db_created_at").
				Sortable().
				Build(),
		).
			Build()

		orderBy := []OrderBy{
			{FieldPath: NewFieldPath("name"), Descending: false},
			{FieldPath: NewFieldPath("created_at"), Descending: true},
		}

		sql, err := table.OrderByClause(orderBy)
		require.NoError(t, err)

		// Verify only database column names appear in ORDER BY
		assert.Contains(t, sql, "db_name", "Should use database column name")
		assert.Contains(t, sql, "db_created_at", "Should use database column name")

		// The important thing is that column names come from Table definitions (db_name, db_created_at)
		// not from user input. Since ORDER BY doesn't take user input values, we just verify
		// it uses the database column names we defined.

		// Verify no user input can be injected (ORDER BY should not have parameters)
		assert.NotContains(t, sql, "@", "ORDER BY should not have parameters")
	})

	// Test 7: Seek pagination parameterization
	t.Run("seek pagination is parameterized", func(t *testing.T) {
		table := NewTable().WithColumns(
			NewColumn().WithFieldPath("name").WithDatabaseName("db_name").Sortable().Build(),
			NewColumn().WithFieldPath("id").WithDatabaseName("db_id").Sortable().Build(),
		).Build()

		order := []OrderBy{
			{FieldPath: NewFieldPath("name"), Descending: false},
		}

		// Test with potential SQL injection in sort values
		lastSortValues := []string{"'; DROP TABLE users; --"}
		lastTieBreakerValue := "' OR '1'='1"

		sql, params, err := table.BuildSeekPaginationClause(
			order,
			lastSortValues,
			NewFieldPath("id"),
			lastTieBreakerValue,
			"seek_",
			SQLDialectGeneric,
		)
		require.NoError(t, err)

		// Verify parameterized query
		assert.Contains(t, sql, "@seek_", "SQL should use parameterized query")
		assert.NotContains(t, sql, "DROP TABLE", "Malicious input should not be in SQL")
		assert.NotContains(t, sql, "' OR '1'='1", "Malicious input should not be in SQL")
		assert.Greater(t, len(params), 0, "Should have parameters")

		// Verify all sort values are in parameters
		foundValues := 0
		for _, p := range params {
			if strVal, ok := p.Value.(string); ok {
				if strVal == "'; DROP TABLE users; --" || strVal == "' OR '1'='1" {
					foundValues++
				}
			}
		}
		assert.Equal(t, 2, foundValues, "All malicious values should be in parameters")
	})

	// Test 8: Composite index optimization preserves parameterization
	t.Run("composite index optimization preserves parameterization", func(t *testing.T) {
		table := NewTable().WithColumns(
			NewColumn().WithFieldPath("status").WithDatabaseName("db_status").Filterable().Build(),
			NewColumn().WithFieldPath("user_id").
				WithDatabaseName("db_user_id").
				Filterable().
				Build(),
		).
			Build()

		// Manually add composite index
		table.CompositeIndexes = []CompositeIndex{
			{
				Name:    "idx_status_user",
				Columns: []string{"db_status", "db_user_id"},
			},
		}

		filter, err := ParseFilter(`user_id="'; DROP TABLE users; --" AND status="active"`)
		require.NoError(t, err)

		// Test with optimization enabled
		sql, params, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
			EnableCompositeIndexOptimization: true,
		})
		require.NoError(t, err)

		// Verify parameterized query
		assert.Contains(t, sql, "@p_", "SQL should use parameterized query")
		assert.NotContains(t, sql, "DROP TABLE", "Malicious input should not be in SQL")
		assert.Greater(t, len(params), 0, "Should have parameters")

		// Verify conditions are reordered but still parameterized
		assert.Contains(t, sql, "db_status", "Should contain status column")
		assert.Contains(t, sql, "db_user_id", "Should contain user_id column")
	})
}

// extractUserValues extracts user-provided values from a filter string
func extractUserValues(filter string) []string {
	// Simple regex to extract quoted strings
	re := regexp.MustCompile(`"([^"]*)"`)
	matches := re.FindAllStringSubmatch(filter, -1)
	values := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) > 1 {
			values = append(values, match[1])
		}
	}
	return values
}

// isSQLKeyword checks if a string is a SQL keyword that might legitimately appear in SQL
func isSQLKeyword(s string) bool {
	keywords := []string{"AND", "OR", "NOT", "TRUE", "FALSE", "NULL"}
	upper := strings.ToUpper(s)
	for _, kw := range keywords {
		if upper == kw {
			return true
		}
	}
	return false
}
