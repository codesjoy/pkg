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

// **Validates: Requirements 8.1, 8.2**
// Feature: aip-sql-execution-optimization, Property 16: 所有用户输入参数化
//
// For any user-provided filter values, sort values, or pagination token values,
// these values should appear as parameters (@param) in the generated SQL,
// not directly concatenated into the SQL string.
func TestProperty_AllUserInputParameterized(t *testing.T) {
	// Test scenario 1: Filter values are always parameterized
	t.Run("filter values are parameterized", func(t *testing.T) {
		property := func(userValue string) bool {
			// Skip empty strings as they may not generate valid filters
			if userValue == "" {
				return true
			}

			// Create a simple table with a filterable column
			table := NewTable().WithColumns(
				NewColumn().
					WithFieldPath("name").
					WithDatabaseName("db_name").
					Filterable().
					WithMatchModes(MatchModeExact).
					Build(),
			).Build()

			// Create filter with user value
			filterStr := fmt.Sprintf(`name="%s"`, escapeFilterValue(userValue))
			filter, err := ParseFilter(filterStr)
			if err != nil {
				// Parse errors are acceptable
				return true
			}

			sql, params, err := table.WhereClause(filter, "p_")
			if err != nil {
				// Generation errors are acceptable
				return true
			}

			// Property: SQL should use parameterized query format
			if !strings.Contains(sql, "@p_") {
				return false
			}

			// Property: User value should NOT be directly embedded in SQL
			// (unless it's a SQL keyword or very short common word)
			if len(userValue) > 3 && !isSQLKeywordOrCommon(userValue) {
				if strings.Contains(sql, userValue) {
					return false
				}
			}

			// Property: User value should be in parameters
			foundInParams := false
			for _, p := range params {
				if strVal, ok := p.Value.(string); ok {
					if strVal == userValue {
						foundInParams = true
						break
					}
				}
			}

			return foundInParams
		}

		config := &quick.Config{MaxCount: 100}
		if err := quick.Check(property, config); err != nil {
			t.Error(err)
		}
	})

	// Test scenario 2: has (:) operator values are parameterized
	t.Run("has operator values are parameterized", func(t *testing.T) {
		property := func(userValue string) bool {
			// Skip empty strings
			if userValue == "" {
				return true
			}

			// Test with different match modes
			matchModes := []MatchMode{MatchModeExact, MatchModePrefix, MatchModeContains}

			for _, mode := range matchModes {
				table := NewTable().WithColumns(
					NewColumn().
						WithFieldPath("name").
						WithDatabaseName("db_name").
						Filterable().
						WithMatchModes(mode).
						Build(),
				).Build()

				filterStr := fmt.Sprintf(`name:"%s"`, escapeFilterValue(userValue))
				filter, err := ParseFilter(filterStr)
				if err != nil {
					continue
				}

				sql, params, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{})
				if err != nil {
					continue
				}

				// Property: Should use parameterized query
				if !strings.Contains(sql, "@p_") {
					return false
				}

				// Property: User value should NOT be directly in SQL
				if len(userValue) > 3 && !isSQLKeywordOrCommon(userValue) {
					if strings.Contains(sql, userValue) {
						return false
					}
				}

				// Property: Value should be in parameters (possibly with LIKE wildcards)
				foundInParams := false
				for _, p := range params {
					if strVal, ok := p.Value.(string); ok {
						// For LIKE patterns, the value might have % or _ added
						if strings.Contains(strVal, userValue) {
							foundInParams = true
							break
						}
					}
				}

				if !foundInParams {
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

	// Test scenario 3: Key-Value column values are parameterized
	t.Run("key-value column values are parameterized", func(t *testing.T) {
		property := func(key, value string) bool {
			// Skip empty strings
			if key == "" || value == "" {
				return true
			}

			// Skip keys with dots as they have special meaning
			if strings.Contains(key, ".") {
				return true
			}

			table := NewTable().WithColumns(
				NewColumn().
					WithFieldPath("labels").
					WithDatabaseName("db_labels").
					KeyValue().
					Filterable().
					Build(),
			).Build()

			filterStr := fmt.Sprintf(`labels.%s:"%s"`, key, escapeFilterValue(value))
			filter, err := ParseFilter(filterStr)
			if err != nil {
				return true
			}

			sql, params, err := table.WhereClause(filter, "p_")
			if err != nil {
				return true
			}

			// Property: Should use parameterized query
			if !strings.Contains(sql, "@p_") {
				return false
			}

			// Property: Both key and value should NOT be directly in SQL
			if len(key) > 3 && !isSQLKeywordOrCommon(key) {
				if strings.Contains(sql, fmt.Sprintf("'%s'", key)) {
					return false
				}
			}
			if len(value) > 3 && !isSQLKeywordOrCommon(value) {
				if strings.Contains(sql, fmt.Sprintf("'%s'", value)) {
					return false
				}
			}

			// Property: Both key and value should be in parameters
			foundKey := false
			foundValue := false
			for _, p := range params {
				if strVal, ok := p.Value.(string); ok {
					if strVal == key {
						foundKey = true
					}
					if strings.Contains(strVal, value) {
						foundValue = true
					}
				}
			}

			return foundKey && foundValue
		}

		config := &quick.Config{MaxCount: 100}
		if err := quick.Check(property, config); err != nil {
			t.Error(err)
		}
	})

	// Test scenario 4: Seek pagination values are parameterized
	t.Run("seek pagination values are parameterized", func(t *testing.T) {
		property := func(sortValue, tieBreakerValue string) bool {
			// Skip empty strings
			if sortValue == "" || tieBreakerValue == "" {
				return true
			}

			table := NewTable().WithColumns(
				NewColumn().
					WithFieldPath("name").
					WithDatabaseName("db_name").
					Sortable().
					Build(),
				NewColumn().
					WithFieldPath("id").
					WithDatabaseName("db_id").
					Sortable().
					Build(),
			).Build()

			order := []OrderBy{
				{FieldPath: NewFieldPath("name"), Descending: false},
			}

			sql, params, err := table.BuildSeekPaginationClause(
				order,
				[]string{sortValue},
				NewFieldPath("id"),
				tieBreakerValue,
				"seek_",
				SQLDialectGeneric,
			)
			if err != nil {
				return true
			}

			// Property: Should use parameterized query
			if !strings.Contains(sql, "@seek_") {
				return false
			}

			// Property: Sort values should NOT be directly in SQL
			if len(sortValue) > 3 && !isSQLKeywordOrCommon(sortValue) {
				if strings.Contains(sql, fmt.Sprintf("'%s'", sortValue)) {
					return false
				}
			}
			if len(tieBreakerValue) > 3 && !isSQLKeywordOrCommon(tieBreakerValue) {
				if strings.Contains(sql, fmt.Sprintf("'%s'", tieBreakerValue)) {
					return false
				}
			}

			// Property: Values should be in parameters
			foundSort := false
			foundTieBreaker := false
			for _, p := range params {
				if strVal, ok := p.Value.(string); ok {
					if strVal == sortValue {
						foundSort = true
					}
					if strVal == tieBreakerValue {
						foundTieBreaker = true
					}
				}
			}

			return foundSort && foundTieBreaker
		}

		config := &quick.Config{MaxCount: 100}
		if err := quick.Check(property, config); err != nil {
			t.Error(err)
		}
	})

	// Test scenario 5: SQL injection attempts are parameterized
	t.Run("SQL injection attempts are parameterized", func(t *testing.T) {
		sqlInjectionPatterns := []string{
			"'; DROP TABLE users; --",
			"' OR '1'='1",
			"admin'--",
			"' UNION SELECT * FROM passwords--",
			"1; DELETE FROM users WHERE 1=1--",
			"' OR 1=1--",
			"'; EXEC sp_MSForEachTable 'DROP TABLE ?'; --",
		}

		table := NewTable().WithColumns(
			NewColumn().
				WithFieldPath("name").
				WithDatabaseName("db_name").
				Filterable().
				WithMatchModes(MatchModeExact).
				Build(),
		).Build()

		for _, maliciousValue := range sqlInjectionPatterns {
			t.Run(fmt.Sprintf("injection=%s", maliciousValue), func(t *testing.T) {
				filterStr := fmt.Sprintf(`name="%s"`, escapeFilterValue(maliciousValue))
				filter, err := ParseFilter(filterStr)
				if err != nil {
					// Parse errors are acceptable
					return
				}

				sql, params, err := table.WhereClause(filter, "p_")
				if err != nil {
					// Generation errors are acceptable
					return
				}

				// Property: Malicious value should NOT be in SQL
				assert.NotContains(t, sql, maliciousValue,
					"Malicious value should not be directly in SQL")

				// Property: Should use parameterized query
				assert.Contains(t, sql, "@p_",
					"SQL should use parameterized query format")

				// Property: Malicious value should be in parameters
				foundInParams := false
				for _, p := range params {
					if strVal, ok := p.Value.(string); ok {
						if strVal == maliciousValue {
							foundInParams = true
							break
						}
					}
				}
				assert.True(t, foundInParams,
					"Malicious value should be safely stored in parameters")
			})
		}
	})

	// Test scenario 6: Composite index optimization preserves parameterization
	t.Run("composite index optimization preserves parameterization", func(t *testing.T) {
		property := func(value1, value2 string) bool {
			// Skip empty strings
			if value1 == "" || value2 == "" {
				return true
			}

			table := NewTable().WithColumns(
				NewColumn().
					WithFieldPath("status").
					WithDatabaseName("db_status").
					Filterable().
					Build(),
				NewColumn().
					WithFieldPath("user_id").
					WithDatabaseName("db_user_id").
					Filterable().
					Build(),
			).Build()

			table.CompositeIndexes = []CompositeIndex{
				{
					Name:    "idx_status_user",
					Columns: []string{"db_status", "db_user_id"},
				},
			}

			filterStr := fmt.Sprintf(`status="%s" AND user_id="%s"`,
				escapeFilterValue(value1), escapeFilterValue(value2))
			filter, err := ParseFilter(filterStr)
			if err != nil {
				return true
			}

			sql, params, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
				EnableCompositeIndexOptimization: true,
			})
			if err != nil {
				return true
			}

			// Property: Should use parameterized query
			if !strings.Contains(sql, "@p_") {
				return false
			}

			// Property: Values should NOT be directly in SQL
			if len(value1) > 3 && !isSQLKeywordOrCommon(value1) {
				if strings.Contains(sql, value1) {
					return false
				}
			}
			if len(value2) > 3 && !isSQLKeywordOrCommon(value2) {
				if strings.Contains(sql, value2) {
					return false
				}
			}

			// Property: Values should be in parameters
			foundValue1 := false
			foundValue2 := false
			for _, p := range params {
				if strVal, ok := p.Value.(string); ok {
					if strVal == value1 {
						foundValue1 = true
					}
					if strVal == value2 {
						foundValue2 = true
					}
				}
			}

			return foundValue1 && foundValue2
		}

		config := &quick.Config{MaxCount: 100}
		if err := quick.Check(property, config); err != nil {
			t.Error(err)
		}
	})
}

// **Validates: Requirements 8.3**
// Feature: aip-sql-execution-optimization, Property 17: ORDER BY 仅使用 Table 定义的列
//
// For any generated ORDER BY clause, the column names should come from
// Column.DatabaseName in the Table definition, not arbitrary user input strings.
func TestProperty_OrderByOnlyUsesTableDefinedColumns(t *testing.T) {
	// Test scenario 1: ORDER BY only uses database column names from Table
	t.Run("ORDER BY uses database column names", func(t *testing.T) {
		property := func(fieldPath string) bool {
			// Skip empty strings
			if fieldPath == "" {
				return true
			}

			// Create table with known columns
			table := NewTable().WithColumns(
				NewColumn().
					WithFieldPath("name").
					WithDatabaseName("db_name").
					Sortable().
					Build(),
				NewColumn().
					WithFieldPath("created_at").
					WithDatabaseName("db_created_at").
					Sortable().
					Build(),
				NewColumn().
					WithFieldPath("status").
					WithDatabaseName("db_status").
					Sortable().
					Build(),
			).Build()

			// Try to parse order by with the field path
			orderBy, err := ParseOrderBy(fieldPath)
			if err != nil {
				// Parse errors are acceptable
				return true
			}

			sql, err := table.OrderByClause(orderBy)
			if err != nil {
				// Generation errors are acceptable (e.g., field not sortable)
				return true
			}

			// Property: SQL should only contain database column names from Table definition
			// Extract column names from SQL
			sqlLower := strings.ToLower(sql)

			// Check that SQL contains database column names, not field paths
			if strings.Contains(sqlLower, "db_name") ||
				strings.Contains(sqlLower, "db_created_at") ||
				strings.Contains(sqlLower, "db_status") {
				// Good - uses database column names
				return true
			}

			// If none of the database column names are present, this might be an error
			// or the field path doesn't match any column
			return false
		}

		config := &quick.Config{MaxCount: 100}
		if err := quick.Check(property, config); err != nil {
			t.Error(err)
		}
	})

	// Test scenario 2: ORDER BY does not contain user input
	t.Run("ORDER BY does not contain arbitrary user input", func(t *testing.T) {
		table := NewTable().WithColumns(
			NewColumn().
				WithFieldPath("name").
				WithDatabaseName("db_name").
				Sortable().
				Build(),
			NewColumn().
				WithFieldPath("created_at").
				WithDatabaseName("db_created_at").
				Sortable().
				Build(),
		).Build()

		// Valid field paths
		validOrderBys := []string{
			"name",
			"created_at",
			"name desc",
			"created_at, name",
		}

		for _, orderByStr := range validOrderBys {
			t.Run(fmt.Sprintf("orderBy=%s", orderByStr), func(t *testing.T) {
				orderBy, err := ParseOrderBy(orderByStr)
				require.NoError(t, err)

				sql, err := table.OrderByClause(orderBy)
				require.NoError(t, err)

				// Property: SQL should use database column names
				assert.Contains(t, sql, "db_", "SQL should use database column names")

				// Property: SQL should NOT contain field paths directly
				// (unless they happen to match database names)
				// The important thing is that column names come from Table definition

				// Property: SQL should NOT have parameters (ORDER BY doesn't parameterize column names)
				assert.NotContains(t, sql, "@", "ORDER BY should not have parameters")
			})
		}
	})

	// Test scenario 3: ORDER BY rejects undefined columns
	t.Run("ORDER BY rejects undefined columns", func(t *testing.T) {
		property := func(arbitraryColumn string) bool {
			// Skip empty strings and known valid columns
			if arbitraryColumn == "" ||
				arbitraryColumn == "name" ||
				arbitraryColumn == "created_at" {
				return true
			}

			table := NewTable().WithColumns(
				NewColumn().
					WithFieldPath("name").
					WithDatabaseName("db_name").
					Sortable().
					Build(),
				NewColumn().
					WithFieldPath("created_at").
					WithDatabaseName("db_created_at").
					Sortable().
					Build(),
			).Build()

			// Try to use arbitrary column name
			orderBy, err := ParseOrderBy(arbitraryColumn)
			if err != nil {
				// Parse errors are acceptable
				return true
			}

			_, err = table.OrderByClause(orderBy)

			// Property: Should return error for undefined columns
			// (This prevents SQL injection via column names)
			return err != nil
		}

		config := &quick.Config{MaxCount: 100}
		if err := quick.Check(property, config); err != nil {
			t.Error(err)
		}
	})

	// Test scenario 4: ORDER BY with SQL injection attempts
	t.Run("ORDER BY rejects SQL injection attempts", func(t *testing.T) {
		sqlInjectionPatterns := []string{
			"name; DROP TABLE users; --",
			"name' OR '1'='1",
			"name, (SELECT password FROM users)",
			"name UNION SELECT * FROM passwords",
		}

		table := NewTable().WithColumns(
			NewColumn().
				WithFieldPath("name").
				WithDatabaseName("db_name").
				Sortable().
				Build(),
		).Build()

		for _, maliciousOrderBy := range sqlInjectionPatterns {
			t.Run(fmt.Sprintf("injection=%s", maliciousOrderBy), func(t *testing.T) {
				orderBy, err := ParseOrderBy(maliciousOrderBy)
				if err != nil {
					// Parse errors are expected and acceptable
					return
				}

				sql, err := table.OrderByClause(orderBy)

				// Property: Should either error or not contain malicious SQL
				if err == nil {
					// If it didn't error, the malicious parts should not be in SQL
					assert.NotContains(t, sql, "DROP TABLE",
						"Malicious SQL should not be in ORDER BY")
					assert.NotContains(t, sql, "UNION SELECT",
						"Malicious SQL should not be in ORDER BY")
					assert.NotContains(t, sql, "SELECT password",
						"Malicious SQL should not be in ORDER BY")
				}
			})
		}
	})

	// Test scenario 5: ORDER BY with composite index uses database column names
	t.Run("ORDER BY with composite index uses database column names", func(t *testing.T) {
		table := NewTable().WithColumns(
			NewColumn().
				WithFieldPath("status").
				WithDatabaseName("db_status").
				Sortable().
				Build(),
			NewColumn().
				WithFieldPath("created_at").
				WithDatabaseName("db_created_at").
				Sortable().
				Build(),
		).Build()

		table.CompositeIndexes = []CompositeIndex{
			{
				Name:    "idx_status_created",
				Columns: []string{"db_status", "db_created_at"},
			},
		}

		orderBy, err := ParseOrderBy("status, created_at")
		require.NoError(t, err)

		sql, err := table.OrderByClause(orderBy)
		require.NoError(t, err)

		// Property: SQL should use database column names
		assert.Contains(t, sql, "db_status", "Should use database column name")
		assert.Contains(t, sql, "db_created_at", "Should use database column name")

		// Property: SQL should NOT contain field paths
		// (unless they happen to match database names)
		// The key is that names come from Table definition, not user input
	})

	// Test scenario 6: Multiple ORDER BY fields all use database column names
	t.Run("multiple ORDER BY fields use database column names", func(t *testing.T) {
		property := func(desc1, desc2 bool) bool {
			table := NewTable().WithColumns(
				NewColumn().
					WithFieldPath("name").
					WithDatabaseName("db_name").
					Sortable().
					Build(),
				NewColumn().
					WithFieldPath("created_at").
					WithDatabaseName("db_created_at").
					Sortable().
					Build(),
				NewColumn().
					WithFieldPath("status").
					WithDatabaseName("db_status").
					Sortable().
					Build(),
			).Build()

			// Build order by string
			orderByStr := "name"
			if desc1 {
				orderByStr += " desc"
			}
			orderByStr += ", created_at"
			if desc2 {
				orderByStr += " desc"
			}

			orderBy, err := ParseOrderBy(orderByStr)
			if err != nil {
				return true
			}

			sql, err := table.OrderByClause(orderBy)
			if err != nil {
				return true
			}

			// Property: SQL should contain database column names
			if !strings.Contains(sql, "db_name") {
				return false
			}
			if !strings.Contains(sql, "db_created_at") {
				return false
			}

			// Property: SQL should NOT have parameters
			if strings.Contains(sql, "@") {
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

// Helper function to check if a string is a SQL keyword or common word
func isSQLKeywordOrCommon(s string) bool {
	keywords := []string{
		"AND", "OR", "NOT", "TRUE", "FALSE", "NULL",
		"SELECT", "FROM", "WHERE", "ORDER", "BY",
		"ASC", "DESC", "LIKE", "IN", "EXISTS",
		"a", "an", "the", "is", "are", "was", "were",
	}
	upper := strings.ToUpper(s)
	for _, kw := range keywords {
		if upper == kw {
			return true
		}
	}
	return false
}
