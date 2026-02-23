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
)

// ============================================================================
// Test Table Setup Functions
// ============================================================================

// setupMatchModeTable creates a table with columns configured for different match modes
func setupMatchModeTable() *Table {
	return NewTable().
		WithColumns(
			NewColumn().WithFieldPath("title").WithDatabaseName("title").Filterable().
				WithMatchModes(MatchModePrefix, MatchModeExact).Build(),
			NewColumn().WithFieldPath("description").WithDatabaseName("description").Filterable().
				WithMatchModes(MatchModeFullText, MatchModeContains).Build(),
			NewColumn().WithFieldPath("name").WithDatabaseName("name").Filterable().
				WithMatchModes(MatchModeExact).Build(),
			NewColumn().WithFieldPath("content").WithDatabaseName("content").Filterable().
				WithMatchModes(MatchModeContains).Build(),
		).
		Build()
}

// setupImplicitFilterTable creates a table with implicit filter columns
func setupImplicitFilterTable() *Table {
	return NewTable().
		WithColumns(
			NewColumn().WithFieldPath("title").WithDatabaseName("title").FilterableImplicitly().
				WithMatchModes(MatchModePrefix).Build(),
			NewColumn().WithFieldPath("summary").WithDatabaseName("summary").FilterableImplicitly().
				WithMatchModes(MatchModePrefix).Build(),
			NewColumn().WithFieldPath("content").WithDatabaseName("content").FilterableImplicitly().
				WithMatchModes(MatchModeFullText).Build(),
		).
		Build()
}

// setupCompositeIndexTable creates a table with composite indexes
func setupCompositeIndexTable() *Table {
	table := NewTable().
		WithColumns(
			NewColumn().WithFieldPath("status").
				WithDatabaseName("status").
				Filterable().
				Sortable().
				Build(),
			NewColumn().WithFieldPath("user_id").
				WithDatabaseName("user_id").
				Filterable().
				Sortable().
				Build(),
			NewColumn().WithFieldPath("created_at").
				WithDatabaseName("created_at").
				Filterable().
				Sortable().
				Build(),
			NewColumn().WithFieldPath("priority").
				WithDatabaseName("priority").
				Filterable().
				Sortable().
				Build(),
			NewColumn().WithFieldPath("id").WithDatabaseName("id").Filterable().Sortable().Build(),
		).
		Build()

	table.CompositeIndexes = []CompositeIndex{
		{
			Name:    "idx_status_user_created",
			Columns: []string{"status", "user_id", "created_at"},
		},
		{
			Name:    "idx_priority_status",
			Columns: []string{"priority", "status"},
		},
	}

	return table
}

// buildTestTable creates a simple test table for benchmarking
func buildTestTable() *Table {
	return NewTable().WithColumns(
		NewColumn().WithFieldPath("id").WithDatabaseName("id").Sortable().Build(),
		NewColumn().WithFieldPath("status").WithDatabaseName("status").Filterable().Build(),
		NewColumn().WithFieldPath("user_id").WithDatabaseName("user_id").Filterable().Build(),
		NewColumn().WithFieldPath("created_at").
			WithDatabaseName("created_at").
			Filterable().
			Sortable().
			Build(),
	).
		Build()
}

// buildLargeTestTable creates a table with many columns for benchmarking
func buildLargeTestTable(columnCount int) *Table {
	columns := make([]*Column, columnCount)
	for i := 0; i < columnCount; i++ {
		fieldPath := fmt.Sprintf("field_%d", i)
		databaseName := fmt.Sprintf("col_%d", i)
		columns[i] = NewColumn().WithFieldPath(fieldPath).
			WithDatabaseName(databaseName).
			Filterable().
			Build()
	}
	return NewTable().WithColumns(columns...).Build()
}

// buildTableWithCompositeIndexes creates a table with composite indexes
func buildTableWithCompositeIndexes() *Table {
	table := buildTestTable()
	table.CompositeIndexes = []CompositeIndex{
		{
			Name:    "idx_status_user_created",
			Columns: []string{"status", "user_id", "created_at"},
		},
	}
	return table
}

// ============================================================================
// Filter Performance Benchmarks
// ============================================================================

// BenchmarkFilterGeneration_Simple benchmarks simple filter generation
func BenchmarkFilterGeneration_Simple(b *testing.B) {
	table := buildTestTable()
	filter, err := ParseFilter("status=\"active\"")
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, err := table.WhereClause(filter, "p")
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkFilterGeneration_Complex benchmarks complex filter generation
func BenchmarkFilterGeneration_Complex(b *testing.B) {
	table := buildTestTable()
	filter, err := ParseFilter("status=\"active\" AND user_id=123 AND created_at=\"2024-01-01\"")
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, err := table.WhereClause(filter, "p")
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkCompositeIndexOptimization benchmarks composite index optimization
func BenchmarkCompositeIndexOptimization(b *testing.B) {
	table := buildTableWithCompositeIndexes()
	filter, err := ParseFilter("user_id=123 AND created_at=\"2024-01-01\" AND status=\"active\"")
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, err := table.WhereClauseWithOptions(filter, "p", WhereClauseOptions{
			EnableCompositeIndexOptimization: true,
		})
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkFilterGeneration_LargeTable benchmarks filter generation on large table
func BenchmarkFilterGeneration_LargeTable(b *testing.B) {
	table := buildLargeTestTable(100)
	filter, err := ParseFilter("field_0=\"value\"")
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, err := table.WhereClause(filter, "p")
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkFilterGeneration_MultipleConditions benchmarks multiple conditions
func BenchmarkFilterGeneration_MultipleConditions(b *testing.B) {
	table := buildTestTable()
	filter, err := ParseFilter(
		"status=\"active\" AND user_id=\"123\" AND created_at=\"2024-01-01\"",
	)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, err := table.WhereClause(filter, "p")
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkFilterParsing benchmarks filter parsing
func BenchmarkFilterParsing(b *testing.B) {
	filterStr := "status=\"active\" AND user_id=123 AND created_at>\"2024-01-01\""

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := ParseFilter(filterStr)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkOrderByGeneration benchmarks order by clause generation
func BenchmarkOrderByGeneration(b *testing.B) {
	table := buildTestTable()
	orderBy, err := ParseOrderBy("created_at desc, id")
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := table.OrderByClause(orderBy)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// ============================================================================
// Optimization Benchmarks - Match Modes
// ============================================================================

// BenchmarkMatchMode_Exact benchmarks exact match mode
func BenchmarkMatchMode_Exact(b *testing.B) {
	table := setupMatchModeTable()
	filter, err := ParseFilter("name:\"test\"")
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
			Dialect: SQLDialectGeneric,
		})
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkMatchMode_Prefix benchmarks prefix match mode
func BenchmarkMatchMode_Prefix(b *testing.B) {
	table := setupMatchModeTable()
	filter, err := ParseFilter("title:\"search term\"")
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
			Dialect: SQLDialectGeneric,
		})
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkMatchMode_Contains benchmarks contains match mode
func BenchmarkMatchMode_Contains(b *testing.B) {
	table := setupMatchModeTable()
	filter, err := ParseFilter("content:\"search term\"")
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
			Dialect: SQLDialectGeneric,
		})
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkMatchMode_Fulltext_Postgres benchmarks fulltext search with PostgreSQL
func BenchmarkMatchMode_Fulltext_Postgres(b *testing.B) {
	table := setupMatchModeTable()
	filter, err := ParseFilter("description:\"search term\"")
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
			Dialect: SQLDialectPostgres,
		})
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkMatchMode_Fulltext_MySQL benchmarks fulltext search with MySQL
func BenchmarkMatchMode_Fulltext_MySQL(b *testing.B) {
	table := setupMatchModeTable()
	filter, err := ParseFilter("description:\"search term\"")
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
			Dialect: SQLDialectMySQL,
		})
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkMatchMode_Fallback benchmarks match mode fallback behavior
func BenchmarkMatchMode_Fallback(b *testing.B) {
	table := setupMatchModeTable()
	filter, err := ParseFilter("description:\"search term\"")
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
			Dialect:    SQLDialectGeneric, // Generic doesn't support fulltext, will fallback
			StrictMode: false,
		})
		if err != nil {
			b.Fatal(err)
		}
	}
}

// ============================================================================
// Optimization Benchmarks - Implicit Filter
// ============================================================================

// BenchmarkImplicitFilter_SingleColumn benchmarks implicit filter with single column
func BenchmarkImplicitFilter_SingleColumn(b *testing.B) {
	table := NewTable().
		WithColumns(
			NewColumn().WithFieldPath("title").WithDatabaseName("title").FilterableImplicitly().
				WithMatchModes(MatchModePrefix).Build(),
		).
		Build()

	filter, err := ParseFilter("\"search term\"")
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
			Dialect: SQLDialectGeneric,
		})
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkImplicitFilter_TwoColumns benchmarks implicit filter with two columns
func BenchmarkImplicitFilter_TwoColumns(b *testing.B) {
	table := NewTable().
		WithColumns(
			NewColumn().WithFieldPath("title").WithDatabaseName("title").FilterableImplicitly().
				WithMatchModes(MatchModePrefix).Build(),
			NewColumn().WithFieldPath("summary").WithDatabaseName("summary").FilterableImplicitly().
				WithMatchModes(MatchModePrefix).Build(),
		).
		Build()

	filter, err := ParseFilter("\"search term\"")
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
			Dialect: SQLDialectGeneric,
		})
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkImplicitFilter_ThreeColumns benchmarks implicit filter with three columns
func BenchmarkImplicitFilter_ThreeColumns(b *testing.B) {
	table := setupImplicitFilterTable()
	filter, err := ParseFilter("\"search term\"")
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
			Dialect: SQLDialectPostgres,
		})
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkImplicitFilter_FiveColumns benchmarks implicit filter with five columns
func BenchmarkImplicitFilter_FiveColumns(b *testing.B) {
	table := NewTable().
		WithColumns(
			NewColumn().WithFieldPath("col1").WithDatabaseName("col1").FilterableImplicitly().
				WithMatchModes(MatchModePrefix).Build(),
			NewColumn().WithFieldPath("col2").WithDatabaseName("col2").FilterableImplicitly().
				WithMatchModes(MatchModePrefix).Build(),
			NewColumn().WithFieldPath("col3").WithDatabaseName("col3").FilterableImplicitly().
				WithMatchModes(MatchModeExact).Build(),
			NewColumn().WithFieldPath("col4").WithDatabaseName("col4").FilterableImplicitly().
				WithMatchModes(MatchModeContains).Build(),
			NewColumn().WithFieldPath("col5").WithDatabaseName("col5").FilterableImplicitly().
				WithMatchModes(MatchModePrefix).Build(),
		).
		Build()

	filter, err := ParseFilter("\"search term\"")
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
			Dialect: SQLDialectGeneric,
		})
		if err != nil {
			b.Fatal(err)
		}
	}
}

// ============================================================================
// Optimization Benchmarks - Seek Pagination
// ============================================================================

// BenchmarkSeekPagination_SingleField benchmarks seek pagination with single field
func BenchmarkSeekPagination_SingleField(b *testing.B) {
	table := NewTable().
		WithColumns(
			NewColumn().WithFieldPath("created_at").
				WithDatabaseName("created_at").
				Sortable().
				Build(),
			NewColumn().WithFieldPath("id").WithDatabaseName("id").Sortable().Build(),
		).
		Build()

	order := []OrderBy{
		{FieldPath: NewFieldPath("created_at"), Descending: false},
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _, err := table.BuildSeekPaginationClause(
			order,
			[]string{"2024-01-15T10:30:00Z"},
			NewFieldPath("id"),
			"12345",
			"seek_",
			SQLDialectGeneric,
		)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkSeekPagination_TwoFields benchmarks seek pagination with two fields
func BenchmarkSeekPagination_TwoFields(b *testing.B) {
	table := NewTable().
		WithColumns(
			NewColumn().WithFieldPath("created_at").
				WithDatabaseName("created_at").
				Sortable().
				Build(),
			NewColumn().WithFieldPath("id").WithDatabaseName("id").Sortable().Build(),
		).
		Build()

	order := []OrderBy{
		{FieldPath: NewFieldPath("created_at"), Descending: true},
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _, err := table.BuildSeekPaginationClause(
			order,
			[]string{"2024-01-15T10:30:00Z"},
			NewFieldPath("id"),
			"12345",
			"seek_",
			SQLDialectGeneric,
		)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkSeekPagination_ThreeFields benchmarks seek pagination with three fields
func BenchmarkSeekPagination_ThreeFields(b *testing.B) {
	table := NewTable().
		WithColumns(
			NewColumn().WithFieldPath("status").WithDatabaseName("status").Sortable().Build(),
			NewColumn().WithFieldPath("created_at").
				WithDatabaseName("created_at").
				Sortable().
				Build(),
			NewColumn().WithFieldPath("id").WithDatabaseName("id").Sortable().Build(),
		).
		Build()

	order := []OrderBy{
		{FieldPath: NewFieldPath("status"), Descending: false},
		{FieldPath: NewFieldPath("created_at"), Descending: true},
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _, err := table.BuildSeekPaginationClause(
			order,
			[]string{"active", "2024-01-15T10:30:00Z"},
			NewFieldPath("id"),
			"12345",
			"seek_",
			SQLDialectGeneric,
		)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkSeekPagination_FiveFields benchmarks seek pagination with five fields
func BenchmarkSeekPagination_FiveFields(b *testing.B) {
	table := NewTable().
		WithColumns(
			NewColumn().WithFieldPath("f1").WithDatabaseName("f1").Sortable().Build(),
			NewColumn().WithFieldPath("f2").WithDatabaseName("f2").Sortable().Build(),
			NewColumn().WithFieldPath("f3").WithDatabaseName("f3").Sortable().Build(),
			NewColumn().WithFieldPath("f4").WithDatabaseName("f4").Sortable().Build(),
			NewColumn().WithFieldPath("id").WithDatabaseName("id").Sortable().Build(),
		).
		Build()

	order := []OrderBy{
		{FieldPath: NewFieldPath("f1"), Descending: false},
		{FieldPath: NewFieldPath("f2"), Descending: true},
		{FieldPath: NewFieldPath("f3"), Descending: false},
		{FieldPath: NewFieldPath("f4"), Descending: true},
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _, err := table.BuildSeekPaginationClause(
			order,
			[]string{"val1", "val2", "val3", "val4"},
			NewFieldPath("id"),
			"12345",
			"seek_",
			SQLDialectGeneric,
		)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkSeekPagination_MixedDirections benchmarks seek pagination with mixed sort directions
func BenchmarkSeekPagination_MixedDirections(b *testing.B) {
	table := NewTable().
		WithColumns(
			NewColumn().WithFieldPath("priority").WithDatabaseName("priority").Sortable().Build(),
			NewColumn().WithFieldPath("created_at").
				WithDatabaseName("created_at").
				Sortable().
				Build(),
			NewColumn().WithFieldPath("updated_at").
				WithDatabaseName("updated_at").
				Sortable().
				Build(),
			NewColumn().WithFieldPath("id").WithDatabaseName("id").Sortable().Build(),
		).
		Build()

	order := []OrderBy{
		{FieldPath: NewFieldPath("priority"), Descending: true},
		{FieldPath: NewFieldPath("created_at"), Descending: false},
		{FieldPath: NewFieldPath("updated_at"), Descending: true},
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _, err := table.BuildSeekPaginationClause(
			order,
			[]string{"high", "2024-01-01T00:00:00Z", "2024-01-15T10:30:00Z"},
			NewFieldPath("id"),
			"12345",
			"seek_",
			SQLDialectGeneric,
		)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// ============================================================================
// Optimization Benchmarks - Composite Index
// ============================================================================

// BenchmarkCompositeIndex_Disabled_Simple benchmarks composite index optimization disabled (simple)
func BenchmarkCompositeIndex_Disabled_Simple(b *testing.B) {
	table := setupCompositeIndexTable()
	filter, err := ParseFilter("status=\"active\" AND user_id=123")
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
			Dialect:                          SQLDialectGeneric,
			EnableCompositeIndexOptimization: false,
		})
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkCompositeIndex_Enabled_Simple benchmarks composite index optimization enabled (simple)
func BenchmarkCompositeIndex_Enabled_Simple(b *testing.B) {
	table := setupCompositeIndexTable()
	filter, err := ParseFilter("status=\"active\" AND user_id=123")
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
			Dialect:                          SQLDialectGeneric,
			EnableCompositeIndexOptimization: true,
		})
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkCompositeIndex_Disabled_Complex benchmarks composite index optimization disabled (complex)
func BenchmarkCompositeIndex_Disabled_Complex(b *testing.B) {
	table := setupCompositeIndexTable()
	filter, err := ParseFilter(
		"user_id=123 AND created_at=\"2024-01-01\" AND status=\"active\" AND priority=\"high\"",
	)
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
			Dialect:                          SQLDialectGeneric,
			EnableCompositeIndexOptimization: false,
		})
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkCompositeIndex_Enabled_Complex benchmarks composite index optimization enabled (complex)
func BenchmarkCompositeIndex_Enabled_Complex(b *testing.B) {
	table := setupCompositeIndexTable()
	filter, err := ParseFilter(
		"user_id=123 AND created_at=\"2024-01-01\" AND status=\"active\" AND priority=\"high\"",
	)
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
			Dialect:                          SQLDialectGeneric,
			EnableCompositeIndexOptimization: true,
		})
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkCompositeIndex_Disabled_VeryComplex benchmarks composite index optimization disabled (very complex)
func BenchmarkCompositeIndex_Disabled_VeryComplex(b *testing.B) {
	table := setupCompositeIndexTable()
	filter, err := ParseFilter(
		"priority=\"high\" AND user_id=123 AND created_at=\"2024-01-01\" AND status=\"active\" AND id=1000",
	)
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
			Dialect:                          SQLDialectGeneric,
			EnableCompositeIndexOptimization: false,
		})
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkCompositeIndex_Enabled_VeryComplex benchmarks composite index optimization enabled (very complex)
func BenchmarkCompositeIndex_Enabled_VeryComplex(b *testing.B) {
	table := setupCompositeIndexTable()
	filter, err := ParseFilter(
		"priority=\"high\" AND user_id=123 AND created_at=\"2024-01-01\" AND status=\"active\" AND id=1000",
	)
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
			Dialect:                          SQLDialectGeneric,
			EnableCompositeIndexOptimization: true,
		})
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkCompositeIndex_MultipleIndexes benchmarks multiple composite indexes
func BenchmarkCompositeIndex_MultipleIndexes(b *testing.B) {
	table := NewTable().
		WithColumns(
			NewColumn().WithFieldPath("a").WithDatabaseName("a").Filterable().Build(),
			NewColumn().WithFieldPath("b").WithDatabaseName("b").Filterable().Build(),
			NewColumn().WithFieldPath("c").WithDatabaseName("c").Filterable().Build(),
			NewColumn().WithFieldPath("d").WithDatabaseName("d").Filterable().Build(),
			NewColumn().WithFieldPath("e").WithDatabaseName("e").Filterable().Build(),
		).
		Build()

	table.CompositeIndexes = []CompositeIndex{
		{Name: "idx1", Columns: []string{"a", "b", "c"}},
		{Name: "idx2", Columns: []string{"b", "c", "d"}},
		{Name: "idx3", Columns: []string{"c", "d", "e"}},
		{Name: "idx4", Columns: []string{"a", "c", "e"}},
		{Name: "idx5", Columns: []string{"b", "d"}},
	}

	filter, err := ParseFilter("c=\"val1\" AND d=\"val2\" AND e=\"val3\"")
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
			Dialect:                          SQLDialectGeneric,
			EnableCompositeIndexOptimization: true,
		})
		if err != nil {
			b.Fatal(err)
		}
	}
}

// ============================================================================
// Optimization Benchmarks - Overhead Comparison
// ============================================================================

// BenchmarkOptimization_Overhead_NoOptimizations benchmarks without any optimizations
func BenchmarkOptimization_Overhead_NoOptimizations(b *testing.B) {
	table := setupTestTable()
	filter, err := ParseFilter("foo=bar AND baz=qux AND test=value")
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _, err := table.WhereClause(filter, "p_")
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkOptimization_Overhead_WithMatchModes benchmarks with match mode optimizations
func BenchmarkOptimization_Overhead_WithMatchModes(b *testing.B) {
	table := setupMatchModeTable()
	filter, err := ParseFilter("title:\"test\" AND name:\"value\" AND content:\"search\"")
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
			Dialect: SQLDialectGeneric,
		})
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkOptimization_Overhead_AllOptimizations benchmarks with all optimizations enabled
func BenchmarkOptimization_Overhead_AllOptimizations(b *testing.B) {
	table := NewTable().
		WithColumns(
			NewColumn().WithFieldPath("status").WithDatabaseName("status").Filterable().
				WithMatchModes(MatchModeExact).Build(),
			NewColumn().WithFieldPath("user_id").WithDatabaseName("user_id").Filterable().
				WithMatchModes(MatchModeExact).Build(),
			NewColumn().WithFieldPath("created_at").
				WithDatabaseName("created_at").
				Filterable().
				Build(),
		).
		Build()

	table.CompositeIndexes = []CompositeIndex{
		{Name: "idx", Columns: []string{"status", "user_id", "created_at"}},
	}

	filter, err := ParseFilter("user_id=123 AND created_at=\"2024-01-01\" AND status:\"active\"")
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
			Dialect:                          SQLDialectGeneric,
			EnableCompositeIndexOptimization: true,
		})
		if err != nil {
			b.Fatal(err)
		}
	}
}
