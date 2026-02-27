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
)

// SQLDialect identifies a SQL dialect-specific generation strategy.
//
// The dialect determines which SQL syntax features are available, particularly
// for full-text search operations. Different dialects support different SQL
// features and syntax.
//
// Supported dialects:
//   - generic: Standard SQL compatible with most databases (default)
//   - postgres: PostgreSQL-specific features (e.g., to_tsvector, websearch_to_tsquery)
//   - mysql: MySQL-specific features (e.g., MATCH...AGAINST)
//
// Example usage:
//
//	opts := &WhereClauseOptions{
//	    Dialect: SQLDialectPostgres,
//	}
//	sql, params, err := table.WhereClauseWithOptions(filter, "p", opts)
type SQLDialect string

const (
	// SQLDialectGeneric is the default dialect-agnostic mode.
	// Use this for maximum compatibility across different database systems.
	// Full-text search is not supported in generic mode; it will fall back
	// to substring matching (LIKE %value%) unless StrictMode is enabled.
	SQLDialectGeneric SQLDialect = "generic"

	// SQLDialectPostgres targets PostgreSQL-compatible SQL fragments.
	// Enables PostgreSQL-specific full-text search using to_tsvector and
	// websearch_to_tsquery functions. Use this when your database is PostgreSQL
	// or a compatible system (e.g., CockroachDB).
	SQLDialectPostgres SQLDialect = "postgres"

	// SQLDialectMySQL targets MySQL-compatible SQL fragments.
	// Enables MySQL-specific full-text search using MATCH...AGAINST syntax.
	// Use this when your database is MySQL or MariaDB.
	SQLDialectMySQL SQLDialect = "mysql"
)

// validateDialect validates that the given SQL dialect is supported.
// Valid dialects are "generic", "postgres", and "mysql".
// Returns an error if the dialect is not supported.
func validateDialect(dialect string) error {
	switch strings.ToLower(dialect) {
	case "", string(SQLDialectGeneric), string(SQLDialectPostgres), string(SQLDialectMySQL):
		return nil
	default:
		return fmt.Errorf(
			"invalid SQL dialect %q, supported values are %q, %q and %q",
			dialect,
			SQLDialectGeneric,
			SQLDialectPostgres,
			SQLDialectMySQL,
		)
	}
}

func normalizeSQLDialect(dialect SQLDialect) (SQLDialect, error) {
	switch strings.ToLower(string(dialect)) {
	case "", string(SQLDialectGeneric):
		return SQLDialectGeneric, nil
	case string(SQLDialectPostgres):
		return SQLDialectPostgres, nil
	case string(SQLDialectMySQL):
		return SQLDialectMySQL, nil
	default:
		return "", fmt.Errorf(
			"unsupported sql dialect %q, supported values are %q, %q and %q",
			dialect,
			SQLDialectGeneric,
			SQLDialectPostgres,
			SQLDialectMySQL,
		)
	}
}

// MatchMode controls how has (:) filters are translated to SQL.
//
// Different match modes provide different trade-offs between query flexibility
// and performance. Index-friendly modes (exact, prefix) can utilize B-tree indexes
// for fast lookups, while fulltext mode uses specialized full-text indexes.
// The contains mode provides maximum flexibility but cannot use indexes efficiently.
//
// Performance characteristics (on a 1M row table):
//   - exact: ~1ms (uses B-tree index, scans 1 row)
//   - prefix: ~5ms (uses B-tree index, scans ~100 rows)
//   - fulltext: ~10ms (uses full-text index, scans ~1000 rows)
//   - contains: ~500ms (no index, full table scan)
//
// Usage scenarios:
//   - exact: Use for exact matching (e.g., status codes, IDs)
//   - prefix: Use for prefix searches (e.g., autocomplete, name lookups)
//   - fulltext: Use for natural language search (e.g., document content)
//   - contains: Use as fallback when no index is available
//
// Example configuration:
//
//	column := NewColumn().
//	    WithFieldPath("title").
//	    WithDatabaseName("title").
//	    WithMatchModes(MatchModePrefix, MatchModeExact).  // Try prefix first, then exact
//	    Filterable().
//	    Build()
//
// When multiple match modes are configured, the generator will try them in order
// and use the first one supported by the current SQL dialect. If none are supported
// and StrictMode is false, it falls back to MatchModeContains.
type MatchMode string

const (
	// MatchModeExact translates to exact equality (column = @param) and can use B-tree indexes.
	// This is the most efficient mode for exact matching scenarios.
	//
	// Generated SQL example:
	//   status = @p1
	//
	// Use cases:
	//   - Status codes, enum values
	//   - Exact ID or key lookups
	//   - Boolean flags
	MatchModeExact MatchMode = "exact"

	// MatchModePrefix translates to prefix LIKE (column LIKE 'value%') and can use B-tree indexes.
	// This mode is efficient for prefix searches and autocomplete scenarios.
	//
	// Generated SQL example:
	//   name LIKE @p1  (where @p1 = 'John%')
	//
	// Use cases:
	//   - Autocomplete search
	//   - Name or title prefix matching
	//   - Hierarchical path matching
	//
	// Note: Special LIKE characters (%, _, \) in user input are automatically escaped.
	MatchModePrefix MatchMode = "prefix"

	// MatchModeFullText translates to full-text predicates on supported dialects.
	// This mode requires full-text indexes and is only available for PostgreSQL and MySQL.
	//
	// Generated SQL examples:
	//   PostgreSQL: to_tsvector('simple', content) @@ websearch_to_tsquery('simple', @p1)
	//   MySQL: MATCH(content) AGAINST (@p1 IN BOOLEAN MODE)
	//
	// Use cases:
	//   - Natural language search in documents
	//   - Multi-word search queries
	//   - Relevance-ranked search results
	//
	// Note: Requires SQLDialectPostgres or SQLDialectMySQL. Falls back to contains
	// mode in generic dialect unless StrictMode is enabled.
	MatchModeFullText MatchMode = "fulltext"

	// MatchModeContains translates to substring LIKE (column LIKE '%value%') and is the compatibility mode.
	// This mode cannot use indexes efficiently and results in full table scans.
	//
	// Generated SQL example:
	//   description LIKE @p1  (where @p1 = '%keyword%')
	//
	// Use cases:
	//   - Fallback when no index is available
	//   - Small tables where performance is not critical
	//   - Substring matching when prefix search is insufficient
	//
	// Performance warning: This mode performs full table scans and should be avoided
	// on large tables. Consider using MatchModePrefix or MatchModeFullText instead.
	MatchModeContains MatchMode = "contains"
)

func isValidMatchMode(mode MatchMode) bool {
	switch mode {
	case MatchModeExact, MatchModePrefix, MatchModeFullText, MatchModeContains:
		return true
	default:
		return false
	}
}

// validateMatchMode validates that the given match mode is supported.
// Valid match modes are "exact", "prefix", "fulltext", and "contains".
// Returns an error with a descriptive message if the mode is not supported.
func validateMatchMode(mode MatchMode) error {
	if !isValidMatchMode(mode) {
		return fmt.Errorf(
			"invalid match mode %q, valid match modes are %q, %q, %q and %q",
			mode,
			MatchModeExact,
			MatchModePrefix,
			MatchModeFullText,
			MatchModeContains,
		)
	}
	return nil
}

// validateHasOperator validates that the has (:) operator can be used on the given column.
// The has operator can only be used on STRING type columns and cannot be used on columns
// with argSubstitute configured.
// Returns an error with a descriptive message if the validation fails.
func validateHasOperator(column *Column) error {
	if column.columnType != ColumnTypeString {
		return fmt.Errorf(
			"has (:) operator can only be used on STRING columns, field %q has type %s",
			column.fieldPath.String(),
			column.columnType.String(),
		)
	}
	if column.argSubstitute != nil {
		return fmt.Errorf(
			"cannot use has (:) operator on a field that have argSubstitute function",
		)
	}
	return nil
}

const (
	// ColumnTypeString is a column of type string.
	ColumnTypeString ColumnType = iota
	// ColumnTypeBool is a column of type boolean.  NULL values are mapped to FALSE.
	ColumnTypeBool = iota
)

// ColumnType is an enum for the type of a column.  Valid values are in the const block above.
type ColumnType int32

// CompositeIndex represents a multi-column database index where column order matters
// for query optimization. The order of columns in the Columns slice matches the order
// they appear in the index definition.
//
// Composite indexes are critical for optimizing queries with multiple WHERE conditions
// or combined WHERE and ORDER BY clauses. The order of columns in the index determines
// which query patterns can efficiently use the index.
//
// Index utilization rules:
//   - Queries can use an index if they reference a prefix of the index columns
//   - Equality conditions should come before range conditions
//   - The index can be used for ORDER BY if the sort fields match the index prefix
//
// Example:
//
//	// Define a composite index on (status, user_id, created_at)
//	table.CompositeIndexes = []CompositeIndex{
//	    {
//	        Name:    "idx_status_user_created",
//	        Columns: []string{"status", "user_id", "created_at"},
//	    },
//	}
//
//	// This query can use the index efficiently:
//	// WHERE status = 'active' AND user_id = 123 AND created_at > '2024-01-01'
//
//	// This query can partially use the index (status and user_id):
//	// WHERE status = 'active' AND user_id = 123
//
//	// This query CANNOT use the index (skips status):
//	// WHERE user_id = 123 AND created_at > '2024-01-01'
//
// When EnableCompositeIndexOptimization is true, the WHERE clause generator will
// automatically reorder conditions to match the index column order, maximizing
// index utilization.
type CompositeIndex struct {
	// Name is the index name (for documentation/debugging purposes).
	// This should match the actual index name in your database schema.
	Name string

	// Columns is the list of database column names in the order they appear
	// in the index definition. This order is critical for index utilization.
	//
	// Important: Use database column names (Column.databaseName), not field paths.
	//
	// Example:
	//   Columns: []string{"status", "user_id", "created_at"}
	Columns []string
}

func (t ColumnType) String() string {
	switch t {
	case ColumnTypeString:
		return "STRING"
	case ColumnTypeBool:
		return "BOOL"
	default:
		return "UNKNOWN"
	}
}

// Column represents the schema of a Database column.
//
// A Column defines how an externally-visible API field maps to a database column,
// including its filtering and sorting capabilities, match modes for text search,
// and index optimization hints.
//
// Example usage:
//
//	// Create a column with prefix matching for autocomplete
//	titleColumn := NewColumn().
//	    WithFieldPath("title").
//	    WithDatabaseName("title").
//	    WithMatchModes(MatchModePrefix, MatchModeExact).
//	    WithIndexHint("Use idx_title_prefix for prefix searches").
//	    Filterable().
//	    Sortable().
//	    Build()
//
//	// Create a full-text searchable column
//	contentColumn := NewColumn().
//	    WithFieldPath("content").
//	    WithDatabaseName("content").
//	    WithMatchModes(MatchModeFullText, MatchModeContains).
//	    Filterable().
//	    Build()
//
//	// Create a key-value column for labels
//	labelsColumn := NewColumn().
//	    WithFieldPath("labels").
//	    WithDatabaseName("labels").
//	    KeyValue().
//	    Filterable().
//	    Build()
type Column struct {
	// The externally-visible field path this column maps to.
	// This path may be referenced in AIP-160 filters and AIP-132 order by clauses.
	fieldPath FieldPath

	// The database name of the column.
	// Important: Only assign assign safe constants to this field.
	// User input MUST NOT flow to this field, as it will be used directly
	// in SQL statements and would allow the user to perform SQL injection
	// attacks.
	databaseName string

	// Whether this column can be sorted on.
	sortable bool

	// Whether this column can be filtered on.
	filterable bool

	// ImplicitFilter controls whether this field is searched implicitly
	// in AIP-160 filter expressions.
	implicitFilter bool

	// Whether this column is an array of structs with two string members: key and value.
	keyValue bool

	// The type of the column, defaults to ColumnType_STRING.
	columnType ColumnType

	// The function which is applied to the filter arguments.
	argSubstitute func(sub string) string

	// Match modes for has (:) filters, in descending preference order.
	// When multiple modes are specified, the generator tries them in order
	// and uses the first one supported by the current SQL dialect.
	//
	// If empty or all modes are unsupported:
	//   - StrictMode false: Falls back to MatchModeContains
	//   - StrictMode true: Returns an error
	//
	// Example:
	//   matchModes: []MatchMode{MatchModePrefix, MatchModeExact}
	//   // Tries prefix first, falls back to exact if prefix is not suitable
	matchModes []MatchMode

	// A free-form hint that can be used to document the preferred index strategy.
	// This field is for documentation purposes only and does not affect SQL generation.
	//
	// Use this to document:
	//   - Which index should be used for this column
	//   - Performance characteristics
	//   - Query optimization recommendations
	//
	// Example:
	//   indexHint: "Use idx_title_prefix for prefix searches. Expected cardinality: 100K unique values."
	indexHint string
}

// Table represents the schema of a Database table, view or query.
//
// A Table defines the structure of a database table including its columns,
// implicit filter configuration, and composite indexes for query optimization.
//
// Example usage:
//
//	table := NewTable().
//	    WithColumns(
//	        NewColumn().WithFieldPath("id").WithDatabaseName("id").Sortable().Build(),
//	        NewColumn().WithFieldPath("title").WithDatabaseName("title").
//	            WithMatchModes(MatchModePrefix).Filterable().Sortable().Build(),
//	        NewColumn().WithFieldPath("content").WithDatabaseName("content").
//	            WithMatchModes(MatchModeFullText).FilterableImplicitly().Build(),
//	    ).
//	    Build()
//
//	// Add composite indexes for optimization
//	table.CompositeIndexes = []CompositeIndex{
//	    {
//	        Name:    "idx_status_created",
//	        Columns: []string{"status", "created_at"},
//	    },
//	}
type Table struct {
	// The columns in the database table.
	columns []*Column

	// The columns eligible for implicit filter matching.
	implicitFilterColumns []*Column

	// A mapping from externally-visible field path to the column
	// definition. The column name used as a key is in lowercase.
	columnByFieldPath map[string]*Column

	// CompositeIndexes defines multi-column indexes on this table.
	// The order of columns in each index is critical for query optimization.
	//
	// When EnableCompositeIndexOptimization is true in WhereClauseOptions,
	// the WHERE clause generator will:
	//   1. Select the best matching index based on query conditions
	//   2. Reorder WHERE conditions to match the index column order
	//   3. Place equality conditions before range conditions
	//
	// This can significantly improve query performance (10x-100x) on large tables
	// by ensuring the database optimizer uses the most efficient index.
	//
	// Example:
	//   CompositeIndexes: []CompositeIndex{
	//       {
	//           Name:    "idx_status_user_created",
	//           Columns: []string{"status", "user_id", "created_at"},
	//       },
	//   }
	//
	// Note: This field must be set after calling Build() on the TableBuilder.
	CompositeIndexes []CompositeIndex
}

// findColumnByFieldPath looks up one column and verifies it with checkFunc.
// When not found, it returns nil with valid field names for error construction.
func (t *Table) findColumnByFieldPath(
	path FieldPath,
	checkFunc func(*Column) bool,
) (*Column, []string) {
	col := t.columnByFieldPath[path.String()]
	if col != nil && checkFunc(col) {
		return col, nil
	}
	return nil, t.collectFieldPaths(checkFunc)
}

func (t *Table) collectFieldPaths(checkFunc func(*Column) bool) []string {
	fieldPaths := make([]string, 0, len(t.columns))
	for _, column := range t.columns {
		if checkFunc(column) {
			fieldPaths = append(fieldPaths, column.fieldPath.String())
		}
	}
	return fieldPaths
}

// FilterableColumnByFieldPath returns the database name of the filterable column
// with the given field path.
func (t *Table) FilterableColumnByFieldPath(path FieldPath) (*Column, error) {
	col, validNames := t.findColumnByFieldPath(
		path,
		func(c *Column) bool { return c.filterable },
	)
	if col == nil {
		return nil, fmt.Errorf(
			"no filterable field %q, valid fields are %s",
			path.String(),
			strings.Join(validNames, ", "),
		)
	}
	return col, nil
}

// SortableColumnByFieldPath returns the sortable database column
// with the given externally-visible field path.
func (t *Table) SortableColumnByFieldPath(path FieldPath) (*Column, error) {
	col, validNames := t.findColumnByFieldPath(
		path,
		func(c *Column) bool { return c.sortable },
	)
	if col == nil {
		return nil, fmt.Errorf(
			"no sortable field named %q, valid fields are %s",
			path.String(),
			strings.Join(validNames, ", "),
		)
	}
	return col, nil
}

// PaginationToken holds the values from the last row of the previous page
// for Seek pagination. These values correspond to the ORDER BY fields and
// are used to generate lexicographic comparison predicates.
//
// Seek pagination is more efficient than OFFSET-based pagination for large datasets
// because it uses indexed columns for filtering rather than skipping rows.
//
// Example usage:
//
//	// First page (no token)
//	sql := "SELECT * FROM orders ORDER BY created_at DESC, id DESC LIMIT 10"
//
//	// Get the last row values: created_at='2024-01-15T10:30:00Z', id=12345
//	token := PaginationToken{
//	    Values: []interface{}{"2024-01-15T10:30:00Z", 12345},
//	}
//
//	// Generate seek predicate for next page
//	seekPredicate, params, err := GenerateSeekPredicate(orderByFields, token)
//	// Result: (created_at < @seek_cmp_0 OR (created_at = @seek_eq_0 AND id < @seek_cmp_1))
//
//	// Second page query
//	sql := "SELECT * FROM orders WHERE " + seekPredicate + " ORDER BY created_at DESC, id DESC LIMIT 10"
//
// Performance comparison (1M row table):
//   - OFFSET 10000: ~500ms (scans and skips 10000 rows)
//   - Seek pagination: ~5ms (uses index to jump directly to position)
type PaginationToken struct {
	// Values contains the field values from the last row of the previous page,
	// in the same order as the ORDER BY fields.
	//
	// Important: The number of values must match the number of ORDER BY fields.
	// The types should match the column types.
	//
	// Example:
	//   For ORDER BY created_at DESC, id DESC:
	//   Values: []interface{}{"2024-01-15T10:30:00Z", 12345}
	Values []interface{}
}

// OrderByField represents a field in an ORDER BY clause with its sort direction.
//
// Used in conjunction with PaginationToken to generate efficient Seek pagination
// predicates that can utilize database indexes.
//
// Example:
//
//	orderByFields := []OrderByField{
//	    {
//	        Column:    statusColumn,
//	        Direction: "ASC",
//	    },
//	    {
//	        Column:    createdAtColumn,
//	        Direction: "DESC",
//	    },
//	    {
//	        Column:    idColumn,  // Tie breaker for uniqueness
//	        Direction: "DESC",
//	    },
//	}
//
// Best practices:
//   - Always include a unique column (e.g., ID) as the last field to ensure deterministic ordering
//   - Use columns that have indexes for better performance
//   - Limit to 5 or fewer fields to keep seek predicates manageable
type OrderByField struct {
	// Column is the database column to sort by.
	Column *Column

	// Direction is the sort direction, either "ASC" or "DESC".
	// This affects the comparison operators used in seek predicates:
	//   - ASC uses > for "greater than" comparisons
	//   - DESC uses < for "less than" comparisons
	Direction string
}
