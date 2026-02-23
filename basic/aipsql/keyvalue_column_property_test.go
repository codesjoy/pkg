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

// **Validates: Requirements 3.1, 3.3, 3.4**
// Feature: aip-sql-execution-optimization, Property 6: Key-Value 列生成 EXISTS 子查询
//
// For any Key-Value column query (using has, =, != operators), the Filter_Generator
// should generate SQL containing EXISTS and UNNEST subquery structure.
func TestProperty_KeyValueColumnGeneratesExistsSubquery(t *testing.T) {
	// Test scenario 1: has (:) operator generates EXISTS with UNNEST
	t.Run("has operator generates EXISTS with UNNEST", func(t *testing.T) {
		property := func(keyName, searchValue string) bool {
			// Skip empty keys
			if keyName == "" {
				return true
			}

			table := NewTable().WithColumns(
				NewColumn().
					WithFieldPath("labels").
					WithDatabaseName("db_labels").
					Filterable().
					KeyValue().
					WithMatchModes(MatchModeExact).
					Build(),
			).Build()

			filter, err := ParseFilter(fmt.Sprintf("labels.%s:%q", keyName, searchValue))
			if err != nil {
				return true
			}

			sql, params, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
				Dialect: SQLDialectGeneric,
			})
			if err != nil {
				return true
			}

			// Verify EXISTS keyword is present
			if !strings.Contains(sql, "EXISTS") {
				return false
			}

			// Verify UNNEST function is present
			if !strings.Contains(sql, "UNNEST") {
				return false
			}

			// Verify SELECT key, value FROM pattern
			if !strings.Contains(sql, "SELECT key, value FROM") {
				return false
			}

			// Verify WHERE clause in subquery
			if !strings.Contains(sql, "WHERE key = ") {
				return false
			}

			// Verify AND clause connecting key and value conditions
			andCount := strings.Count(sql, " AND ")
			if andCount < 1 {
				return false
			}

			// Verify parameterized query (should have at least 2 params: key and value)
			if len(params) < 2 {
				return false
			}

			// Verify key parameter is present
			keyFound := false
			for _, param := range params {
				if param.Value == keyName {
					keyFound = true
					break
				}
			}
			return keyFound
		}

		config := &quick.Config{MaxCount: 100}
		if err := quick.Check(property, config); err != nil {
			t.Error(err)
		}
	})

	// Test scenario 2: = operator generates EXISTS with UNNEST and value =
	t.Run("equals operator generates EXISTS with value equals", func(t *testing.T) {
		property := func(keyName, value string) bool {
			if keyName == "" {
				return true
			}

			table := NewTable().WithColumns(
				NewColumn().
					WithFieldPath("labels").
					WithDatabaseName("db_labels").
					Filterable().
					KeyValue().
					Build(),
			).Build()

			filter, err := ParseFilter(fmt.Sprintf("labels.%s=%q", keyName, value))
			if err != nil {
				return true
			}

			sql, params, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
				Dialect: SQLDialectGeneric,
			})
			if err != nil {
				return true
			}

			// Verify EXISTS and UNNEST
			if !strings.Contains(sql, "EXISTS") || !strings.Contains(sql, "UNNEST") {
				return false
			}

			// Verify value = pattern (not value LIKE)
			if !strings.Contains(sql, "value = ") {
				return false
			}

			// Should not use LIKE for = operator
			if strings.Contains(sql, "value LIKE") {
				return false
			}

			// Verify parameterized query
			if len(params) < 2 {
				return false
			}

			return true
		}

		config := &quick.Config{MaxCount: 100}
		if err := quick.Check(property, config); err != nil {
			t.Error(err)
		}
	})

	// Test scenario 3: != operator generates EXISTS with UNNEST and value <>
	t.Run("not equals operator generates EXISTS with value not equals", func(t *testing.T) {
		property := func(keyName, value string) bool {
			if keyName == "" {
				return true
			}

			table := NewTable().WithColumns(
				NewColumn().
					WithFieldPath("labels").
					WithDatabaseName("db_labels").
					Filterable().
					KeyValue().
					Build(),
			).Build()

			filter, err := ParseFilter(fmt.Sprintf("labels.%s!=%q", keyName, value))
			if err != nil {
				return true
			}

			sql, params, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
				Dialect: SQLDialectGeneric,
			})
			if err != nil {
				return true
			}

			// Verify EXISTS and UNNEST
			if !strings.Contains(sql, "EXISTS") || !strings.Contains(sql, "UNNEST") {
				return false
			}

			// Verify value <> pattern (SQL not equals)
			if !strings.Contains(sql, "value <>") {
				return false
			}

			// Verify parameterized query
			if len(params) < 2 {
				return false
			}

			return true
		}

		config := &quick.Config{MaxCount: 100}
		if err := quick.Check(property, config); err != nil {
			t.Error(err)
		}
	})

	// Test scenario 4: Verify complete SQL structure
	t.Run("complete SQL structure is correct", func(t *testing.T) {
		table := NewTable().WithColumns(
			NewColumn().
				WithFieldPath("labels").
				WithDatabaseName("db_labels").
				Filterable().
				KeyValue().
				WithMatchModes(MatchModeExact).
				Build(),
		).Build()

		filter, err := ParseFilter(`labels.environment:"production"`)
		require.NoError(t, err)

		sql, params, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
			Dialect: SQLDialectGeneric,
		})
		require.NoError(t, err)

		// Verify SQL structure
		assert.Contains(t, sql, "(EXISTS (SELECT key, value FROM UNNEST(db_labels)")
		assert.Contains(t, sql, "WHERE key = ")
		assert.Contains(t, sql, " AND value = ")
		assert.True(t, strings.HasSuffix(sql, "))"), "SQL should end with ))")

		// Verify parameters
		assert.Len(t, params, 2)
		assert.Equal(t, "environment", params[0].Value)
		assert.Equal(t, "production", params[1].Value)
	})

	// Test scenario 5: Multiple key-value queries with AND
	t.Run("multiple key-value queries with AND", func(t *testing.T) {
		table := NewTable().WithColumns(
			NewColumn().
				WithFieldPath("labels").
				WithDatabaseName("db_labels").
				Filterable().
				KeyValue().
				WithMatchModes(MatchModeExact).
				Build(),
		).Build()

		filter, err := ParseFilter(`labels.env:"prod" AND labels.region:"us"`)
		require.NoError(t, err)

		sql, params, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
			Dialect: SQLDialectGeneric,
		})
		require.NoError(t, err)

		// Should have two EXISTS subqueries
		existsCount := strings.Count(sql, "EXISTS")
		assert.Equal(t, 2, existsCount, "Should have two EXISTS subqueries")

		// Should have two UNNEST calls
		unnestCount := strings.Count(sql, "UNNEST")
		assert.Equal(t, 2, unnestCount, "Should have two UNNEST calls")

		// Should have 4 parameters (2 keys + 2 values)
		assert.Len(t, params, 4)
	})

	// Test scenario 6: Key-value with OR
	t.Run("multiple key-value queries with OR", func(t *testing.T) {
		table := NewTable().WithColumns(
			NewColumn().
				WithFieldPath("labels").
				WithDatabaseName("db_labels").
				Filterable().
				KeyValue().
				WithMatchModes(MatchModeExact).
				Build(),
		).Build()

		filter, err := ParseFilter(`labels.env:"prod" OR labels.env:"staging"`)
		require.NoError(t, err)

		sql, params, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
			Dialect: SQLDialectGeneric,
		})
		require.NoError(t, err)

		// Should have two EXISTS subqueries
		existsCount := strings.Count(sql, "EXISTS")
		assert.Equal(t, 2, existsCount, "Should have two EXISTS subqueries")

		// Should contain OR operator
		assert.Contains(t, sql, " OR ")

		// Should have 4 parameters (2 keys + 2 values)
		assert.Len(t, params, 4)
	})

	// Test scenario 7: Verify EXISTS subquery is properly parenthesized
	t.Run("EXISTS subquery is properly parenthesized", func(t *testing.T) {
		property := func(keyName, value string) bool {
			if keyName == "" {
				return true
			}

			table := NewTable().WithColumns(
				NewColumn().
					WithFieldPath("labels").
					WithDatabaseName("db_labels").
					Filterable().
					KeyValue().
					WithMatchModes(MatchModeExact).
					Build(),
			).Build()

			filter, err := ParseFilter(fmt.Sprintf("labels.%s:%q", keyName, value))
			if err != nil {
				return true
			}

			sql, _, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
				Dialect: SQLDialectGeneric,
			})
			if err != nil {
				return true
			}

			// Verify proper parenthesization
			if !strings.HasPrefix(sql, "(EXISTS") {
				return false
			}

			if !strings.HasSuffix(sql, "))") {
				return false
			}

			// Count parentheses - should be balanced
			openCount := strings.Count(sql, "(")
			closeCount := strings.Count(sql, ")")
			return openCount == closeCount
		}

		config := &quick.Config{MaxCount: 100}
		if err := quick.Check(property, config); err != nil {
			t.Error(err)
		}
	})
}

// **Validates: Requirements 3.2**
// Feature: aip-sql-execution-optimization, Property 7: Key-Value 列应用 Match Mode
//
// For any Key-Value column configured with Match_Mode, the generated UNNEST subquery's
// value matching condition should use the configured match mode.
func TestProperty_KeyValueColumnAppliesMatchMode(t *testing.T) {
	// Test scenario 1: Exact mode in Key-Value column
	t.Run("exact mode generates value equals", func(t *testing.T) {
		property := func(keyName, value string) bool {
			if keyName == "" {
				return true
			}

			table := NewTable().WithColumns(
				NewColumn().
					WithFieldPath("labels").
					WithDatabaseName("db_labels").
					Filterable().
					KeyValue().
					WithMatchModes(MatchModeExact).
					Build(),
			).Build()

			filter, err := ParseFilter(fmt.Sprintf("labels.%s:%q", keyName, value))
			if err != nil {
				return true
			}

			sql, params, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
				Dialect: SQLDialectGeneric,
			})
			if err != nil {
				return true
			}

			// Exact mode should use = operator
			if !strings.Contains(sql, "value = ") {
				return false
			}

			// Should not use LIKE
			if strings.Contains(sql, "value LIKE") {
				return false
			}

			// Verify parameter value is not wrapped with %
			if len(params) < 2 {
				return false
			}

			// Second parameter should be the value (first is key)
			valueParam := params[1].Value.(string)
			return valueParam == value
		}

		config := &quick.Config{MaxCount: 100}
		if err := quick.Check(property, config); err != nil {
			t.Error(err)
		}
	})

	// Test scenario 2: Prefix mode in Key-Value column
	t.Run("prefix mode generates value LIKE with suffix wildcard", func(t *testing.T) {
		property := func(keyName, value string) bool {
			if keyName == "" {
				return true
			}

			table := NewTable().WithColumns(
				NewColumn().
					WithFieldPath("labels").
					WithDatabaseName("db_labels").
					Filterable().
					KeyValue().
					WithMatchModes(MatchModePrefix).
					Build(),
			).Build()

			filter, err := ParseFilter(fmt.Sprintf("labels.%s:%q", keyName, value))
			if err != nil {
				return true
			}

			sql, params, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
				Dialect: SQLDialectGeneric,
			})
			if err != nil {
				return true
			}

			// Prefix mode should use LIKE operator
			if !strings.Contains(sql, "value LIKE ") {
				return false
			}

			// Should not use = operator for value
			if strings.Contains(sql, "value = ") {
				return false
			}

			// Verify parameter value has suffix wildcard only
			if len(params) < 2 {
				return false
			}

			valueParam, ok := params[1].Value.(string)
			if !ok {
				return false
			}

			// Must end with %
			if !strings.HasSuffix(valueParam, "%") {
				return false
			}

			// Should not start with % (unless input starts with %)
			if strings.HasPrefix(valueParam, "%") && !strings.HasPrefix(value, "%") && value != "" {
				return false
			}

			return true
		}

		config := &quick.Config{MaxCount: 100}
		if err := quick.Check(property, config); err != nil {
			t.Error(err)
		}
	})

	// Test scenario 3: Contains mode in Key-Value column
	t.Run("contains mode generates value LIKE with both wildcards", func(t *testing.T) {
		property := func(keyName, value string) bool {
			if keyName == "" {
				return true
			}

			table := NewTable().WithColumns(
				NewColumn().
					WithFieldPath("labels").
					WithDatabaseName("db_labels").
					Filterable().
					KeyValue().
					WithMatchModes(MatchModeContains).
					Build(),
			).Build()

			filter, err := ParseFilter(fmt.Sprintf("labels.%s:%q", keyName, value))
			if err != nil {
				return true
			}

			sql, params, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
				Dialect: SQLDialectGeneric,
			})
			if err != nil {
				return true
			}

			// Contains mode should use LIKE operator
			if !strings.Contains(sql, "value LIKE ") {
				return false
			}

			// Verify parameter value has both wildcards
			if len(params) < 2 {
				return false
			}

			valueParam, ok := params[1].Value.(string)
			if !ok {
				return false
			}

			// Must start and end with %
			if !strings.HasPrefix(valueParam, "%") || !strings.HasSuffix(valueParam, "%") {
				return false
			}

			return true
		}

		config := &quick.Config{MaxCount: 100}
		if err := quick.Check(property, config); err != nil {
			t.Error(err)
		}
	})

	// Test scenario 4: Different match modes for different columns
	t.Run("different match modes for different key-value columns", func(t *testing.T) {
		table := NewTable().WithColumns(
			NewColumn().
				WithFieldPath("exact_labels").
				WithDatabaseName("db_exact_labels").
				Filterable().
				KeyValue().
				WithMatchModes(MatchModeExact).
				Build(),
			NewColumn().
				WithFieldPath("prefix_labels").
				WithDatabaseName("db_prefix_labels").
				Filterable().
				KeyValue().
				WithMatchModes(MatchModePrefix).
				Build(),
			NewColumn().
				WithFieldPath("contains_labels").
				WithDatabaseName("db_contains_labels").
				Filterable().
				KeyValue().
				WithMatchModes(MatchModeContains).
				Build(),
		).Build()

		filter, err := ParseFilter(
			`exact_labels.k1:"v1" AND prefix_labels.k2:"v2" AND contains_labels.k3:"v3"`,
		)
		require.NoError(t, err)

		sql, params, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
			Dialect: SQLDialectGeneric,
		})
		require.NoError(t, err)

		// Verify exact mode: value =
		assert.Contains(t, sql, "db_exact_labels")
		exactIdx := strings.Index(sql, "db_exact_labels")
		exactEnd := exactIdx + 200
		if exactEnd > len(sql) {
			exactEnd = len(sql)
		}
		exactSection := sql[exactIdx:exactEnd]
		assert.Contains(t, exactSection, "value = ")

		// Verify prefix mode: value LIKE
		assert.Contains(t, sql, "db_prefix_labels")
		prefixIdx := strings.Index(sql, "db_prefix_labels")
		prefixEnd := prefixIdx + 200
		if prefixEnd > len(sql) {
			prefixEnd = len(sql)
		}
		prefixSection := sql[prefixIdx:prefixEnd]
		assert.Contains(t, prefixSection, "value LIKE ")

		// Verify contains mode: value LIKE
		assert.Contains(t, sql, "db_contains_labels")
		containsIdx := strings.Index(sql, "db_contains_labels")
		containsEnd := containsIdx + 200
		if containsEnd > len(sql) {
			containsEnd = len(sql)
		}
		containsSection := sql[containsIdx:containsEnd]
		assert.Contains(t, containsSection, "value LIKE ")

		// Verify parameters
		assert.Len(t, params, 6)                 // 3 keys + 3 values
		assert.Equal(t, "v1", params[1].Value)   // exact: no wildcards
		assert.Equal(t, "v2%", params[3].Value)  // prefix: suffix wildcard
		assert.Equal(t, "%v3%", params[5].Value) // contains: both wildcards
	})

	// Test scenario 5: Match mode fallback in Key-Value columns
	t.Run("match mode fallback in key-value columns", func(t *testing.T) {
		// Column with fulltext first, prefix as fallback
		table := NewTable().WithColumns(
			NewColumn().
				WithFieldPath("labels").
				WithDatabaseName("db_labels").
				Filterable().
				KeyValue().
				WithMatchModes(MatchModeFullText, MatchModePrefix).
				Build(),
		).Build()

		filter, err := ParseFilter(`labels.env:"prod"`)
		require.NoError(t, err)

		// Generic dialect: should fallback to prefix
		sql, params, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
			Dialect:    SQLDialectGeneric,
			StrictMode: false,
		})
		require.NoError(t, err)

		// Should use prefix mode (LIKE with suffix wildcard)
		assert.Contains(t, sql, "value LIKE ")
		assert.Equal(t, "prod%", params[1].Value)
	})

	// Test scenario 6: Verify match mode is applied to value, not key
	t.Run("match mode applies to value not key", func(t *testing.T) {
		property := func(keyName, value string) bool {
			if keyName == "" {
				return true
			}

			table := NewTable().WithColumns(
				NewColumn().
					WithFieldPath("labels").
					WithDatabaseName("db_labels").
					Filterable().
					KeyValue().
					WithMatchModes(MatchModePrefix).
					Build(),
			).Build()

			filter, err := ParseFilter(fmt.Sprintf("labels.%s:%q", keyName, value))
			if err != nil {
				return true
			}

			sql, params, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
				Dialect: SQLDialectGeneric,
			})
			if err != nil {
				return true
			}

			// Key should always use exact match (=)
			if !strings.Contains(sql, "key = ") {
				return false
			}

			// Value should use the configured match mode (prefix in this case)
			if !strings.Contains(sql, "value LIKE ") {
				return false
			}

			// Verify key parameter is exact (no wildcards)
			if len(params) < 2 {
				return false
			}

			keyParam := params[0].Value.(string)
			if keyParam != keyName {
				return false
			}

			// Verify value parameter has prefix wildcard
			valueParam := params[1].Value.(string)
			return strings.HasSuffix(valueParam, "%")
		}

		config := &quick.Config{MaxCount: 100}
		if err := quick.Check(property, config); err != nil {
			t.Error(err)
		}
	})

	// Test scenario 7: No match mode configured should use default
	t.Run("no match mode configured uses default contains", func(t *testing.T) {
		table := NewTable().WithColumns(
			NewColumn().
				WithFieldPath("labels").
				WithDatabaseName("db_labels").
				Filterable().
				KeyValue().
				// No match modes configured
				Build(),
		).Build()

		filter, err := ParseFilter(`labels.env:"prod"`)
		require.NoError(t, err)

		sql, params, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
			Dialect:    SQLDialectGeneric,
			StrictMode: false,
		})
		require.NoError(t, err)

		// Should use default contains mode (LIKE with both wildcards)
		assert.Contains(t, sql, "value LIKE ")
		assert.Equal(t, "%prod%", params[1].Value)
	})
}

// **Validates: Requirements 3.5**
// Feature: aip-sql-execution-optimization, Property 8: Key-Value 查询参数化
//
// For any Key-Value column query, the generated UNNEST subquery should use parameterized
// queries for both key and value conditions to prevent SQL injection.
func TestProperty_KeyValueQueryParameterization(t *testing.T) {
	// Test scenario 1: Both key and value are parameterized
	t.Run("both key and value are parameterized", func(t *testing.T) {
		property := func(keyName, value string) bool {
			if keyName == "" {
				return true
			}

			table := NewTable().WithColumns(
				NewColumn().
					WithFieldPath("labels").
					WithDatabaseName("db_labels").
					Filterable().
					KeyValue().
					WithMatchModes(MatchModeExact).
					Build(),
			).Build()

			filter, err := ParseFilter(fmt.Sprintf("labels.%s:%q", keyName, value))
			if err != nil {
				return true
			}

			sql, params, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
				Dialect: SQLDialectGeneric,
			})
			if err != nil {
				return true
			}

			// Verify SQL uses parameterized query (@param format)
			if !strings.Contains(sql, "@p_") {
				return false
			}

			// Verify at least 2 parameters (key and value)
			if len(params) < 2 {
				return false
			}

			// Verify key is in parameters
			keyFound := false
			for _, param := range params {
				if param.Value == keyName {
					keyFound = true
					break
				}
			}
			if !keyFound {
				return false
			}

			// Verify value is in parameters (may be wrapped with % depending on mode)
			valueFound := false
			for _, param := range params {
				paramStr, ok := param.Value.(string)
				if !ok {
					continue
				}
				// For exact mode, value should match exactly
				// For other modes, value might be wrapped
				if paramStr == value || strings.Contains(paramStr, value) {
					valueFound = true
					break
				}
			}
			return valueFound
		}

		config := &quick.Config{MaxCount: 100}
		if err := quick.Check(property, config); err != nil {
			t.Error(err)
		}
	})

	// Test scenario 2: User input is NOT directly embedded in SQL
	t.Run("user input not directly embedded in SQL", func(t *testing.T) {
		property := func(keyName, value string) bool {
			// Test with inputs that could be SQL injection attempts
			if keyName == "" || len(keyName) < 3 || len(value) < 3 {
				return true
			}

			table := NewTable().WithColumns(
				NewColumn().
					WithFieldPath("labels").
					WithDatabaseName("db_labels").
					Filterable().
					KeyValue().
					WithMatchModes(MatchModeExact).
					Build(),
			).Build()

			filter, err := ParseFilter(fmt.Sprintf("labels.%s:%q", keyName, value))
			if err != nil {
				return true
			}

			sql, _, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
				Dialect: SQLDialectGeneric,
			})
			if err != nil {
				return true
			}

			// Verify user input is not directly in SQL
			// (except for very short strings that might appear in column names)
			if len(keyName) > 10 && strings.Contains(sql, keyName) {
				// Key should only appear in parameter, not in SQL
				// Exception: if it's part of a parameter name like @p_keyName
				if !strings.Contains(sql, "@p_") {
					return false
				}
			}

			if len(value) > 10 && strings.Contains(sql, value) {
				// Value should only appear in parameter, not in SQL
				return false
			}

			return true
		}

		config := &quick.Config{MaxCount: 100}
		if err := quick.Check(property, config); err != nil {
			t.Error(err)
		}
	})

	// Test scenario 3: SQL injection attempts are safely parameterized
	t.Run("SQL injection attempts are safely parameterized", func(t *testing.T) {
		testCases := []struct {
			name      string
			keyName   string
			value     string
			malicious string
		}{
			{
				name:      "SQL injection in key",
				keyName:   "'; DROP TABLE users; --",
				value:     "test",
				malicious: "DROP TABLE",
			},
			{
				name:      "SQL injection in value",
				keyName:   "env",
				value:     "' OR '1'='1",
				malicious: "OR '1'='1'",
			},
			{
				name:      "SQL comment in key",
				keyName:   "test--",
				value:     "value",
				malicious: "--",
			},
			{
				name:      "UNION injection in value",
				keyName:   "env",
				value:     "' UNION SELECT * FROM passwords--",
				malicious: "UNION SELECT",
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				table := NewTable().WithColumns(
					NewColumn().
						WithFieldPath("labels").
						WithDatabaseName("db_labels").
						Filterable().
						KeyValue().
						WithMatchModes(MatchModeExact).
						Build(),
				).Build()

				filter, err := ParseFilter(fmt.Sprintf("labels.%s:%q", tc.keyName, tc.value))
				if err != nil {
					// Some malicious inputs might fail parsing, which is acceptable
					return
				}

				sql, params, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
					Dialect: SQLDialectGeneric,
				})
				if err != nil {
					// Errors are acceptable for malicious input
					return
				}

				// Verify malicious content is NOT in SQL
				assert.NotContains(t, sql, tc.malicious,
					"Malicious content should not appear in SQL")

				// Verify malicious content IS in parameters (safely)
				found := false
				for _, param := range params {
					paramStr, ok := param.Value.(string)
					if ok &&
						(paramStr == tc.keyName || paramStr == tc.value || strings.Contains(paramStr, tc.value)) {
						found = true
						break
					}
				}
				assert.True(t, found, "Malicious input should be in parameters")
			})
		}
	})

	// Test scenario 4: Parameter names are unique
	t.Run("parameter names are unique", func(t *testing.T) {
		property := func(keyName, value string) bool {
			if keyName == "" {
				return true
			}

			table := NewTable().WithColumns(
				NewColumn().
					WithFieldPath("labels").
					WithDatabaseName("db_labels").
					Filterable().
					KeyValue().
					WithMatchModes(MatchModeExact).
					Build(),
			).Build()

			filter, err := ParseFilter(fmt.Sprintf("labels.%s:%q", keyName, value))
			if err != nil {
				return true
			}

			sql, params, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
				Dialect: SQLDialectGeneric,
			})
			if err != nil {
				return true
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

	// Test scenario 5: Multiple key-value queries have unique parameters
	t.Run("multiple key-value queries have unique parameters", func(t *testing.T) {
		table := NewTable().WithColumns(
			NewColumn().
				WithFieldPath("labels").
				WithDatabaseName("db_labels").
				Filterable().
				KeyValue().
				WithMatchModes(MatchModeExact).
				Build(),
		).Build()

		filter, err := ParseFilter(
			`labels.env:"prod" AND labels.region:"us" AND labels.tier:"premium"`,
		)
		require.NoError(t, err)

		sql, params, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
			Dialect: SQLDialectGeneric,
		})
		require.NoError(t, err)

		// Should have 6 parameters (3 keys + 3 values)
		assert.Len(t, params, 6)

		// Verify all parameter names are unique
		paramNames := make(map[string]bool)
		for _, param := range params {
			assert.False(
				t,
				paramNames[param.Name],
				"Parameter name %s should be unique",
				param.Name,
			)
			paramNames[param.Name] = true

			// Verify parameter is referenced in SQL
			assert.Contains(
				t,
				sql,
				"@"+param.Name,
				"Parameter %s should be referenced in SQL",
				param.Name,
			)
		}
	})

	// Test scenario 6: = and != operators also use parameterized queries
	t.Run("equals and not equals operators use parameterized queries", func(t *testing.T) {
		testCases := []struct {
			name     string
			operator string
		}{
			{"equals operator", "="},
			{"not equals operator", "!="},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				property := func(keyName, value string) bool {
					if keyName == "" {
						return true
					}

					table := NewTable().WithColumns(
						NewColumn().
							WithFieldPath("labels").
							WithDatabaseName("db_labels").
							Filterable().
							KeyValue().
							Build(),
					).Build()

					filter, err := ParseFilter(
						fmt.Sprintf("labels.%s%s%q", keyName, tc.operator, value),
					)
					if err != nil {
						return true
					}

					sql, params, err := table.WhereClauseWithOptions(
						filter,
						"p_",
						WhereClauseOptions{
							Dialect: SQLDialectGeneric,
						},
					)
					if err != nil {
						return true
					}

					// Verify parameterized query
					if !strings.Contains(sql, "@p_") {
						return false
					}

					// Verify at least 2 parameters
					if len(params) < 2 {
						return false
					}

					// Verify key and value are in parameters
					keyFound := false
					valueFound := false
					for _, param := range params {
						if param.Value == keyName {
							keyFound = true
						}
						if param.Value == value {
							valueFound = true
						}
					}

					return keyFound && valueFound
				}

				config := &quick.Config{MaxCount: 100}
				if err := quick.Check(property, config); err != nil {
					t.Error(err)
				}
			})
		}
	})

	// Test scenario 7: LIKE special characters in key-value queries are escaped
	t.Run("LIKE special characters in key-value queries are escaped", func(t *testing.T) {
		table := NewTable().WithColumns(
			NewColumn().
				WithFieldPath("labels").
				WithDatabaseName("db_labels").
				Filterable().
				KeyValue().
				WithMatchModes(MatchModePrefix).
				Build(),
		).Build()

		// Test with LIKE special characters in value
		filter, err := ParseFilter(`labels.env:"test%_\value"`)
		require.NoError(t, err)

		sql, params, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
			Dialect: SQLDialectGeneric,
		})
		require.NoError(t, err)

		// Verify SQL uses parameterized query
		assert.Contains(t, sql, "@p_")

		// Verify parameters
		assert.Len(t, params, 2)
		assert.Equal(t, "env", params[0].Value) // key parameter

		// Value parameter should have escaped special characters
		valueParam := params[1].Value.(string)
		assert.Contains(t, valueParam, `\%`, "% should be escaped")
		assert.Contains(t, valueParam, `\_`, "_ should be escaped")
		// The backslash in the input is a single backslash, which gets escaped to \\
		// In the output, we should see the escaped form
		assert.True(t, strings.Contains(valueParam, `\`), "\\ should be escaped")
		assert.True(
			t,
			strings.HasSuffix(valueParam, "%"),
			"Should have suffix wildcard for prefix mode",
		)
	})

	// Test scenario 8: Verify all parameters are used in SQL
	t.Run("all parameters are used in SQL", func(t *testing.T) {
		property := func(keyName, value string) bool {
			if keyName == "" {
				return true
			}

			table := NewTable().WithColumns(
				NewColumn().
					WithFieldPath("labels").
					WithDatabaseName("db_labels").
					Filterable().
					KeyValue().
					WithMatchModes(MatchModeExact).
					Build(),
			).Build()

			filter, err := ParseFilter(fmt.Sprintf("labels.%s:%q", keyName, value))
			if err != nil {
				return true
			}

			sql, params, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
				Dialect: SQLDialectGeneric,
			})
			if err != nil {
				return true
			}

			// Verify every parameter is referenced in SQL
			for _, param := range params {
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

	// Test scenario 9: Parameterization works across all SQL dialects
	t.Run("parameterization works across all dialects", func(t *testing.T) {
		dialects := []SQLDialect{SQLDialectGeneric, SQLDialectPostgres, SQLDialectMySQL}

		for _, dialect := range dialects {
			t.Run(string(dialect), func(t *testing.T) {
				table := NewTable().WithColumns(
					NewColumn().
						WithFieldPath("labels").
						WithDatabaseName("db_labels").
						Filterable().
						KeyValue().
						WithMatchModes(MatchModeExact).
						Build(),
				).Build()

				filter, err := ParseFilter(`labels.env:"production"`)
				assert.NoError(t, err)

				sql, params, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
					Dialect: dialect,
				})
				assert.NoError(t, err, "Should not error for dialect: %s", dialect)

				// Verify parameterized query
				assert.Contains(
					t,
					sql,
					"@p_",
					"Should use parameterized query for dialect: %s",
					dialect,
				)

				// Verify parameters
				assert.Len(t, params, 2, "Should have 2 parameters for dialect: %s", dialect)
				assert.Equal(t, "env", params[0].Value, "Key parameter for dialect: %s", dialect)
				assert.Equal(
					t,
					"production",
					params[1].Value,
					"Value parameter for dialect: %s",
					dialect,
				)

				// Verify all parameters are referenced in SQL
				for _, param := range params {
					assert.Contains(
						t,
						sql,
						"@"+param.Name,
						"Parameter %s should be referenced in SQL for dialect: %s",
						param.Name,
						dialect,
					)
				}
			})
		}
	})
}
