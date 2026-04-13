// Copyright 2022 The codesjoy Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package aipsql

// ColumnBuilder incrementally builds a Column.
type ColumnBuilder struct {
	column Column
}

// NewColumn starts building a new column.
func NewColumn() *ColumnBuilder {
	return &ColumnBuilder{Column{columnType: ColumnTypeString}}
}

// WithFieldPath specifies the field path the column maps to
// in the returned resource. Field paths are described in AIP-161.
//
// For convenience, the field path is described here as a set of
// segments where each segment is joined by the traversal operator (.).
// E.g. the field path "metrics.`some-metric`.value" would be specified
// as ["metrics", "some-metric", "value"].
func (c *ColumnBuilder) WithFieldPath(segments ...string) *ColumnBuilder {
	c.column.fieldPath = NewFieldPath(segments...)
	return c
}

// WithDatabaseName specifies the database name of the column.
// Important: Only pass safe values (e.g. compile-time constants) to this
// field.
// User input MUST NOT flow to this field, as it will be used directly
// in SQL statements and would allow the user to perform SQL injection
// attacks.
func (c *ColumnBuilder) WithDatabaseName(name string) *ColumnBuilder {
	c.column.databaseName = name
	return c
}

// KeyValue specifies this column is an array of structs with two string members: key and value.
// The key is exposed as a field on the column name, the value can be queried with :, = and !=
//
// This is useful for representing flexible key-value pairs like labels, tags, or metadata
// that don't have a fixed schema.
//
// Database schema example (PostgreSQL):
//
//	labels ARRAY<STRUCT<key STRING, value STRING>>
//
// Query examples:
//
//	labels.environment:"production"     // Has key "environment" with value containing "production"
//	labels.region="us-west"             // Has key "region" with exact value "us-west"
//	labels.tier!="premium"              // Has key "tier" with value not equal to "premium"
//
// Generated SQL uses EXISTS with UNNEST:
//
//	EXISTS (SELECT 1 FROM UNNEST(labels) WHERE key = @key AND value LIKE @value)
//
// Match modes apply to the value matching:
//   - MatchModeExact: value = @value
//   - MatchModePrefix: value LIKE 'prefix%'
//   - MatchModeContains: value LIKE '%substring%'
//
// Example usage:
//
//	labelsColumn := NewColumn().
//	    WithFieldPath("labels").
//	    WithDatabaseName("labels").
//	    KeyValue().
//	    WithMatchModes(MatchModeExact).
//	    Filterable().
//	    Build()
func (c *ColumnBuilder) KeyValue() *ColumnBuilder {
	c.column.keyValue = true
	return c
}

// Bool specifies this column has bool type in the database.
func (c *ColumnBuilder) Bool() *ColumnBuilder {
	c.column.columnType = ColumnTypeBool
	return c
}

// Sortable specifies this column can be sorted on.
func (c *ColumnBuilder) Sortable() *ColumnBuilder {
	c.column.sortable = true
	return c
}

// Filterable specifies this column can be filtered on.
func (c *ColumnBuilder) Filterable() *ColumnBuilder {
	c.column.filterable = true
	return c
}

// FilterableImplicitly specifies this column can be filtered on implicitly.
// This means that AIP-160 filter expressions not referencing any
// particular field will try to search in this column.
func (c *ColumnBuilder) FilterableImplicitly() *ColumnBuilder {
	c.column.filterable = true
	c.column.implicitFilter = true
	return c
}

// WithArgumentSubstitutor specifies a substitution that should happen to the user-specified
// filter argument before it is matched against the database value. If this option is enabled,
// the filter operators permitted will be limited to = (equals) and != (not equals).
func (c *ColumnBuilder) WithArgumentSubstitutor(f func(sub string) string) *ColumnBuilder {
	c.column.argSubstitute = f
	return c
}

// WithMatchModes configures preferred match modes for has (:) filters.
// If omitted, the compatibility default is MatchModeContains.
//
// Match modes are tried in the order specified. The first mode supported by
// the current SQL dialect will be used. If no mode is supported:
//   - StrictMode false: Falls back to MatchModeContains
//   - StrictMode true: Returns an error
//
// Performance recommendations:
//   - For autocomplete/prefix search: Use MatchModePrefix first
//   - For exact matching: Use MatchModeExact
//   - For natural language search: Use MatchModeFullText (requires postgres/mysql)
//   - Avoid MatchModeContains on large tables (causes full table scans)
//
// Example usage:
//
//	// Prefix search with exact match fallback
//	column.WithMatchModes(MatchModePrefix, MatchModeExact)
//
//	// Full-text search with contains fallback
//	column.WithMatchModes(MatchModeFullText, MatchModeContains)
//
//	// Exact match only (strict)
//	column.WithMatchModes(MatchModeExact)
//
// To reset to default behavior, call with no arguments:
//
//	column.WithMatchModes()  // Resets to default (contains mode)
func (c *ColumnBuilder) WithMatchModes(modes ...MatchMode) *ColumnBuilder {
	if len(modes) == 0 {
		c.column.matchModes = nil
		return c
	}
	c.column.matchModes = append(c.column.matchModes[:0], modes...)
	return c
}

// WithIndexHint sets a free-form hint documenting expected index strategy.
//
// This field is for documentation purposes only and does not affect SQL generation
// or query execution. Use it to document:
//   - Which database index should be used for this column
//   - Expected query performance characteristics
//   - Index cardinality and selectivity information
//   - Recommendations for query optimization
//
// Example usage:
//
//	column.WithIndexHint("Use idx_title_prefix for prefix searches. Cardinality: ~100K unique values.")
//	column.WithIndexHint("Full-text index idx_content_fts. Average document length: 500 words.")
//	column.WithIndexHint("Composite index idx_status_user covers status + user_id queries.")
//
// This information can be useful for:
//   - Database administrators planning index strategies
//   - Developers debugging slow queries
//   - Documentation and code reviews
//   - Performance analysis and optimization
func (c *ColumnBuilder) WithIndexHint(tag string) *ColumnBuilder {
	c.column.indexHint = tag
	return c
}

// Build returns the built column.
func (c *ColumnBuilder) Build() *Column {
	result := &Column{}
	*result = c.column
	if c.column.matchModes != nil {
		result.matchModes = append([]MatchMode(nil), c.column.matchModes...)
	}
	return result
}

// TableBuilder incrementally builds a Table.
type TableBuilder struct {
	columns []*Column
}

// NewTable starts building a new table.
func NewTable() *TableBuilder {
	return &TableBuilder{}
}

// WithColumns specifies the columns in the table.
func (t *TableBuilder) WithColumns(columns ...*Column) *TableBuilder {
	t.columns = columns
	return t
}

// Build returns the built table.
//
// The build process:
//  1. Builds a lookup map from field path to column for O(1) resolution.
//  2. Collects columns marked as implicit filter targets.
//  3. Panics if two columns share the same field path — this is a programmer error
//     and should be caught during development, not at runtime in production.
func (t *TableBuilder) Build() *Table {
	columnByFieldPath := make(map[string]*Column)
	implicitFilterColumns := make([]*Column, 0, len(t.columns))
	for _, c := range t.columns {
		registerColumn(columnByFieldPath, c)
		implicitFilterColumns = appendImplicitFilterColumn(implicitFilterColumns, c)
	}

	return &Table{
		columns:               t.columns,
		implicitFilterColumns: implicitFilterColumns,
		columnByFieldPath:     columnByFieldPath,
	}
}

// registerColumn inserts the column into the lookup map keyed by lowercase field path.
// Panics if a duplicate field path is detected — this indicates a misconfigured table schema.
func registerColumn(columnByFieldPath map[string]*Column, column *Column) {
	fieldPath := column.fieldPath.String()
	if _, ok := columnByFieldPath[fieldPath]; ok {
		panic("multiple columns with the same field path: " + fieldPath)
	}
	columnByFieldPath[fieldPath] = column
}

// appendImplicitFilterColumn appends the column to the list if it is marked for implicit filtering.
func appendImplicitFilterColumn(columns []*Column, column *Column) []*Column {
	if !column.implicitFilter {
		return columns
	}
	return append(columns, column)
}
