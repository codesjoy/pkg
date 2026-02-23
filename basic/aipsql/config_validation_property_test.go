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
)

// **Validates: Requirements 9.1, 9.2, 9.3, 9.5**
// Feature: aip-sql-execution-optimization, Property 19: 配置验证错误处理
//
// For any invalid configuration, the system should return descriptive error messages:
// - Invalid Match_Mode values
// - has operator applied to non-string columns
// - has operator applied to columns with argSubstitute
// - Seek pagination field count mismatch with value count
func TestProperty_ConfigurationValidationErrorHandling(t *testing.T) {
	// Test scenario 1: Invalid Match Mode values return descriptive errors
	t.Run("invalid match mode returns descriptive error", func(t *testing.T) {
		// Use a custom generator for more predictable invalid mode strings
		testCases := []string{
			"fuzzy",
			"regex",
			"wildcard",
			"invalid",
			"unknown",
			"bad_mode",
		}

		for _, invalidMode := range testCases {
			err := validateMatchMode(MatchMode(invalidMode))

			// Should return an error
			if err == nil {
				t.Errorf("validateMatchMode(%q) expected error, got nil", invalidMode)
				continue
			}

			// Error message should be descriptive
			errMsg := err.Error()
			if !strings.Contains(errMsg, "invalid match mode") {
				t.Errorf(
					"validateMatchMode(%q) error should contain 'invalid match mode', got: %v",
					invalidMode,
					errMsg,
				)
			}

			// Error message should mention the invalid mode
			if !strings.Contains(errMsg, invalidMode) {
				t.Errorf(
					"validateMatchMode(%q) error should mention the invalid mode, got: %v",
					invalidMode,
					errMsg,
				)
			}

			// Error message should list valid modes
			if !strings.Contains(errMsg, string(MatchModeExact)) ||
				!strings.Contains(errMsg, string(MatchModePrefix)) ||
				!strings.Contains(errMsg, string(MatchModeFullText)) ||
				!strings.Contains(errMsg, string(MatchModeContains)) {
				t.Errorf(
					"validateMatchMode(%q) error should list all valid modes, got: %v",
					invalidMode,
					errMsg,
				)
			}
		}
	})

	// Test scenario 2: has operator on non-string columns returns error
	t.Run("has operator on non-string column returns error", func(t *testing.T) {
		property := func() bool {
			// Test with BOOL column type
			column := &Column{
				fieldPath:    NewFieldPath("is_active"),
				databaseName: "is_active",
				columnType:   ColumnTypeBool,
			}

			err := validateHasOperator(column)

			// Should return an error
			if err == nil {
				return false
			}

			// Error message should be descriptive
			errMsg := err.Error()
			if !strings.Contains(errMsg, "has (:) operator") {
				return false
			}
			if !strings.Contains(errMsg, "STRING") {
				return false
			}

			return true
		}

		config := &quick.Config{MaxCount: 100}
		if err := quick.Check(property, config); err != nil {
			t.Error(err)
		}
	})

	// Test scenario 3: has operator on column with argSubstitute returns error
	t.Run("has operator on column with argSubstitute returns error", func(t *testing.T) {
		property := func() bool {
			column := &Column{
				fieldPath:     NewFieldPath("name"),
				databaseName:  "name",
				columnType:    ColumnTypeString,
				argSubstitute: func(s string) string { return s },
			}

			err := validateHasOperator(column)

			// Should return an error
			if err == nil {
				return false
			}

			// Error message should be descriptive
			errMsg := err.Error()
			if !strings.Contains(errMsg, "has (:) operator") {
				return false
			}
			if !strings.Contains(errMsg, "argSubstitute") {
				return false
			}

			return true
		}

		config := &quick.Config{MaxCount: 100}
		if err := quick.Check(property, config); err != nil {
			t.Error(err)
		}
	})

	// Test scenario 4: Seek pagination field count mismatch returns descriptive error
	t.Run("seek pagination field count mismatch returns descriptive error", func(t *testing.T) {
		property := func(fieldCount, valueCount uint8) bool {
			// Limit to reasonable range
			if fieldCount == 0 || fieldCount > 5 || valueCount == 0 || valueCount > 5 {
				return true
			}

			// Skip matching counts
			if fieldCount == valueCount {
				return true
			}

			nFields := int(fieldCount)
			nValues := int(valueCount)

			fields := make([]OrderByField, nFields)
			values := make([]interface{}, nValues)

			for i := 0; i < nFields; i++ {
				fields[i] = OrderByField{
					Column: &Column{
						fieldPath:    NewFieldPath(fmt.Sprintf("field%d", i)),
						databaseName: fmt.Sprintf("field%d", i),
						columnType:   ColumnTypeString,
					},
					Direction: "ASC",
				}
			}

			for i := 0; i < nValues; i++ {
				values[i] = fmt.Sprintf("value%d", i)
			}

			_, _, err := buildLexicographicComparison(fields, values)

			// Should return an error
			if err == nil {
				return false
			}

			// Error message should be descriptive
			errMsg := err.Error()

			// Should mention "field count" or "value count"
			if !strings.Contains(errMsg, "field count") &&
				!strings.Contains(errMsg, "value count") {
				return false
			}

			// Should mention "does not match" or "mismatch"
			if !strings.Contains(errMsg, "does not match") &&
				!strings.Contains(errMsg, "mismatch") {
				return false
			}

			// Should include the actual counts
			if !strings.Contains(errMsg, fmt.Sprintf("%d", nFields)) ||
				!strings.Contains(errMsg, fmt.Sprintf("%d", nValues)) {
				return false
			}

			return true
		}

		config := &quick.Config{MaxCount: 100}
		if err := quick.Check(property, config); err != nil {
			t.Error(err)
		}
	})

	// Test scenario 5: BuildSeekPaginationClause also validates field count mismatch
	t.Run("BuildSeekPaginationClause validates field count mismatch", func(t *testing.T) {
		property := func(orderCount, valueCount uint8) bool {
			// Limit to reasonable range
			if orderCount == 0 || orderCount > 5 || valueCount > 5 {
				return true
			}

			// Skip matching counts
			if orderCount == valueCount {
				return true
			}

			nOrder := int(orderCount)
			nValues := int(valueCount)

			// Create columns for the table
			columns := make([]*Column, nOrder+1)
			for i := 0; i < nOrder; i++ {
				fieldName := fmt.Sprintf("field%d", i)
				columns[i] = NewColumn().
					WithFieldPath(fieldName).
					WithDatabaseName(fieldName).
					Sortable().
					Build()
			}
			// Add tie breaker column
			columns[nOrder] = NewColumn().
				WithFieldPath("id").
				WithDatabaseName("id").
				Sortable().
				Build()

			// Build the table
			table := NewTable().WithColumns(columns...).Build()

			// Create order by entries
			order := make([]OrderBy, nOrder)
			for i := 0; i < nOrder; i++ {
				order[i] = OrderBy{
					FieldPath:  NewFieldPath(fmt.Sprintf("field%d", i)),
					Descending: false,
				}
			}

			// Create sort values
			lastSortValues := make([]string, nValues)
			for i := 0; i < nValues; i++ {
				lastSortValues[i] = fmt.Sprintf("value%d", i)
			}

			_, _, err := table.BuildSeekPaginationClause(
				order,
				lastSortValues,
				NewFieldPath("id"),
				"last_id",
				"seek_",
				SQLDialectGeneric,
			)

			// Should return an error when counts don't match
			if err == nil {
				return false
			}

			// Error message should be descriptive
			errMsg := err.Error()

			// Should mention the mismatch
			if !strings.Contains(errMsg, "expected") || !strings.Contains(errMsg, "values") {
				return false
			}

			// Should include the actual counts
			if !strings.Contains(errMsg, fmt.Sprintf("%d", nOrder)) ||
				!strings.Contains(errMsg, fmt.Sprintf("%d", nValues)) {
				return false
			}

			return true
		}

		config := &quick.Config{MaxCount: 50} // Fewer iterations since this is more expensive
		if err := quick.Check(property, config); err != nil {
			t.Error(err)
		}
	})

	// Test scenario 6: Error messages are consistent across validation functions
	t.Run("error messages are consistent and descriptive", func(t *testing.T) {
		property := func() bool {
			// Test that all validation errors follow a consistent pattern:
			// 1. They describe what went wrong
			// 2. They include relevant context (field names, values, etc.)
			// 3. They suggest what is valid (when applicable)

			// Test Match Mode validation
			err1 := validateMatchMode(MatchMode("invalid"))
			if err1 == nil || !strings.Contains(err1.Error(), "invalid match mode") {
				return false
			}

			// Test has operator validation
			err2 := validateHasOperator(&Column{
				fieldPath:    NewFieldPath("test"),
				databaseName: "test",
				columnType:   ColumnTypeBool,
			})
			if err2 == nil || !strings.Contains(err2.Error(), "has (:) operator") {
				return false
			}

			// Test Seek pagination validation
			fields := []OrderByField{
				{
					Column: &Column{
						fieldPath:    NewFieldPath("field1"),
						databaseName: "field1",
						columnType:   ColumnTypeString,
					},
					Direction: "ASC",
				},
			}
			values := []interface{}{"value1", "value2"}
			_, _, err3 := buildLexicographicComparison(fields, values)
			if err3 == nil || !strings.Contains(err3.Error(), "does not match") {
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

// Additional unit tests for specific edge cases
func TestConfigValidationEdgeCases(t *testing.T) {
	t.Run("empty match mode returns error", func(t *testing.T) {
		err := validateMatchMode(MatchMode(""))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid match mode")
	})

	t.Run("seek pagination with zero fields returns error", func(t *testing.T) {
		fields := []OrderByField{}
		values := []interface{}{}

		_, _, err := buildLexicographicComparison(fields, values)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be empty")
	})

	t.Run("seek pagination with more values than fields returns error", func(t *testing.T) {
		fields := []OrderByField{
			{
				Column: &Column{
					fieldPath:    NewFieldPath("field1"),
					databaseName: "field1",
					columnType:   ColumnTypeString,
				},
				Direction: "ASC",
			},
		}
		values := []interface{}{"value1", "value2", "value3"}

		_, _, err := buildLexicographicComparison(fields, values)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "does not match")
		assert.Contains(t, err.Error(), "1") // field count
		assert.Contains(t, err.Error(), "3") // value count
	})

	t.Run("seek pagination with fewer values than fields returns error", func(t *testing.T) {
		fields := []OrderByField{
			{
				Column: &Column{
					fieldPath:    NewFieldPath("field1"),
					databaseName: "field1",
					columnType:   ColumnTypeString,
				},
				Direction: "ASC",
			},
			{
				Column: &Column{
					fieldPath:    NewFieldPath("field2"),
					databaseName: "field2",
					columnType:   ColumnTypeString,
				},
				Direction: "ASC",
			},
			{
				Column: &Column{
					fieldPath:    NewFieldPath("field3"),
					databaseName: "field3",
					columnType:   ColumnTypeString,
				},
				Direction: "ASC",
			},
		}
		values := []interface{}{"value1"}

		_, _, err := buildLexicographicComparison(fields, values)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "does not match")
		assert.Contains(t, err.Error(), "3") // field count
		assert.Contains(t, err.Error(), "1") // value count
	})

	t.Run("BuildSeekPaginationClause validates field count", func(t *testing.T) {
		table := NewTable().WithColumns(
			NewColumn().
				WithFieldPath("name").
				WithDatabaseName("name").
				Sortable().
				Build(),
			NewColumn().
				WithFieldPath("id").
				WithDatabaseName("id").
				Sortable().
				Build(),
		).Build()

		order := []OrderBy{
			{
				FieldPath:  NewFieldPath("name"),
				Descending: false,
			},
		}

		// Provide 2 values for 1 order field
		lastSortValues := []string{"value1", "value2"}

		_, _, err := table.BuildSeekPaginationClause(
			order,
			lastSortValues,
			NewFieldPath("id"),
			"last_id",
			"seek_",
			SQLDialectGeneric,
		)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "expected")
		assert.Contains(t, err.Error(), "1") // expected count
		assert.Contains(t, err.Error(), "2") // actual count
	})
}
