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
	"sync"
)

var builderPool = sync.Pool{
	New: func() interface{} {
		return &strings.Builder{}
	},
}

func getBuilder() *strings.Builder {
	return builderPool.Get().(*strings.Builder)
}

func putBuilder(b *strings.Builder) {
	b.Reset()
	builderPool.Put(b)
}

// whereClause constructs Standard SQL WHERE clause parts from
// column definitions and a parsed AIP-160 filter.
type whereClause struct {
	table                            *Table
	parameters                       []QueryParameter
	namePrefix                       string
	nextValueName                    int
	dialect                          SQLDialect
	strictMode                       bool
	fallbackMode                     MatchMode
	optimizeMatch                    bool
	enableCompositeIndexOptimization bool
}

// QueryParameter represents a parameterized query parameter used to prevent SQL injection.
type QueryParameter struct {
	Name  string
	Value interface{}
}

// WhereClauseOptions controls SQL generation behavior for has (:) filters.
type WhereClauseOptions struct {
	Dialect                          SQLDialect
	StrictMode                       bool
	EnableCompositeIndexOptimization bool
}

func isSupported(mode MatchMode, dialect SQLDialect) bool {
	switch mode {
	case MatchModeExact, MatchModePrefix, MatchModeContains:
		return true
	case MatchModeFullText:
		return dialect == SQLDialectPostgres || dialect == SQLDialectMySQL
	default:
		return false
	}
}

// QuoteLike turns a literal string into an escaped like expression.
// This means strings like test_name will only match as expected, rather than
// also matching test3name.
func QuoteLike(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "%", "\\%")
	value = strings.ReplaceAll(value, "_", "\\_")
	return value
}

// WhereClause creates a Standard SQL WHERE clause fragment for the given filter.
func (t *Table) WhereClause(
	filter *Filter,
	parameterPrefix string,
) (string, []QueryParameter, error) {
	return t.whereClause(filter, parameterPrefix, WhereClauseOptions{}, false)
}

// WhereClauseWithOptions creates a Standard SQL WHERE clause fragment while applying
// dialect-aware, index-friendly match mode selection.
func (t *Table) WhereClauseWithOptions(
	filter *Filter,
	parameterPrefix string,
	options WhereClauseOptions,
) (string, []QueryParameter, error) {
	return t.whereClause(filter, parameterPrefix, options, true)
}

func (t *Table) whereClause(
	filter *Filter,
	parameterPrefix string,
	options WhereClauseOptions,
	optimizeMatch bool,
) (string, []QueryParameter, error) {
	if filter.Expression == nil {
		return "(TRUE)", []QueryParameter{}, nil
	}

	dialect, err := normalizeSQLDialect(options.Dialect)
	if err != nil {
		return "", nil, err
	}

	query := &whereClause{
		table:                            t,
		parameters:                       make([]QueryParameter, 0, 32),
		namePrefix:                       parameterPrefix,
		dialect:                          dialect,
		strictMode:                       options.StrictMode,
		fallbackMode:                     MatchModeContains,
		optimizeMatch:                    optimizeMatch,
		enableCompositeIndexOptimization: options.EnableCompositeIndexOptimization,
	}

	clause, err := query.expressionQuery(filter.Expression)
	if err != nil {
		return "", []QueryParameter{}, err
	}
	return clause, query.parameters, nil
}

func (w *whereClause) expressionQuery(expression *Expression) (string, error) {
	factors := flattenExpressionFactors(expression)
	if len(factors) == 0 {
		return "()", nil
	}
	if w.enableCompositeIndexOptimization && len(factors) > 1 && len(w.table.CompositeIndexes) > 0 {
		reorderedFactors, ok := w.reorderedExpressionFactors(factors)
		if ok {
			factors = reorderedFactors
		}
	}
	return w.generateSQLFromFactors(factors)
}

func flattenExpressionFactors(expression *Expression) []*Factor {
	if expression == nil {
		return nil
	}

	totalFactors := 0
	for _, sequence := range expression.Sequences {
		totalFactors += len(sequence.Factors)
	}
	result := make([]*Factor, 0, totalFactors)
	return appendFlattenedExpressionFactors(result, expression)
}

func appendFlattenedExpressionFactors(dst []*Factor, expression *Expression) []*Factor {
	if expression == nil {
		return dst
	}
	for _, sequence := range expression.Sequences {
		dst = appendFlattenedSequenceFactors(dst, sequence)
	}
	return dst
}

func appendFlattenedSequenceFactors(dst []*Factor, sequence *Sequence) []*Factor {
	if sequence == nil {
		return dst
	}
	for _, factor := range sequence.Factors {
		dst = appendFlattenedFactor(dst, factor)
	}
	return dst
}

func appendFlattenedFactor(dst []*Factor, factor *Factor) []*Factor {
	if factor == nil {
		return dst
	}
	if len(factor.Terms) == 1 {
		term := factor.Terms[0]
		if term != nil && !term.Negated && term.Simple != nil &&
			term.Simple.Composite != nil && term.Simple.Restriction == nil {
			return appendFlattenedExpressionFactors(dst, term.Simple.Composite)
		}
	}
	return append(dst, factor)
}

func (w *whereClause) generateSQLFromFactors(factors []*Factor) (string, error) {
	return joinQueries(
		factors,
		" AND ",
		func(factor *Factor) (string, error) { return w.factorQuery(factor) },
	)
}

func (w *whereClause) factorQuery(factor *Factor) (string, error) {
	if inQuery, ok, err := w.tryFactorAsInQuery(factor); err != nil {
		return "", err
	} else if ok {
		return inQuery, nil
	}

	return joinQueries(
		factor.Terms,
		" OR ",
		func(term *Term) (string, error) { return w.termQuery(term) },
	)
}

func joinQueries[T any](
	items []T,
	separator string,
	query func(T) (string, error),
) (string, error) {
	var first string
	itemCount := 0
	var builder *strings.Builder

	for _, item := range items {
		part, err := query(item)
		if err != nil {
			return "", err
		}
		itemCount++
		switch itemCount {
		case 1:
			first = part
		case 2:
			builder = getBuilder()
			builder.Grow(len(first) + len(part) + len(separator) + 8)
			builder.WriteString("(")
			builder.WriteString(first)
			builder.WriteString(separator)
			builder.WriteString(part)
		default:
			builder.WriteString(separator)
			builder.WriteString(part)
		}
	}

	if itemCount == 0 {
		return "()", nil
	}
	if itemCount == 1 {
		return first, nil
	}
	builder.WriteString(")")
	result := builder.String()
	putBuilder(builder)
	return result, nil
}

func (w *whereClause) tryFactorAsInQuery(factor *Factor) (string, bool, error) {
	if factor == nil || len(factor.Terms) < 2 {
		return "", false, nil
	}

	var column *Column
	values := make([]string, 0, len(factor.Terms))
	for _, term := range factor.Terms {
		if term == nil || term.Negated || term.Simple == nil || term.Simple.Restriction == nil {
			return "", false, nil
		}
		restriction := term.Simple.Restriction
		if restriction.Comparator != "=" || restriction.Comparable == nil ||
			restriction.Comparable.Member == nil {
			return "", false, nil
		}
		if len(restriction.Comparable.Member.Fields) > 0 || restriction.Arg == nil {
			return "", false, nil
		}
		if restriction.Arg.Composite != nil || restriction.Arg.Comparable == nil ||
			restriction.Arg.Comparable.Member == nil {
			return "", false, nil
		}
		if len(restriction.Arg.Comparable.Member.Fields) > 0 {
			return "", false, nil
		}

		currentColumn, err := w.table.FilterableColumnByFieldPath(
			NewFieldPath(restriction.Comparable.Member.Value),
		)
		if err != nil || currentColumn.keyValue {
			return "", false, nil
		}
		if column == nil {
			column = currentColumn
		} else if column.databaseName != currentColumn.databaseName {
			return "", false, nil
		}

		value, err := w.comparableValue(restriction.Arg.Comparable, column)
		if err != nil {
			return "", false, fmt.Errorf(
				"argument for field %s: %w",
				column.fieldPath.String(),
				err,
			)
		}
		values = append(values, value)
	}

	if column == nil || len(values) < 2 {
		return "", false, nil
	}

	builder := getBuilder()
	builder.Grow(len(column.databaseName) + len(values)*8 + 16)
	builder.WriteString("(")
	builder.WriteString(column.databaseName)
	builder.WriteString(" IN (")
	for i, value := range values {
		if i > 0 {
			builder.WriteString(", ")
		}
		builder.WriteString(value)
	}
	builder.WriteString("))")
	result := builder.String()
	putBuilder(builder)
	return result, true, nil
}

func (w *whereClause) termQuery(term *Term) (string, error) {
	simpleQuery, err := w.simpleQuery(term.Simple)
	if err != nil {
		return "", err
	}
	if !term.Negated {
		return simpleQuery, nil
	}

	builder := getBuilder()
	builder.Grow(len(simpleQuery) + 6)
	builder.WriteString("(NOT ")
	builder.WriteString(simpleQuery)
	builder.WriteString(")")
	result := builder.String()
	putBuilder(builder)
	return result, nil
}

func (w *whereClause) simpleQuery(simple *Simple) (string, error) {
	if simple.Restriction != nil {
		return w.restrictionQuery(simple.Restriction)
	}
	if simple.Composite != nil {
		return w.expressionQuery(simple.Composite)
	}
	return "", fmt.Errorf("invalid 'simple' clause in query filter")
}

func (w *whereClause) restrictionQuery(restriction *Restriction) (string, error) {
	if restriction.Comparable.Member == nil {
		return "", fmt.Errorf("invalid comparable")
	}
	if restriction.Comparator == "" {
		if len(restriction.Comparable.Member.Fields) > 0 {
			value := restriction.Comparable.Member.Value
			fields := strings.Join(restriction.Comparable.Member.Fields, ".")
			return "", fmt.Errorf(
				"fields are not allowed without an operator, try wrapping %s.%s in double quotes: \"%s.%s\"",
				value,
				fields,
				value,
				fields,
			)
		}
		if !w.optimizeMatch {
			return w.implicitRestrictionLikeQuery(restriction.Comparable)
		}
		return w.implicitRestrictionMatchQuery(restriction.Comparable)
	}

	column, err := w.table.FilterableColumnByFieldPath(
		NewFieldPath(restriction.Comparable.Member.Value),
	)
	if err != nil {
		return "", err
	}
	if len(restriction.Comparable.Member.Fields) > 0 {
		return w.restrictionQueryWithFields(restriction, column)
	}
	if column.keyValue {
		return "", fmt.Errorf(
			"key value columns must specify the key to search on.  Instead of '%s%s' try '%s.key%s'",
			column.fieldPath.String(),
			restriction.Comparator,
			column.fieldPath.String(),
			restriction.Comparator,
		)
	}

	return w.restrictionQueryForColumn(restriction, column)
}

func (w *whereClause) restrictionQueryWithFields(
	restriction *Restriction,
	column *Column,
) (string, error) {
	if !column.keyValue {
		return "", fmt.Errorf(
			"fields are only supported for key value columns.  Try removing the '.' from after your column named %q",
			column.fieldPath.String(),
		)
	}
	if len(restriction.Comparable.Member.Fields) > 1 {
		return "", fmt.Errorf(
			"expected only a single '.' in keyvalue column named %q",
			column.fieldPath.String(),
		)
	}

	key := w.bind(restriction.Comparable.Member.Fields[0])
	if restriction.Comparator == ":" {
		keyValueClause, err := w.keyValueMatchClause(column, restriction.Arg)
		if err != nil {
			return "", fmt.Errorf("argument for field %s: %w", column.fieldPath.String(), err)
		}
		return buildExistsUnnestClause(column.databaseName, key, keyValueClause), nil
	}

	value, err := w.argValue(restriction.Arg, column)
	if err != nil {
		return "", fmt.Errorf("argument for field %s: %w", column.fieldPath.String(), err)
	}

	switch restriction.Comparator {
	case "=":
		return buildExistsUnnestClause(column.databaseName, key, "value = "+value), nil
	case "!=":
		return buildExistsUnnestClause(column.databaseName, key, "value <> "+value), nil
	default:
		return "", fmt.Errorf("comparator operator not implemented for fields yet")
	}
}

func buildExistsUnnestClause(columnName, key, valueClause string) string {
	builder := getBuilder()
	builder.Grow(len(columnName) + len(key) + len(valueClause) + 68)
	builder.WriteString("(EXISTS (SELECT key, value FROM UNNEST(")
	builder.WriteString(columnName)
	builder.WriteString(") WHERE key = ")
	builder.WriteString(key)
	builder.WriteString(" AND ")
	builder.WriteString(valueClause)
	builder.WriteString("))")
	result := builder.String()
	putBuilder(builder)
	return result
}

func (w *whereClause) restrictionQueryForColumn(
	restriction *Restriction,
	column *Column,
) (string, error) {
	switch restriction.Comparator {
	case "=":
		return w.comparisonRestriction(column, "=", restriction.Arg)
	case "!=":
		return w.comparisonRestriction(column, "<>", restriction.Arg)
	case ">", "<", ">=", "<=":
		if column.columnType == ColumnTypeBool {
			return "", fmt.Errorf(
				"comparator %q is not supported for boolean field %q",
				restriction.Comparator,
				column.fieldPath.String(),
			)
		}
		return w.comparisonRestriction(column, restriction.Comparator, restriction.Arg)
	case ":":
		if err := validateHasOperator(column); err != nil {
			return "", err
		}
		matchClause, err := w.columnMatchClause(column, restriction.Arg)
		if err != nil {
			return "", fmt.Errorf("argument for field %s: %w", column.fieldPath.String(), err)
		}
		return matchClause, nil
	default:
		return "", fmt.Errorf("comparator operator not implemented yet")
	}
}

func (w *whereClause) comparisonRestriction(
	column *Column,
	operator string,
	arg *Arg,
) (string, error) {
	value, err := w.argValue(arg, column)
	if err != nil {
		return "", fmt.Errorf("argument for field %s: %w", column.fieldPath.String(), err)
	}
	return comparisonClause(column.databaseName, operator, value), nil
}

func (w *whereClause) implicitRestrictionLikeQuery(comparable *Comparable) (string, error) {
	arg, err := w.likeComparableValue(comparable)
	if err != nil {
		return "", err
	}
	return w.joinImplicitRestrictionColumns(func(column *Column) (string, error) {
		return column.databaseName + " LIKE " + arg, nil
	})
}

func (w *whereClause) implicitRestrictionMatchQuery(comparable *Comparable) (string, error) {
	return w.joinImplicitRestrictionColumns(func(column *Column) (string, error) {
		return w.implicitRestrictionQuery(column, comparable)
	})
}

func (w *whereClause) joinImplicitRestrictionColumns(
	clauseForColumn func(column *Column) (string, error),
) (string, error) {
	implicitColumns := w.table.implicitFilterColumns
	if len(implicitColumns) == 0 {
		return "()", nil
	}
	if len(implicitColumns) == 1 {
		return clauseForColumn(implicitColumns[0])
	}

	builder := getBuilder()
	builder.Grow(len(implicitColumns) * 24)
	builder.WriteString("(")
	for i, column := range implicitColumns {
		if i > 0 {
			builder.WriteString(" OR ")
		}
		clause, err := clauseForColumn(column)
		if err != nil {
			putBuilder(builder)
			return "", err
		}
		builder.WriteString(clause)
	}
	builder.WriteString(")")
	result := builder.String()
	putBuilder(builder)
	return result, nil
}

func comparisonClause(lhs, operator, rhs string) string {
	builder := getBuilder()
	builder.Grow(len(lhs) + len(operator) + len(rhs) + 6)
	builder.WriteString("(")
	builder.WriteString(lhs)
	builder.WriteString(" ")
	builder.WriteString(operator)
	builder.WriteString(" ")
	builder.WriteString(rhs)
	builder.WriteString(")")
	result := builder.String()
	putBuilder(builder)
	return result
}

func (w *whereClause) implicitRestrictionQuery(
	column *Column,
	comparable *Comparable,
) (string, error) {
	if err := validateHasOperator(column); err != nil {
		return "", err
	}
	return w.matchClause(column, &Arg{Comparable: comparable}, column.databaseName, true)
}

func (w *whereClause) columnMatchClause(column *Column, arg *Arg) (string, error) {
	clause, err := w.matchClause(column, arg, column.databaseName, true)
	if err != nil {
		return "", err
	}
	return "(" + clause + ")", nil
}

func (w *whereClause) keyValueMatchClause(column *Column, arg *Arg) (string, error) {
	return w.matchClause(column, arg, "value", false)
}

func (w *whereClause) matchClause(
	column *Column,
	arg *Arg,
	lhs string,
	allowFullText bool,
) (string, error) {
	mode, err := w.selectMatchMode(column, allowFullText)
	if err != nil {
		return "", err
	}

	switch mode {
	case MatchModeExact:
		value, err := w.argValue(arg, column)
		if err != nil {
			return "", err
		}
		return lhs + " = " + value, nil
	case MatchModePrefix:
		value, err := w.prefixArgValue(arg, column)
		if err != nil {
			return "", err
		}
		return lhs + " LIKE " + value, nil
	case MatchModeContains:
		value, err := w.likeArgValue(arg, column)
		if err != nil {
			return "", err
		}
		return lhs + " LIKE " + value, nil
	case MatchModeFullText:
		value, err := w.fullTextArgValue(arg, column)
		if err != nil {
			return "", err
		}
		switch w.dialect {
		case SQLDialectPostgres:
			return "to_tsvector('simple', " + lhs + ") @@ websearch_to_tsquery('simple', " + value + ")", nil
		case SQLDialectMySQL:
			return "MATCH(" + lhs + ") AGAINST (" + value + " IN BOOLEAN MODE)", nil
		default:
			return "", fmt.Errorf("fulltext match mode requires postgres or mysql dialect")
		}
	default:
		return "", fmt.Errorf("unsupported match mode %q", mode)
	}
}

func (w *whereClause) selectMatchMode(column *Column, allowFullText bool) (MatchMode, error) {
	if !w.optimizeMatch {
		return w.fallbackMode, nil
	}
	if column.columnType != ColumnTypeString {
		return "", fmt.Errorf(
			"cannot use has (:) operator on a non-string field %q",
			column.columnType.String(),
		)
	}

	modes := column.matchModes
	if len(modes) == 0 {
		if w.strictMode {
			return "", fmt.Errorf(
				"no match mode configured for field %q",
				column.fieldPath.String(),
			)
		}
		return w.fallbackMode, nil
	}
	for _, mode := range modes {
		if !isValidMatchMode(mode) {
			return "", fmt.Errorf(
				"invalid match mode %q on field %q",
				mode,
				column.fieldPath.String(),
			)
		}
		if w.matchModeSupported(mode, allowFullText) {
			return mode, nil
		}
	}
	if w.strictMode {
		return "", fmt.Errorf(
			"no supported match mode for field %q with dialect %q",
			column.fieldPath.String(),
			w.dialect,
		)
	}
	return w.fallbackMode, nil
}

func (w *whereClause) matchModeSupported(mode MatchMode, allowFullText bool) bool {
	if mode == MatchModeFullText && !allowFullText {
		return false
	}
	return isSupported(mode, w.dialect)
}

func (w *whereClause) argValue(arg *Arg, column *Column) (string, error) {
	if arg.Composite != nil {
		return "", fmt.Errorf("composite expressions in arguments not implemented yet")
	}
	if arg.Comparable == nil {
		return "", fmt.Errorf("missing comparable in argument")
	}
	return w.comparableValue(arg.Comparable, column)
}

func (w *whereClause) comparableValue(comparable *Comparable, column *Column) (string, error) {
	if comparable.Member == nil {
		return "", fmt.Errorf("invalid comparable")
	}
	if len(comparable.Member.Fields) > 0 {
		return "", fmt.Errorf("fields not implemented yet")
	}

	switch column.columnType {
	case ColumnTypeString:
		value := comparable.Member.Value
		if column.argSubstitute != nil {
			value = column.argSubstitute(value)
		}
		return w.bind(value), nil
	case ColumnTypeBool:
		if strings.EqualFold(comparable.Member.Value, "true") {
			return "TRUE", nil
		}
		if strings.EqualFold(comparable.Member.Value, "false") {
			return "FALSE", nil
		}
		return "", fmt.Errorf(
			"only TRUE or FALSE can be specified as the value for a boolean field",
		)
	default:
		return "", fmt.Errorf(
			"unable to generate SQL value for unknown field type: %s",
			column.columnType.String(),
		)
	}
}

func (w *whereClause) likeArgValue(arg *Arg, column *Column) (string, error) {
	if err := validateMatchArgument(arg, column); err != nil {
		return "", err
	}
	return w.likeComparableValue(arg.Comparable)
}

func (w *whereClause) prefixArgValue(arg *Arg, column *Column) (string, error) {
	if err := validateMatchArgument(arg, column); err != nil {
		return "", err
	}
	return w.prefixComparableValue(arg.Comparable)
}

func (w *whereClause) fullTextArgValue(arg *Arg, column *Column) (string, error) {
	if err := validateMatchArgument(arg, column); err != nil {
		return "", err
	}
	if arg.Comparable.Member == nil {
		return "", fmt.Errorf("invalid comparable")
	}
	if len(arg.Comparable.Member.Fields) > 0 {
		return "", fmt.Errorf("fields are not allowed on the RHS of has (:) operator")
	}
	return w.bind(arg.Comparable.Member.Value), nil
}

func validateMatchArgument(arg *Arg, column *Column) error {
	if arg.Composite != nil {
		return fmt.Errorf("composite expressions are not allowed as RHS to has (:) operator")
	}
	if arg.Comparable == nil {
		return fmt.Errorf("missing comparable in argument")
	}
	if column.columnType != ColumnTypeString {
		return fmt.Errorf(
			"cannot use has (:) operator on a non-string field %q",
			column.columnType.String(),
		)
	}
	return nil
}

func (w *whereClause) likeComparableValue(comparable *Comparable) (string, error) {
	if comparable.Member == nil {
		return "", fmt.Errorf("invalid comparable")
	}
	if len(comparable.Member.Fields) > 0 {
		return "", fmt.Errorf("fields are not allowed on the RHS of has (:) operator")
	}
	return w.bind("%" + QuoteLike(comparable.Member.Value) + "%"), nil
}

func (w *whereClause) prefixComparableValue(comparable *Comparable) (string, error) {
	if comparable.Member == nil {
		return "", fmt.Errorf("invalid comparable")
	}
	if len(comparable.Member.Fields) > 0 {
		return "", fmt.Errorf("fields are not allowed on the RHS of has (:) operator")
	}
	return w.bind(QuoteLike(comparable.Member.Value) + "%"), nil
}

func (w *whereClause) bind(value string) string {
	name := w.namePrefix + strconv.Itoa(w.nextValueName)
	w.nextValueName++
	w.parameters = append(w.parameters, QueryParameter{Name: name, Value: value})
	return "@" + name
}
