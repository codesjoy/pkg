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
// Seek Pagination Property Tests
// ============================================================================

// **Validates: Requirements 5.1, 5.2**
// Feature: aip-sql-execution-optimization, Property 10: Seek 分页生成词典序 OR 条件链
//
// For any list of sort fields and pagination token values, the Seek_Paginator should
// generate n OR-connected conditions, where n equals the number of sort fields.
// Each condition represents one level of lexicographic comparison.
func TestProperty_SeekPaginationGeneratesLexicographicORChain(t *testing.T) {
	// Test scenario 1: Single field generates single OR condition
	t.Run("single field generates one OR condition", func(t *testing.T) {
		property := func(value string) bool {
			if value == "" {
				return true // Skip empty values
			}

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

			values := []interface{}{value}

			sql, params, err := buildLexicographicComparison(fields, values)
			if err != nil {
				return false
			}

			// Should generate exactly 1 OR condition (no OR operator in the result)
			// Format: (field1 > @seek_cmp_0)
			orCount := strings.Count(sql, " OR ")
			if orCount != 0 {
				return false
			}

			// Should have 1 parameter
			if len(params) != 1 {
				return false
			}

			// Should contain the comparison operator
			if !strings.Contains(sql, "field1 >") {
				return false
			}

			return true
		}

		config := &quick.Config{MaxCount: 100}
		if err := quick.Check(property, config); err != nil {
			t.Error(err)
		}
	})

	// Test scenario 2: Two fields generate two OR-connected conditions
	t.Run("two fields generate two OR conditions", func(t *testing.T) {
		property := func(value1, value2 string) bool {
			if value1 == "" || value2 == "" {
				return true
			}

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
			}

			values := []interface{}{value1, value2}

			sql, params, err := buildLexicographicComparison(fields, values)
			if err != nil {
				return false
			}

			// Should generate exactly 1 OR operator (connecting 2 conditions)
			orCount := strings.Count(sql, " OR ")
			if orCount != 1 {
				return false
			}

			// Should have 3 parameters: 1 for first level, 2 for second level
			if len(params) != 3 {
				return false
			}

			// First condition: (field1 > @seek_cmp_0)
			// Second condition: (field1 = @seek_eq_0 AND field2 > @seek_cmp_1)
			if !strings.Contains(sql, "field1 >") {
				return false
			}
			if !strings.Contains(sql, "field1 = @seek_eq") {
				return false
			}
			if !strings.Contains(sql, "field2 >") {
				return false
			}

			return true
		}

		config := &quick.Config{MaxCount: 100}
		if err := quick.Check(property, config); err != nil {
			t.Error(err)
		}
	})

	// Test scenario 3: Three fields generate three OR-connected conditions
	t.Run("three fields generate three OR conditions", func(t *testing.T) {
		property := func(value1, value2, value3 string) bool {
			if value1 == "" || value2 == "" || value3 == "" {
				return true
			}

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

			values := []interface{}{value1, value2, value3}

			sql, params, err := buildLexicographicComparison(fields, values)
			if err != nil {
				return false
			}

			// Should generate exactly 2 OR operators (connecting 3 conditions)
			orCount := strings.Count(sql, " OR ")
			if orCount != 2 {
				return false
			}

			// Should have 6 parameters: 1 for first level, 2 for second level, 3 for third level
			if len(params) != 6 {
				return false
			}

			// Verify all three fields appear in the SQL
			if !strings.Contains(sql, "field1") {
				return false
			}
			if !strings.Contains(sql, "field2") {
				return false
			}
			if !strings.Contains(sql, "field3") {
				return false
			}

			return true
		}

		config := &quick.Config{MaxCount: 100}
		if err := quick.Check(property, config); err != nil {
			t.Error(err)
		}
	})

	// Test scenario 4: Number of OR conditions equals number of fields
	t.Run("OR count equals field count minus one", func(t *testing.T) {
		property := func(fieldCount uint8) bool {
			// Limit field count to reasonable range (1-5)
			if fieldCount == 0 || fieldCount > 5 {
				return true
			}

			n := int(fieldCount)
			fields := make([]OrderByField, n)
			values := make([]interface{}, n)

			for i := 0; i < n; i++ {
				fields[i] = OrderByField{
					Column: &Column{
						fieldPath:    NewFieldPath(fmt.Sprintf("field%d", i)),
						databaseName: fmt.Sprintf("field%d", i),
						columnType:   ColumnTypeString,
					},
					Direction: "ASC",
				}
				values[i] = fmt.Sprintf("value%d", i)
			}

			sql, _, err := buildLexicographicComparison(fields, values)
			if err != nil {
				return false
			}

			// Number of OR operators should be n-1
			orCount := strings.Count(sql, " OR ")
			expectedORCount := n - 1

			return orCount == expectedORCount
		}

		config := &quick.Config{MaxCount: 100}
		if err := quick.Check(property, config); err != nil {
			t.Error(err)
		}
	})

	// Test scenario 5: Each level has correct number of equality conditions
	t.Run("each level has correct equality conditions", func(t *testing.T) {
		property := func() bool {
			// Test with 3 fields
			fields := []OrderByField{
				{
					Column: &Column{
						fieldPath:    NewFieldPath("a"),
						databaseName: "a",
						columnType:   ColumnTypeString,
					},
					Direction: "ASC",
				},
				{
					Column: &Column{
						fieldPath:    NewFieldPath("b"),
						databaseName: "b",
						columnType:   ColumnTypeString,
					},
					Direction: "ASC",
				},
				{
					Column: &Column{
						fieldPath:    NewFieldPath("c"),
						databaseName: "c",
						columnType:   ColumnTypeString,
					},
					Direction: "ASC",
				},
			}

			values := []interface{}{"val1", "val2", "val3"}

			sql, _, err := buildLexicographicComparison(fields, values)
			if err != nil {
				return false
			}

			// Expected structure:
			// (a > @seek_cmp_0) OR
			// (a = @seek_eq_0 AND b > @seek_cmp_1) OR
			// (a = @seek_eq_1 AND b = @seek_eq_2 AND c > @seek_cmp_3)

			// Split by OR to get individual conditions
			conditions := strings.Split(sql, " OR ")
			if len(conditions) != 3 {
				return false
			}

			// First condition: no equality, just comparison
			if strings.Count(conditions[0], " = ") != 0 {
				return false
			}

			// Second condition: 1 equality + 1 comparison
			if strings.Count(conditions[1], " = ") != 1 {
				return false
			}

			// Third condition: 2 equalities + 1 comparison
			if strings.Count(conditions[2], " = ") != 2 {
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

// **Validates: Requirements 5.3, 8.4**
// Feature: aip-sql-execution-optimization, Property 11: Seek 分页参数化查询
//
// For any Seek pagination input, all sort value comparisons in the generated SQL clause
// should use parameterized queries (@param format) to prevent SQL injection.
func TestProperty_SeekPaginationUsesParameterizedQueries(t *testing.T) {
	// Test scenario 1: All values are parameterized, not embedded in SQL
	t.Run("all values are parameterized", func(t *testing.T) {
		property := func(value1, value2 string) bool {
			if value1 == "" || value2 == "" {
				return true
			}

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
			}

			values := []interface{}{value1, value2}

			sql, params, err := buildLexicographicComparison(fields, values)
			if err != nil {
				return false
			}

			// SQL should NOT contain the actual values directly
			if strings.Contains(sql, value1) || strings.Contains(sql, value2) {
				return false
			}

			// SQL should contain parameter placeholders
			if !strings.Contains(sql, "@seek_") {
				return false
			}

			// All parameters should have names starting with @seek_
			for _, param := range params {
				if !strings.HasPrefix(param.Name, "@seek_") {
					return false
				}
			}

			// Parameter values should match input values
			foundValue1 := false
			foundValue2 := false
			for _, param := range params {
				if param.Value == value1 {
					foundValue1 = true
				}
				if param.Value == value2 {
					foundValue2 = true
				}
			}

			return foundValue1 && foundValue2
		}

		config := &quick.Config{MaxCount: 100}
		if err := quick.Check(property, config); err != nil {
			t.Error(err)
		}
	})

	// Test scenario 2: Parameter naming follows expected pattern
	t.Run("parameter naming follows pattern", func(t *testing.T) {
		property := func(value1, value2, value3 string) bool {
			if value1 == "" || value2 == "" || value3 == "" {
				return true
			}

			fields := []OrderByField{
				{
					Column: &Column{
						fieldPath:    NewFieldPath("a"),
						databaseName: "a",
						columnType:   ColumnTypeString,
					},
					Direction: "ASC",
				},
				{
					Column: &Column{
						fieldPath:    NewFieldPath("b"),
						databaseName: "b",
						columnType:   ColumnTypeString,
					},
					Direction: "ASC",
				},
				{
					Column: &Column{
						fieldPath:    NewFieldPath("c"),
						databaseName: "c",
						columnType:   ColumnTypeString,
					},
					Direction: "ASC",
				},
			}

			values := []interface{}{value1, value2, value3}

			sql, params, err := buildLexicographicComparison(fields, values)
			if err != nil {
				return false
			}

			// Expected parameters:
			// @seek_cmp_0 (first level comparison)
			// @seek_eq_0, @seek_cmp_1 (second level)
			// @seek_eq_1, @seek_eq_2, @seek_cmp_2 (third level)

			// Check that equality parameters use @seek_eq_ prefix
			hasSeekEq := false
			for _, param := range params {
				if strings.HasPrefix(param.Name, "@seek_eq_") {
					hasSeekEq = true
					break
				}
			}

			// Check that comparison parameters use @seek_cmp_ prefix
			hasSeekCmp := false
			for _, param := range params {
				if strings.HasPrefix(param.Name, "@seek_cmp_") {
					hasSeekCmp = true
					break
				}
			}

			// Both types should be present for multi-field pagination
			if !hasSeekEq || !hasSeekCmp {
				return false
			}

			// SQL should reference these parameters
			if !strings.Contains(sql, "@seek_eq_") || !strings.Contains(sql, "@seek_cmp_") {
				return false
			}

			return true
		}

		config := &quick.Config{MaxCount: 100}
		if err := quick.Check(property, config); err != nil {
			t.Error(err)
		}
	})

	// Test scenario 3: SQL injection attempts are prevented
	t.Run("SQL injection attempts are prevented", func(t *testing.T) {
		property := func() bool {
			// Test with malicious input values
			maliciousValues := []string{
				"'; DROP TABLE users; --",
				"' OR '1'='1",
				"admin'--",
				"1; DELETE FROM users--",
			}

			for _, malicious := range maliciousValues {
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

				values := []interface{}{malicious}

				sql, params, err := buildLexicographicComparison(fields, values)
				if err != nil {
					continue // Error is acceptable
				}

				// Malicious value should NOT appear directly in SQL
				if strings.Contains(sql, malicious) {
					return false
				}

				// Malicious value should be in parameters
				found := false
				for _, param := range params {
					if param.Value == malicious {
						found = true
						break
					}
				}

				if !found {
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

	// Test scenario 4: Parameter count matches expected formula
	t.Run("parameter count matches formula", func(t *testing.T) {
		property := func(fieldCount uint8) bool {
			// Limit field count to reasonable range (1-5)
			if fieldCount == 0 || fieldCount > 5 {
				return true
			}

			n := int(fieldCount)
			fields := make([]OrderByField, n)
			values := make([]interface{}, n)

			for i := 0; i < n; i++ {
				fields[i] = OrderByField{
					Column: &Column{
						fieldPath:    NewFieldPath(fmt.Sprintf("field%d", i)),
						databaseName: fmt.Sprintf("field%d", i),
						columnType:   ColumnTypeString,
					},
					Direction: "ASC",
				}
				values[i] = fmt.Sprintf("value%d", i)
			}

			_, params, err := buildLexicographicComparison(fields, values)
			if err != nil {
				return false
			}

			// Expected parameter count: n(n+1)/2
			// For n=1: 1 param
			// For n=2: 3 params
			// For n=3: 6 params
			expectedCount := n * (n + 1) / 2

			return len(params) == expectedCount
		}

		config := &quick.Config{MaxCount: 100}
		if err := quick.Check(property, config); err != nil {
			t.Error(err)
		}
	})

	// Test scenario 5: All parameter names are unique
	t.Run("all parameter names are unique", func(t *testing.T) {
		property := func(value1, value2, value3 string) bool {
			if value1 == "" || value2 == "" || value3 == "" {
				return true
			}

			fields := []OrderByField{
				{
					Column: &Column{
						fieldPath:    NewFieldPath("a"),
						databaseName: "a",
						columnType:   ColumnTypeString,
					},
					Direction: "ASC",
				},
				{
					Column: &Column{
						fieldPath:    NewFieldPath("b"),
						databaseName: "b",
						columnType:   ColumnTypeString,
					},
					Direction: "ASC",
				},
				{
					Column: &Column{
						fieldPath:    NewFieldPath("c"),
						databaseName: "c",
						columnType:   ColumnTypeString,
					},
					Direction: "ASC",
				},
			}

			values := []interface{}{value1, value2, value3}

			_, params, err := buildLexicographicComparison(fields, values)
			if err != nil {
				return false
			}

			// Check that all parameter names are unique
			nameSet := make(map[string]bool)
			for _, param := range params {
				if nameSet[param.Name] {
					return false // Duplicate found
				}
				nameSet[param.Name] = true
			}

			return true
		}

		config := &quick.Config{MaxCount: 100}
		if err := quick.Check(property, config); err != nil {
			t.Error(err)
		}
	})
}

// **Validates: Requirements 5.5**
// Feature: aip-sql-execution-optimization, Property 12: Seek 分页支持混合排序方向
//
// For any sort fields containing mixed ASC and DESC directions, the Seek_Paginator
// should generate the correct comparison operator for each field (> for ASC, < for DESC).
func TestProperty_SeekPaginationSupportsMixedSortDirections(t *testing.T) {
	// Test scenario 1: ASC direction uses > operator
	t.Run("ASC direction uses greater than operator", func(t *testing.T) {
		property := func(value string) bool {
			if value == "" {
				return true
			}

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

			values := []interface{}{value}

			sql, _, err := buildLexicographicComparison(fields, values)
			if err != nil {
				return false
			}

			// Should use > operator for ASC
			if !strings.Contains(sql, "field1 >") {
				return false
			}

			// Should NOT use < operator
			if strings.Contains(sql, "field1 <") {
				return false
			}

			return true
		}

		config := &quick.Config{MaxCount: 100}
		if err := quick.Check(property, config); err != nil {
			t.Error(err)
		}
	})

	// Test scenario 2: DESC direction uses < operator
	t.Run("DESC direction uses less than operator", func(t *testing.T) {
		property := func(value string) bool {
			if value == "" {
				return true
			}

			fields := []OrderByField{
				{
					Column: &Column{
						fieldPath:    NewFieldPath("field1"),
						databaseName: "field1",
						columnType:   ColumnTypeString,
					},
					Direction: "DESC",
				},
			}

			values := []interface{}{value}

			sql, _, err := buildLexicographicComparison(fields, values)
			if err != nil {
				return false
			}

			// Should use < operator for DESC
			if !strings.Contains(sql, "field1 <") {
				return false
			}

			// Should NOT use > operator
			if strings.Contains(sql, "field1 >") {
				return false
			}

			return true
		}

		config := &quick.Config{MaxCount: 100}
		if err := quick.Check(property, config); err != nil {
			t.Error(err)
		}
	})

	// Test scenario 3: Mixed ASC and DESC directions use correct operators
	t.Run("mixed directions use correct operators", func(t *testing.T) {
		property := func(value1, value2 string) bool {
			if value1 == "" || value2 == "" {
				return true
			}

			// First field ASC, second field DESC
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
					Direction: "DESC",
				},
			}

			values := []interface{}{value1, value2}

			sql, _, err := buildLexicographicComparison(fields, values)
			if err != nil {
				return false
			}

			// First field should use > (ASC)
			// Expected: (field1 > @seek_cmp_0) OR (field1 = @seek_eq_0 AND field2 < @seek_cmp_1)
			if !strings.Contains(sql, "field1 >") {
				return false
			}

			// Second field should use < (DESC)
			if !strings.Contains(sql, "field2 <") {
				return false
			}

			return true
		}

		config := &quick.Config{MaxCount: 100}
		if err := quick.Check(property, config); err != nil {
			t.Error(err)
		}
	})

	// Test scenario 4: All DESC directions use < operator
	t.Run("all DESC directions use less than", func(t *testing.T) {
		property := func(value1, value2, value3 string) bool {
			if value1 == "" || value2 == "" || value3 == "" {
				return true
			}

			// All fields DESC
			fields := []OrderByField{
				{
					Column: &Column{
						fieldPath:    NewFieldPath("a"),
						databaseName: "a",
						columnType:   ColumnTypeString,
					},
					Direction: "DESC",
				},
				{
					Column: &Column{
						fieldPath:    NewFieldPath("b"),
						databaseName: "b",
						columnType:   ColumnTypeString,
					},
					Direction: "DESC",
				},
				{
					Column: &Column{
						fieldPath:    NewFieldPath("c"),
						databaseName: "c",
						columnType:   ColumnTypeString,
					},
					Direction: "DESC",
				},
			}

			values := []interface{}{value1, value2, value3}

			sql, _, err := buildLexicographicComparison(fields, values)
			if err != nil {
				return false
			}

			// All comparison operators should be <
			// Count the number of < operators (should be 3, one for each field)
			lessCount := strings.Count(sql, " < ")
			if lessCount != 3 {
				return false
			}

			// Should NOT contain > operator
			if strings.Contains(sql, " > ") {
				return false
			}

			return true
		}

		config := &quick.Config{MaxCount: 100}
		if err := quick.Check(property, config); err != nil {
			t.Error(err)
		}
	})

	// Test scenario 5: Direction case insensitivity
	t.Run("direction case insensitivity", func(t *testing.T) {
		property := func() bool {
			// Test various case combinations
			testCases := []struct {
				direction        string
				expectedOperator string
			}{
				{"ASC", ">"},
				{"asc", ">"},
				{"Asc", ">"},
				{"DESC", "<"},
				{"desc", "<"},
				{"Desc", "<"},
			}

			for _, tc := range testCases {
				fields := []OrderByField{
					{
						Column: &Column{
							fieldPath:    NewFieldPath("field1"),
							databaseName: "field1",
							columnType:   ColumnTypeString,
						},
						Direction: tc.direction,
					},
				}

				values := []interface{}{"test"}

				sql, _, err := buildLexicographicComparison(fields, values)
				if err != nil {
					return false
				}

				// Should use the expected operator
				expectedPattern := fmt.Sprintf("field1 %s", tc.expectedOperator)
				if !strings.Contains(sql, expectedPattern) {
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

	// Test scenario 6: Complex mixed direction pattern
	t.Run("complex mixed direction pattern", func(t *testing.T) {
		property := func() bool {
			// Pattern: ASC, DESC, ASC, DESC
			fields := []OrderByField{
				{
					Column: &Column{
						fieldPath:    NewFieldPath("a"),
						databaseName: "a",
						columnType:   ColumnTypeString,
					},
					Direction: "ASC",
				},
				{
					Column: &Column{
						fieldPath:    NewFieldPath("b"),
						databaseName: "b",
						columnType:   ColumnTypeString,
					},
					Direction: "DESC",
				},
				{
					Column: &Column{
						fieldPath:    NewFieldPath("c"),
						databaseName: "c",
						columnType:   ColumnTypeString,
					},
					Direction: "ASC",
				},
				{
					Column: &Column{
						fieldPath:    NewFieldPath("d"),
						databaseName: "d",
						columnType:   ColumnTypeString,
					},
					Direction: "DESC",
				},
			}

			values := []interface{}{"val1", "val2", "val3", "val4"}

			sql, _, err := buildLexicographicComparison(fields, values)
			if err != nil {
				return false
			}

			// Verify each field uses the correct operator
			// a should use >
			if !strings.Contains(sql, "a >") {
				return false
			}

			// b should use <
			if !strings.Contains(sql, "b <") {
				return false
			}

			// c should use >
			if !strings.Contains(sql, "c >") {
				return false
			}

			// d should use <
			if !strings.Contains(sql, "d <") {
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

// Additional unit tests for edge cases and error conditions
func TestSeekPaginationEdgeCases(t *testing.T) {
	t.Run("empty fields returns error", func(t *testing.T) {
		fields := []OrderByField{}
		values := []interface{}{}

		_, _, err := buildLexicographicComparison(fields, values)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be empty")
	})

	t.Run("field count mismatch returns error", func(t *testing.T) {
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
		values := []interface{}{"value1", "value2"} // Mismatch: 1 field, 2 values

		_, _, err := buildLexicographicComparison(fields, values)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "does not match")
	})

	t.Run("single field with ASC generates correct SQL", func(t *testing.T) {
		fields := []OrderByField{
			{
				Column: &Column{
					fieldPath:    NewFieldPath("id"),
					databaseName: "id",
					columnType:   ColumnTypeString,
				},
				Direction: "ASC",
			},
		}
		values := []interface{}{"123"}

		sql, params, err := buildLexicographicComparison(fields, values)

		assert.NoError(t, err)
		assert.Contains(t, sql, "id >")
		assert.Len(t, params, 1)
		assert.Equal(t, "@seek_cmp_0", params[0].Name)
		assert.Equal(t, "123", params[0].Value)
	})

	t.Run("single field with DESC generates correct SQL", func(t *testing.T) {
		fields := []OrderByField{
			{
				Column: &Column{
					fieldPath:    NewFieldPath("created_at"),
					databaseName: "created_at",
					columnType:   ColumnTypeString,
				},
				Direction: "DESC",
			},
		}
		values := []interface{}{"2024-01-01"}

		sql, params, err := buildLexicographicComparison(fields, values)

		assert.NoError(t, err)
		assert.Contains(t, sql, "created_at <")
		assert.Len(t, params, 1)
		assert.Equal(t, "@seek_cmp_0", params[0].Name)
		assert.Equal(t, "2024-01-01", params[0].Value)
	})
}

// ============================================================================
// OrderBy Seek Tests
// ============================================================================

func TestOrderByClauseWithDialect(t *testing.T) {
	table := NewTable().WithColumns(
		NewColumn().WithFieldPath("foo").WithDatabaseName("db_foo").Sortable().Build(),
	).Build()

	t.Run("supports postgres dialect", func(t *testing.T) {
		query, err := table.OrderByClauseWithDialect([]OrderBy{
			{FieldPath: NewFieldPath("foo"), Descending: true},
		}, SQLDialectPostgres)
		require.NoError(t, err)
		assert.Equal(t, "db_foo DESC", query)
	})

	t.Run("rejects invalid dialect", func(t *testing.T) {
		_, err := table.OrderByClauseWithDialect([]OrderBy{
			{FieldPath: NewFieldPath("foo")},
		}, SQLDialect("sqlite"))
		ErrLike(t, err, []any{`unsupported sql dialect "sqlite"`})
	})
}

func TestBuildSeekPaginationClause(t *testing.T) {
	table := NewTable().WithColumns(
		NewColumn().WithFieldPath("priority").WithDatabaseName("db_priority").Sortable().Build(),
		NewColumn().WithFieldPath("created_at").
			WithDatabaseName("db_created_at").
			Sortable().
			Build(),
		NewColumn().WithFieldPath("id").WithDatabaseName("db_id").Sortable().Build(),
	).
		Build()

	t.Run("single sort field", func(t *testing.T) {
		query, params, err := table.BuildSeekPaginationClause(
			[]OrderBy{
				{FieldPath: NewFieldPath("created_at"), Descending: true},
			},
			[]string{"2025-01-01T00:00:00Z"},
			NewFieldPath("id"),
			"100",
			"p_",
			SQLDialectPostgres,
		)
		require.NoError(t, err)
		assert.Equal(
			t,
			"((db_created_at < @p_0) OR (db_created_at = @p_0 AND db_id > @p_1))",
			query,
		)
		assert.Equal(t, []QueryParameter{
			{Name: "p_0", Value: "2025-01-01T00:00:00Z"},
			{Name: "p_1", Value: "100"},
		}, params)
	})

	t.Run("multiple sort fields", func(t *testing.T) {
		query, params, err := table.BuildSeekPaginationClause(
			[]OrderBy{
				{FieldPath: NewFieldPath("priority")},
				{FieldPath: NewFieldPath("created_at"), Descending: true},
			},
			[]string{"HIGH", "2025-01-01T00:00:00Z"},
			NewFieldPath("id"),
			"100",
			"p_",
			SQLDialectMySQL,
		)
		require.NoError(t, err)
		assert.Equal(
			t,
			"((db_priority > @p_0) OR (db_priority = @p_0 AND db_created_at < @p_1) OR (db_priority = @p_0 AND db_created_at = @p_1 AND db_id > @p_2))",
			query,
		)
		assert.Equal(t, []QueryParameter{
			{Name: "p_0", Value: "HIGH"},
			{Name: "p_1", Value: "2025-01-01T00:00:00Z"},
			{Name: "p_2", Value: "100"},
		}, params)
	})

	t.Run("rejects invalid cursor size", func(t *testing.T) {
		_, _, err := table.BuildSeekPaginationClause(
			[]OrderBy{
				{FieldPath: NewFieldPath("priority")},
				{FieldPath: NewFieldPath("created_at"), Descending: true},
			},
			[]string{"HIGH"},
			NewFieldPath("id"),
			"100",
			"p_",
			SQLDialectMySQL,
		)
		ErrLike(
			t,
			err,
			[]any{"lastSortValues has 1 values, expected 2 values matching order fields"},
		)
	})
}

// ============================================================================
// Lexicographic Comparison Tests
// ============================================================================

func TestBuildLexicographicComparison_SingleField(t *testing.T) {
	fields := []OrderByField{
		{
			Column:    &Column{databaseName: "created_at"},
			Direction: "ASC",
		},
	}
	values := []interface{}{"2024-01-01"}

	sql, params, err := buildLexicographicComparison(fields, values)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedSQL := "((created_at > @seek_cmp_0))"
	if sql != expectedSQL {
		t.Errorf("expected SQL %q, got %q", expectedSQL, sql)
	}

	if len(params) != 1 {
		t.Fatalf("expected 1 parameter, got %d", len(params))
	}

	if params[0].Name != "@seek_cmp_0" || params[0].Value != "2024-01-01" {
		t.Errorf("unexpected parameter: %+v", params[0])
	}
}

func TestBuildLexicographicComparison_TwoFields(t *testing.T) {
	fields := []OrderByField{
		{
			Column:    &Column{databaseName: "status"},
			Direction: "ASC",
		},
		{
			Column:    &Column{databaseName: "id"},
			Direction: "ASC",
		},
	}
	values := []interface{}{"active", 123}

	sql, params, err := buildLexicographicComparison(fields, values)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedSQL := "((status > @seek_cmp_0) OR (status = @seek_eq_1 AND id > @seek_cmp_2))"
	if sql != expectedSQL {
		t.Errorf("expected SQL:\n%s\ngot:\n%s", expectedSQL, sql)
	}

	if len(params) != 3 {
		t.Fatalf("expected 3 parameters, got %d", len(params))
	}

	// Verify parameters
	expectedParams := []QueryParameter{
		{Name: "@seek_cmp_0", Value: "active"},
		{Name: "@seek_eq_1", Value: "active"},
		{Name: "@seek_cmp_2", Value: 123},
	}

	for i, expected := range expectedParams {
		if params[i].Name != expected.Name || params[i].Value != expected.Value {
			t.Errorf("parameter %d: expected %+v, got %+v", i, expected, params[i])
		}
	}
}

func TestBuildLexicographicComparison_ThreeFields(t *testing.T) {
	fields := []OrderByField{
		{
			Column:    &Column{databaseName: "a"},
			Direction: "ASC",
		},
		{
			Column:    &Column{databaseName: "b"},
			Direction: "ASC",
		},
		{
			Column:    &Column{databaseName: "c"},
			Direction: "ASC",
		},
	}
	values := []interface{}{1, 2, 3}

	sql, params, err := buildLexicographicComparison(fields, values)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedSQL := "((a > @seek_cmp_0) OR (a = @seek_eq_1 AND b > @seek_cmp_2) OR (a = @seek_eq_3 AND b = @seek_eq_4 AND c > @seek_cmp_5))"
	if sql != expectedSQL {
		t.Errorf("expected SQL:\n%s\ngot:\n%s", expectedSQL, sql)
	}

	if len(params) != 6 {
		t.Fatalf("expected 6 parameters, got %d", len(params))
	}
}

func TestBuildLexicographicComparison_MixedDirections(t *testing.T) {
	fields := []OrderByField{
		{
			Column:    &Column{databaseName: "created_at"},
			Direction: "DESC",
		},
		{
			Column:    &Column{databaseName: "id"},
			Direction: "DESC",
		},
	}
	values := []interface{}{"2024-01-15T10:30:00Z", 12345}

	sql, params, err := buildLexicographicComparison(fields, values)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// For DESC, we should use < operator
	if !strings.Contains(sql, "created_at < @seek_cmp_0") {
		t.Errorf("expected DESC to use < operator, got: %s", sql)
	}

	if !strings.Contains(sql, "id < @seek_cmp_2") {
		t.Errorf("expected DESC to use < operator for id, got: %s", sql)
	}

	expectedSQL := "((created_at < @seek_cmp_0) OR (created_at = @seek_eq_1 AND id < @seek_cmp_2))"
	if sql != expectedSQL {
		t.Errorf("expected SQL:\n%s\ngot:\n%s", expectedSQL, sql)
	}

	// Verify parameters
	if len(params) != 3 {
		t.Fatalf("expected 3 parameters, got %d", len(params))
	}
}

func TestBuildLexicographicComparison_MixedAscDesc(t *testing.T) {
	fields := []OrderByField{
		{
			Column:    &Column{databaseName: "priority"},
			Direction: "DESC",
		},
		{
			Column:    &Column{databaseName: "created_at"},
			Direction: "ASC",
		},
	}
	values := []interface{}{5, "2024-01-01"}

	sql, params, err := buildLexicographicComparison(fields, values)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// First level: priority DESC should use <
	if !strings.Contains(sql, "priority < @seek_cmp_0") {
		t.Errorf("expected priority DESC to use < operator, got: %s", sql)
	}

	// Second level: created_at ASC should use >
	if !strings.Contains(sql, "created_at > @seek_cmp_2") {
		t.Errorf("expected created_at ASC to use > operator, got: %s", sql)
	}

	// Verify parameters
	if len(params) != 3 {
		t.Fatalf("expected 3 parameters, got %d", len(params))
	}
}

func TestBuildLexicographicComparison_FieldValueMismatch(t *testing.T) {
	fields := []OrderByField{
		{
			Column:    &Column{databaseName: "a"},
			Direction: "ASC",
		},
		{
			Column:    &Column{databaseName: "b"},
			Direction: "ASC",
		},
	}
	values := []interface{}{1} // Only 1 value, but 2 fields

	sql, params, err := buildLexicographicComparison(fields, values)

	if err == nil {
		t.Fatal("expected error for mismatched field and value counts")
	}

	if sql != "" || len(params) != 0 {
		t.Errorf("expected empty results on error, got sql=%q, params=%v", sql, params)
	}

	expectedError := "field count (2) does not match value count (1)"
	if err.Error() != expectedError {
		t.Errorf("expected error %q, got %q", expectedError, err.Error())
	}
}

func TestBuildLexicographicComparison_EmptyFields(t *testing.T) {
	fields := []OrderByField{}
	values := []interface{}{}

	sql, params, err := buildLexicographicComparison(fields, values)

	if err == nil {
		t.Fatal("expected error for empty fields")
	}

	if sql != "" || len(params) != 0 {
		t.Errorf("expected empty results on error, got sql=%q, params=%v", sql, params)
	}

	expectedError := "fields cannot be empty"
	if err.Error() != expectedError {
		t.Errorf("expected error %q, got %q", expectedError, err.Error())
	}
}

func TestGetComparisonOperator(t *testing.T) {
	tests := []struct {
		direction string
		expected  string
	}{
		{"ASC", ">"},
		{"asc", ">"},
		{"DESC", "<"},
		{"desc", "<"},
		{"", ">"},        // Default to ASC
		{"invalid", ">"}, // Default to ASC
	}

	for _, tt := range tests {
		result := getComparisonOperator(tt.direction)
		if result != tt.expected {
			t.Errorf(
				"getComparisonOperator(%q) = %q, expected %q",
				tt.direction,
				result,
				tt.expected,
			)
		}
	}
}

func TestBuildLexicographicComparison_ParameterNaming(t *testing.T) {
	// Verify that parameters are named correctly with seek_eq and seek_cmp prefixes
	fields := []OrderByField{
		{
			Column:    &Column{databaseName: "a"},
			Direction: "ASC",
		},
		{
			Column:    &Column{databaseName: "b"},
			Direction: "ASC",
		},
		{
			Column:    &Column{databaseName: "c"},
			Direction: "ASC",
		},
	}
	values := []interface{}{1, 2, 3}

	_, params, err := buildLexicographicComparison(fields, values)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Expected parameter names in order
	expectedNames := []string{
		"@seek_cmp_0", // Level 0: a > @seek_cmp_0
		"@seek_eq_1",  // Level 1: a = @seek_eq_1
		"@seek_cmp_2", // Level 1: b > @seek_cmp_2
		"@seek_eq_3",  // Level 2: a = @seek_eq_3
		"@seek_eq_4",  // Level 2: b = @seek_eq_4
		"@seek_cmp_5", // Level 2: c > @seek_cmp_5
	}

	if len(params) != len(expectedNames) {
		t.Fatalf("expected %d parameters, got %d", len(expectedNames), len(params))
	}

	for i, expected := range expectedNames {
		if params[i].Name != expected {
			t.Errorf("parameter %d: expected name %q, got %q", i, expected, params[i].Name)
		}
	}
}
