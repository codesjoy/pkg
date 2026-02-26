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
	"testing"
	"testing/quick"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ===================================================================
// OrderBy Parser Tests
// ===================================================================

func TestParseOrderBy(t *testing.T) {
	t.Run("Values should be a comma separated list of fields", func(t *testing.T) {
		result, err := ParseOrderBy("foo,bar")
		require.NoError(t, err)
		assert.Equal(t, []OrderBy{
			{
				FieldPath: NewFieldPath("foo"),
			},
			{
				FieldPath: NewFieldPath("bar"),
			},
		}, result)

		result, err = ParseOrderBy("foo")
		require.NoError(t, err)
		assert.Equal(t, []OrderBy{
			{
				FieldPath: NewFieldPath("foo"),
			},
		}, result)
	})

	t.Run("The default sort order is ascending", func(t *testing.T) {
		result, err := ParseOrderBy("foo desc, bar")
		require.NoError(t, err)
		assert.Equal(t, []OrderBy{
			{
				FieldPath:  NewFieldPath("foo"),
				Descending: true,
			},
			{
				FieldPath: NewFieldPath("bar"),
			},
		}, result)
	})

	t.Run("Redundant space characters in the syntax are insignificant", func(t *testing.T) {
		expectedResult := []OrderBy{
			{
				FieldPath: NewFieldPath("foo"),
			},
			{
				FieldPath:  NewFieldPath("bar"),
				Descending: true,
			},
		}
		result, err := ParseOrderBy("foo, bar desc")
		require.NoError(t, err)
		assert.Equal(t, expectedResult, result)

		result, err = ParseOrderBy("  foo  ,  bar desc  ")
		require.NoError(t, err)
		assert.Equal(t, expectedResult, result)

		result, err = ParseOrderBy("foo,bar desc")
		require.NoError(t, err)
		assert.Equal(t, expectedResult, result)
	})

	t.Run("Subfields are specified with a . character", func(t *testing.T) {
		result, err := ParseOrderBy("foo.bar, foo.foo.bar desc")
		require.NoError(t, err)
		assert.Equal(t, []OrderBy{
			{
				FieldPath: NewFieldPath("foo", "bar"),
			},
			{
				FieldPath:  NewFieldPath("foo", "foo", "bar"),
				Descending: true,
			},
		}, result)
	})

	t.Run("Quoted strings can be used instead of string literals", func(t *testing.T) {
		result, err := ParseOrderBy("foo.`bar`, foo.foo.`a-backtick-```.bar desc")
		require.NoError(t, err)
		assert.Equal(t, []OrderBy{
			{
				FieldPath: NewFieldPath("foo", "bar"),
			},
			{
				FieldPath:  NewFieldPath("foo", "foo", "a-backtick-`", "bar"),
				Descending: true,
			},
		}, result)
	})

	t.Run("Invalid input is rejected", func(t *testing.T) {
		_, err := ParseOrderBy("`something")
		ErrLike(t, err, []any{"syntax error: 1:1: invalid input text \"`something\""})
	})

	t.Run("Empty order by", func(t *testing.T) {
		t.Run("Spaces only", func(t *testing.T) {
			result, err := ParseOrderBy("   ")
			require.NoError(t, err)
			assert.Equal(t, 0, len(result))
		})

		t.Run("Totally empty", func(t *testing.T) {
			result, err := ParseOrderBy("")
			require.NoError(t, err)
			assert.Equal(t, 0, len(result))
		})
	})
}

// ===================================================================
// OrderBy Generator Tests
// ===================================================================

func TestOrderByClause(t *testing.T) {
	table := NewTable().WithColumns(
		NewColumn().WithFieldPath("foo").WithDatabaseName("db_foo").Sortable().Build(),
		NewColumn().WithFieldPath("bar").WithDatabaseName("db_bar").Sortable().Build(),
		NewColumn().WithFieldPath("baz").WithDatabaseName("db_baz").Sortable().Build(),
		NewColumn().WithFieldPath("unsortable").WithDatabaseName("unsortable").Build(),
	).Build()

	t.Run("Empty order by", func(t *testing.T) {
		result, err := table.OrderByClause([]OrderBy{})
		require.NoError(t, err)
		assert.Equal(t, "", result)
	})

	t.Run("Single order by", func(t *testing.T) {
		result, err := table.OrderByClause([]OrderBy{
			{
				FieldPath: NewFieldPath("foo"),
			},
		})
		require.NoError(t, err)
		assert.Equal(t, "db_foo", result)
	})

	t.Run("Multiple order by", func(t *testing.T) {
		result, err := table.OrderByClause([]OrderBy{
			{
				FieldPath:  NewFieldPath("foo"),
				Descending: true,
			},
			{
				FieldPath: NewFieldPath("bar"),
			},
			{
				FieldPath:  NewFieldPath("baz"),
				Descending: true,
			},
		})
		require.NoError(t, err)
		assert.Equal(t, "db_foo DESC, db_bar, db_baz DESC", result)
	})

	t.Run("Unsortable field in order by", func(t *testing.T) {
		_, err := table.OrderByClause([]OrderBy{
			{
				FieldPath:  NewFieldPath("unsortable"),
				Descending: true,
			},
		})
		ErrLike(
			t,
			err,
			[]any{`no sortable field named "unsortable", valid fields are foo, bar, baz`},
		)
	})

	t.Run("Repeated field in order by", func(t *testing.T) {
		_, err := table.OrderByClause([]OrderBy{
			{
				FieldPath: NewFieldPath("foo"),
			},
			{
				FieldPath: NewFieldPath("foo"),
			},
		})
		ErrLike(t, err, []any{`field appears in order_by multiple times: "foo"`})
	})
}

func TestMergeWithDefaultOrder(t *testing.T) {
	defaultOrder := []OrderBy{
		{
			FieldPath:  NewFieldPath("foo"),
			Descending: true,
		}, {
			FieldPath: NewFieldPath("bar"),
		}, {
			FieldPath:  NewFieldPath("baz"),
			Descending: true,
		},
	}

	t.Run("Empty order", func(t *testing.T) {
		result := MergeWithDefaultOrder(defaultOrder, nil)
		assert.Equal(t, defaultOrder, result)
	})

	t.Run("Non-empty order", func(t *testing.T) {
		order := []OrderBy{
			{
				FieldPath:  NewFieldPath("other"),
				Descending: true,
			},
			{
				FieldPath: NewFieldPath("baz"),
			},
		}
		result := MergeWithDefaultOrder(defaultOrder, order)
		assert.Equal(t, []OrderBy{
			{
				FieldPath:  NewFieldPath("other"),
				Descending: true,
			},
			{
				FieldPath: NewFieldPath("baz"),
			},
			{
				FieldPath:  NewFieldPath("foo"),
				Descending: true,
			},
			{
				FieldPath: NewFieldPath("bar"),
			},
		}, result)
	})
}

// ===================================================================
// Composite Index Tests
// ===================================================================

func TestIndexMatchesOrderBy(t *testing.T) {
	tests := []struct {
		name   string
		fields []OrderByField
		index  CompositeIndex
		want   bool
	}{
		{
			name: "exact match - all columns",
			fields: []OrderByField{
				{Column: &Column{databaseName: "a"}, Direction: "ASC"},
				{Column: &Column{databaseName: "b"}, Direction: "ASC"},
				{Column: &Column{databaseName: "c"}, Direction: "ASC"},
			},
			index: CompositeIndex{
				Name:    "idx_abc",
				Columns: []string{"a", "b", "c"},
			},
			want: true,
		},
		{
			name: "prefix match - two columns",
			fields: []OrderByField{
				{Column: &Column{databaseName: "a"}, Direction: "ASC"},
				{Column: &Column{databaseName: "b"}, Direction: "ASC"},
			},
			index: CompositeIndex{
				Name:    "idx_abc",
				Columns: []string{"a", "b", "c"},
			},
			want: true,
		},
		{
			name: "prefix match - single column",
			fields: []OrderByField{
				{Column: &Column{databaseName: "a"}, Direction: "ASC"},
			},
			index: CompositeIndex{
				Name:    "idx_abc",
				Columns: []string{"a", "b", "c"},
			},
			want: true,
		},
		{
			name: "no match - skips middle column",
			fields: []OrderByField{
				{Column: &Column{databaseName: "a"}, Direction: "ASC"},
				{Column: &Column{databaseName: "c"}, Direction: "ASC"},
			},
			index: CompositeIndex{
				Name:    "idx_abc",
				Columns: []string{"a", "b", "c"},
			},
			want: false,
		},
		{
			name: "no match - wrong order",
			fields: []OrderByField{
				{Column: &Column{databaseName: "b"}, Direction: "ASC"},
				{Column: &Column{databaseName: "a"}, Direction: "ASC"},
			},
			index: CompositeIndex{
				Name:    "idx_abc",
				Columns: []string{"a", "b", "c"},
			},
			want: false,
		},
		{
			name: "no match - too many fields",
			fields: []OrderByField{
				{Column: &Column{databaseName: "a"}, Direction: "ASC"},
				{Column: &Column{databaseName: "b"}, Direction: "ASC"},
				{Column: &Column{databaseName: "c"}, Direction: "ASC"},
				{Column: &Column{databaseName: "d"}, Direction: "ASC"},
			},
			index: CompositeIndex{
				Name:    "idx_abc",
				Columns: []string{"a", "b", "c"},
			},
			want: false,
		},
		{
			name: "no match - different column name",
			fields: []OrderByField{
				{Column: &Column{databaseName: "x"}, Direction: "ASC"},
			},
			index: CompositeIndex{
				Name:    "idx_abc",
				Columns: []string{"a", "b", "c"},
			},
			want: false,
		},
		{
			name:   "empty fields - matches",
			fields: []OrderByField{},
			index: CompositeIndex{
				Name:    "idx_abc",
				Columns: []string{"a", "b", "c"},
			},
			want: true,
		},
		{
			name: "empty index - no match",
			fields: []OrderByField{
				{Column: &Column{databaseName: "a"}, Direction: "ASC"},
			},
			index: CompositeIndex{
				Name:    "empty_idx",
				Columns: []string{},
			},
			want: false,
		},
		{
			name: "direction doesn't affect matching",
			fields: []OrderByField{
				{Column: &Column{databaseName: "a"}, Direction: "DESC"},
				{Column: &Column{databaseName: "b"}, Direction: "ASC"},
			},
			index: CompositeIndex{
				Name:    "idx_abc",
				Columns: []string{"a", "b", "c"},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := indexMatchesOrderBy(tt.fields, tt.index)
			if got != tt.want {
				t.Errorf("indexMatchesOrderBy() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFindMatchingOrderByIndex(t *testing.T) {
	tests := []struct {
		name      string
		fields    []OrderByField
		indexes   []CompositeIndex
		wantIndex *string // nil means no index should be found
	}{
		{
			name: "finds first matching index",
			fields: []OrderByField{
				{Column: &Column{databaseName: "a"}, Direction: "ASC"},
				{Column: &Column{databaseName: "b"}, Direction: "ASC"},
			},
			indexes: []CompositeIndex{
				{
					Name:    "idx_abc",
					Columns: []string{"a", "b", "c"},
				},
				{
					Name:    "idx_ab",
					Columns: []string{"a", "b"},
				},
			},
			wantIndex: stringPtr("idx_abc"), // First matching index
		},
		{
			name: "finds matching index when first doesn't match",
			fields: []OrderByField{
				{Column: &Column{databaseName: "b"}, Direction: "ASC"},
				{Column: &Column{databaseName: "c"}, Direction: "ASC"},
			},
			indexes: []CompositeIndex{
				{
					Name:    "idx_abc",
					Columns: []string{"a", "b", "c"},
				},
				{
					Name:    "idx_bcd",
					Columns: []string{"b", "c", "d"},
				},
			},
			wantIndex: stringPtr("idx_bcd"),
		},
		{
			name: "no matching index",
			fields: []OrderByField{
				{Column: &Column{databaseName: "x"}, Direction: "ASC"},
			},
			indexes: []CompositeIndex{
				{
					Name:    "idx_abc",
					Columns: []string{"a", "b", "c"},
				},
			},
			wantIndex: nil,
		},
		{
			name: "empty indexes",
			fields: []OrderByField{
				{Column: &Column{databaseName: "a"}, Direction: "ASC"},
			},
			indexes:   []CompositeIndex{},
			wantIndex: nil,
		},
		{
			name:   "empty fields - finds first index",
			fields: []OrderByField{},
			indexes: []CompositeIndex{
				{
					Name:    "idx_abc",
					Columns: []string{"a", "b", "c"},
				},
			},
			wantIndex: stringPtr("idx_abc"),
		},
		{
			name: "single column ORDER BY matches multi-column index",
			fields: []OrderByField{
				{Column: &Column{databaseName: "status"}, Direction: "ASC"},
			},
			indexes: []CompositeIndex{
				{
					Name:    "idx_status_created",
					Columns: []string{"status", "created_at"},
				},
			},
			wantIndex: stringPtr("idx_status_created"),
		},
		{
			name: "ORDER BY with DESC direction still matches",
			fields: []OrderByField{
				{Column: &Column{databaseName: "created_at"}, Direction: "DESC"},
				{Column: &Column{databaseName: "id"}, Direction: "DESC"},
			},
			indexes: []CompositeIndex{
				{
					Name:    "idx_created_id",
					Columns: []string{"created_at", "id"},
				},
			},
			wantIndex: stringPtr("idx_created_id"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotIndex := findMatchingOrderByIndex(tt.fields, tt.indexes)
			if tt.wantIndex == nil {
				if gotIndex != nil {
					t.Errorf("findMatchingOrderByIndex() = %v, want nil", gotIndex.Name)
				}
			} else {
				if gotIndex == nil {
					t.Errorf("findMatchingOrderByIndex() = nil, want %v", *tt.wantIndex)
				} else if gotIndex.Name != *tt.wantIndex {
					t.Errorf("findMatchingOrderByIndex() = %v, want %v", gotIndex.Name, *tt.wantIndex)
				}
			}
		})
	}
}

// ===================================================================
// Composite Index Property Tests
// ===================================================================

// **Validates: Requirements 11.4**
// Feature: aip-sql-execution-optimization, Property 21: ORDER BY 使用匹配的复合索引
//
// For any ORDER BY fields and configured Composite_Index, when the ORDER BY fields
// form a prefix of the index columns, the OrderBy_Generator should correctly identify
// the match. The matching should be independent of sort direction (ASC/DESC).
func TestProperty_OrderByUsesMatchingCompositeIndex(t *testing.T) {
	t.Run("ORDER BY prefix matches composite index", func(t *testing.T) {
		property := func(numFields uint8) bool {
			// Limit to reasonable range (1-5 fields)
			if numFields == 0 || numFields > 5 {
				return true
			}

			n := int(numFields)

			// Create a composite index with 5 columns: a, b, c, d, e
			index := CompositeIndex{
				Name:    "idx_abcde",
				Columns: []string{"a", "b", "c", "d", "e"},
			}

			// Create ORDER BY fields that are a prefix of the index (first n columns)
			fields := make([]OrderByField, n)
			for i := 0; i < n; i++ {
				colName := string(rune('a' + i))
				fields[i] = OrderByField{
					Column: &Column{
						databaseName: colName,
					},
					Direction: "ASC",
				}
			}

			// The fields should match the index since they form a prefix
			matches := indexMatchesOrderBy(fields, index)

			return matches == true
		}

		config := &quick.Config{MaxCount: 100}
		if err := quick.Check(property, config); err != nil {
			t.Error(err)
		}
	})

	t.Run("ORDER BY non-prefix does not match composite index", func(t *testing.T) {
		property := func(skipFirst bool) bool {
			// Create a composite index: a, b, c
			index := CompositeIndex{
				Name:    "idx_abc",
				Columns: []string{"a", "b", "c"},
			}

			var fields []OrderByField

			if skipFirst {
				// Skip first column - ORDER BY b, c (not a prefix)
				fields = []OrderByField{
					{
						Column: &Column{
							databaseName: "b",
						},
						Direction: "ASC",
					},
					{
						Column: &Column{
							databaseName: "c",
						},
						Direction: "ASC",
					},
				}
			} else {
				// Skip middle column - ORDER BY a, c (not a prefix)
				fields = []OrderByField{
					{
						Column: &Column{
							databaseName: "a",
						},
						Direction: "ASC",
					},
					{
						Column: &Column{
							databaseName: "c",
						},
						Direction: "ASC",
					},
				}
			}

			// Should not match since it's not a prefix
			matches := indexMatchesOrderBy(fields, index)

			return matches == false
		}

		config := &quick.Config{MaxCount: 100}
		if err := quick.Check(property, config); err != nil {
			t.Error(err)
		}
	})

	t.Run("ORDER BY matching is independent of sort direction", func(t *testing.T) {
		property := func(dir1Asc, dir2Asc, dir3Asc bool) bool {
			// Create a composite index: a, b, c
			index := CompositeIndex{
				Name:    "idx_abc",
				Columns: []string{"a", "b", "c"},
			}

			// Create ORDER BY fields with varying directions
			getDirection := func(isAsc bool) string {
				if isAsc {
					return "ASC"
				}
				return "DESC"
			}

			fields := []OrderByField{
				{
					Column: &Column{
						databaseName: "a",
					},
					Direction: getDirection(dir1Asc),
				},
				{
					Column: &Column{
						databaseName: "b",
					},
					Direction: getDirection(dir2Asc),
				},
				{
					Column: &Column{
						databaseName: "c",
					},
					Direction: getDirection(dir3Asc),
				},
			}

			// Should match regardless of direction
			matches := indexMatchesOrderBy(fields, index)

			return matches == true
		}

		config := &quick.Config{MaxCount: 100}
		if err := quick.Check(property, config); err != nil {
			t.Error(err)
		}
	})

	t.Run("empty ORDER BY matches any composite index", func(t *testing.T) {
		property := func(numCols uint8) bool {
			// Limit to reasonable range
			if numCols == 0 || numCols > 10 {
				return true
			}

			n := int(numCols)

			// Create an index with n columns
			cols := make([]string, n)
			for i := 0; i < n; i++ {
				cols[i] = fmt.Sprintf("col%d", i)
			}

			index := CompositeIndex{
				Name:    "test_idx",
				Columns: cols,
			}

			// Empty ORDER BY should match (it's a valid prefix)
			fields := []OrderByField{}

			matches := indexMatchesOrderBy(fields, index)

			return matches == true
		}

		config := &quick.Config{MaxCount: 100}
		if err := quick.Check(property, config); err != nil {
			t.Error(err)
		}
	})

	t.Run("ORDER BY longer than index does not match", func(t *testing.T) {
		property := func() bool {
			// Create a composite index with 3 columns
			index := CompositeIndex{
				Name:    "idx_abc",
				Columns: []string{"a", "b", "c"},
			}

			// Create ORDER BY with 4 fields (more than index)
			fields := []OrderByField{
				{Column: &Column{databaseName: "a"}, Direction: "ASC"},
				{Column: &Column{databaseName: "b"}, Direction: "ASC"},
				{Column: &Column{databaseName: "c"}, Direction: "ASC"},
				{Column: &Column{databaseName: "d"}, Direction: "ASC"},
			}

			// Should not match since ORDER BY is longer than index
			matches := indexMatchesOrderBy(fields, index)

			return matches == false
		}

		config := &quick.Config{MaxCount: 100}
		if err := quick.Check(property, config); err != nil {
			t.Error(err)
		}
	})

	t.Run("findMatchingOrderByIndex selects first matching index", func(t *testing.T) {
		property := func(numIndexes uint8) bool {
			// Limit to reasonable range (2-5 indexes)
			if numIndexes < 2 || numIndexes > 5 {
				return true
			}

			n := int(numIndexes)

			// Create multiple indexes, all matching the ORDER BY
			indexes := make([]CompositeIndex, n)
			for i := 0; i < n; i++ {
				indexes[i] = CompositeIndex{
					Name:    fmt.Sprintf("idx_%d", i),
					Columns: []string{"a", "b", "c"},
				}
			}

			// Create ORDER BY that matches all indexes
			fields := []OrderByField{
				{Column: &Column{databaseName: "a"}, Direction: "ASC"},
				{Column: &Column{databaseName: "b"}, Direction: "ASC"},
			}

			// Should return the first matching index
			matchedIndex := findMatchingOrderByIndex(fields, indexes)

			if matchedIndex == nil {
				return false
			}

			// Should be the first index
			return matchedIndex.Name == "idx_0"
		}

		config := &quick.Config{MaxCount: 100}
		if err := quick.Check(property, config); err != nil {
			t.Error(err)
		}
	})

	t.Run("findMatchingOrderByIndex returns nil when no match", func(t *testing.T) {
		property := func(numIndexes uint8) bool {
			// Limit to reasonable range (1-5 indexes)
			if numIndexes == 0 || numIndexes > 5 {
				return true
			}

			n := int(numIndexes)

			// Create indexes that don't match the ORDER BY
			indexes := make([]CompositeIndex, n)
			for i := 0; i < n; i++ {
				indexes[i] = CompositeIndex{
					Name:    fmt.Sprintf("idx_%d", i),
					Columns: []string{"x", "y", "z"},
				}
			}

			// Create ORDER BY that doesn't match any index
			fields := []OrderByField{
				{Column: &Column{databaseName: "a"}, Direction: "ASC"},
				{Column: &Column{databaseName: "b"}, Direction: "ASC"},
			}

			// Should return nil
			matchedIndex := findMatchingOrderByIndex(fields, indexes)

			return matchedIndex == nil
		}

		config := &quick.Config{MaxCount: 100}
		if err := quick.Check(property, config); err != nil {
			t.Error(err)
		}
	})

	t.Run("single column ORDER BY matches multi-column index", func(t *testing.T) {
		property := func(indexSize uint8) bool {
			// Limit to reasonable range (2-10 columns)
			if indexSize < 2 || indexSize > 10 {
				return true
			}

			n := int(indexSize)

			// Create an index with n columns starting with 'a'
			cols := make([]string, n)
			cols[0] = "a"
			for i := 1; i < n; i++ {
				cols[i] = fmt.Sprintf("col%d", i)
			}

			index := CompositeIndex{
				Name:    "test_idx",
				Columns: cols,
			}

			// Single column ORDER BY on first column
			fields := []OrderByField{
				{Column: &Column{databaseName: "a"}, Direction: "ASC"},
			}

			// Should match since it's a valid prefix
			matches := indexMatchesOrderBy(fields, index)

			return matches == true
		}

		config := &quick.Config{MaxCount: 100}
		if err := quick.Check(property, config); err != nil {
			t.Error(err)
		}
	})

	t.Run("ORDER BY with wrong column names does not match", func(t *testing.T) {
		property := func(offset uint8) bool {
			// Limit offset to reasonable range
			if offset == 0 || offset > 20 {
				return true
			}

			// Create an index: a, b, c
			index := CompositeIndex{
				Name:    "idx_abc",
				Columns: []string{"a", "b", "c"},
			}

			// Create ORDER BY with different column names
			// Use offset to generate different column names
			fields := []OrderByField{
				{
					Column: &Column{
						databaseName: fmt.Sprintf("col%d", offset),
					},
					Direction: "ASC",
				},
			}

			// Should not match
			matches := indexMatchesOrderBy(fields, index)

			return matches == false
		}

		config := &quick.Config{MaxCount: 100}
		if err := quick.Check(property, config); err != nil {
			t.Error(err)
		}
	})

	t.Run("ORDER BY prefix property holds for all prefix lengths", func(t *testing.T) {
		property := func(prefixLen uint8) bool {
			// Test with an index of 5 columns
			// prefixLen should be 1-5
			if prefixLen == 0 || prefixLen > 5 {
				return true
			}

			n := int(prefixLen)

			index := CompositeIndex{
				Name:    "idx_abcde",
				Columns: []string{"a", "b", "c", "d", "e"},
			}

			// Create ORDER BY with first n columns
			fields := make([]OrderByField, n)
			for i := 0; i < n; i++ {
				colName := string(rune('a' + i))
				fields[i] = OrderByField{
					Column: &Column{
						databaseName: colName,
					},
					Direction: "ASC",
				}
			}

			// All prefixes should match
			matches := indexMatchesOrderBy(fields, index)

			return matches == true
		}

		config := &quick.Config{MaxCount: 100}
		if err := quick.Check(property, config); err != nil {
			t.Error(err)
		}
	})
}

// TestProperty_OrderByIndexMatchingEdgeCases tests edge cases for ORDER BY index matching
func TestProperty_OrderByIndexMatchingEdgeCases(t *testing.T) {
	t.Run("empty index does not match non-empty ORDER BY", func(t *testing.T) {
		index := CompositeIndex{
			Name:    "empty_idx",
			Columns: []string{},
		}

		fields := []OrderByField{
			{Column: &Column{databaseName: "a"}, Direction: "ASC"},
		}

		matches := indexMatchesOrderBy(fields, index)
		assert.False(t, matches, "empty index should not match non-empty ORDER BY")
	})

	t.Run("empty ORDER BY matches empty index", func(t *testing.T) {
		index := CompositeIndex{
			Name:    "empty_idx",
			Columns: []string{},
		}

		fields := []OrderByField{}

		matches := indexMatchesOrderBy(fields, index)
		assert.True(t, matches, "empty ORDER BY should match empty index")
	})

	t.Run("case sensitivity in column names", func(t *testing.T) {
		index := CompositeIndex{
			Name:    "idx_abc",
			Columns: []string{"a", "b", "c"},
		}

		// Test with uppercase column name
		fields := []OrderByField{
			{Column: &Column{databaseName: "A"}, Direction: "ASC"},
		}

		matches := indexMatchesOrderBy(fields, index)
		// Should not match due to case difference
		assert.False(t, matches, "column name matching should be case-sensitive")
	})

	t.Run("mixed ASC and DESC directions still match", func(t *testing.T) {
		index := CompositeIndex{
			Name:    "idx_abc",
			Columns: []string{"a", "b", "c"},
		}

		fields := []OrderByField{
			{Column: &Column{databaseName: "a"}, Direction: "ASC"},
			{Column: &Column{databaseName: "b"}, Direction: "DESC"},
			{Column: &Column{databaseName: "c"}, Direction: "ASC"},
		}

		matches := indexMatchesOrderBy(fields, index)
		assert.True(t, matches, "mixed directions should still match")
	})

	t.Run("all DESC directions match", func(t *testing.T) {
		index := CompositeIndex{
			Name:    "idx_abc",
			Columns: []string{"a", "b", "c"},
		}

		fields := []OrderByField{
			{Column: &Column{databaseName: "a"}, Direction: "DESC"},
			{Column: &Column{databaseName: "b"}, Direction: "DESC"},
			{Column: &Column{databaseName: "c"}, Direction: "DESC"},
		}

		matches := indexMatchesOrderBy(fields, index)
		assert.True(t, matches, "all DESC directions should match")
	})

	t.Run("findMatchingOrderByIndex with empty indexes list", func(t *testing.T) {
		fields := []OrderByField{
			{Column: &Column{databaseName: "a"}, Direction: "ASC"},
		}

		indexes := []CompositeIndex{}

		matchedIndex := findMatchingOrderByIndex(fields, indexes)
		assert.Nil(t, matchedIndex, "should return nil for empty indexes list")
	})

	t.Run("findMatchingOrderByIndex skips non-matching indexes", func(t *testing.T) {
		indexes := []CompositeIndex{
			{
				Name:    "idx_xyz",
				Columns: []string{"x", "y", "z"},
			},
			{
				Name:    "idx_abc",
				Columns: []string{"a", "b", "c"},
			},
			{
				Name:    "idx_def",
				Columns: []string{"d", "e", "f"},
			},
		}

		fields := []OrderByField{
			{Column: &Column{databaseName: "a"}, Direction: "ASC"},
			{Column: &Column{databaseName: "b"}, Direction: "ASC"},
		}

		matchedIndex := findMatchingOrderByIndex(fields, indexes)
		assert.NotNil(t, matchedIndex)
		assert.Equal(t, "idx_abc", matchedIndex.Name, "should find the matching index")
	})

	t.Run("exact match preferred over partial match", func(t *testing.T) {
		// This tests that when ORDER BY exactly matches an index,
		// that index is found (even if other indexes also match as prefixes)
		indexes := []CompositeIndex{
			{
				Name:    "idx_ab",
				Columns: []string{"a", "b"},
			},
			{
				Name:    "idx_abc",
				Columns: []string{"a", "b", "c"},
			},
		}

		fields := []OrderByField{
			{Column: &Column{databaseName: "a"}, Direction: "ASC"},
			{Column: &Column{databaseName: "b"}, Direction: "ASC"},
		}

		matchedIndex := findMatchingOrderByIndex(fields, indexes)
		assert.NotNil(t, matchedIndex)
		// Should return the first matching index (idx_ab)
		assert.Equal(t, "idx_ab", matchedIndex.Name)
	})
}
