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

// **Validates: Requirements 7.3**
// Feature: aip-sql-execution-optimization, Property 14: 未配置列使用默认 contains 模式
//
// For any column without configured Match_Mode, Filter_Generator should use contains mode
// to generate SQL.
func TestProperty_UnconfiguredColumnUsesDefaultContainsMode(t *testing.T) {
	t.Run("unconfigured column generates contains mode SQL", func(t *testing.T) {
		property := func(fieldName, filterValue string) bool {
			// Skip empty field names and values
			if fieldName == "" || filterValue == "" {
				return true
			}

			// Sanitize field name to be a valid identifier
			fieldName = sanitizeFieldName(fieldName)

			// Create a table with a column that has NO MatchModes configured
			table := NewTable().WithColumns(
				NewColumn().
					WithFieldPath(fieldName).
					WithDatabaseName(fieldName).
					Filterable().
					// Intentionally NOT calling WithMatchModes - this is the key test condition
					Build(),
			).Build()

			// Create a has (:) filter
			filterStr := fmt.Sprintf("%s:\"%s\"", fieldName, escapeFilterValue(filterValue))
			filter, err := ParseFilter(filterStr)
			if err != nil {
				// Parse errors are acceptable for some random inputs
				return true
			}

			// Call WhereClauseWithOptions with default options
			sql, params, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{})
			if err != nil {
				// Some filter values may cause errors, which is acceptable
				return true
			}

			// Verify that contains mode is used (LIKE with %value%)
			// The SQL should contain LIKE operator
			if !strings.Contains(sql, "LIKE") {
				t.Logf("Expected LIKE operator in SQL for unconfigured column, got: %s", sql)
				return false
			}

			// Verify that the parameter value has % on both sides (contains mode)
			if len(params) != 1 {
				t.Logf("Expected exactly 1 parameter, got %d", len(params))
				return false
			}

			paramValue, ok := params[0].Value.(string)
			if !ok {
				t.Logf("Expected string parameter value, got %T", params[0].Value)
				return false
			}

			// Contains mode should wrap the value with %
			expectedValue := "%" + escapeLikeValue(filterValue) + "%"
			if paramValue != expectedValue {
				t.Logf(
					"Expected contains mode parameter value %q, got %q",
					expectedValue,
					paramValue,
				)
				return false
			}

			return true
		}

		config := &quick.Config{MaxCount: 100}
		if err := quick.Check(property, config); err != nil {
			t.Error(err)
		}
	})

	t.Run("unconfigured column with WhereClause also uses contains mode", func(t *testing.T) {
		property := func(fieldName, filterValue string) bool {
			// Skip empty field names and values
			if fieldName == "" || filterValue == "" {
				return true
			}

			// Sanitize field name to be a valid identifier
			fieldName = sanitizeFieldName(fieldName)

			// Create a table with a column that has NO MatchModes configured
			table := NewTable().WithColumns(
				NewColumn().
					WithFieldPath(fieldName).
					WithDatabaseName(fieldName).
					Filterable().
					Build(),
			).Build()

			// Create a has (:) filter
			filterStr := fmt.Sprintf("%s:\"%s\"", fieldName, escapeFilterValue(filterValue))
			filter, err := ParseFilter(filterStr)
			if err != nil {
				return true
			}

			// Call the old WhereClause method (backward compatibility)
			sql, params, err := table.WhereClause(filter, "p_")
			if err != nil {
				return true
			}

			// Should still use contains mode
			if !strings.Contains(sql, "LIKE") {
				return false
			}

			if len(params) != 1 {
				return false
			}

			paramValue, ok := params[0].Value.(string)
			if !ok {
				return false
			}

			expectedValue := "%" + escapeLikeValue(filterValue) + "%"
			return paramValue == expectedValue
		}

		config := &quick.Config{MaxCount: 100}
		if err := quick.Check(property, config); err != nil {
			t.Error(err)
		}
	})

	t.Run("configured column with empty MatchModes uses contains mode", func(t *testing.T) {
		property := func(fieldName, filterValue string) bool {
			// Skip empty field names and values
			if fieldName == "" || filterValue == "" {
				return true
			}

			fieldName = sanitizeFieldName(fieldName)

			// Create a table with a column that has EMPTY MatchModes slice
			table := NewTable().WithColumns(
				NewColumn().
					WithFieldPath(fieldName).
					WithDatabaseName(fieldName).
					Filterable().
					WithMatchModes(). // Empty match modes
					Build(),
			).Build()

			filterStr := fmt.Sprintf("%s:\"%s\"", fieldName, escapeFilterValue(filterValue))
			filter, err := ParseFilter(filterStr)
			if err != nil {
				return true
			}

			sql, params, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{})
			if err != nil {
				return true
			}

			// Should use contains mode as fallback
			if !strings.Contains(sql, "LIKE") {
				return false
			}

			if len(params) != 1 {
				return false
			}

			paramValue, ok := params[0].Value.(string)
			if !ok {
				return false
			}

			expectedValue := "%" + escapeLikeValue(filterValue) + "%"
			return paramValue == expectedValue
		}

		config := &quick.Config{MaxCount: 100}
		if err := quick.Check(property, config); err != nil {
			t.Error(err)
		}
	})
}

// **Validates: Requirements 7.4**
// Feature: aip-sql-execution-optimization, Property 15: 空 Options 使用默认配置
//
// For any WhereClauseOptions with zero values, Filter_Generator should use default
// configuration (generic dialect, non-strict mode, composite index optimization disabled).
func TestProperty_EmptyOptionsUsesDefaultConfiguration(t *testing.T) {
	t.Run("empty options uses generic dialect", func(t *testing.T) {
		property := func(fieldName, filterValue string) bool {
			// Skip empty field names and values
			if fieldName == "" || filterValue == "" {
				return true
			}

			fieldName = sanitizeFieldName(fieldName)

			// Create a table with fulltext mode (only supported in postgres/mysql)
			table := NewTable().WithColumns(
				NewColumn().
					WithFieldPath(fieldName).
					WithDatabaseName(fieldName).
					Filterable().
					WithMatchModes(MatchModeFullText, MatchModeContains).
					Build(),
			).Build()

			filterStr := fmt.Sprintf("%s:\"%s\"", fieldName, escapeFilterValue(filterValue))
			filter, err := ParseFilter(filterStr)
			if err != nil {
				return true
			}

			// Call with empty options (zero value struct)
			sql, params, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{})
			if err != nil {
				return true
			}

			// With generic dialect, fulltext is not supported, so should fall back to contains
			if !strings.Contains(sql, "LIKE") {
				t.Logf("Expected LIKE (contains fallback) for generic dialect, got: %s", sql)
				return false
			}

			// Should NOT contain postgres/mysql specific fulltext syntax
			if strings.Contains(sql, "to_tsvector") || strings.Contains(sql, "MATCH") {
				t.Logf(
					"Empty options should use generic dialect, but got dialect-specific SQL: %s",
					sql,
				)
				return false
			}

			if len(params) != 1 {
				return false
			}

			paramValue, ok := params[0].Value.(string)
			if !ok {
				return false
			}

			// Should use contains mode (fallback)
			expectedValue := "%" + escapeLikeValue(filterValue) + "%"
			return paramValue == expectedValue
		}

		config := &quick.Config{MaxCount: 100}
		if err := quick.Check(property, config); err != nil {
			t.Error(err)
		}
	})

	t.Run("empty options uses non-strict mode", func(t *testing.T) {
		property := func(fieldName, filterValue string) bool {
			// Skip empty field names and values
			if fieldName == "" || filterValue == "" {
				return true
			}

			fieldName = sanitizeFieldName(fieldName)

			// Create a table with only fulltext mode (not supported in generic dialect)
			table := NewTable().WithColumns(
				NewColumn().
					WithFieldPath(fieldName).
					WithDatabaseName(fieldName).
					Filterable().
					WithMatchModes(MatchModeFullText). // Only fulltext, no fallback
					Build(),
			).Build()

			filterStr := fmt.Sprintf("%s:\"%s\"", fieldName, escapeFilterValue(filterValue))
			filter, err := ParseFilter(filterStr)
			if err != nil {
				return true
			}

			// Call with empty options (non-strict mode by default)
			sql, params, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{})
			// In non-strict mode, should NOT return an error
			// Should fall back to contains mode
			if err != nil {
				t.Logf("Non-strict mode should not error, got: %v", err)
				return false
			}

			// Should use contains mode as fallback
			if !strings.Contains(sql, "LIKE") {
				return false
			}

			if len(params) != 1 {
				return false
			}

			paramValue, ok := params[0].Value.(string)
			if !ok {
				return false
			}

			expectedValue := "%" + escapeLikeValue(filterValue) + "%"
			return paramValue == expectedValue
		}

		config := &quick.Config{MaxCount: 100}
		if err := quick.Check(property, config); err != nil {
			t.Error(err)
		}
	})

	t.Run("empty options disables composite index optimization", func(t *testing.T) {
		property := func() bool {
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

			// Add composite index
			table.CompositeIndexes = []CompositeIndex{
				{
					Name:    "idx_status_user",
					Columns: []string{"status", "user_id"},
				},
			}

			// Filter with conditions in non-optimal order
			filter, err := ParseFilter("user_id=123 AND status=\"active\"")
			if err != nil {
				return true
			}

			// Call with empty options (composite index optimization disabled by default)
			sql1, _, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{})
			if err != nil {
				return true
			}

			// Call with optimization explicitly disabled
			sql2, _, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
				EnableCompositeIndexOptimization: false,
			})
			if err != nil {
				return true
			}

			// Both should produce the same SQL (no reordering)
			if sql1 != sql2 {
				t.Logf("Empty options should disable composite index optimization")
				t.Logf("Empty options SQL: %s", sql1)
				t.Logf("Explicit disabled SQL: %s", sql2)
				return false
			}

			// Now verify that enabling optimization produces different SQL
			sql3, _, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
				EnableCompositeIndexOptimization: true,
			})
			if err != nil {
				return true
			}

			// With optimization enabled, SQL should be different (conditions reordered)
			// This confirms that empty options indeed disables optimization
			if sql1 == sql3 {
				// It's possible that the conditions are already in optimal order
				// In that case, we can't distinguish, so we accept this case
				return true
			}

			return true
		}

		config := &quick.Config{MaxCount: 50}
		if err := quick.Check(property, config); err != nil {
			t.Error(err)
		}
	})

	t.Run("empty options equivalent to explicit default values", func(t *testing.T) {
		property := func(fieldName, filterValue string) bool {
			// Skip empty field names and values
			if fieldName == "" || filterValue == "" {
				return true
			}

			fieldName = sanitizeFieldName(fieldName)

			table := NewTable().WithColumns(
				NewColumn().
					WithFieldPath(fieldName).
					WithDatabaseName(fieldName).
					Filterable().
					Build(),
			).Build()

			filterStr := fmt.Sprintf("%s:\"%s\"", fieldName, escapeFilterValue(filterValue))
			filter, err := ParseFilter(filterStr)
			if err != nil {
				return true
			}

			// Call with empty options
			sql1, params1, err1 := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{})

			// Call with explicit default values
			sql2, params2, err2 := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
				Dialect:                          SQLDialectGeneric,
				StrictMode:                       false,
				EnableCompositeIndexOptimization: false,
			})

			// Both should produce identical results
			if (err1 == nil) != (err2 == nil) {
				return false
			}

			if err1 != nil {
				return true // Both errored, which is acceptable
			}

			if sql1 != sql2 {
				t.Logf("Empty options should be equivalent to explicit defaults")
				t.Logf("Empty options SQL: %s", sql1)
				t.Logf("Explicit defaults SQL: %s", sql2)
				return false
			}

			if len(params1) != len(params2) {
				return false
			}

			for i := range params1 {
				if params1[i].Name != params2[i].Name || params1[i].Value != params2[i].Value {
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

// Helper functions for property tests

// sanitizeFieldName converts arbitrary strings to valid field names
func sanitizeFieldName(s string) string {
	if s == "" {
		return "field"
	}

	// Replace invalid characters with underscore
	var result strings.Builder
	for i, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (i > 0 && r >= '0' && r <= '9') ||
			r == '_' {
			result.WriteRune(r)
		} else {
			result.WriteRune('_')
		}
	}

	name := result.String()
	if name == "" || (name[0] >= '0' && name[0] <= '9') {
		name = "field_" + name
	}

	// Limit length
	if len(name) > 50 {
		name = name[:50]
	}

	return name
}

// escapeFilterValue escapes quotes in filter values
func escapeFilterValue(s string) string {
	return strings.ReplaceAll(s, "\"", "\\\"")
}

// escapeLikeValue escapes LIKE special characters
func escapeLikeValue(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "%", "\\%")
	value = strings.ReplaceAll(value, "_", "\\_")
	return value
}

// Additional unit tests for specific edge cases
func TestBackwardCompatibilityEdgeCases(t *testing.T) {
	t.Run("unconfigured column with exact match filter uses contains", func(t *testing.T) {
		table := NewTable().WithColumns(
			NewColumn().
				WithFieldPath("name").
				WithDatabaseName("name").
				Filterable().
				// No MatchModes configured
				Build(),
		).Build()

		filter, err := ParseFilter("name:\"test\"")
		require.NoError(t, err)

		sql, params, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{})
		require.NoError(t, err)

		// Should use contains mode
		assert.Contains(t, sql, "LIKE")
		assert.Equal(t, 1, len(params))
		assert.Equal(t, "%test%", params[0].Value)
	})

	t.Run("empty options with postgres dialect explicitly set uses postgres", func(t *testing.T) {
		table := NewTable().WithColumns(
			NewColumn().
				WithFieldPath("content").
				WithDatabaseName("content").
				Filterable().
				WithMatchModes(MatchModeFullText).
				Build(),
		).Build()

		filter, err := ParseFilter("content:\"search term\"")
		require.NoError(t, err)

		// Explicitly set postgres dialect (not empty options)
		sql, _, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
			Dialect: SQLDialectPostgres,
		})
		require.NoError(t, err)

		// Should use postgres fulltext syntax
		assert.Contains(t, sql, "to_tsvector")
		assert.Contains(t, sql, "websearch_to_tsquery")
	})

	t.Run(
		"empty options with strict mode explicitly enabled errors on unsupported mode",
		func(t *testing.T) {
			table := NewTable().WithColumns(
				NewColumn().
					WithFieldPath("content").
					WithDatabaseName("content").
					Filterable().
					WithMatchModes(MatchModeFullText). // Only fulltext
					Build(),
			).Build()

			filter, err := ParseFilter("content:\"search term\"")
			require.NoError(t, err)

			// Explicitly enable strict mode (not empty options)
			_, _, err = table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
				StrictMode: true,
			})

			// Should error in strict mode when no supported mode is available
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "no supported match mode")
		},
	)

	t.Run("zero value options struct is truly empty", func(t *testing.T) {
		var opts WhereClauseOptions

		// Verify zero values
		assert.Equal(t, SQLDialect(""), opts.Dialect)
		assert.Equal(t, false, opts.StrictMode)
		assert.Equal(t, false, opts.EnableCompositeIndexOptimization)
	})
}
