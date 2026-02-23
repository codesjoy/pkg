// Copyright 2024 The codesjoy Authors.
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
)

// ExampleMatchMode_exact demonstrates using exact match mode for precise lookups.
// This mode generates "column = @param" SQL and can efficiently use B-tree indexes.
func ExampleMatchMode_exact() {
	// Create a column with exact match mode for status codes
	statusColumn := NewColumn().
		WithFieldPath("status").
		WithDatabaseName("status").
		WithMatchModes(MatchModeExact).
		Filterable().
		Build()

	table := NewTable().
		WithColumns(statusColumn).
		Build()

	filter, _ := ParseFilter("status:\"active\"")
	sql, params, _ := table.WhereClauseWithOptions(filter, "p", WhereClauseOptions{
		Dialect: SQLDialectGeneric,
	})

	fmt.Println("SQL:", sql)
	fmt.Printf("Params: %v\n", params)
	// Output:
	// SQL: (status = @p0)
	// Params: [{p0 active}]
}

// ExampleMatchMode_prefix demonstrates using prefix match mode for autocomplete.
// This mode generates "column LIKE 'value%'" SQL and can use B-tree indexes.
func ExampleMatchMode_prefix() {
	// Create a column with prefix match mode for name autocomplete
	nameColumn := NewColumn().
		WithFieldPath("name").
		WithDatabaseName("name").
		WithMatchModes(MatchModePrefix).
		Filterable().
		Build()

	table := NewTable().
		WithColumns(nameColumn).
		Build()

	filter, _ := ParseFilter("name:\"John\"")
	sql, params, _ := table.WhereClauseWithOptions(filter, "p", WhereClauseOptions{
		Dialect: SQLDialectGeneric,
	})

	fmt.Println("SQL:", sql)
	fmt.Printf("Params: %v\n", params)
	// Output:
	// SQL: (name LIKE @p0)
	// Params: [{p0 John%}]
}

// ExampleMatchMode_fulltext demonstrates using full-text search with PostgreSQL.
// This mode requires a full-text index and provides natural language search.
func ExampleMatchMode_fulltext() {
	// Create a column with full-text search for document content
	contentColumn := NewColumn().
		WithFieldPath("content").
		WithDatabaseName("content").
		WithMatchModes(MatchModeFullText, MatchModeContains).
		Filterable().
		Build()

	table := NewTable().
		WithColumns(contentColumn).
		Build()

	filter, _ := ParseFilter("content:\"machine learning\"")
	opts := WhereClauseOptions{
		Dialect: SQLDialectPostgres,
	}
	sql, params, _ := table.WhereClauseWithOptions(filter, "p", opts)

	fmt.Println("SQL:", sql)
	fmt.Printf("Params: %v\n", params)
	// Output:
	// SQL: (to_tsvector('simple', content) @@ websearch_to_tsquery('simple', @p0))
	// Params: [{p0 machine learning}]
}

// ExampleCompositeIndex demonstrates composite index optimization.
// The WHERE conditions are reordered to match the index column order.
func ExampleCompositeIndex() {
	// Create a table with multiple columns
	table := NewTable().
		WithColumns(
			NewColumn().WithFieldPath("status").WithDatabaseName("status").Filterable().Build(),
			NewColumn().WithFieldPath("user_id").WithDatabaseName("user_id").Filterable().Build(),
			NewColumn().WithFieldPath("created_at").
				WithDatabaseName("created_at").
				Filterable().
				Build(),
		).
		Build()

	// Configure composite index
	table.CompositeIndexes = []CompositeIndex{
		{
			Name:    "idx_status_user_created",
			Columns: []string{"status", "user_id", "created_at"},
		},
	}

	// Original filter has conditions in non-optimal order
	filter, _ := ParseFilter("user_id=123 AND created_at>\"2024-01-01\" AND status=\"active\"")

	// Enable composite index optimization
	opts := WhereClauseOptions{
		EnableCompositeIndexOptimization: true,
	}
	sql, params, _ := table.WhereClauseWithOptions(filter, "p", opts)

	fmt.Println("SQL:", sql)
	fmt.Printf("Params: %v\n", params)
	// Output:
	// SQL: ((status = @p0) AND (user_id = @p1) AND (created_at > @p2))
	// Params: [{p0 active} {p1 123} {p2 2024-01-01}]
}

// ExampleColumnBuilder_KeyValue demonstrates querying key-value columns like labels.
// This generates EXISTS subqueries with UNNEST.
func ExampleColumnBuilder_KeyValue() {
	// Create a key-value column for labels
	labelsColumn := NewColumn().
		WithFieldPath("labels").
		WithDatabaseName("labels").
		KeyValue().
		WithMatchModes(MatchModeExact).
		Filterable().
		Build()

	table := NewTable().
		WithColumns(labelsColumn).
		Build()

	filter, _ := ParseFilter("labels.environment=\"production\"")
	sql, params, _ := table.WhereClause(filter, "p")

	fmt.Println("SQL:", sql)
	fmt.Printf("Params: %v\n", params)
	// Output:
	// SQL: (EXISTS (SELECT key, value FROM UNNEST(labels) WHERE key = @p0 AND value = @p1))
	// Params: [{p0 environment} {p1 production}]
}

// ExampleWhereClauseOptions_strictMode demonstrates strict mode behavior.
// When enabled, it returns an error if no supported match mode is available.
func ExampleWhereClauseOptions_strictMode() {
	// Create a column with only full-text mode
	contentColumn := NewColumn().
		WithFieldPath("content").
		WithDatabaseName("content").
		WithMatchModes(MatchModeFullText). // Only fulltext, no fallback
		Filterable().
		Build()

	table := NewTable().
		WithColumns(contentColumn).
		Build()

	filter, _ := ParseFilter("content:\"test\"")

	// With strict mode and generic dialect, this will fail
	opts := WhereClauseOptions{
		Dialect:    SQLDialectGeneric, // Doesn't support fulltext
		StrictMode: true,
	}
	_, _, err := table.WhereClauseWithOptions(filter, "p", opts)
	if err != nil {
		fmt.Println("Error:", err)
	}
	// Output:
	// Error: argument for field content: no supported match mode for field "content" with dialect "generic"
}

// ExampleColumnBuilder_FilterableImplicitly demonstrates implicit filtering across multiple columns.
// When no field is specified, the search applies to all implicitly filterable columns.
func ExampleColumnBuilder_FilterableImplicitly() {
	// Create columns with implicit filtering
	table := NewTable().
		WithColumns(
			NewColumn().WithFieldPath("title").WithDatabaseName("title").
				WithMatchModes(MatchModePrefix).FilterableImplicitly().Build(),
			NewColumn().WithFieldPath("description").WithDatabaseName("description").
				WithMatchModes(MatchModePrefix).FilterableImplicitly().Build(),
		).
		Build()

	// Search without specifying a field
	filter, _ := ParseFilter("\"search term\"")
	sql, params, _ := table.WhereClauseWithOptions(filter, "p", WhereClauseOptions{
		Dialect: SQLDialectGeneric,
	})

	fmt.Println("SQL:", sql)
	fmt.Printf("Params: %v\n", params)
	// Output:
	// SQL: (title LIKE @p0 OR description LIKE @p1)
	// Params: [{p0 search term%} {p1 search term%}]
}

// ExamplePaginationToken demonstrates Seek pagination for efficient paging.
// This is much faster than OFFSET-based pagination on large datasets.
func ExamplePaginationToken() {
	// This example shows the concept of Seek pagination
	// In practice, you would use the orderby package to generate the seek predicate

	// Assume we have ORDER BY created_at DESC, id DESC
	// Last row from previous page: created_at='2024-01-15T10:30:00Z', id=12345

	token := PaginationToken{
		Values: []interface{}{"2024-01-15T10:30:00Z", 12345},
	}

	fmt.Printf("Token values: %v\n", token.Values)
	fmt.Println("Next page query would use:")
	fmt.Println(
		"WHERE (created_at < @seek_cmp_0 OR (created_at = @seek_eq_0 AND id < @seek_cmp_1))",
	)
	fmt.Println("ORDER BY created_at DESC, id DESC LIMIT 10")
	// Output:
	// Token values: [2024-01-15T10:30:00Z 12345]
	// Next page query would use:
	// WHERE (created_at < @seek_cmp_0 OR (created_at = @seek_eq_0 AND id < @seek_cmp_1))
	// ORDER BY created_at DESC, id DESC LIMIT 10
}

// ExampleColumnBuilder_WithIndexHint demonstrates documenting index strategies.
// The hint is for documentation only and doesn't affect SQL generation.
func ExampleColumnBuilder_WithIndexHint() {
	// Create a column with index hint documentation
	titleColumn := NewColumn().
		WithFieldPath("title").
		WithDatabaseName("title").
		WithMatchModes(MatchModePrefix).
		WithIndexHint("Use idx_title_prefix for prefix searches. Cardinality: ~100K unique values.").
		Filterable().
		Build()

	table := NewTable().
		WithColumns(titleColumn).
		Build()

	filter, _ := ParseFilter("title:\"Go\"")
	sql, params, _ := table.WhereClauseWithOptions(filter, "p", WhereClauseOptions{
		Dialect: SQLDialectGeneric,
	})

	// The index hint doesn't affect the generated SQL
	fmt.Println("SQL:", sql)
	fmt.Printf("Params: %v\n", params)
	// Output:
	// SQL: (title LIKE @p0)
	// Params: [{p0 Go%}]
}
