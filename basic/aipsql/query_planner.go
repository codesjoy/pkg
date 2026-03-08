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
	"strconv"
	"strings"
	"sync"
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

// TableSpec describes code-registered metadata for a queryable table.
type TableSpec struct {
	// Name is the registry key used by PlanList.
	Name string

	// Table contains AIP field/column metadata.
	Table *Table

	// FromClause is appended after FROM in generated SQL.
	// It must be a trusted constant.
	FromClause string

	// SelectClause is appended after SELECT in generated SQL.
	// Default: "*".
	SelectClause string

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

	// EnableDebug includes rewrite/index decisions in QueryPlan.Debug.
	EnableDebug bool
}

// SeekPageToken is the cursor payload used for seek pagination.
type SeekPageToken struct {
	SortValues      []string `json:"sort_values"`
	TieBreakerValue string   `json:"tie_breaker_value"`
}

// SeekTokenDescriptor describes how to produce/consume seek tokens for a plan.
type SeekTokenDescriptor struct {
	// SortOrder matches the values order in SeekPageToken.SortValues.
	SortOrder []OrderBy

	// TieBreakerFieldPath matches SeekPageToken.TieBreakerValue.
	TieBreakerFieldPath FieldPath
}

// PlanDebugInfo carries optional planner decisions for debugging.
type PlanDebugInfo struct {
	Dialect                    SQLDialect
	StrictMode                 bool
	CompositeIndexOptimization bool
	AppliedRewrites            []string
	RequestedOrder             []OrderBy
	EffectiveOrder             []OrderBy
	OrderByCompositeIndex      string
	FilterCompositeIndex       string
	FilterCompositeReordered   bool
	SeekPaginationEnabled      bool
	ParameterPrefix            string
}

// QueryParts contains reusable query fragments for one planned list query.
type QueryParts struct {
	TableName string

	SelectClause     string
	FromClause       string
	FilterClause     string
	PaginationClause string
	WhereClause      string
	OrderByClause    string

	Parameters []QueryParameter
	Limit      int
	Offset     int

	PaginationMode PaginationMode

	TokenDescriptor SeekTokenDescriptor
	Debug           *PlanDebugInfo
}

// QueryPlan is the planned SQL shape for one list query.
type QueryPlan struct {
	TableName string

	SQL string

	SelectClause     string
	FromClause       string
	FilterClause     string
	PaginationClause string
	SeekClause       string
	WhereClause      string
	OrderByClause    string

	Parameters []QueryParameter
	Limit      int
	Offset     int

	PaginationMode PaginationMode

	TokenDescriptor SeekTokenDescriptor
	Debug           *PlanDebugInfo
}

type registeredTableSpec struct {
	name         string
	table        *Table
	fromClause   string
	selectClause string
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
	mu      sync.RWMutex
	tables  map[string]registeredTableSpec
	options resolvedPlannerOptions
}

// NewQueryPlanner builds a planner with validated defaults.
func NewQueryPlanner(options QueryPlannerOptions) (*QueryPlanner, error) {
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

	return &QueryPlanner{
		tables: make(map[string]registeredTableSpec),
		options: resolvedPlannerOptions{
			dialect:                          dialect,
			strictMode:                       options.StrictMode,
			enableCompositeIndexOptimization: enableComposite,
			parameterPrefix:                  prefix,
			defaultPageSize:                  defaultPageSize,
			maxPageSize:                      maxPageSize,
		},
	}, nil
}

// RegisterTableSpec registers one table metadata definition.
func (p *QueryPlanner) RegisterTableSpec(spec TableSpec) error {
	registered, err := p.normalizeTableSpec(spec)
	if err != nil {
		return err
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if _, ok := p.tables[registered.name]; ok {
		return fmt.Errorf("table %q is already registered", registered.name)
	}
	p.tables[registered.name] = registered
	return nil
}

// RegisterTableSpecs registers multiple table metadata definitions.
func (p *QueryPlanner) RegisterTableSpecs(specs ...TableSpec) error {
	for _, spec := range specs {
		if err := p.RegisterTableSpec(spec); err != nil {
			return err
		}
	}
	return nil
}

// PlanListParts generates reusable query fragments from filter/order_by EBNF text.
func (p *QueryPlanner) PlanListParts(
	ctx context.Context,
	tableName string,
	req QueryRequest,
) (*QueryParts, error) {
	return p.planListParts(ctx, tableName, req)
}

// PlanList generates index-friendly SQL from filter/order_by EBNF text.
func (p *QueryPlanner) PlanList(
	ctx context.Context,
	tableName string,
	req QueryRequest,
) (*QueryPlan, error) {
	parts, err := p.planListParts(ctx, tableName, req)
	if err != nil {
		return nil, err
	}

	plan := &QueryPlan{
		TableName:        parts.TableName,
		SQL:              buildSQL(parts.SelectClause, parts.FromClause, parts.WhereClause, parts.OrderByClause, parts.Limit, parts.Offset),
		SelectClause:     parts.SelectClause,
		FromClause:       parts.FromClause,
		FilterClause:     parts.FilterClause,
		PaginationClause: parts.PaginationClause,
		WhereClause:      parts.WhereClause,
		OrderByClause:    parts.OrderByClause,
		Parameters:       parts.Parameters,
		Limit:            parts.Limit,
		Offset:           parts.Offset,
		PaginationMode:   parts.PaginationMode,
		TokenDescriptor:  parts.TokenDescriptor,
		Debug:            parts.Debug,
	}
	if parts.PaginationMode == PaginationModeSeek {
		plan.SeekClause = parts.PaginationClause
	}
	return plan, nil
}

func (p *QueryPlanner) planListParts(
	ctx context.Context,
	tableName string,
	req QueryRequest,
) (*QueryParts, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	registered, err := p.lookupTableSpec(tableName)
	if err != nil {
		return nil, err
	}

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

	tokenDescriptor := SeekTokenDescriptor{}
	if paginationMode == PaginationModeSeek {
		tokenDescriptor = SeekTokenDescriptor{
			SortOrder:           copyOrderByList(effectiveOrder),
			TieBreakerFieldPath: registered.tieBreaker,
		}
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

	paginationClause, paginationParams, offset, seekEnabled, err := p.planPagination(
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

	parts := &QueryParts{
		TableName:        registered.name,
		SelectClause:     registered.selectClause,
		FromClause:       registered.fromClause,
		FilterClause:     filterClause,
		PaginationClause: paginationClause,
		WhereClause:      whereClause,
		OrderByClause:    orderByClause,
		Parameters:       mergePlanParams(filterParams, paginationParams),
		Limit:            limit,
		Offset:           offset,
		PaginationMode:   paginationMode,
		TokenDescriptor:  tokenDescriptor,
	}

	if req.EnableDebug {
		parts.Debug = p.buildDebugInfo(
			registered,
			filterAST,
			requestedOrder,
			effectiveOrder,
			dialect,
			strictMode,
			enableComposite,
			seekEnabled,
			parameterPrefix,
		)
	}

	return parts, nil
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
) (string, []QueryParameter, int, bool, error) {
	switch mode {
	case PaginationModeSeek:
		clause, params, seekEnabled, err := p.planSeekClause(
			registered,
			effectiveOrder,
			pageToken,
			parameterPrefix,
			dialect,
		)
		return clause, params, 0, seekEnabled, err
	case PaginationModeOffset:
		offset, err := planOffset(pageToken)
		return "", nil, offset, false, err
	default:
		return "", nil, 0, false, fmt.Errorf("unsupported pagination mode %q", mode)
	}
}

func (p *QueryPlanner) planSeekClause(
	registered registeredTableSpec,
	effectiveOrder []OrderBy,
	pageToken string,
	parameterPrefix string,
	dialect SQLDialect,
) (string, []QueryParameter, bool, error) {
	if strings.TrimSpace(pageToken) == "" {
		return "", nil, false, nil
	}

	seekToken, err := DecodeSeekPageToken(pageToken)
	if err != nil {
		return "", nil, false, err
	}
	if len(seekToken.SortValues) != len(effectiveOrder) {
		return "", nil, false, fmt.Errorf(
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
			return "", nil, false, err
		}
		return seekClause, seekParams, true, nil
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
		return "", nil, false, err
	}
	return seekClause, seekParams, true, nil
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

func (p *QueryPlanner) lookupTableSpec(tableName string) (registeredTableSpec, error) {
	name := strings.TrimSpace(tableName)
	if name == "" {
		return registeredTableSpec{}, fmt.Errorf("table name is required")
	}

	p.mu.RLock()
	defer p.mu.RUnlock()
	registered, ok := p.tables[name]
	if !ok {
		return registeredTableSpec{}, fmt.Errorf("table %q is not registered", name)
	}
	return registered, nil
}

func (p *QueryPlanner) normalizeTableSpec(spec TableSpec) (registeredTableSpec, error) {
	name := strings.TrimSpace(spec.Name)
	if name == "" {
		return registeredTableSpec{}, fmt.Errorf("table name is required")
	}
	if spec.Table == nil {
		return registeredTableSpec{}, fmt.Errorf("table %q: table metadata is required", name)
	}
	if spec.TieBreakerFieldPath.String() == "" {
		return registeredTableSpec{}, fmt.Errorf(
			"table %q: tie breaker field path is required",
			name,
		)
	}

	if _, err := spec.Table.SortableColumnByFieldPath(spec.TieBreakerFieldPath); err != nil {
		return registeredTableSpec{}, fmt.Errorf(
			"table %q: invalid tie breaker field: %w",
			name,
			err,
		)
	}

	defaultOrder := copyOrderByList(spec.DefaultOrder)
	for _, entry := range defaultOrder {
		if entry.FieldPath.Equals(spec.TieBreakerFieldPath) {
			return registeredTableSpec{}, fmt.Errorf(
				"table %q: default order must not include tie breaker field %q",
				name,
				spec.TieBreakerFieldPath.String(),
			)
		}
		if _, err := spec.Table.SortableColumnByFieldPath(entry.FieldPath); err != nil {
			return registeredTableSpec{}, fmt.Errorf(
				"table %q: invalid default order: %w",
				name,
				err,
			)
		}
	}

	fromClause := strings.TrimSpace(spec.FromClause)
	if fromClause == "" {
		fromClause = name
	}
	selectClause := strings.TrimSpace(spec.SelectClause)
	if selectClause == "" {
		selectClause = "*"
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
			"table %q: max page size %d cannot be smaller than default page size %d",
			name,
			maxLimit,
			defaultLimit,
		)
	}

	paginationMode, err := normalizePaginationMode(spec.PaginationMode)
	if err != nil {
		return registeredTableSpec{}, fmt.Errorf("table %q: %w", name, err)
	}

	return registeredTableSpec{
		name:         name,
		table:        spec.Table,
		fromClause:   fromClause,
		selectClause: selectClause,
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

func (p *QueryPlanner) buildDebugInfo(
	spec registeredTableSpec,
	filterAST *Filter,
	requestedOrder []OrderBy,
	effectiveOrder []OrderBy,
	dialect SQLDialect,
	strictMode bool,
	enableComposite bool,
	seekEnabled bool,
	parameterPrefix string,
) *PlanDebugInfo {
	rewrites := make([]string, 0, 4)
	if len(spec.defaultOrder) > 0 {
		rewrites = append(rewrites, "merge_default_order")
	}
	if len(effectiveOrder) == 0 {
		rewrites = append(rewrites, "order_by_tie_breaker_only")
	}
	rewrites = append(rewrites, "append_tie_breaker")
	if seekEnabled {
		rewrites = append(rewrites, "seek_pagination")
	}

	orderByIndexName := ""
	if fields, err := orderByFieldsFor(spec.table, effectiveOrder); err == nil {
		if matched := findMatchingOrderByIndex(fields, spec.table.CompositeIndexes); matched != nil {
			orderByIndexName = matched.Name
		}
	}

	filterIndexName := ""
	filterReordered := false
	if enableComposite {
		if name, reordered := analyzeFilterCompositeIndex(spec.table, filterAST, dialect, strictMode); name != "" {
			filterIndexName = name
			filterReordered = reordered
		}
	}

	return &PlanDebugInfo{
		Dialect:                    dialect,
		StrictMode:                 strictMode,
		CompositeIndexOptimization: enableComposite,
		AppliedRewrites:            rewrites,
		RequestedOrder:             copyOrderByList(requestedOrder),
		EffectiveOrder:             copyOrderByList(effectiveOrder),
		OrderByCompositeIndex:      orderByIndexName,
		FilterCompositeIndex:       filterIndexName,
		FilterCompositeReordered:   filterReordered,
		SeekPaginationEnabled:      seekEnabled,
		ParameterPrefix:            parameterPrefix,
	}
}

func buildSQL(
	selectClause string,
	fromClause string,
	whereClause string,
	orderByClause string,
	limit int,
	offset int,
) string {
	var b strings.Builder
	b.Grow(len(selectClause) + len(fromClause) + len(whereClause) + len(orderByClause) + 64)
	b.WriteString("SELECT ")
	b.WriteString(selectClause)
	b.WriteString(" FROM ")
	b.WriteString(fromClause)
	if whereClause != "" {
		b.WriteString(" WHERE ")
		b.WriteString(whereClause)
	}
	if orderByClause != "" {
		b.WriteString(" ORDER BY ")
		b.WriteString(orderByClause)
	}
	b.WriteString(" LIMIT ")
	b.WriteString(strconv.Itoa(limit))
	if offset > 0 {
		b.WriteString(" OFFSET ")
		b.WriteString(strconv.Itoa(offset))
	}
	return b.String()
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

func orderByFieldsFor(table *Table, order []OrderBy) ([]OrderByField, error) {
	result := make([]OrderByField, 0, len(order))
	for _, entry := range order {
		column, err := table.SortableColumnByFieldPath(entry.FieldPath)
		if err != nil {
			return nil, err
		}
		direction := "ASC"
		if entry.Descending {
			direction = "DESC"
		}
		result = append(result, OrderByField{Column: column, Direction: direction})
	}
	return result, nil
}

func analyzeFilterCompositeIndex(
	table *Table,
	filter *Filter,
	dialect SQLDialect,
	strictMode bool,
) (string, bool) {
	if filter == nil || filter.Expression == nil || len(table.CompositeIndexes) == 0 {
		return "", false
	}

	w := &whereClause{
		table:                            table,
		dialect:                          dialect,
		strictMode:                       strictMode,
		fallbackMode:                     MatchModeContains,
		optimizeMatch:                    true,
		enableCompositeIndexOptimization: true,
	}

	factors := flattenExpressionFactors(filter.Expression)
	if len(factors) < 2 {
		return "", false
	}

	conditions, factorConditionIndexes, err := w.extractConditionsFromFactors(factors)
	if err != nil || len(conditions) == 0 {
		return "", false
	}

	bestIndex := findBestCompositeIndex(conditions, table.CompositeIndexes)
	if bestIndex == nil {
		return "", false
	}

	reorderedConditions := reorderConditions(conditions, bestIndex)
	if sameConditionOrder(conditions, reorderedConditions) {
		return bestIndex.Name, false
	}

	reorderedFactors := w.reorderFactors(
		factors,
		conditions,
		factorConditionIndexes,
		reorderedConditions,
	)
	return bestIndex.Name, !sameFactorPointers(factors, reorderedFactors)
}

func sameFactorPointers(a, b []*Factor) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
