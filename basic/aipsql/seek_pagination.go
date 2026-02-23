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
	"strconv"
	"strings"
)

type seekBinder struct {
	prefix string
	next   int
	params []QueryParameter
}

func (b *seekBinder) bind(value string) string {
	name := b.prefix + strconv.Itoa(b.next)
	b.next++
	b.params = append(b.params, QueryParameter{Name: name, Value: value})
	return "@" + name
}

func (b *seekBinder) sortValue(column *Column, value string) (string, error) {
	switch column.columnType {
	case ColumnTypeString:
		return b.bind(value), nil
	case ColumnTypeBool:
		if strings.EqualFold(value, "true") {
			return "TRUE", nil
		}
		if strings.EqualFold(value, "false") {
			return "FALSE", nil
		}
		return "", fmt.Errorf(
			"only TRUE or FALSE can be specified as sort values for boolean fields",
		)
	default:
		return "", fmt.Errorf(
			"unsupported column type for seek pagination: %s",
			column.columnType.String(),
		)
	}
}

// BuildSeekPaginationClause generates a lexicographical seek-pagination predicate
// that can be appended to a WHERE clause.
//
// The generated predicate assumes ORDER BY uses the provided sort order plus the
// tie breaker in ascending order.
func (t *Table) BuildSeekPaginationClause(
	order []OrderBy,
	lastSortValues []string,
	tieBreakerFieldPath FieldPath,
	lastTieBreakerValue string,
	parameterPrefix string,
	dialect SQLDialect,
) (string, []QueryParameter, error) {
	if _, err := normalizeSQLDialect(dialect); err != nil {
		return "", nil, err
	}
	if len(order) == 0 {
		return "", nil, fmt.Errorf("order must contain at least one field")
	}
	if len(order) != len(lastSortValues) {
		return "", nil, fmt.Errorf(
			"lastSortValues has %d values, expected %d values matching order fields",
			len(lastSortValues),
			len(order),
		)
	}
	for _, entry := range order {
		if entry.FieldPath.Equals(tieBreakerFieldPath) {
			return "", nil, fmt.Errorf(
				"tie breaker field %q must not already exist in order",
				tieBreakerFieldPath.String(),
			)
		}
	}

	tieBreakerColumn, err := t.SortableColumnByFieldPath(tieBreakerFieldPath)
	if err != nil {
		return "", nil, err
	}

	type seekColumn struct {
		column     *Column
		descending bool
		valueExpr  string
	}
	seekColumns := make([]seekColumn, 0, len(order))
	binder := &seekBinder{prefix: parameterPrefix}

	for i, entry := range order {
		column, err := t.SortableColumnByFieldPath(entry.FieldPath)
		if err != nil {
			return "", nil, err
		}
		valueExpr, err := binder.sortValue(column, lastSortValues[i])
		if err != nil {
			return "", nil, fmt.Errorf("sort value for field %q: %w", entry.FieldPath.String(), err)
		}
		seekColumns = append(seekColumns, seekColumn{
			column:     column,
			descending: entry.Descending,
			valueExpr:  valueExpr,
		})
	}

	tieBreakerExpr, err := binder.sortValue(tieBreakerColumn, lastTieBreakerValue)
	if err != nil {
		return "", nil, fmt.Errorf(
			"sort value for tie breaker field %q: %w",
			tieBreakerFieldPath.String(),
			err,
		)
	}

	parts := make([]string, 0, len(seekColumns)+1)
	for i := range seekColumns {
		var part strings.Builder
		part.WriteString("(")
		for j := 0; j < i; j++ {
			if j > 0 {
				part.WriteString(" AND ")
			}
			part.WriteString(seekColumns[j].column.databaseName)
			part.WriteString(" = ")
			part.WriteString(seekColumns[j].valueExpr)
		}
		if i > 0 {
			part.WriteString(" AND ")
		}
		part.WriteString(seekColumns[i].column.databaseName)
		if seekColumns[i].descending {
			part.WriteString(" < ")
		} else {
			part.WriteString(" > ")
		}
		part.WriteString(seekColumns[i].valueExpr)
		part.WriteString(")")
		parts = append(parts, part.String())
	}

	var tiePart strings.Builder
	tiePart.WriteString("(")
	for i, entry := range seekColumns {
		if i > 0 {
			tiePart.WriteString(" AND ")
		}
		tiePart.WriteString(entry.column.databaseName)
		tiePart.WriteString(" = ")
		tiePart.WriteString(entry.valueExpr)
	}
	if len(seekColumns) > 0 {
		tiePart.WriteString(" AND ")
	}
	tiePart.WriteString(tieBreakerColumn.databaseName)
	tiePart.WriteString(" > ")
	tiePart.WriteString(tieBreakerExpr)
	tiePart.WriteString(")")
	parts = append(parts, tiePart.String())

	return "(" + strings.Join(parts, " OR ") + ")", binder.params, nil
}

// buildLexicographicComparison generates n OR-connected conditions for Seek pagination,
// where n is the number of sort fields. Each condition represents one level of
// lexicographic comparison.
//
// For example, with ORDER BY a ASC, b ASC, c ASC and values (1, 2, 3), it generates:
//
//	(a > @p0) OR
//	(a = @p1 AND b > @p2) OR
//	(a = @p3 AND b = @p4 AND c > @p5)
//
// The function returns the SQL predicate string, the query parameters, and any error.
func buildLexicographicComparison(
	fields []OrderByField,
	values []interface{},
) (string, []QueryParameter, error) {
	if len(fields) != len(values) {
		return "", nil, fmt.Errorf(
			"field count (%d) does not match value count (%d)",
			len(fields),
			len(values),
		)
	}

	if len(fields) == 0 {
		return "", nil, fmt.Errorf("fields cannot be empty")
	}

	// Pre-allocate capacity for params and clauses to reduce memory allocations
	// For n fields, we need n(n+1)/2 parameters and n clauses
	estimatedParams := len(fields) * (len(fields) + 1) / 2
	params := make([]QueryParameter, 0, estimatedParams)
	clauses := make([]string, 0, len(fields))
	paramIndex := 0

	processedFields := make([]OrderByField, 0, len(fields))
	processedValues := make([]interface{}, 0, len(values))
	remainingFields := fields
	remainingValues := values

	// Generate n OR-connected conditions, one for each lexicographic level.
	for len(remainingFields) > 0 {
		currentField := remainingFields[0]
		currentValue := remainingValues[0]

		// Pre-allocate capacity for parts (prefix equalities + current comparison).
		parts := make([]string, 0, len(processedFields)+1)

		// Add equality conditions for all previously processed fields.
		prevFields := processedFields
		prevValues := processedValues
		for len(prevFields) > 0 {
			paramName := fmt.Sprintf("@seek_eq_%d", paramIndex)
			paramIndex++
			prevColumnName := prevFields[0].Column.databaseName
			parts = append(parts, fmt.Sprintf("%s = %s", prevColumnName, paramName))
			params = append(params, QueryParameter{Name: paramName, Value: prevValues[0]})
			prevFields = prevFields[1:]
			prevValues = prevValues[1:]
		}

		// Add comparison condition for the current level field.
		paramName := fmt.Sprintf("@seek_cmp_%d", paramIndex)
		paramIndex++
		operator := getComparisonOperator(currentField.Direction)
		parts = append(
			parts,
			fmt.Sprintf("%s %s %s", currentField.Column.databaseName, operator, paramName),
		)
		params = append(params, QueryParameter{Name: paramName, Value: currentValue})

		// Combine all parts with AND
		clause := "(" + strings.Join(parts, " AND ") + ")"
		clauses = append(clauses, clause)

		processedFields = append(processedFields, currentField)
		processedValues = append(processedValues, currentValue)
		remainingFields = remainingFields[1:]
		remainingValues = remainingValues[1:]
	}

	// Combine all clauses with OR
	result := "(" + strings.Join(clauses, " OR ") + ")"
	return result, params, nil
}

// getComparisonOperator returns the appropriate comparison operator based on sort direction.
// For ASC (ascending), we want rows greater than the last value (>).
// For DESC (descending), we want rows less than the last value (<).
func getComparisonOperator(direction string) string {
	if strings.ToUpper(direction) == "DESC" {
		return "<"
	}
	return ">"
}
