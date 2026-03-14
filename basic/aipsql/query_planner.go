// Copyright 2026 The codesjoy Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package aipsql

import (
	"context"
	"fmt"
	"strings"
)

const (
	defaultPlannerParameterPrefix = "p_"
	defaultPlannerPageSize        = 50
	defaultPlannerMaxPageSize     = 200
)

// PaginationMode controls how list pagination is planned.
type PaginationMode string

const (
	// PaginationModeSeek uses seek/cursor pagination backed by page tokens.
	PaginationModeSeek PaginationMode = "seek"
	// PaginationModeOffset uses LIMIT/OFFSET pagination backed by offset page tokens.
	PaginationModeOffset PaginationMode = "offset"
)

// QueryPlannerOptions controls default behavior for QueryPlanner.
type QueryPlannerOptions struct {
	// Dialect is the default SQL dialect for planning.
	// Default: SQLDialectGeneric.
	Dialect SQLDialect

	// StrictMode controls default has(:) fallback behavior.
	// Default: false.
	StrictMode bool

	// EnableCompositeIndexOptimization controls default condition reordering behavior.
	// Default: true.
	EnableCompositeIndexOptimization bool

	// ParameterPrefix is the default query parameter prefix.
	// Default: "p_".
	ParameterPrefix string

	// DefaultPageSize is applied when request page_size <= 0.
	// Default: 50.
	DefaultPageSize int

	// MaxPageSize caps request page_size.
	// Default: 200.
	MaxPageSize int
}

// TableSpec describes the queryable table metadata used by QueryPlanner.
type TableSpec struct {
	// Table contains AIP field/column metadata.
	Table *Table

	// DefaultOrder is merged with request order_by using MergeWithDefaultOrder.
	DefaultOrder []OrderBy

	// TieBreakerFieldPath must point to a sortable, unique-ish column.
	// It is appended to ORDER BY for deterministic seek pagination.
	TieBreakerFieldPath FieldPath

	// PaginationMode controls the default list pagination mode for this table.
	// Default: seek.
	PaginationMode PaginationMode

	// DefaultPageSize overrides planner default for this table when request page_size <= 0.
	DefaultPageSize int

	// MaxPageSize overrides planner max for this table.
	MaxPageSize int
}

// QueryRequest contains EBNF inputs and pagination options.
type QueryRequest struct {
	// Filter is the AIP-160 filter expression text.
	Filter string

	// OrderBy is the AIP-132 order_by expression text.
	OrderBy string

	// PageSize is the requested page size.
	PageSize int

	// PageToken is an opaque pagination token.
	// Seek mode expects EncodeSeekPageToken; offset mode expects EncodeOffsetPageToken.
	PageToken string

	// PaginationMode overrides the table/default list pagination mode.
	PaginationMode PaginationMode

	// ParameterPrefix overrides the planner default prefix.
	ParameterPrefix string

	// Dialect overrides planner default SQL dialect.
	Dialect SQLDialect

	// StrictMode overrides planner default strict mode.
	StrictMode *bool

	// EnableCompositeIndexOptimization overrides planner default condition reordering.
	EnableCompositeIndexOptimization *bool
}

// QueryPlan contains the final executable clauses for one planned list query.
type QueryPlan struct {
	WhereClause   string
	OrderByClause string

	Parameters []QueryParameter
	Limit      int
	Offset     int

	paginationMode      PaginationMode
	table               *Table
	sortOrder           []OrderBy
	tieBreakerFieldPath FieldPath
}

type registeredTableSpec struct {
	table        *Table
	defaultOrder []OrderBy
	tieBreaker   FieldPath
	pagination   PaginationMode
	defaultLimit int
	maxLimit     int
}

type resolvedPlannerOptions struct {
	dialect                          SQLDialect
	strictMode                       bool
	enableCompositeIndexOptimization bool
	parameterPrefix                  string
	defaultPageSize                  int
	maxPageSize                      int
}

// QueryPlanner provides one text-direct list planning API for services.
type QueryPlanner struct {
	spec    registeredTableSpec
	options resolvedPlannerOptions
}

// NewQueryPlanner builds a planner for one validated table specification.
func NewQueryPlanner(spec TableSpec, options QueryPlannerOptions) (*QueryPlanner, error) {
	dialect, err := normalizeSQLDialect(options.Dialect)
	if err != nil {
		return nil, err
	}
	prefix := options.ParameterPrefix
	if prefix == "" {
		prefix = defaultPlannerParameterPrefix
	}
	if err := validateParameterPrefix(prefix); err != nil {
		return nil, err
	}
	enableComposite := options.EnableCompositeIndexOptimization
	if !enableComposite {
		// Keep the default optimization-on behavior when caller leaves zero value.
		enableComposite = true
	}

	defaultPageSize := options.DefaultPageSize
	if defaultPageSize <= 0 {
		defaultPageSize = defaultPlannerPageSize
	}
	maxPageSize := options.MaxPageSize
	if maxPageSize <= 0 {
		maxPageSize = defaultPlannerMaxPageSize
	}
	if maxPageSize < defaultPageSize {
		return nil, fmt.Errorf(
			"max page size %d cannot be smaller than default page size %d",
			maxPageSize,
			defaultPageSize,
		)
	}

	planner := &QueryPlanner{
		options: resolvedPlannerOptions{
			dialect:                          dialect,
			strictMode:                       options.StrictMode,
			enableCompositeIndexOptimization: enableComposite,
			parameterPrefix:                  prefix,
			defaultPageSize:                  defaultPageSize,
			maxPageSize:                      maxPageSize,
		},
	}
	registered, err := planner.normalizeTableSpec(spec)
	if err != nil {
		return nil, err
	}
	planner.spec = registered
	return planner, nil
}

// PlanList generates final WHERE/ORDER/LIMIT/OFFSET clauses from filter/order_by EBNF text.
func (p *QueryPlanner) PlanList(ctx context.Context, req QueryRequest) (*QueryPlan, error) {
	return p.planList(ctx, req)
}

func (p *QueryPlanner) planList(
	ctx context.Context,
	req QueryRequest,
) (*QueryPlan, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	registered := p.spec

	dialect, strictMode, enableComposite, parameterPrefix, err := p.resolveRequestOptions(req)
	if err != nil {
		return nil, err
	}

	filterAST, err := ParseFilter(req.Filter)
	if err != nil {
		return nil, err
	}

	filterClause, filterParams, err := registered.table.WhereClauseWithOptions(
		filterAST,
		parameterPrefix,
		WhereClauseOptions{
			Dialect:                          dialect,
			StrictMode:                       strictMode,
			EnableCompositeIndexOptimization: enableComposite,
		},
	)
	if err != nil {
		return nil, err
	}

	requestedOrder, err := ParseOrderBy(req.OrderBy)
	if err != nil {
		return nil, err
	}
	if err := validateRequestedOrder(requestedOrder, registered.tieBreaker); err != nil {
		return nil, err
	}

	effectiveOrder := MergeWithDefaultOrder(registered.defaultOrder, requestedOrder)
	if len(effectiveOrder) == 0 {
		effectiveOrder, _ = p.deriveIndexBackedOrder(registered)
	}

	paginationMode, err := resolvePaginationMode(registered, req)
	if err != nil {
		return nil, err
	}

	orderForSQL := append(
		copyOrderByList(effectiveOrder),
		OrderBy{FieldPath: registered.tieBreaker},
	)
	orderByClause, err := registered.table.OrderByClauseWithDialect(orderForSQL, dialect)
	if err != nil {
		return nil, err
	}

	limit := p.resolvePageSize(registered, req.PageSize)

	paginationClause, paginationParams, offset, err := p.planPagination(
		registered,
		effectiveOrder,
		req.PageToken,
		parameterPrefix,
		dialect,
		paginationMode,
	)
	if err != nil {
		return nil, err
	}

	whereClause := combineWhereClauses(filterClause, paginationClause)

	plan := &QueryPlan{
		WhereClause:         whereClause,
		OrderByClause:       orderByClause,
		Parameters:          mergePlanParams(filterParams, paginationParams),
		Limit:               limit,
		Offset:              offset,
		paginationMode:      paginationMode,
		table:               registered.table,
		tieBreakerFieldPath: registered.tieBreaker,
	}
	if paginationMode == PaginationModeSeek {
		plan.sortOrder = copyOrderByList(effectiveOrder)
	}

	return plan, nil
}

func validateRequestedOrder(requestedOrder []OrderBy, tieBreaker FieldPath) error {
	for _, entry := range requestedOrder {
		if entry.FieldPath.Equals(tieBreaker) {
			return fmt.Errorf(
				"order_by must not contain tie breaker field %q; it is appended automatically",
				tieBreaker.String(),
			)
		}
	}
	return nil
}

func (p *QueryPlanner) planPagination(
	registered registeredTableSpec,
	effectiveOrder []OrderBy,
	pageToken string,
	parameterPrefix string,
	dialect SQLDialect,
	mode PaginationMode,
) (string, []QueryParameter, int, error) {
	switch mode {
	case PaginationModeSeek:
		clause, params, err := p.planSeekClause(
			registered,
			effectiveOrder,
			pageToken,
			parameterPrefix,
			dialect,
		)
		return clause, params, 0, err
	case PaginationModeOffset:
		offset, err := planOffset(pageToken)
		return "", nil, offset, err
	default:
		return "", nil, 0, fmt.Errorf("unsupported pagination mode %q", mode)
	}
}

func (p *QueryPlanner) planSeekClause(
	registered registeredTableSpec,
	effectiveOrder []OrderBy,
	pageToken string,
	parameterPrefix string,
	dialect SQLDialect,
) (string, []QueryParameter, error) {
	if strings.TrimSpace(pageToken) == "" {
		return "", nil, nil
	}

	seekToken, err := DecodeSeekPageToken(pageToken)
	if err != nil {
		return "", nil, err
	}
	if len(seekToken.SortValues) != len(effectiveOrder) {
		return "", nil, fmt.Errorf(
			"page token sort value count %d does not match planned order field count %d",
			len(seekToken.SortValues),
			len(effectiveOrder),
		)
	}

	seekParamPrefix := parameterPrefix + "seek_"
	if len(effectiveOrder) == 0 {
		seekClause, seekParams, err := buildTieBreakerOnlySeekClause(
			registered.table,
			registered.tieBreaker,
			seekToken.TieBreakerValue,
			seekParamPrefix,
		)
		if err != nil {
			return "", nil, err
		}
		return seekClause, seekParams, nil
	}

	seekClause, seekParams, err := registered.table.BuildSeekPaginationClause(
		effectiveOrder,
		seekToken.SortValues,
		registered.tieBreaker,
		seekToken.TieBreakerValue,
		seekParamPrefix,
		dialect,
	)
	if err != nil {
		return "", nil, err
	}
	return seekClause, seekParams, nil
}

func planOffset(pageToken string) (int, error) {
	if strings.TrimSpace(pageToken) == "" {
		return 0, nil
	}
	return DecodeOffsetPageToken(pageToken)
}

func mergePlanParams(filterParams, seekParams []QueryParameter) []QueryParameter {
	merged := make([]QueryParameter, 0, len(filterParams)+len(seekParams))
	merged = append(merged, filterParams...)
	merged = append(merged, seekParams...)
	return merged
}

func (p *QueryPlanner) normalizeTableSpec(spec TableSpec) (registeredTableSpec, error) {
	if spec.Table == nil {
		return registeredTableSpec{}, fmt.Errorf("table metadata is required")
	}
	if spec.TieBreakerFieldPath.String() == "" {
		return registeredTableSpec{}, fmt.Errorf("tie breaker field path is required")
	}

	if _, err := spec.Table.SortableColumnByFieldPath(spec.TieBreakerFieldPath); err != nil {
		return registeredTableSpec{}, fmt.Errorf("invalid tie breaker field: %w", err)
	}

	defaultOrder := copyOrderByList(spec.DefaultOrder)
	for _, entry := range defaultOrder {
		if entry.FieldPath.Equals(spec.TieBreakerFieldPath) {
			return registeredTableSpec{}, fmt.Errorf(
				"default order must not include tie breaker field %q",
				spec.TieBreakerFieldPath.String(),
			)
		}
		if _, err := spec.Table.SortableColumnByFieldPath(entry.FieldPath); err != nil {
			return registeredTableSpec{}, fmt.Errorf("invalid default order: %w", err)
		}
	}

	defaultLimit := spec.DefaultPageSize
	if defaultLimit <= 0 {
		defaultLimit = p.options.defaultPageSize
	}
	maxLimit := spec.MaxPageSize
	if maxLimit <= 0 {
		maxLimit = p.options.maxPageSize
	}
	if maxLimit < defaultLimit {
		return registeredTableSpec{}, fmt.Errorf(
			"max page size %d cannot be smaller than default page size %d",
			maxLimit,
			defaultLimit,
		)
	}

	paginationMode, err := normalizePaginationMode(spec.PaginationMode)
	if err != nil {
		return registeredTableSpec{}, err
	}

	return registeredTableSpec{
		table:        spec.Table,
		defaultOrder: defaultOrder,
		tieBreaker:   spec.TieBreakerFieldPath,
		pagination:   paginationMode,
		defaultLimit: defaultLimit,
		maxLimit:     maxLimit,
	}, nil
}

func (p *QueryPlanner) resolveRequestOptions(
	req QueryRequest,
) (SQLDialect, bool, bool, string, error) {
	dialect := p.options.dialect
	if req.Dialect != "" {
		dialect = req.Dialect
	}
	normalizedDialect, err := normalizeSQLDialect(dialect)
	if err != nil {
		return "", false, false, "", err
	}

	strictMode := p.options.strictMode
	if req.StrictMode != nil {
		strictMode = *req.StrictMode
	}

	enableComposite := p.options.enableCompositeIndexOptimization
	if req.EnableCompositeIndexOptimization != nil {
		enableComposite = *req.EnableCompositeIndexOptimization
	}

	prefix := p.options.parameterPrefix
	if req.ParameterPrefix != "" {
		prefix = req.ParameterPrefix
	}
	if err := validateParameterPrefix(prefix); err != nil {
		return "", false, false, "", err
	}

	return normalizedDialect, strictMode, enableComposite, prefix, nil
}

func (p *QueryPlanner) resolvePageSize(spec registeredTableSpec, requestedSize int) int {
	size := requestedSize
	if size <= 0 {
		size = spec.defaultLimit
	}
	if size > spec.maxLimit {
		size = spec.maxLimit
	}
	return size
}

func (p *QueryPlanner) deriveIndexBackedOrder(spec registeredTableSpec) ([]OrderBy, bool) {
	if len(spec.table.CompositeIndexes) == 0 {
		return nil, false
	}

	for _, index := range spec.table.CompositeIndexes {
		if len(index.Columns) == 0 {
			continue
		}
		derived := make([]OrderBy, 0, len(index.Columns))
		for _, databaseColumn := range index.Columns {
			orderField, ok := sortableFieldPathByDatabaseColumn(spec.table, databaseColumn)
			if !ok {
				derived = nil
				break
			}
			if orderField.Equals(spec.tieBreaker) {
				continue
			}
			derived = append(derived, OrderBy{FieldPath: orderField})
		}
		if len(derived) > 0 {
			return derived, true
		}
	}
	return nil, false
}

func resolvePaginationMode(spec registeredTableSpec, req QueryRequest) (PaginationMode, error) {
	if req.PaginationMode != "" {
		return normalizePaginationMode(req.PaginationMode)
	}
	if spec.pagination != "" {
		return normalizePaginationMode(spec.pagination)
	}
	return PaginationModeSeek, nil
}

func combineWhereClauses(filterClause, seekClause string) string {
	if seekClause == "" {
		if filterClause == "" || filterClause == "(TRUE)" {
			return ""
		}
		return filterClause
	}
	if filterClause == "" || filterClause == "(TRUE)" {
		return seekClause
	}
	return "(" + filterClause + " AND " + seekClause + ")"
}

func buildTieBreakerOnlySeekClause(
	table *Table,
	tieBreakerFieldPath FieldPath,
	lastTieBreakerValue string,
	parameterPrefix string,
) (string, []QueryParameter, error) {
	tieBreakerColumn, err := table.SortableColumnByFieldPath(tieBreakerFieldPath)
	if err != nil {
		return "", nil, err
	}
	binder := &seekBinder{prefix: parameterPrefix}
	tieBreakerExpr, err := binder.sortValue(tieBreakerColumn, lastTieBreakerValue)
	if err != nil {
		return "", nil, fmt.Errorf(
			"sort value for tie breaker field %q: %w",
			tieBreakerFieldPath.String(),
			err,
		)
	}
	return "(" + tieBreakerColumn.databaseName + " > " + tieBreakerExpr + ")", binder.params, nil
}

func validateParameterPrefix(prefix string) error {
	if prefix == "" {
		return fmt.Errorf("parameter prefix cannot be empty")
	}
	for i, ch := range prefix {
		if i == 0 {
			if !isASCIIAlpha(ch) && ch != '_' {
				return fmt.Errorf(
					"invalid parameter prefix %q: first character must be [A-Za-z_]",
					prefix,
				)
			}
			continue
		}
		if !isASCIIAlpha(ch) && !isASCIIDigit(ch) && ch != '_' {
			return fmt.Errorf("invalid parameter prefix %q: only [A-Za-z0-9_] are allowed", prefix)
		}
	}
	return nil
}

func isASCIIAlpha(ch rune) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z')
}

func isASCIIDigit(ch rune) bool {
	return ch >= '0' && ch <= '9'
}

func normalizePaginationMode(mode PaginationMode) (PaginationMode, error) {
	switch mode {
	case "":
		return "", nil
	case PaginationModeSeek, PaginationModeOffset:
		return mode, nil
	default:
		return "", fmt.Errorf("invalid pagination mode %q", mode)
	}
}

func copyOrderByList(order []OrderBy) []OrderBy {
	if len(order) == 0 {
		return nil
	}
	result := make([]OrderBy, len(order))
	copy(result, order)
	return result
}

func sortableFieldPathByDatabaseColumn(table *Table, databaseColumn string) (FieldPath, bool) {
	for _, column := range table.columns {
		if column.databaseName == databaseColumn && column.sortable {
			return column.fieldPath, true
		}
	}
	return FieldPath{}, false
}
