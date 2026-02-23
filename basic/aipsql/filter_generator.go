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
	"sort"
	"strconv"
	"strings"
	"sync"
)

// builderPool is a sync.Pool for reusing strings.Builder objects to reduce memory allocations.
// Builders are reset before being returned to the pool to ensure clean state.
var builderPool = sync.Pool{
	New: func() interface{} {
		return &strings.Builder{}
	},
}

// getBuilder retrieves a strings.Builder from the pool.
func getBuilder() *strings.Builder {
	return builderPool.Get().(*strings.Builder)
}

// putBuilder resets and returns a strings.Builder to the pool for reuse.
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
// It contains the parameter name (e.g., "@p1", "@seek_eq_0") and the parameter value.
// The Value field uses interface{} to support any type, allowing the database driver
// to handle type conversion appropriately.
//
// All user input in generated SQL queries is passed through QueryParameters to ensure
// SQL injection safety. Values are never directly concatenated into SQL strings.
//
// Example usage:
//
//	sql, params, err := table.WhereClause(filter, "p")
//	// sql: "status = @p1 AND user_id = @p2"
//	// params: [
//	//     {Name: "@p1", Value: "active"},
//	//     {Name: "@p2", Value: 123},
//	// ]
//
//	// Execute with your database driver
//	rows, err := db.Query(sql, paramsToArgs(params)...)
//
// Parameter naming conventions:
//   - Regular parameters: @p1, @p2, @p3, ...
//   - Seek equality: @seek_eq_0, @seek_eq_1, ...
//   - Seek comparison: @seek_cmp_0, @seek_cmp_1, ...
//   - Key-value: @kv_key_1, @kv_value_1, ...
type QueryParameter struct {
	Name  string      // Parameter name, e.g., "@p1"
	Value interface{} // Parameter value, keeps original type for proper database binding
}

// WhereClauseOptions controls SQL generation behavior for has (:) filters.
//
// This structure provides fine-grained control over how AIP-160 filter expressions
// are translated to SQL, including dialect selection, match mode behavior, and
// query optimization features.
//
// Example usage:
//
//	// Basic usage with PostgreSQL full-text search
//	opts := &WhereClauseOptions{
//	    Dialect: SQLDialectPostgres,
//	}
//	sql, params, err := table.WhereClauseWithOptions(filter, "p", opts)
//
//	// Strict mode: fail if no supported match mode is available
//	opts := &WhereClauseOptions{
//	    Dialect:    SQLDialectGeneric,
//	    StrictMode: true,  // Don't fall back to contains mode
//	}
//
//	// Enable composite index optimization for better performance
//	opts := &WhereClauseOptions{
//	    Dialect:                          SQLDialectPostgres,
//	    EnableCompositeIndexOptimization: true,
//	}
//
// Default values (when nil or zero values):
//   - Dialect: SQLDialectGeneric
//   - StrictMode: false
//   - EnableCompositeIndexOptimization: false
type WhereClauseOptions struct {
	// Dialect selects dialect-specific SQL fragments (for example full text predicates).
	// Default: "generic" for maximum compatibility.
	//
	// Supported values:
	//   - SQLDialectGeneric: Standard SQL (default)
	//   - SQLDialectPostgres: PostgreSQL-specific features
	//   - SQLDialectMySQL: MySQL-specific features
	//
	// The dialect affects:
	//   - Full-text search syntax (only available for postgres/mysql)
	//   - SQL function availability
	//   - Match mode support
	Dialect SQLDialect

	// StrictMode disables fallback behavior when configured match modes are unsupported.
	// Default: false for backward compatibility.
	//
	// When false (default):
	//   - If no configured match mode is supported, falls back to MatchModeContains
	//   - Provides maximum compatibility at the cost of potential performance
	//
	// When true:
	//   - If no configured match mode is supported, returns an error
	//   - Ensures you're aware when index-friendly modes aren't being used
	//   - Recommended for production systems with well-defined schemas
	//
	// Example:
	//   Column configured with MatchModeFullText only
	//   Dialect: SQLDialectGeneric
	//   StrictMode false: Falls back to MatchModeContains (slow)
	//   StrictMode true: Returns error (forces you to fix the configuration)
	StrictMode bool

	// EnableCompositeIndexOptimization enables reordering of WHERE conditions to match
	// composite index column order for better index utilization.
	// Default: false for backward compatibility.
	//
	// When true:
	//   - Analyzes table's CompositeIndexes configuration
	//   - Selects the best matching index for the query
	//   - Reorders WHERE conditions to match index column order
	//   - Places equality conditions before range conditions
	//   - Can improve query performance by 10x-100x on large tables
	//
	// When false:
	//   - Preserves original condition order from the filter expression
	//   - No performance optimization applied
	//
	// Important notes:
	//   - Only reorders AND-connected conditions (preserves semantics)
	//   - OR-connected conditions are never reordered
	//   - Requires table.CompositeIndexes to be configured
	//   - Adds ~20% overhead to SQL generation time
	//
	// Example:
	//   Original filter: "user_id=123 AND created_at>'2024-01-01' AND status='active'"
	//   Index: (status, user_id, created_at)
	//   Optimized: "status='active' AND user_id=123 AND created_at>'2024-01-01'"
	//   Result: Database can use the full composite index efficiently
	EnableCompositeIndexOptimization bool
}

// isSupported checks if a match mode is supported by the given SQL dialect.
// Returns true if the mode is supported, false otherwise.
//
// Supported modes:
//   - exact, prefix, contains: Always supported (all dialects)
//   - fulltext: Only supported for "postgres" and "mysql" dialects
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

// WhereClause creates a Standard SQL WHERE clause fragment for the given filter.
//
// The fragment will be enclosed in parentheses and does not include the "WHERE" keyword.
// For example: (column LIKE @param1)
// Also returns the query parameters which need to be given to the database.
//
// All field names are replaced with the safe database column names from the specified table.
// All user input strings are passed via query parameters, so the returned query is SQL injection safe.
//
// This method uses default options for backward compatibility:
//   - Dialect: SQLDialectGeneric
//   - StrictMode: false (allows fallback to contains mode)
//   - EnableCompositeIndexOptimization: false
//
// For advanced features like full-text search or composite index optimization,
// use WhereClauseWithOptions instead.
//
// Example usage:
//
//	filter, err := ParseFilter("status:\"active\" AND user_id=123")
//	sql, params, err := table.WhereClause(filter, "p")
//	// sql: "(status LIKE @p1 AND user_id = @p2)"
//	// params: [{Name: "@p1", Value: "%active%"}, {Name: "@p2", Value: 123}]
//
//	// Execute query
//	query := "SELECT * FROM orders WHERE " + sql
//	rows, err := db.Query(query, paramsToArgs(params)...)
//
// Parameters:
//   - filter: Parsed AIP-160 filter expression
//   - parameterPrefix: Prefix for parameter names (e.g., "p" generates @p1, @p2, ...)
//
// Returns:
//   - SQL WHERE clause fragment (with parentheses, without "WHERE" keyword)
//   - Query parameters for safe value binding
//   - Error if filter is invalid or column references are incorrect
func (t *Table) WhereClause(
	filter *Filter,
	parameterPrefix string,
) (string, []QueryParameter, error) {
	return t.whereClause(filter, parameterPrefix, WhereClauseOptions{}, false)
}

// WhereClauseWithOptions creates a Standard SQL WHERE clause fragment while applying
// dialect-aware, index-friendly match mode selection.
//
// This method provides advanced control over SQL generation including:
//   - SQL dialect selection (generic, postgres, mysql)
//   - Match mode behavior (strict vs. fallback)
//   - Composite index optimization
//
// Example usage:
//
//	// PostgreSQL with full-text search
//	opts := &WhereClauseOptions{
//	    Dialect: SQLDialectPostgres,
//	}
//	filter, _ := ParseFilter("content:\"machine learning\"")
//	sql, params, err := table.WhereClauseWithOptions(filter, "p", opts)
//	// sql: "(to_tsvector('simple', content) @@ websearch_to_tsquery('simple', @p1))"
//
//	// Strict mode with composite index optimization
//	opts := &WhereClauseOptions{
//	    Dialect:                          SQLDialectPostgres,
//	    StrictMode:                       true,
//	    EnableCompositeIndexOptimization: true,
//	}
//	filter, _ := ParseFilter("user_id=123 AND created_at>'2024-01-01' AND status='active'")
//	sql, params, err := table.WhereClauseWithOptions(filter, "p", opts)
//	// Conditions reordered to match composite index (status, user_id, created_at)
//	// sql: "(status = @p3 AND user_id = @p1 AND created_at > @p2)"
//
// Match mode selection:
//   - Tries each configured match mode in order
//   - Uses the first mode supported by the dialect
//   - Falls back to contains mode if StrictMode is false
//   - Returns error if no mode is supported and StrictMode is true
//
// Composite index optimization:
//   - Analyzes table.CompositeIndexes configuration
//   - Selects the best matching index based on query conditions
//   - Reorders AND-connected conditions to match index column order
//   - Places equality conditions before range conditions
//   - Can improve performance by 10x-100x on large tables
//
// Parameters:
//   - filter: Parsed AIP-160 filter expression
//   - parameterPrefix: Prefix for parameter names (e.g., "p" generates @p1, @p2, ...)
//   - options: Configuration for SQL generation behavior
//
// Returns:
//   - SQL WHERE clause fragment (with parentheses, without "WHERE" keyword)
//   - Query parameters for safe value binding
//   - Error if filter is invalid, column references are incorrect, or no supported match mode is found in strict mode
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

	q := &whereClause{
		table: t,
		parameters: make(
			[]QueryParameter,
			0,
			32,
		), // Pre-allocate for typical queries
		namePrefix:                       parameterPrefix,
		dialect:                          dialect,
		strictMode:                       options.StrictMode,
		fallbackMode:                     MatchModeContains,
		optimizeMatch:                    optimizeMatch,
		enableCompositeIndexOptimization: options.EnableCompositeIndexOptimization,
	}

	clause, err := q.expressionQuery(filter.Expression)
	if err != nil {
		return "", []QueryParameter{}, err
	}
	return clause, q.parameters, nil
}

// expressionQuery returns the SQL expression equivalent to the given
// filter expression.
// An expression is a conjunction (AND) of sequences or a simple
// sequence.
//
// The returned string is an injection-safe SQL expression.
func (w *whereClause) expressionQuery(expression *Expression) (string, error) {
	// Both Sequence and Factor is equivalent to AND of the
	// component Sequences and Factors (respectively), as we implement
	// exact match semantics and do not support ranking
	// based on the number of factors that match.

	// Flatten nested composites that are pure conjunctions so equivalent EBNF
	// styles can be optimized consistently.
	factors := flattenExpressionFactors(expression)

	if len(factors) == 0 {
		return "()", nil
	}

	// If composite index optimization is enabled and we have multiple factors,
	// try to reorder them
	if w.enableCompositeIndexOptimization && len(factors) > 1 && len(w.table.CompositeIndexes) > 0 {
		// Extract conditions from factors
		conditions, factorConditionIndexes, err := w.extractConditionsFromFactors(factors)
		if err != nil {
			// If extraction fails, fall back to normal processing
			return w.generateSQLFromFactors(factors)
		}

		// Find the best composite index
		bestIndex := findBestCompositeIndex(conditions, w.table.CompositeIndexes)

		// Reorder conditions if we found a suitable index
		if bestIndex != nil {
			reorderedConditions := reorderConditions(conditions, bestIndex)
			if !sameConditionOrder(conditions, reorderedConditions) {
				// Reorder factors based on the reordered conditions
				factors = w.reorderFactors(
					factors,
					conditions,
					factorConditionIndexes,
					reorderedConditions,
				)
			}
		}
	}

	return w.generateSQLFromFactors(factors)
}

func flattenExpressionFactors(expression *Expression) []*Factor {
	if expression == nil {
		return nil
	}

	// Pre-allocate using top-level factors as a lower bound.
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

	// Only flatten parenthesized terms that are guaranteed to be conjunctive:
	// a single, non-negated term containing a composite expression.
	if len(factor.Terms) == 1 {
		term := factor.Terms[0]
		if term != nil && !term.Negated && term.Simple != nil &&
			term.Simple.Composite != nil && term.Simple.Restriction == nil {
			return appendFlattenedExpressionFactors(dst, term.Simple.Composite)
		}
	}

	return append(dst, factor)
}

// generateSQLFromFactors generates SQL from a list of factors (AND-connected)
func (w *whereClause) generateSQLFromFactors(factors []*Factor) (string, error) {
	var firstFactor string
	factorCount := 0
	var b *strings.Builder

	for _, factor := range factors {
		f, err := w.factorQuery(factor)
		if err != nil {
			return "", err
		}
		factorCount++
		switch factorCount {
		case 1:
			firstFactor = f
		case 2:
			b = getBuilder()
			b.Grow(len(firstFactor) + len(f) + 16)
			b.WriteString("(")
			b.WriteString(firstFactor)
			b.WriteString(" AND ")
			b.WriteString(f)
		default:
			b.WriteString(" AND ")
			b.WriteString(f)
		}
	}

	if factorCount == 0 {
		return "()", nil
	}
	if factorCount == 1 {
		return firstFactor, nil
	}
	b.WriteString(")")
	result := b.String()
	putBuilder(b)
	return result, nil
}

// extractConditionsFromFactors extracts Condition metadata from factors
// Returns:
// - conditions: list of all conditions
// - factorConditions: map from factor index to its condition (nil if factor is not a simple condition)
// - error: if extraction fails
func (w *whereClause) extractConditionsFromFactors(
	factors []*Factor,
) ([]Condition, map[int]int, error) {
	// Pre-allocate capacity based on number of factors (upper bound)
	conditions := make([]Condition, 0, len(factors))
	factorConditionIndexes := make(map[int]int, len(factors))

	for i, factor := range factors {
		// Only extract from simple factors (single term, not negated, simple restriction)
		if len(factor.Terms) != 1 {
			continue
		}
		term := factor.Terms[0]
		if term.Negated || term.Simple == nil || term.Simple.Restriction == nil {
			continue
		}
		restriction := term.Simple.Restriction

		// Extract column
		if restriction.Comparable == nil || restriction.Comparable.Member == nil {
			continue
		}
		if len(restriction.Comparable.Member.Fields) > 0 {
			continue // Skip nested fields for now
		}

		column, err := w.table.FilterableColumnByFieldPath(
			NewFieldPath(restriction.Comparable.Member.Value),
		)
		if err != nil {
			continue
		}
		if column.keyValue {
			continue
		}

		var isEquality bool
		switch restriction.Comparator {
		case "=":
			isEquality = true
		case ">", "<", ">=", "<=":
			isEquality = false
		case ":":
			if err := validateHasOperator(column); err != nil {
				continue
			}
			mode, err := w.selectMatchMode(column, true)
			if err != nil {
				continue
			}
			isEquality = mode == MatchModeExact
		default:
			continue
		}

		condition := Condition{
			Column:     column,
			IsEquality: isEquality,
		}

		conditionIndex := len(conditions)
		conditions = append(conditions, condition)
		factorConditionIndexes[i] = conditionIndex
	}

	return conditions, factorConditionIndexes, nil
}

// reorderFactors reorders factors based on the reordered conditions
func (w *whereClause) reorderFactors(
	factors []*Factor,
	conditions []Condition,
	factorConditionIndexes map[int]int,
	reorderedConditions []Condition,
) []*Factor {
	conditionPositions := make(map[int]int, len(conditions))
	usedReordered := make([]bool, len(reorderedConditions))
	for originalIndex, condition := range conditions {
		for reorderedIndex, reorderedCondition := range reorderedConditions {
			if usedReordered[reorderedIndex] {
				continue
			}
			if condition.Column.databaseName == reorderedCondition.Column.databaseName &&
				condition.IsEquality == reorderedCondition.IsEquality {
				conditionPositions[originalIndex] = reorderedIndex
				usedReordered[reorderedIndex] = true
				break
			}
		}
	}

	// Create a list of (factor, position) pairs
	type factorWithPosition struct {
		factor   *Factor
		position int
	}
	// Pre-allocate capacity for all factors
	factorsWithPositions := make([]factorWithPosition, 0, len(factors))

	for i, factor := range factors {
		if conditionIndex, ok := factorConditionIndexes[i]; ok {
			if position, hasPosition := conditionPositions[conditionIndex]; hasPosition {
				factorsWithPositions = append(
					factorsWithPositions,
					factorWithPosition{factor: factor, position: position},
				)
				continue
			}
		}
		// Factors without a matched condition are placed at the end.
		factorsWithPositions = append(
			factorsWithPositions,
			factorWithPosition{factor: factor, position: len(reorderedConditions) + i},
		)
	}

	// Sort factors by position using sort.Slice for O(n log n) performance
	sort.SliceStable(factorsWithPositions, func(i, j int) bool {
		return factorsWithPositions[i].position < factorsWithPositions[j].position
	})

	// Extract the reordered factors
	reorderedFactors := make([]*Factor, len(factors))
	for i, fwp := range factorsWithPositions {
		reorderedFactors[i] = fwp.factor
	}

	return reorderedFactors
}

// factorQuery returns the SQL expression equivalent to the given
// factor. A factor is a disjunction (OR) of terms or a simple term.
//
// The returned string is an injection-safe SQL expression.
func (w *whereClause) factorQuery(factor *Factor) (string, error) {
	if inQuery, ok, err := w.tryFactorAsInQuery(factor); err != nil {
		return "", err
	} else if ok {
		return inQuery, nil
	}

	var firstTerm string
	termCount := 0
	var b *strings.Builder
	for _, term := range factor.Terms {
		tq, err := w.termQuery(term)
		if err != nil {
			return "", err
		}
		termCount++
		switch termCount {
		case 1:
			firstTerm = tq
		case 2:
			b = getBuilder()
			b.Grow(len(firstTerm) + len(tq) + 16)
			b.WriteString("(")
			b.WriteString(firstTerm)
			b.WriteString(" OR ")
			b.WriteString(tq)
		default:
			b.WriteString(" OR ")
			b.WriteString(tq)
		}
	}
	if termCount == 0 {
		return "()", nil
	}
	if termCount == 1 {
		return firstTerm, nil
	}
	b.WriteString(")")
	result := b.String()
	putBuilder(b)
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

		c, err := w.table.FilterableColumnByFieldPath(
			NewFieldPath(restriction.Comparable.Member.Value),
		)
		if err != nil {
			return "", false, nil
		}
		if c.keyValue {
			return "", false, nil
		}
		if column == nil {
			column = c
		} else if column.databaseName != c.databaseName {
			return "", false, nil
		}

		value, err := w.comparableValue(restriction.Arg.Comparable, column)
		if err != nil {
			return "", false, fmt.Errorf("argument for field %s: %w",
				column.fieldPath.String(),
				err,
			)
		}
		values = append(values, value)
	}

	if column == nil || len(values) < 2 {
		return "", false, nil
	}

	b := getBuilder()
	b.Grow(len(column.databaseName) + len(values)*8 + 16)
	b.WriteString("(")
	b.WriteString(column.databaseName)
	b.WriteString(" IN (")
	for i, value := range values {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(value)
	}
	b.WriteString("))")
	result := b.String()
	putBuilder(b)
	return result, true, nil
}

// termQuery returns the SQL expression equivalent to the given
// term.
//
// The returned string is an injection-safe SQL expression.
func (w *whereClause) termQuery(term *Term) (string, error) {
	simpleQuery, err := w.simpleQuery(term.Simple)
	if err != nil {
		return "", err
	}
	if term.Negated {
		b := getBuilder()
		b.Grow(len(simpleQuery) + 6) // "(NOT )" = 6 chars
		b.WriteString("(NOT ")
		b.WriteString(simpleQuery)
		b.WriteString(")")
		result := b.String()
		putBuilder(b)
		return result, nil
	}
	return simpleQuery, nil
}

// simpleQuery returns the SQL expression equivalent to the given simple
// filter.
// The returned string is an injection-safe SQL expression.
func (w *whereClause) simpleQuery(simple *Simple) (string, error) {
	if simple.Restriction != nil {
		return w.restrictionQuery(simple.Restriction)
	}
	if simple.Composite != nil {
		return w.expressionQuery(simple.Composite)
	}
	return "", fmt.Errorf("invalid 'simple' clause in query filter")
}

// restrictionQuery returns the SQL expression equivalent to the given
// restriction.
// The returned string is an injection-safe SQL expression.
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
				return "", fmt.Errorf("argument for field %s: %w",
					column.fieldPath.String(),
					err,
				)
			}
			// Use strings.Builder for complex SQL
			b := getBuilder()
			b.Grow(len(column.databaseName) + len(key) + len(keyValueClause) + 68)
			b.WriteString("(EXISTS (SELECT key, value FROM UNNEST(")
			b.WriteString(column.databaseName)
			b.WriteString(") WHERE key = ")
			b.WriteString(key)
			b.WriteString(" AND ")
			b.WriteString(keyValueClause)
			b.WriteString("))")
			result := b.String()
			putBuilder(b)
			return result, nil
		}
		value, err := w.argValue(restriction.Arg, column)
		if err != nil {
			return "", fmt.Errorf("argument for field %s: %w",
				column.fieldPath.String(),
				err,
			)
		}
		switch restriction.Comparator {
		case "=":
			b := getBuilder()
			b.Grow(len(column.databaseName) + len(key) + len(value) + 58)
			b.WriteString("(EXISTS (SELECT key, value FROM UNNEST(")
			b.WriteString(column.databaseName)
			b.WriteString(") WHERE key = ")
			b.WriteString(key)
			b.WriteString(" AND value = ")
			b.WriteString(value)
			b.WriteString("))")
			result := b.String()
			putBuilder(b)
			return result, nil
		case "!=":
			b := getBuilder()
			b.Grow(len(column.databaseName) + len(key) + len(value) + 59)
			b.WriteString("(EXISTS (SELECT key, value FROM UNNEST(")
			b.WriteString(column.databaseName)
			b.WriteString(") WHERE key = ")
			b.WriteString(key)
			b.WriteString(" AND value <> ")
			b.WriteString(value)
			b.WriteString("))")
			result := b.String()
			putBuilder(b)
			return result, nil
		}
		return "", fmt.Errorf("comparator operator not implemented for fields yet")
	} else if column.keyValue {
		// Key-value columns require explicit key specification.
		// While AIP-160 allows has operator on maps to check for key presence,
		// this implementation requires explicit key specification for clarity and
		// to avoid ambiguous queries.
		// nolint: lll
		return "", fmt.Errorf("key value columns must specify the key to search on.  Instead of '%s%s' try '%s.key%s'", column.fieldPath.String(), restriction.Comparator, column.fieldPath.String(), restriction.Comparator)
	}
	switch restriction.Comparator {
	case "=":
		arg, err := w.argValue(restriction.Arg, column)
		if err != nil {
			return "", fmt.Errorf("argument for field %s: %w",
				column.fieldPath.String(),
				err,
			)
		}
		return comparisonClause(column.databaseName, "=", arg), nil
	case "!=":
		arg, err := w.argValue(restriction.Arg, column)
		if err != nil {
			return "", fmt.Errorf("argument for field %s: %w",
				column.fieldPath.String(),
				err,
			)
		}
		return comparisonClause(column.databaseName, "<>", arg), nil
	case ">", "<", ">=", "<=":
		if column.columnType == ColumnTypeBool {
			return "", fmt.Errorf(
				"comparator %q is not supported for boolean field %q",
				restriction.Comparator,
				column.fieldPath.String(),
			)
		}
		arg, err := w.argValue(restriction.Arg, column)
		if err != nil {
			return "", fmt.Errorf("argument for field %s: %w",
				column.fieldPath.String(),
				err,
			)
		}
		return comparisonClause(column.databaseName, restriction.Comparator, arg), nil
	case ":":
		// Validate that the has operator can be used on this column
		if err := validateHasOperator(column); err != nil {
			return "", err
		}
		matchClause, err := w.columnMatchClause(column, restriction.Arg)
		if err != nil {
			return "", fmt.Errorf("argument for field %s: %w",
				column.fieldPath.String(),
				err,
			)
		}
		return matchClause, nil
	default:
		return "", fmt.Errorf("comparator operator not implemented yet")
	}
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

	b := getBuilder()
	b.Grow(len(implicitColumns) * 24)
	b.WriteString("(")
	for i, column := range implicitColumns {
		if i > 0 {
			b.WriteString(" OR ")
		}
		clause, err := clauseForColumn(column)
		if err != nil {
			putBuilder(b)
			return "", err
		}
		b.WriteString(clause)
	}
	b.WriteString(")")
	result := b.String()
	putBuilder(b)
	return result, nil
}

func comparisonClause(lhs, operator, rhs string) string {
	b := getBuilder()
	b.Grow(len(lhs) + len(operator) + len(rhs) + 6)
	b.WriteString("(")
	b.WriteString(lhs)
	b.WriteString(" ")
	b.WriteString(operator)
	b.WriteString(" ")
	b.WriteString(rhs)
	b.WriteString(")")
	result := b.String()
	putBuilder(b)
	return result
}

func (w *whereClause) implicitRestrictionQuery(
	column *Column,
	comparable *Comparable,
) (string, error) {
	// Validate that the has operator can be used on this column
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

// argValue returns a SQL expression representing the value of the specified
// arg.
// The returned string is an injection-safe SQL expression.
func (w *whereClause) argValue(arg *Arg, column *Column) (string, error) {
	if arg.Composite != nil {
		return "", fmt.Errorf("composite expressions in arguments not implemented yet")
	}
	if arg.Comparable == nil {
		return "", fmt.Errorf("missing comparable in argument")
	}
	return w.comparableValue(arg.Comparable, column)
}

// argValue returns a SQL expression representing the value of the specified
// comparable.
// The returned string is an injection-safe SQL expression.
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
		// Bind unsanitised user input to a parameter to protect against SQL injection.
		return w.bind(value), nil
	case ColumnTypeBool:
		if strings.EqualFold(comparable.Member.Value, "true") {
			return "TRUE", nil
		} else if strings.EqualFold(comparable.Member.Value, "false") {
			return "FALSE", nil
		}
		return "", fmt.Errorf(
			"only TRUE or FALSE can be specified as the value for a boolean field",
		)
	}
	return "", fmt.Errorf(
		"unable to generate SQL value for unknown field type: %s",
		column.columnType.String(),
	)
}

// likeArgValue returns a SQL expression that, when passed to the
// right hand side of a LIKE operator, performs substring matching against
// the value of the argument.
// The returned string is an injection-safe SQL expression.
func (w *whereClause) likeArgValue(arg *Arg, column *Column) (string, error) {
	if arg.Composite != nil {
		return "", fmt.Errorf("composite expressions are not allowed as RHS to has (:) operator")
	}
	if arg.Comparable == nil {
		return "", fmt.Errorf("missing comparable in argument")
	}
	if column.columnType != ColumnTypeString {
		return "", fmt.Errorf(
			"cannot use has (:) operator on a non-string field %q",
			column.columnType.String(),
		)
	}
	return w.likeComparableValue(arg.Comparable)
}

// prefixArgValue returns a SQL expression that, when passed to the right hand
// side of a LIKE operator, performs prefix matching.
func (w *whereClause) prefixArgValue(arg *Arg, column *Column) (string, error) {
	if arg.Composite != nil {
		return "", fmt.Errorf("composite expressions are not allowed as RHS to has (:) operator")
	}
	if arg.Comparable == nil {
		return "", fmt.Errorf("missing comparable in argument")
	}
	if column.columnType != ColumnTypeString {
		return "", fmt.Errorf(
			"cannot use has (:) operator on a non-string field %q",
			column.columnType.String(),
		)
	}
	return w.prefixComparableValue(arg.Comparable)
}

// fullTextArgValue returns a SQL expression representing full-text input.
func (w *whereClause) fullTextArgValue(arg *Arg, column *Column) (string, error) {
	if arg.Composite != nil {
		return "", fmt.Errorf("composite expressions are not allowed as RHS to has (:) operator")
	}
	if arg.Comparable == nil {
		return "", fmt.Errorf("missing comparable in argument")
	}
	if column.columnType != ColumnTypeString {
		return "", fmt.Errorf(
			"cannot use has (:) operator on a non-string field %q",
			column.columnType.String(),
		)
	}
	if arg.Comparable.Member == nil {
		return "", fmt.Errorf("invalid comparable")
	}
	if len(arg.Comparable.Member.Fields) > 0 {
		return "", fmt.Errorf("fields are not allowed on the RHS of has (:) operator")
	}
	// Bind unsanitised user input to a parameter to protect against SQL injection.
	return w.bind(arg.Comparable.Member.Value), nil
}

// likeComparableValue returns a SQL expression that, when passed to the
// right hand side of a LIKE operator, performs substring matching against
// the value of the comparable.
// The returned string is an injection-safe SQL expression.
func (w *whereClause) likeComparableValue(comparable *Comparable) (string, error) {
	if comparable.Member == nil {
		return "", fmt.Errorf("invalid comparable")
	}
	if len(comparable.Member.Fields) > 0 {
		return "", fmt.Errorf("fields are not allowed on the RHS of has (:) operator")
	}
	// Bind unsanitised user input to a parameter to protect against SQL injection.
	return w.bind("%" + QuoteLike(comparable.Member.Value) + "%"), nil
}

// prefixComparableValue returns a SQL expression that, when passed to the
// right hand side of a LIKE operator, performs prefix matching against
// the value of the comparable.
func (w *whereClause) prefixComparableValue(comparable *Comparable) (string, error) {
	if comparable.Member == nil {
		return "", fmt.Errorf("invalid comparable")
	}
	if len(comparable.Member.Fields) > 0 {
		return "", fmt.Errorf("fields are not allowed on the RHS of has (:) operator")
	}
	// Bind unsanitised user input to a parameter to protect against SQL injection.
	return w.bind(QuoteLike(comparable.Member.Value) + "%"), nil
}

// bind binds a new query parameter with the given value, and returns
// the name of the parameter (including '@').
// The returned string is an injection-safe SQL expression.
func (w *whereClause) bind(value string) string {
	name := w.namePrefix + strconv.Itoa(w.nextValueName)
	w.nextValueName++
	w.parameters = append(w.parameters, QueryParameter{Name: name, Value: value})
	return "@" + name
}

// Condition represents a WHERE clause condition extracted from the filter AST.
// It contains the column being filtered and whether it's an equality condition.
type Condition struct {
	// Column is the database column being filtered.
	Column *Column
	// IsEquality indicates if this is an equality condition (=) vs a range condition (>, <, >=, <=, LIKE).
	IsEquality bool
}

// findBestCompositeIndex selects the composite index that best matches the given conditions.
// It returns nil if no suitable index is found.
//
// The algorithm scores each index based on:
//   - Prefix match length (columns must match continuously from the start)
//   - Column position weight (earlier columns have higher weight)
//   - Equality bonus (equality conditions score higher than range conditions)
//
// Example:
//
//	Index: idx(a, b, c)
//	Conditions: WHERE a = 1 AND b = 2 AND d > 10
//	Score: a matches (position 0, equality) + b matches (position 1, equality) = 35 + 25 = 60
func findBestCompositeIndex(conditions []Condition, indexes []CompositeIndex) *CompositeIndex {
	if len(indexes) == 0 || len(conditions) == 0 {
		return nil
	}

	var bestIndex *CompositeIndex
	bestScore := 0

	for i := range indexes {
		score := calculateIndexScore(conditions, indexes[i])
		if score > bestScore {
			bestScore = score
			bestIndex = &indexes[i]
		}
	}

	return bestIndex
}

// calculateIndexScore computes a score indicating how well the index matches the conditions.
// Higher scores indicate better matches.
//
// Scoring rules:
//  1. Only continuous prefix matches count (if column 2 is used, column 1 must also be used)
//  2. Earlier columns in the index have higher weight: (n-i) * 10, where n is index length and i is position
//  3. Equality conditions receive a bonus of +5 points
//  4. Once the prefix is broken (a column doesn't match), scoring stops
//
// Example:
//
//	Index: idx(status, user_id, created_at)  // 3 columns
//	Conditions: status = 'active' AND user_id = 123 AND created_at > '2024-01-01'
//
//	Scoring:
//	- status matches at position 0: (3-0)*10 + 5 (equality) = 35
//	- user_id matches at position 1: (3-1)*10 + 5 (equality) = 25
//	- created_at matches at position 2: (3-2)*10 + 0 (range) = 10
//	Total score: 70
func calculateIndexScore(conditions []Condition, index CompositeIndex) int {
	if len(index.Columns) == 0 || len(conditions) == 0 {
		return 0
	}

	// Build a map of condition columns for quick lookup
	conditionColumns := make(map[string]*Condition)
	for i := range conditions {
		conditionColumns[conditions[i].Column.databaseName] = &conditions[i]
	}

	score := 0
	indexLen := len(index.Columns)

	// Calculate score based on continuous prefix matching
	for i, indexCol := range index.Columns {
		condition, found := conditionColumns[indexCol]
		if !found {
			// Prefix is broken - stop scoring
			break
		}

		// Position weight: earlier columns have higher weight
		positionWeight := (indexLen - i) * 10
		score += positionWeight

		// Equality bonus: equality conditions are more valuable for index usage
		if condition.IsEquality {
			score += 5
		}
	}

	return score
}

// reorderConditions reorders WHERE conditions to maximize composite index utilization.
// It places equality conditions before range conditions and orders them according to
// the composite index column order.
//
// Algorithm:
//  1. Separate conditions into three groups: equality, range, and other
//  2. Sort equality conditions by their position in the composite index
//  3. Sort range conditions by their position in the composite index
//  4. Combine: equality conditions + range conditions + other conditions
//
// Only AND-connected conditions should be reordered. OR-connected conditions
// must preserve their original order to maintain query semantics.
//
// Example:
//
//	Index: idx(status, user_id, created_at)
//	Input: user_id = 1 AND created_at > '2024-01-01' AND status = 'active'
//	Output: status = 'active' AND user_id = 1 AND created_at > '2024-01-01'
//
// Returns the reordered conditions slice. If index is nil, returns conditions unchanged.
func reorderConditions(conditions []Condition, index *CompositeIndex) []Condition {
	if index == nil || len(conditions) < 2 {
		return conditions
	}

	// Build a map of column positions in the index for O(1) lookup
	indexPositions := make(map[string]int)
	for i, col := range index.Columns {
		indexPositions[col] = i
	}

	// Separate conditions into three groups
	// Pre-allocate capacity to reduce memory allocations
	equalityConditions := make([]Condition, 0, len(conditions))
	rangeConditions := make([]Condition, 0, len(conditions))
	otherConditions := make([]Condition, 0, len(conditions))

	for _, cond := range conditions {
		// Check if column is in the index
		_, inIndex := indexPositions[cond.Column.databaseName]

		if cond.IsEquality {
			if inIndex {
				equalityConditions = append(equalityConditions, cond)
			} else {
				// Equality conditions not in index go to "other"
				otherConditions = append(otherConditions, cond)
			}
		} else {
			if inIndex {
				rangeConditions = append(rangeConditions, cond)
			} else {
				// Range conditions not in index go to "other"
				otherConditions = append(otherConditions, cond)
			}
		}
	}

	// Sort equality conditions by index position
	sortByIndexOrder(equalityConditions, indexPositions)

	// Sort range conditions by index position
	sortByIndexOrder(rangeConditions, indexPositions)

	// Combine: equality + range + other
	result := make([]Condition, 0, len(conditions))
	result = append(result, equalityConditions...)
	result = append(result, rangeConditions...)
	result = append(result, otherConditions...)

	return result
}

// sortByIndexOrder sorts conditions by their column position in the composite index.
// Columns not in the index are placed at the end (assigned MAX_INT position).
func sortByIndexOrder(conditions []Condition, indexPositions map[string]int) {
	// Use sort.Slice for O(n log n) performance instead of O(n²) bubble sort
	sort.Slice(conditions, func(i, j int) bool {
		pos1, found1 := indexPositions[conditions[i].Column.databaseName]
		pos2, found2 := indexPositions[conditions[j].Column.databaseName]

		// Assign MAX_INT to columns not in index so they sort to the end
		if !found1 {
			pos1 = 1<<31 - 1 // MAX_INT
		}
		if !found2 {
			pos2 = 1<<31 - 1 // MAX_INT
		}

		return pos1 < pos2
	})
}

func sameConditionOrder(a, b []Condition) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Column.databaseName != b[i].Column.databaseName ||
			a[i].IsEquality != b[i].IsEquality {
			return false
		}
	}
	return true
}
