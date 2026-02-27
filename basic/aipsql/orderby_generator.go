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

// MergeWithDefaultOrder merges the specified order with the given
// defaultOrder. The merge occurs as follows:
//   - Ordering specified in `order` takes precedence.
//   - For columns not specified in the `order` that appear in `defaultOrder`,
//     ordering is applied in the order they apply in defaultOrder.
func MergeWithDefaultOrder(defaultOrder []OrderBy, order []OrderBy) []OrderBy {
	result := make([]OrderBy, 0, len(order)+len(defaultOrder))
	seenColumns := make(map[string]struct{})
	for _, o := range order {
		result = append(result, o)
		seenColumns[o.FieldPath.String()] = struct{}{}
	}
	for _, o := range defaultOrder {
		if _, ok := seenColumns[o.FieldPath.String()]; !ok {
			result = append(result, o)
		}
	}
	return result
}

// OrderByClause returns a Standard SQL Order by clause, including
// "ORDER BY" and trailing new line (if an order is specified).
// If no order is specified, returns "".
//
// The returned order clause is safe against SQL injection; only
// strings appearing from Table appear in the output.
func (t *Table) OrderByClause(order []OrderBy) (string, error) {
	return t.orderByClause(order, SQLDialectGeneric)
}

// OrderByClauseWithDialect returns a SQL ORDER BY fragment while validating the
// provided dialect value.
func (t *Table) OrderByClauseWithDialect(order []OrderBy, dialect SQLDialect) (string, error) {
	normalizedDialect, err := normalizeSQLDialect(dialect)
	if err != nil {
		return "", err
	}
	return t.orderByClause(order, normalizedDialect)
}

func (t *Table) orderByClause(order []OrderBy, dialect SQLDialect) (string, error) {
	// Dialect is currently validated for forward compatibility.
	_ = dialect
	if len(order) == 0 {
		return "", nil
	}
	seenColumns := make(map[string]struct{})
	var result strings.Builder
	for i, o := range order {
		if i > 0 {
			result.WriteString(", ")
		}
		clause, err := t.buildOrderByFieldClause(o, seenColumns)
		if err != nil {
			return "", err
		}
		result.WriteString(clause)
	}
	return result.String(), nil
}

func (t *Table) buildOrderByFieldClause(order OrderBy, seenColumns map[string]struct{}) (string, error) {
	column, err := t.SortableColumnByFieldPath(order.FieldPath)
	if err != nil {
		return "", err
	}
	if _, ok := seenColumns[column.databaseName]; ok {
		return "", fmt.Errorf(
			"field appears in order_by multiple times: %q",
			order.FieldPath.String(),
		)
	}
	seenColumns[column.databaseName] = struct{}{}

	var clause strings.Builder
	clause.WriteString(column.databaseName)
	if order.Descending {
		clause.WriteString(" DESC")
	}
	return clause.String(), nil
}

// findMatchingOrderByIndex finds a composite index that matches the ORDER BY fields.
// A matching index is one where the ORDER BY fields form a prefix of the index columns.
//
// For example:
//   - ORDER BY a, b can use index (a, b, c)
//   - ORDER BY a, b, c can use index (a, b, c)
//   - ORDER BY a, c CANNOT use index (a, b, c) - skips column b
//
// Returns the first matching index, or nil if no match is found.
func findMatchingOrderByIndex(fields []OrderByField, indexes []CompositeIndex) *CompositeIndex {
	for i := range indexes {
		if indexMatchesOrderBy(fields, indexes[i]) {
			return &indexes[i]
		}
	}
	return nil
}

// indexMatchesOrderBy checks if an ORDER BY field list matches a composite index.
// The ORDER BY fields must form a prefix of the index columns in the same order.
//
// Matching rules:
//   - The number of ORDER BY fields must not exceed the number of index columns
//   - Each ORDER BY field must match the corresponding index column by database name
//   - The fields must be in the same order as the index columns
//
// Examples:
//   - ORDER BY a, b matches index (a, b, c) ✓
//   - ORDER BY a matches index (a, b, c) ✓
//   - ORDER BY a, b, c matches index (a, b, c) ✓
//   - ORDER BY a, c does NOT match index (a, b, c) ✗ (skips b)
//   - ORDER BY b, a does NOT match index (a, b, c) ✗ (wrong order)
//   - ORDER BY a, b, c, d does NOT match index (a, b, c) ✗ (too many fields)
func indexMatchesOrderBy(fields []OrderByField, index CompositeIndex) bool {
	// ORDER BY fields cannot exceed index columns
	if len(fields) > len(index.Columns) {
		return false
	}

	// Check if each ORDER BY field matches the corresponding index column
	for i, field := range fields {
		if field.Column.databaseName != index.Columns[i] {
			return false
		}
	}

	return true
}
