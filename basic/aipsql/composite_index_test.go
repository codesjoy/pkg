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
// Simple Condition Reordering Tests
// ============================================================================

// TestSimpleConditionReordering tests basic condition reordering with simple values
func TestSimpleConditionReordering(t *testing.T) {
	t.Run("simple three column reordering", func(t *testing.T) {
		// Create table with composite index: (a, b, c)
		table := NewTable().WithColumns(
			NewColumn().
				WithFieldPath("a").
				WithDatabaseName("a").
				Filterable().
				Build(),
			NewColumn().
				WithFieldPath("b").
				WithDatabaseName("b").
				Filterable().
				Build(),
			NewColumn().
				WithFieldPath("c").
				WithDatabaseName("c").
				Filterable().
				Build(),
		).Build()

		// Add composite index
		table.CompositeIndexes = []CompositeIndex{
			{
				Name:    "idx_abc",
				Columns: []string{"a", "b", "c"},
			},
		}

		// Filter with reverse order: c, b, a
		filter := "c=\"value_c\" AND b=\"value_b\" AND a=\"value_a\""

		parsedFilter, err := ParseFilter(filter)
		require.NoError(t, err)

		sql, params, err := table.WhereClauseWithOptions(parsedFilter, "p_", WhereClauseOptions{
			EnableCompositeIndexOptimization: true,
		})

		require.NoError(t, err)
		t.Logf("Generated SQL: %s", sql)
		t.Logf("Parameters: %+v", params)

		// Extract condition positions
		aPos := strings.Index(sql, "a =")
		bPos := strings.Index(sql, "b =")
		cPos := strings.Index(sql, "c =")

		// All conditions should be present
		assert.NotEqual(t, -1, aPos, "a condition should be present")
		assert.NotEqual(t, -1, bPos, "b condition should be present")
		assert.NotEqual(t, -1, cPos, "c condition should be present")

		// Conditions should be reordered to match index: a, b, c
		assert.Less(t, aPos, bPos, "a should come before b")
		assert.Less(t, bPos, cPos, "b should come before c")
	})

	t.Run("without optimization preserves order", func(t *testing.T) {
		// Create table with composite index
		table := NewTable().WithColumns(
			NewColumn().
				WithFieldPath("a").
				WithDatabaseName("a").
				Filterable().
				Build(),
			NewColumn().
				WithFieldPath("b").
				WithDatabaseName("b").
				Filterable().
				Build(),
		).Build()

		// Add composite index
		table.CompositeIndexes = []CompositeIndex{
			{
				Name:    "idx_ab",
				Columns: []string{"a", "b"},
			},
		}

		// Filter with b before a
		filter := "b=\"value_b\" AND a=\"value_a\""

		parsedFilter, err := ParseFilter(filter)
		require.NoError(t, err)

		// Generate SQL without optimization
		sqlNoOpt, _, err := table.WhereClauseWithOptions(parsedFilter, "p_", WhereClauseOptions{
			EnableCompositeIndexOptimization: false,
		})

		require.NoError(t, err)
		t.Logf("SQL without optimization: %s", sqlNoOpt)

		// Extract positions without optimization
		bPosNoOpt := strings.Index(sqlNoOpt, "b =")
		aPosNoOpt := strings.Index(sqlNoOpt, "a =")

		// Without optimization, original order should be preserved (b before a)
		assert.NotEqual(t, -1, bPosNoOpt, "b condition should be present")
		assert.NotEqual(t, -1, aPosNoOpt, "a condition should be present")
		assert.Less(t, bPosNoOpt, aPosNoOpt, "b should come before a (original order)")
	})
}

// ============================================================================
// Nested Composite Tests
// ============================================================================

func TestWhereClauseWithOptions_ReordersAcrossNestedConjunction(t *testing.T) {
	table := NewTable().WithColumns(
		NewColumn().WithFieldPath("status").WithDatabaseName("status").Filterable().Build(),
		NewColumn().WithFieldPath("user_id").WithDatabaseName("user_id").Filterable().Build(),
		NewColumn().WithFieldPath("created_at").WithDatabaseName("created_at").Filterable().Build(),
	).Build()
	table.CompositeIndexes = []CompositeIndex{{
		Name:    "idx_status_user_created",
		Columns: []string{"status", "user_id", "created_at"},
	}}

	filter, err := ParseFilter(`status="active" AND (created_at>"2024-01-01" AND user_id=123)`)
	require.NoError(t, err)

	sql, params, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
		EnableCompositeIndexOptimization: true,
	})
	require.NoError(t, err)

	assert.Equal(t, "((status = @p_0) AND (user_id = @p_1) AND (created_at > @p_2))", sql)
	assert.Equal(t, []QueryParameter{
		{Name: "p_0", Value: "active"},
		{Name: "p_1", Value: "123"},
		{Name: "p_2", Value: "2024-01-01"},
	}, params)
}

// ============================================================================
// Property-Based Tests
// ============================================================================

// **Validates: Requirements 11.2, 11.3**
// Feature: aip-sql-execution-optimization, Property 20: 复合索引条件重排序
//
// For any WHERE conditions and configured Composite_Index, when composite index optimization
// is enabled, the Filter_Generator should reorder conditions to match the index column order,
// with equality conditions placed before range conditions.
func TestProperty_CompositeIndexConditionReordering(t *testing.T) {
	t.Run("equality conditions placed before range conditions", func(t *testing.T) {
		property := func(statusValue string, userID int, createdAfter string) bool {
			// Skip empty values
			if statusValue == "" || createdAfter == "" {
				return true
			}

			// Create table with composite index: (status, user_id, created_at)
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
				NewColumn().
					WithFieldPath("created_at").
					WithDatabaseName("created_at").
					Filterable().
					Build(),
			).Build()

			// Add composite index
			table.CompositeIndexes = []CompositeIndex{
				{
					Name:    "idx_status_user_created",
					Columns: []string{"status", "user_id", "created_at"},
				},
			}

			// Filter with range condition first, then equality conditions
			// This tests that reordering happens correctly
			filter := fmt.Sprintf("created_at>\"%s\" AND user_id=%d AND status=\"%s\"",
				createdAfter, userID, statusValue)

			parsedFilter, err := ParseFilter(filter)
			if err != nil {
				return true
			}

			sql, _, err := table.WhereClauseWithOptions(parsedFilter, "p_", WhereClauseOptions{
				EnableCompositeIndexOptimization: true,
			})
			if err != nil {
				return true
			}

			// Extract condition order from SQL
			// Expected order: status (equality), user_id (equality), created_at (range)
			statusPos := strings.Index(sql, "status")
			userIDPos := strings.Index(sql, "user_id")
			createdAtPos := strings.Index(sql, "created_at")

			// All conditions should be present
			if statusPos == -1 || userIDPos == -1 || createdAtPos == -1 {
				return false
			}

			// Equality conditions (status, user_id) should come before range condition (created_at)
			if statusPos >= createdAtPos || userIDPos >= createdAtPos {
				return false
			}

			// Conditions should follow index order: status before user_id
			if statusPos >= userIDPos {
				return false
			}

			return true
		}

		config := &quick.Config{MaxCount: 100}
		if err := quick.Check(property, config); err != nil {
			t.Error(err)
		}
	})

	t.Run("conditions reordered to match index column order", func(t *testing.T) {
		property := func(aValue, bValue, cValue string) bool {
			// Skip empty values or values with special characters that might interfere with SQL parsing
			if aValue == "" || bValue == "" || cValue == "" {
				return true
			}
			// Skip values with quotes that would break the filter string
			if strings.Contains(aValue, "\"") || strings.Contains(bValue, "\"") ||
				strings.Contains(cValue, "\"") {
				return true
			}

			// Create table with composite index: (a, b, c)
			table := NewTable().WithColumns(
				NewColumn().
					WithFieldPath("a").
					WithDatabaseName("a").
					Filterable().
					Build(),
				NewColumn().
					WithFieldPath("b").
					WithDatabaseName("b").
					Filterable().
					Build(),
				NewColumn().
					WithFieldPath("c").
					WithDatabaseName("c").
					Filterable().
					Build(),
			).Build()

			// Add composite index
			table.CompositeIndexes = []CompositeIndex{
				{
					Name:    "idx_abc",
					Columns: []string{"a", "b", "c"},
				},
			}

			// Filter with reverse order: c, b, a
			filter := fmt.Sprintf("c=\"%s\" AND b=\"%s\" AND a=\"%s\"",
				cValue, bValue, aValue)

			parsedFilter, err := ParseFilter(filter)
			if err != nil {
				return true
			}

			sql, _, err := table.WhereClauseWithOptions(parsedFilter, "p_", WhereClauseOptions{
				EnableCompositeIndexOptimization: true,
			})
			if err != nil {
				return true
			}

			// Extract condition positions
			aPos := strings.Index(sql, "a =")
			bPos := strings.Index(sql, "b =")
			cPos := strings.Index(sql, "c =")

			// All conditions should be present
			if aPos == -1 || bPos == -1 || cPos == -1 {
				return false
			}

			// Conditions should be reordered to match index: a, b, c
			if aPos >= bPos || bPos >= cPos {
				return false
			}

			return true
		}

		config := &quick.Config{MaxCount: 100}
		if err := quick.Check(property, config); err != nil {
			t.Error(err)
		}
	})

	t.Run("optimization disabled preserves original order", func(t *testing.T) {
		property := func(aValue, bValue string) bool {
			// Skip empty values
			if aValue == "" || bValue == "" {
				return true
			}

			// Create table with composite index
			table := NewTable().WithColumns(
				NewColumn().
					WithFieldPath("a").
					WithDatabaseName("a").
					Filterable().
					Build(),
				NewColumn().
					WithFieldPath("b").
					WithDatabaseName("b").
					Filterable().
					Build(),
			).Build()

			// Add composite index
			table.CompositeIndexes = []CompositeIndex{
				{
					Name:    "idx_ab",
					Columns: []string{"a", "b"},
				},
			}

			// Filter with b before a
			filter := fmt.Sprintf("b=\"%s\" AND a=\"%s\"", bValue, aValue)

			parsedFilter, err := ParseFilter(filter)
			if err != nil {
				return true
			}

			// Generate SQL without optimization
			sqlNoOpt, _, err := table.WhereClauseWithOptions(parsedFilter, "p_", WhereClauseOptions{
				EnableCompositeIndexOptimization: false,
			})
			if err != nil {
				return true
			}

			// Extract positions without optimization
			bPosNoOpt := strings.Index(sqlNoOpt, "b =")
			aPosNoOpt := strings.Index(sqlNoOpt, "a =")

			// Without optimization, original order should be preserved (b before a)
			if bPosNoOpt == -1 || aPosNoOpt == -1 {
				return false
			}

			// b should come before a (original order)
			if bPosNoOpt >= aPosNoOpt {
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

// **Validates: Requirements 11.10, 11.11**
// Feature: aip-sql-execution-optimization, Property 24: 优化后语义等价性
//
// For any filter expression, the SQL statements generated before and after enabling
// composite index optimization should be semantically equivalent (return the same query results),
// and condition reordering should preserve AND and OR logic correctness.
func TestProperty_OptimizationPreservesSemantics(t *testing.T) {
	t.Run("all original conditions present after reordering", func(t *testing.T) {
		property := func(aValue, bValue, cValue string) bool {
			// Skip empty values
			if aValue == "" || bValue == "" || cValue == "" {
				return true
			}

			// Create table with composite index
			table := NewTable().WithColumns(
				NewColumn().
					WithFieldPath("a").
					WithDatabaseName("a").
					Filterable().
					Build(),
				NewColumn().
					WithFieldPath("b").
					WithDatabaseName("b").
					Filterable().
					Build(),
				NewColumn().
					WithFieldPath("c").
					WithDatabaseName("c").
					Filterable().
					Build(),
			).Build()

			// Add composite index
			table.CompositeIndexes = []CompositeIndex{
				{
					Name:    "idx_abc",
					Columns: []string{"a", "b", "c"},
				},
			}

			filter := fmt.Sprintf("c=\"%s\" AND b=\"%s\" AND a=\"%s\"",
				cValue, bValue, aValue)

			parsedFilter, err := ParseFilter(filter)
			if err != nil {
				return true
			}

			// Generate SQL with optimization
			sqlOpt, paramsOpt, err := table.WhereClauseWithOptions(
				parsedFilter,
				"p_",
				WhereClauseOptions{
					EnableCompositeIndexOptimization: true,
				},
			)
			if err != nil {
				return true
			}

			// Verify all three conditions are present
			if !strings.Contains(sqlOpt, "a =") {
				return false
			}
			if !strings.Contains(sqlOpt, "b =") {
				return false
			}
			if !strings.Contains(sqlOpt, "c =") {
				return false
			}

			// Verify we have 3 parameters (one for each condition)
			if len(paramsOpt) != 3 {
				return false
			}

			// Verify parameter values match the input values
			paramValues := make(map[string]bool)
			for _, p := range paramsOpt {
				if str, ok := p.Value.(string); ok {
					paramValues[str] = true
				}
			}

			if !paramValues[aValue] || !paramValues[bValue] || !paramValues[cValue] {
				return false
			}

			return true
		}

		config := &quick.Config{MaxCount: 100}
		if err := quick.Check(property, config); err != nil {
			t.Error(err)
		}
	})

	t.Run("AND logic preserved after reordering", func(t *testing.T) {
		property := func(aValue, bValue string) bool {
			// Skip empty values
			if aValue == "" || bValue == "" {
				return true
			}

			// Create table with composite index
			table := NewTable().WithColumns(
				NewColumn().
					WithFieldPath("a").
					WithDatabaseName("a").
					Filterable().
					Build(),
				NewColumn().
					WithFieldPath("b").
					WithDatabaseName("b").
					Filterable().
					Build(),
			).Build()

			// Add composite index
			table.CompositeIndexes = []CompositeIndex{
				{
					Name:    "idx_ab",
					Columns: []string{"a", "b"},
				},
			}

			filter := fmt.Sprintf("b=\"%s\" AND a=\"%s\"", bValue, aValue)

			parsedFilter, err := ParseFilter(filter)
			if err != nil {
				return true
			}

			sqlOpt, _, err := table.WhereClauseWithOptions(parsedFilter, "p_", WhereClauseOptions{
				EnableCompositeIndexOptimization: true,
			})
			if err != nil {
				return true
			}

			// Verify AND is used (not OR)
			andCount := strings.Count(sqlOpt, " AND ")
			orCount := strings.Count(sqlOpt, " OR ")

			// Should have exactly 1 AND and 0 OR
			if andCount != 1 || orCount != 0 {
				return false
			}

			return true
		}

		config := &quick.Config{MaxCount: 100}
		if err := quick.Check(property, config); err != nil {
			t.Error(err)
		}
	})

	t.Run("OR conditions not reordered", func(t *testing.T) {
		property := func(aValue, bValue string) bool {
			// Skip empty values
			if aValue == "" || bValue == "" {
				return true
			}

			// Create table with composite index
			table := NewTable().WithColumns(
				NewColumn().
					WithFieldPath("a").
					WithDatabaseName("a").
					Filterable().
					Build(),
				NewColumn().
					WithFieldPath("b").
					WithDatabaseName("b").
					Filterable().
					Build(),
			).Build()

			// Add composite index
			table.CompositeIndexes = []CompositeIndex{
				{
					Name:    "idx_ab",
					Columns: []string{"a", "b"},
				},
			}

			// Filter with OR (should not be reordered)
			filter := fmt.Sprintf("b=\"%s\" OR a=\"%s\"", bValue, aValue)

			parsedFilter, err := ParseFilter(filter)
			if err != nil {
				return true
			}

			sqlOpt, _, err := table.WhereClauseWithOptions(parsedFilter, "p_", WhereClauseOptions{
				EnableCompositeIndexOptimization: true,
			})
			if err != nil {
				return true
			}

			// Verify OR is preserved
			if !strings.Contains(sqlOpt, " OR ") {
				return false
			}

			// Both conditions should still be present
			if !strings.Contains(sqlOpt, "a =") || !strings.Contains(sqlOpt, "b =") {
				return false
			}

			return true
		}

		config := &quick.Config{MaxCount: 100}
		if err := quick.Check(property, config); err != nil {
			t.Error(err)
		}
	})

	t.Run("same number of conditions before and after optimization", func(t *testing.T) {
		property := func(aValue, bValue, cValue string) bool {
			// Skip empty values
			if aValue == "" || bValue == "" || cValue == "" {
				return true
			}

			// Create table with composite index
			table := NewTable().WithColumns(
				NewColumn().
					WithFieldPath("a").
					WithDatabaseName("a").
					Filterable().
					Build(),
				NewColumn().
					WithFieldPath("b").
					WithDatabaseName("b").
					Filterable().
					Build(),
				NewColumn().
					WithFieldPath("c").
					WithDatabaseName("c").
					Filterable().
					Build(),
			).Build()

			// Add composite index
			table.CompositeIndexes = []CompositeIndex{
				{
					Name:    "idx_abc",
					Columns: []string{"a", "b", "c"},
				},
			}

			filter := fmt.Sprintf("c=\"%s\" AND b=\"%s\" AND a=\"%s\"",
				cValue, bValue, aValue)

			parsedFilter, err := ParseFilter(filter)
			if err != nil {
				return true
			}

			// Generate SQL without optimization
			sqlNoOpt, paramsNoOpt, err := table.WhereClauseWithOptions(
				parsedFilter,
				"p_",
				WhereClauseOptions{
					EnableCompositeIndexOptimization: false,
				},
			)
			if err != nil {
				return true
			}

			// Generate SQL with optimization
			sqlOpt, paramsOpt, err := table.WhereClauseWithOptions(
				parsedFilter,
				"p_",
				WhereClauseOptions{
					EnableCompositeIndexOptimization: true,
				},
			)
			if err != nil {
				return true
			}

			// Count AND operators (should be same)
			andCountNoOpt := strings.Count(sqlNoOpt, " AND ")
			andCountOpt := strings.Count(sqlOpt, " AND ")

			if andCountNoOpt != andCountOpt {
				return false
			}

			// Parameter count should be same
			if len(paramsNoOpt) != len(paramsOpt) {
				return false
			}

			return true
		}

		config := &quick.Config{MaxCount: 100}
		if err := quick.Check(property, config); err != nil {
			t.Error(err)
		}
	})

	t.Run("mixed equality and range conditions preserve semantics", func(t *testing.T) {
		property := func(statusValue string, minID, maxID int) bool {
			// Skip invalid ranges
			if statusValue == "" || minID >= maxID {
				return true
			}

			// Create table with composite index
			table := NewTable().WithColumns(
				NewColumn().
					WithFieldPath("status").
					WithDatabaseName("status").
					Filterable().
					Build(),
				NewColumn().
					WithFieldPath("id").
					WithDatabaseName("id").
					Filterable().
					Build(),
			).Build()

			// Add composite index
			table.CompositeIndexes = []CompositeIndex{
				{
					Name:    "idx_status_id",
					Columns: []string{"status", "id"},
				},
			}

			// Filter with range condition first, then equality
			filter := fmt.Sprintf("id>%d AND id<%d AND status=\"%s\"",
				minID, maxID, statusValue)

			parsedFilter, err := ParseFilter(filter)
			if err != nil {
				return true
			}

			sqlOpt, paramsOpt, err := table.WhereClauseWithOptions(
				parsedFilter,
				"p_",
				WhereClauseOptions{
					EnableCompositeIndexOptimization: true,
				},
			)
			if err != nil {
				return true
			}

			// Verify all conditions are present
			if !strings.Contains(sqlOpt, "status =") {
				return false
			}
			if !strings.Contains(sqlOpt, "id >") {
				return false
			}
			if !strings.Contains(sqlOpt, "id <") {
				return false
			}

			// Verify we have 3 parameters
			if len(paramsOpt) != 3 {
				return false
			}

			// Verify equality condition (status) comes before range conditions (id)
			statusPos := strings.Index(sqlOpt, "status =")
			idPos1 := strings.Index(sqlOpt, "id >")
			idPos2 := strings.Index(sqlOpt, "id <")

			if statusPos >= idPos1 || statusPos >= idPos2 {
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

// TestProperty_ConditionReorderingEdgeCases tests edge cases for condition reordering
func TestProperty_ConditionReorderingEdgeCases(t *testing.T) {
	t.Run("single condition not reordered", func(t *testing.T) {
		table := NewTable().WithColumns(
			NewColumn().
				WithFieldPath("a").
				WithDatabaseName("a").
				Filterable().
				Build(),
		).Build()

		// Add composite index
		table.CompositeIndexes = []CompositeIndex{
			{
				Name:    "idx_a",
				Columns: []string{"a"},
			},
		}

		filter := "a=\"test\""
		parsedFilter, err := ParseFilter(filter)
		assert.NoError(t, err)

		sql, _, err := table.WhereClauseWithOptions(parsedFilter, "p_", WhereClauseOptions{
			EnableCompositeIndexOptimization: true,
		})

		assert.NoError(t, err)
		assert.Contains(t, sql, "a =")
	})

	t.Run("conditions not in index placed at end", func(t *testing.T) {
		table := NewTable().WithColumns(
			NewColumn().
				WithFieldPath("a").
				WithDatabaseName("a").
				Filterable().
				Build(),
			NewColumn().
				WithFieldPath("b").
				WithDatabaseName("b").
				Filterable().
				Build(),
			NewColumn().
				WithFieldPath("x").
				WithDatabaseName("x").
				Filterable().
				Build(),
		).Build()

		// Add composite index
		table.CompositeIndexes = []CompositeIndex{
			{
				Name:    "idx_ab",
				Columns: []string{"a", "b"},
			},
		}

		// x is not in the index
		filter := "x=\"test\" AND b=\"value\" AND a=\"key\""
		parsedFilter, err := ParseFilter(filter)
		assert.NoError(t, err)

		sql, _, err := table.WhereClauseWithOptions(parsedFilter, "p_", WhereClauseOptions{
			EnableCompositeIndexOptimization: true,
		})

		assert.NoError(t, err)

		// Extract positions
		aPos := strings.Index(sql, "a =")
		bPos := strings.Index(sql, "b =")
		xPos := strings.Index(sql, "x =")

		// a and b should come before x
		assert.Less(t, aPos, xPos, "a should come before x")
		assert.Less(t, bPos, xPos, "b should come before x")
		// a should come before b (index order)
		assert.Less(t, aPos, bPos, "a should come before b")
	})

	t.Run("no composite index configured", func(t *testing.T) {
		table := NewTable().WithColumns(
			NewColumn().
				WithFieldPath("a").
				WithDatabaseName("a").
				Filterable().
				Build(),
			NewColumn().
				WithFieldPath("b").
				WithDatabaseName("b").
				Filterable().
				Build(),
		).Build() // No composite indexes

		filter := "b=\"value\" AND a=\"key\""
		parsedFilter, err := ParseFilter(filter)
		assert.NoError(t, err)

		sql, _, err := table.WhereClauseWithOptions(parsedFilter, "p_", WhereClauseOptions{
			EnableCompositeIndexOptimization: true,
		})

		assert.NoError(t, err)
		// Should still generate valid SQL, just without reordering
		assert.Contains(t, sql, "a =")
		assert.Contains(t, sql, "b =")
	})
}

// **Validates: Requirements 11.8**
// Feature: aip-sql-execution-optimization, Property 23: 复合索引优化开关
//
// For any query, when enableCompositeIndexOptimization is false, the generated WHERE
// condition order should be the same as when optimization is not enabled.
func TestProperty_CompositeIndexOptimizationFlag(t *testing.T) {
	t.Run("flag disabled preserves original condition order", func(t *testing.T) {
		property := func(aValue, bValue, cValue string) bool {
			// Skip empty values or values with special characters
			if aValue == "" || bValue == "" || cValue == "" {
				return true
			}
			if strings.Contains(aValue, "\"") || strings.Contains(bValue, "\"") ||
				strings.Contains(cValue, "\"") {
				return true
			}

			// Create table with composite index: (a, b, c)
			table := NewTable().WithColumns(
				NewColumn().
					WithFieldPath("a").
					WithDatabaseName("a").
					Filterable().
					Build(),
				NewColumn().
					WithFieldPath("b").
					WithDatabaseName("b").
					Filterable().
					Build(),
				NewColumn().
					WithFieldPath("c").
					WithDatabaseName("c").
					Filterable().
					Build(),
			).Build()

			// Add composite index
			table.CompositeIndexes = []CompositeIndex{
				{
					Name:    "idx_abc",
					Columns: []string{"a", "b", "c"},
				},
			}

			// Filter with reverse order: c, b, a (opposite of index order)
			filter := fmt.Sprintf("c=\"%s\" AND b=\"%s\" AND a=\"%s\"",
				cValue, bValue, aValue)

			parsedFilter, err := ParseFilter(filter)
			if err != nil {
				return true
			}

			// Generate SQL with optimization disabled
			sqlDisabled, _, err := table.WhereClauseWithOptions(
				parsedFilter,
				"p_",
				WhereClauseOptions{
					EnableCompositeIndexOptimization: false,
				},
			)
			if err != nil {
				return true
			}

			// Extract condition positions when optimization is disabled
			cPosDisabled := strings.Index(sqlDisabled, "c =")
			bPosDisabled := strings.Index(sqlDisabled, "b =")
			aPosDisabled := strings.Index(sqlDisabled, "a =")

			// All conditions should be present
			if cPosDisabled == -1 || bPosDisabled == -1 || aPosDisabled == -1 {
				return false
			}

			// When optimization is disabled, original order should be preserved (c, b, a)
			if cPosDisabled >= bPosDisabled || bPosDisabled >= aPosDisabled {
				return false
			}

			return true
		}

		config := &quick.Config{MaxCount: 100}
		if err := quick.Check(property, config); err != nil {
			t.Error(err)
		}
	})

	t.Run("flag enabled reorders conditions to match index", func(t *testing.T) {
		property := func(aValue, bValue, cValue string) bool {
			// Skip empty values or values with special characters
			if aValue == "" || bValue == "" || cValue == "" {
				return true
			}
			if strings.Contains(aValue, "\"") || strings.Contains(bValue, "\"") ||
				strings.Contains(cValue, "\"") {
				return true
			}

			// Create table with composite index: (a, b, c)
			table := NewTable().WithColumns(
				NewColumn().
					WithFieldPath("a").
					WithDatabaseName("a").
					Filterable().
					Build(),
				NewColumn().
					WithFieldPath("b").
					WithDatabaseName("b").
					Filterable().
					Build(),
				NewColumn().
					WithFieldPath("c").
					WithDatabaseName("c").
					Filterable().
					Build(),
			).Build()

			// Add composite index
			table.CompositeIndexes = []CompositeIndex{
				{
					Name:    "idx_abc",
					Columns: []string{"a", "b", "c"},
				},
			}

			// Filter with reverse order: c, b, a (opposite of index order)
			filter := fmt.Sprintf("c=\"%s\" AND b=\"%s\" AND a=\"%s\"",
				cValue, bValue, aValue)

			parsedFilter, err := ParseFilter(filter)
			if err != nil {
				return true
			}

			// Generate SQL with optimization enabled
			sqlEnabled, _, err := table.WhereClauseWithOptions(
				parsedFilter,
				"p_",
				WhereClauseOptions{
					EnableCompositeIndexOptimization: true,
				},
			)
			if err != nil {
				return true
			}

			// Extract condition positions when optimization is enabled
			cPosEnabled := strings.Index(sqlEnabled, "c =")
			bPosEnabled := strings.Index(sqlEnabled, "b =")
			aPosEnabled := strings.Index(sqlEnabled, "a =")

			// All conditions should be present
			if cPosEnabled == -1 || bPosEnabled == -1 || aPosEnabled == -1 {
				return false
			}

			// When optimization is enabled, conditions should be reordered to match index (a, b, c)
			if aPosEnabled >= bPosEnabled || bPosEnabled >= cPosEnabled {
				return false
			}

			return true
		}

		config := &quick.Config{MaxCount: 100}
		if err := quick.Check(property, config); err != nil {
			t.Error(err)
		}
	})

	t.Run(
		"flag disabled and enabled produce different order for non-optimal filters",
		func(t *testing.T) {
			property := func(aValue, bValue string) bool {
				// Skip empty values
				if aValue == "" || bValue == "" {
					return true
				}
				// Skip values with special characters that might interfere with parsing
				if strings.Contains(aValue, "\"") || strings.Contains(bValue, "\"") {
					return true
				}
				// Skip values with backslashes or other problematic characters
				if strings.ContainsAny(aValue, "\\'\n\r\t") ||
					strings.ContainsAny(bValue, "\\'\n\r\t") {
					return true
				}

				// Create table with composite index: (a, b)
				table := NewTable().WithColumns(
					NewColumn().
						WithFieldPath("a").
						WithDatabaseName("a").
						Filterable().
						Build(),
					NewColumn().
						WithFieldPath("b").
						WithDatabaseName("b").
						Filterable().
						Build(),
				).Build()

				// Add composite index
				table.CompositeIndexes = []CompositeIndex{
					{
						Name:    "idx_ab",
						Columns: []string{"a", "b"},
					},
				}

				// Filter with reverse order: b, a (opposite of index order)
				filter := fmt.Sprintf("b=\"%s\" AND a=\"%s\"", bValue, aValue)

				parsedFilter, err := ParseFilter(filter)
				if err != nil {
					return true
				}

				// Generate SQL with optimization disabled
				sqlDisabled, _, err := table.WhereClauseWithOptions(
					parsedFilter,
					"p_",
					WhereClauseOptions{
						EnableCompositeIndexOptimization: false,
					},
				)
				if err != nil {
					return true
				}

				// Generate SQL with optimization enabled
				sqlEnabled, _, err := table.WhereClauseWithOptions(
					parsedFilter,
					"p_",
					WhereClauseOptions{
						EnableCompositeIndexOptimization: true,
					},
				)
				if err != nil {
					return true
				}

				// Extract positions for disabled optimization
				bPosDisabled := strings.Index(sqlDisabled, "b =")
				aPosDisabled := strings.Index(sqlDisabled, "a =")

				// Extract positions for enabled optimization
				bPosEnabled := strings.Index(sqlEnabled, "b =")
				aPosEnabled := strings.Index(sqlEnabled, "a =")

				// All conditions should be present in both
				if bPosDisabled == -1 || aPosDisabled == -1 || bPosEnabled == -1 ||
					aPosEnabled == -1 {
					return false
				}

				// When disabled: b should come before a (original order)
				if bPosDisabled >= aPosDisabled {
					return false
				}

				// When enabled: a should come before b (index order)
				if aPosEnabled >= bPosEnabled {
					return false
				}

				return true
			}

			config := &quick.Config{MaxCount: 100}
			if err := quick.Check(property, config); err != nil {
				t.Error(err)
			}
		},
	)

	t.Run(
		"flag disabled with equality and range conditions preserves original order",
		func(t *testing.T) {
			property := func(statusValue string, minID, maxID int) bool {
				// Skip invalid ranges
				if statusValue == "" || minID >= maxID {
					return true
				}
				if strings.Contains(statusValue, "\"") {
					return true
				}

				// Create table with composite index: (status, id)
				table := NewTable().WithColumns(
					NewColumn().
						WithFieldPath("status").
						WithDatabaseName("status").
						Filterable().
						Build(),
					NewColumn().
						WithFieldPath("id").
						WithDatabaseName("id").
						Filterable().
						Build(),
				).Build()

				// Add composite index
				table.CompositeIndexes = []CompositeIndex{
					{
						Name:    "idx_status_id",
						Columns: []string{"status", "id"},
					},
				}

				// Filter with range conditions first, then equality (non-optimal order)
				filter := fmt.Sprintf("id>%d AND id<%d AND status=\"%s\"",
					minID, maxID, statusValue)

				parsedFilter, err := ParseFilter(filter)
				if err != nil {
					return true
				}

				// Generate SQL with optimization disabled
				sqlDisabled, _, err := table.WhereClauseWithOptions(
					parsedFilter,
					"p_",
					WhereClauseOptions{
						EnableCompositeIndexOptimization: false,
					},
				)
				if err != nil {
					return true
				}

				// Extract positions
				idPos1 := strings.Index(sqlDisabled, "id >")
				idPos2 := strings.Index(sqlDisabled, "id <")
				statusPos := strings.Index(sqlDisabled, "status =")

				// All conditions should be present
				if idPos1 == -1 || idPos2 == -1 || statusPos == -1 {
					return false
				}

				// When optimization is disabled, original order should be preserved
				// (id range conditions before status equality)
				// At least one id condition should come before status
				if idPos1 >= statusPos && idPos2 >= statusPos {
					return false
				}

				return true
			}

			config := &quick.Config{MaxCount: 100}
			if err := quick.Check(property, config); err != nil {
				t.Error(err)
			}
		},
	)

	t.Run("flag enabled with equality and range conditions reorders optimally", func(t *testing.T) {
		property := func(statusValue string, minID, maxID int) bool {
			// Skip invalid ranges
			if statusValue == "" || minID >= maxID {
				return true
			}
			if strings.Contains(statusValue, "\"") {
				return true
			}

			// Create table with composite index: (status, id)
			table := NewTable().WithColumns(
				NewColumn().
					WithFieldPath("status").
					WithDatabaseName("status").
					Filterable().
					Build(),
				NewColumn().
					WithFieldPath("id").
					WithDatabaseName("id").
					Filterable().
					Build(),
			).Build()

			// Add composite index
			table.CompositeIndexes = []CompositeIndex{
				{
					Name:    "idx_status_id",
					Columns: []string{"status", "id"},
				},
			}

			// Filter with range conditions first, then equality (non-optimal order)
			filter := fmt.Sprintf("id>%d AND id<%d AND status=\"%s\"",
				minID, maxID, statusValue)

			parsedFilter, err := ParseFilter(filter)
			if err != nil {
				return true
			}

			// Generate SQL with optimization enabled
			sqlEnabled, _, err := table.WhereClauseWithOptions(
				parsedFilter,
				"p_",
				WhereClauseOptions{
					EnableCompositeIndexOptimization: true,
				},
			)
			if err != nil {
				return true
			}

			// Extract positions
			idPos1 := strings.Index(sqlEnabled, "id >")
			idPos2 := strings.Index(sqlEnabled, "id <")
			statusPos := strings.Index(sqlEnabled, "status =")

			// All conditions should be present
			if idPos1 == -1 || idPos2 == -1 || statusPos == -1 {
				return false
			}

			// When optimization is enabled, equality condition (status) should come before
			// range conditions (id)
			if statusPos >= idPos1 || statusPos >= idPos2 {
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

// TestCompositeIndexOptimizationFlagEdgeCases tests edge cases for the optimization flag
func TestCompositeIndexOptimizationFlagEdgeCases(t *testing.T) {
	t.Run("flag disabled with no composite index configured", func(t *testing.T) {
		table := NewTable().WithColumns(
			NewColumn().
				WithFieldPath("a").
				WithDatabaseName("a").
				Filterable().
				Build(),
			NewColumn().
				WithFieldPath("b").
				WithDatabaseName("b").
				Filterable().
				Build(),
		).Build() // No composite indexes

		filter := "b=\"value\" AND a=\"key\""
		parsedFilter, err := ParseFilter(filter)
		assert.NoError(t, err)

		sql, _, err := table.WhereClauseWithOptions(parsedFilter, "p_", WhereClauseOptions{
			EnableCompositeIndexOptimization: false,
		})

		assert.NoError(t, err)
		assert.Contains(t, sql, "a =")
		assert.Contains(t, sql, "b =")

		// Original order should be preserved (b before a)
		bPos := strings.Index(sql, "b =")
		aPos := strings.Index(sql, "a =")
		assert.Less(t, bPos, aPos, "b should come before a when optimization is disabled")
	})

	t.Run("flag enabled with no composite index configured", func(t *testing.T) {
		table := NewTable().WithColumns(
			NewColumn().
				WithFieldPath("a").
				WithDatabaseName("a").
				Filterable().
				Build(),
			NewColumn().
				WithFieldPath("b").
				WithDatabaseName("b").
				Filterable().
				Build(),
		).Build() // No composite indexes

		filter := "b=\"value\" AND a=\"key\""
		parsedFilter, err := ParseFilter(filter)
		assert.NoError(t, err)

		sql, _, err := table.WhereClauseWithOptions(parsedFilter, "p_", WhereClauseOptions{
			EnableCompositeIndexOptimization: true,
		})

		assert.NoError(t, err)
		assert.Contains(t, sql, "a =")
		assert.Contains(t, sql, "b =")
		// Should still generate valid SQL, just without reordering
	})

	t.Run("flag disabled with single condition", func(t *testing.T) {
		table := NewTable().WithColumns(
			NewColumn().
				WithFieldPath("a").
				WithDatabaseName("a").
				Filterable().
				Build(),
		).Build()

		table.CompositeIndexes = []CompositeIndex{
			{
				Name:    "idx_a",
				Columns: []string{"a"},
			},
		}

		filter := "a=\"test\""
		parsedFilter, err := ParseFilter(filter)
		assert.NoError(t, err)

		sql, _, err := table.WhereClauseWithOptions(parsedFilter, "p_", WhereClauseOptions{
			EnableCompositeIndexOptimization: false,
		})

		assert.NoError(t, err)
		assert.Contains(t, sql, "a =")
	})

	t.Run("flag enabled with single condition", func(t *testing.T) {
		table := NewTable().WithColumns(
			NewColumn().
				WithFieldPath("a").
				WithDatabaseName("a").
				Filterable().
				Build(),
		).Build()

		table.CompositeIndexes = []CompositeIndex{
			{
				Name:    "idx_a",
				Columns: []string{"a"},
			},
		}

		filter := "a=\"test\""
		parsedFilter, err := ParseFilter(filter)
		assert.NoError(t, err)

		sql, _, err := table.WhereClauseWithOptions(parsedFilter, "p_", WhereClauseOptions{
			EnableCompositeIndexOptimization: true,
		})

		assert.NoError(t, err)
		assert.Contains(t, sql, "a =")
	})

	t.Run("default options behavior matches disabled flag", func(t *testing.T) {
		table := NewTable().WithColumns(
			NewColumn().
				WithFieldPath("a").
				WithDatabaseName("a").
				Filterable().
				Build(),
			NewColumn().
				WithFieldPath("b").
				WithDatabaseName("b").
				Filterable().
				Build(),
		).Build()

		table.CompositeIndexes = []CompositeIndex{
			{
				Name:    "idx_ab",
				Columns: []string{"a", "b"},
			},
		}

		filter := "b=\"value\" AND a=\"key\""
		parsedFilter, err := ParseFilter(filter)
		assert.NoError(t, err)

		// Default options (flag should be false by default)
		sqlDefault, _, err := table.WhereClauseWithOptions(parsedFilter, "p_", WhereClauseOptions{})
		assert.NoError(t, err)

		// Explicitly disabled
		sqlDisabled, _, err := table.WhereClauseWithOptions(parsedFilter, "p_", WhereClauseOptions{
			EnableCompositeIndexOptimization: false,
		})
		assert.NoError(t, err)

		// Both should preserve original order (b before a)
		bPosDefault := strings.Index(sqlDefault, "b =")
		aPosDefault := strings.Index(sqlDefault, "a =")
		assert.Less(t, bPosDefault, aPosDefault, "default should preserve original order")

		bPosDisabled := strings.Index(sqlDisabled, "b =")
		aPosDisabled := strings.Index(sqlDisabled, "a =")
		assert.Less(t, bPosDisabled, aPosDisabled, "disabled should preserve original order")
	})
}

// ============================================================================
// Unit Tests for Internal Functions
// ============================================================================

func TestCalculateIndexScore(t *testing.T) {
	tests := []struct {
		name       string
		conditions []Condition
		index      CompositeIndex
		wantScore  int
	}{
		{
			name: "perfect match with all equality conditions",
			conditions: []Condition{
				{Column: &Column{databaseName: "status"}, IsEquality: true},
				{Column: &Column{databaseName: "user_id"}, IsEquality: true},
				{Column: &Column{databaseName: "created_at"}, IsEquality: true},
			},
			index: CompositeIndex{
				Name:    "idx_status_user_created",
				Columns: []string{"status", "user_id", "created_at"},
			},
			wantScore: 35 + 25 + 15, // (3-0)*10+5 + (3-1)*10+5 + (3-2)*10+5 = 75
		},
		{
			name: "partial match with equality and range",
			conditions: []Condition{
				{Column: &Column{databaseName: "status"}, IsEquality: true},
				{Column: &Column{databaseName: "user_id"}, IsEquality: true},
				{Column: &Column{databaseName: "created_at"}, IsEquality: false}, // range condition
			},
			index: CompositeIndex{
				Name:    "idx_status_user_created",
				Columns: []string{"status", "user_id", "created_at"},
			},
			wantScore: 35 + 25 + 10, // (3-0)*10+5 + (3-1)*10+5 + (3-2)*10+0 = 70
		},
		{
			name: "prefix match only",
			conditions: []Condition{
				{Column: &Column{databaseName: "status"}, IsEquality: true},
				{Column: &Column{databaseName: "user_id"}, IsEquality: true},
				{Column: &Column{databaseName: "other_col"}, IsEquality: true}, // not in index
			},
			index: CompositeIndex{
				Name:    "idx_status_user_created",
				Columns: []string{"status", "user_id", "created_at"},
			},
			wantScore: 35 + 25, // (3-0)*10+5 + (3-1)*10+5 = 60
		},
		{
			name: "broken prefix - no match after first column",
			conditions: []Condition{
				{Column: &Column{databaseName: "status"}, IsEquality: true},
				{Column: &Column{databaseName: "created_at"}, IsEquality: true}, // skips user_id
			},
			index: CompositeIndex{
				Name:    "idx_status_user_created",
				Columns: []string{"status", "user_id", "created_at"},
			},
			wantScore: 35, // Only first column matches: (3-0)*10+5 = 35
		},
		{
			name: "no matching columns",
			conditions: []Condition{
				{Column: &Column{databaseName: "other_col"}, IsEquality: true},
			},
			index: CompositeIndex{
				Name:    "idx_status_user_created",
				Columns: []string{"status", "user_id", "created_at"},
			},
			wantScore: 0,
		},
		{
			name:       "empty conditions",
			conditions: []Condition{},
			index: CompositeIndex{
				Name:    "idx_status_user_created",
				Columns: []string{"status", "user_id", "created_at"},
			},
			wantScore: 0,
		},
		{
			name: "empty index",
			conditions: []Condition{
				{Column: &Column{databaseName: "status"}, IsEquality: true},
			},
			index: CompositeIndex{
				Name:    "empty_idx",
				Columns: []string{},
			},
			wantScore: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotScore := calculateIndexScore(tt.conditions, tt.index)
			if gotScore != tt.wantScore {
				t.Errorf("calculateIndexScore() = %v, want %v", gotScore, tt.wantScore)
			}
		})
	}
}

func TestFindBestCompositeIndex(t *testing.T) {
	tests := []struct {
		name       string
		conditions []Condition
		indexes    []CompositeIndex
		wantIndex  *string // nil means no index should be selected
	}{
		{
			name: "select best matching index",
			conditions: []Condition{
				{Column: &Column{databaseName: "status"}, IsEquality: true},
				{Column: &Column{databaseName: "user_id"}, IsEquality: true},
				{Column: &Column{databaseName: "created_at"}, IsEquality: false},
			},
			indexes: []CompositeIndex{
				{
					Name:    "idx_status_user_created",
					Columns: []string{"status", "user_id", "created_at"},
				},
				{
					Name:    "idx_user_created",
					Columns: []string{"user_id", "created_at"},
				},
			},
			wantIndex: stringPtr(
				"idx_status_user_created",
			), // Better match (3 columns vs 0 - user_id is not first)
		},
		{
			name: "select index with better prefix match",
			conditions: []Condition{
				{Column: &Column{databaseName: "a"}, IsEquality: true},
				{Column: &Column{databaseName: "b"}, IsEquality: true},
				{Column: &Column{databaseName: "d"}, IsEquality: true},
			},
			indexes: []CompositeIndex{
				{
					Name:    "idx1",
					Columns: []string{"a", "b", "c"},
				},
				{
					Name:    "idx2",
					Columns: []string{"b", "c", "d"},
				},
			},
			wantIndex: stringPtr(
				"idx1",
			), // idx1 matches a,b (score 60) vs idx2 matches only b at position 0 (score 35)
		},
		{
			name: "no matching index",
			conditions: []Condition{
				{Column: &Column{databaseName: "x"}, IsEquality: true},
			},
			indexes: []CompositeIndex{
				{
					Name:    "idx1",
					Columns: []string{"a", "b", "c"},
				},
			},
			wantIndex: nil,
		},
		{
			name: "empty indexes",
			conditions: []Condition{
				{Column: &Column{databaseName: "status"}, IsEquality: true},
			},
			indexes:   []CompositeIndex{},
			wantIndex: nil,
		},
		{
			name:       "empty conditions",
			conditions: []Condition{},
			indexes: []CompositeIndex{
				{
					Name:    "idx1",
					Columns: []string{"a", "b", "c"},
				},
			},
			wantIndex: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotIndex := findBestCompositeIndex(tt.conditions, tt.indexes)
			if tt.wantIndex == nil {
				if gotIndex != nil {
					t.Errorf("findBestCompositeIndex() = %v, want nil", gotIndex.Name)
				}
			} else {
				if gotIndex == nil {
					t.Errorf("findBestCompositeIndex() = nil, want %v", *tt.wantIndex)
				} else if gotIndex.Name != *tt.wantIndex {
					t.Errorf("findBestCompositeIndex() = %v, want %v", gotIndex.Name, *tt.wantIndex)
				}
			}
		})
	}
}

func stringPtr(s string) *string {
	return &s
}

func TestReorderConditions(t *testing.T) {
	tests := []struct {
		name       string
		conditions []Condition
		index      *CompositeIndex
		want       []string // Expected order of column names
	}{
		{
			name: "reorder equality before range, following index order",
			conditions: []Condition{
				{Column: &Column{databaseName: "user_id"}, IsEquality: true},
				{Column: &Column{databaseName: "created_at"}, IsEquality: false}, // range
				{Column: &Column{databaseName: "status"}, IsEquality: true},
			},
			index: &CompositeIndex{
				Name:    "idx_status_user_created",
				Columns: []string{"status", "user_id", "created_at"},
			},
			want: []string{"status", "user_id", "created_at"},
		},
		{
			name: "all equality conditions - reorder by index",
			conditions: []Condition{
				{Column: &Column{databaseName: "c"}, IsEquality: true},
				{Column: &Column{databaseName: "a"}, IsEquality: true},
				{Column: &Column{databaseName: "b"}, IsEquality: true},
			},
			index: &CompositeIndex{
				Name:    "idx_abc",
				Columns: []string{"a", "b", "c"},
			},
			want: []string{"a", "b", "c"},
		},
		{
			name: "all range conditions - reorder by index",
			conditions: []Condition{
				{Column: &Column{databaseName: "c"}, IsEquality: false},
				{Column: &Column{databaseName: "a"}, IsEquality: false},
				{Column: &Column{databaseName: "b"}, IsEquality: false},
			},
			index: &CompositeIndex{
				Name:    "idx_abc",
				Columns: []string{"a", "b", "c"},
			},
			want: []string{"a", "b", "c"},
		},
		{
			name: "conditions not in index go to end",
			conditions: []Condition{
				{Column: &Column{databaseName: "other"}, IsEquality: true},
				{Column: &Column{databaseName: "a"}, IsEquality: true},
				{Column: &Column{databaseName: "b"}, IsEquality: false},
			},
			index: &CompositeIndex{
				Name:    "idx_ab",
				Columns: []string{"a", "b"},
			},
			want: []string{"a", "b", "other"},
		},
		{
			name: "nil index - no reordering",
			conditions: []Condition{
				{Column: &Column{databaseName: "c"}, IsEquality: true},
				{Column: &Column{databaseName: "a"}, IsEquality: true},
				{Column: &Column{databaseName: "b"}, IsEquality: true},
			},
			index: nil,
			want:  []string{"c", "a", "b"}, // Original order preserved
		},
		{
			name: "single condition - no reordering needed",
			conditions: []Condition{
				{Column: &Column{databaseName: "a"}, IsEquality: true},
			},
			index: &CompositeIndex{
				Name:    "idx_abc",
				Columns: []string{"a", "b", "c"},
			},
			want: []string{"a"},
		},
		{
			name:       "empty conditions",
			conditions: []Condition{},
			index: &CompositeIndex{
				Name:    "idx_abc",
				Columns: []string{"a", "b", "c"},
			},
			want: []string{},
		},
		{
			name: "complex case - equality, range, and other",
			conditions: []Condition{
				{Column: &Column{databaseName: "other1"}, IsEquality: true},
				{Column: &Column{databaseName: "c"}, IsEquality: false}, // range in index
				{Column: &Column{databaseName: "b"}, IsEquality: true},
				{Column: &Column{databaseName: "other2"}, IsEquality: false}, // range not in index
				{Column: &Column{databaseName: "a"}, IsEquality: true},
			},
			index: &CompositeIndex{
				Name:    "idx_abc",
				Columns: []string{"a", "b", "c"},
			},
			// Expected: equality (a, b) + range in index (c) + others (other1, other2)
			want: []string{"a", "b", "c", "other1", "other2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := reorderConditions(tt.conditions, tt.index)

			// Extract column names from result
			gotNames := make([]string, len(got))
			for i, cond := range got {
				gotNames[i] = cond.Column.databaseName
			}

			// Compare
			if len(gotNames) != len(tt.want) {
				t.Errorf(
					"reorderConditions() returned %d conditions, want %d",
					len(gotNames),
					len(tt.want),
				)
				return
			}

			for i := range gotNames {
				if gotNames[i] != tt.want[i] {
					t.Errorf("reorderConditions() position %d = %v, want %v\nGot:  %v\nWant: %v",
						i, gotNames[i], tt.want[i], gotNames, tt.want)
					break
				}
			}
		})
	}
}

func TestReorderConditionsPreservesSemantics(t *testing.T) {
	// This test verifies that reordering preserves the logical semantics
	// by checking that all original conditions are present in the result
	conditions := []Condition{
		{Column: &Column{databaseName: "c", fieldPath: NewFieldPath("c")}, IsEquality: false},
		{Column: &Column{databaseName: "a", fieldPath: NewFieldPath("a")}, IsEquality: true},
		{Column: &Column{databaseName: "b", fieldPath: NewFieldPath("b")}, IsEquality: true},
	}

	index := &CompositeIndex{
		Name:    "idx_abc",
		Columns: []string{"a", "b", "c"},
	}

	result := reorderConditions(conditions, index)

	// Verify same number of conditions
	if len(result) != len(conditions) {
		t.Errorf(
			"reorderConditions() changed number of conditions: got %d, want %d",
			len(result),
			len(conditions),
		)
	}

	// Verify all original conditions are present (by column name)
	originalCols := make(map[string]bool)
	for _, cond := range conditions {
		originalCols[cond.Column.databaseName] = true
	}

	resultCols := make(map[string]bool)
	for _, cond := range result {
		resultCols[cond.Column.databaseName] = true
	}

	for col := range originalCols {
		if !resultCols[col] {
			t.Errorf("reorderConditions() lost condition for column %s", col)
		}
	}

	for col := range resultCols {
		if !originalCols[col] {
			t.Errorf("reorderConditions() added unexpected condition for column %s", col)
		}
	}
}
