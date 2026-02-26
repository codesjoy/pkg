package main

import (
	"fmt"
	"strings"
)

// ResolveTables merges metadata defaults and YAML overrides.
func ResolveTables(
	metas []TableMeta,
	overrides OverrideConfig,
	opts Options,
) ([]ResolvedTable, []string, error) {
	cliTableSet := makeStringSet(opts.Tables)
	includeSet := makeStringSet(overrides.IncludeTables)
	excludeSet := makeStringSet(overrides.ExcludeTables)

	resolved := make([]ResolvedTable, 0, len(metas))
	warnings := make([]string, 0)
	for _, meta := range metas {
		if len(cliTableSet) > 0 && !containsKey(cliTableSet, meta.Name) {
			continue
		}
		if len(includeSet) > 0 && !containsKey(includeSet, meta.Name) {
			continue
		}
		if containsKey(excludeSet, meta.Name) {
			continue
		}

		tableOv, hasTableOv := overrides.tableOverride(meta.Name)
		if hasTableOv && tableOv.Skip != nil && *tableOv.Skip {
			continue
		}

		modelName := ToPascalCase(meta.Name)
		if hasTableOv && strings.TrimSpace(tableOv.ModelName) != "" {
			modelName = strings.TrimSpace(tableOv.ModelName)
		}
		aipsqlBuilder := "New" + modelName + "AIPTable"
		if hasTableOv && strings.TrimSpace(tableOv.AIPSQLBuilder) != "" {
			aipsqlBuilder = strings.TrimSpace(tableOv.AIPSQLBuilder)
		}

		generateAIPTable := true
		if overrides.GenAIPSQL != nil {
			generateAIPTable = *overrides.GenAIPSQL
		}
		if hasTableOv && tableOv.GenAIPSQL != nil {
			generateAIPTable = *tableOv.GenAIPSQL
		}
		if opts.GenAIPSQLSet {
			generateAIPTable = opts.GenAIPSQL
		}

		timestampMode := timestampModeUnixSec
		if strings.TrimSpace(overrides.TimestampMode) != "" {
			timestampMode = strings.ToLower(strings.TrimSpace(overrides.TimestampMode))
		}
		if hasTableOv && strings.TrimSpace(tableOv.TimestampMode) != "" {
			timestampMode = strings.ToLower(strings.TrimSpace(tableOv.TimestampMode))
		}
		if opts.TimestampModeSet {
			timestampMode = strings.ToLower(strings.TrimSpace(opts.TimestampMode))
		}
		if err := validateTimestampMode(timestampMode); err != nil {
			return nil, nil, fmt.Errorf("table %s: %w", meta.Name, err)
		}

		columns := make([]ResolvedColumn, 0, len(meta.Columns))
		for _, col := range meta.Columns {
			colOv, hasColOv := tableColumnOverride(tableOv, col.Name)
			if hasColOv && colOv.Skip != nil && *colOv.Skip {
				continue
			}

			resolvedCol, colWarnings, err := resolveColumn(col, colOv, timestampMode, opts)
			if err != nil {
				return nil, nil, fmt.Errorf("table %s column %s: %w", meta.Name, col.Name, err)
			}
			for _, warning := range colWarnings {
				warnings = append(warnings, fmt.Sprintf("table %s: %s", meta.Name, warning))
			}
			columns = append(columns, resolvedCol)
		}
		if len(columns) == 0 {
			continue
		}

		columns, err := dedupeFieldNames(columns)
		if err != nil {
			return nil, nil, fmt.Errorf("table %s: %w", meta.Name, err)
		}

		keptColumns := makeStringSetFromResolved(columns)
		compositeIndexes := make([]IndexMeta, 0, len(meta.Indexes))
		for _, index := range meta.Indexes {
			if len(index.Columns) < 2 {
				continue
			}
			if !allColumnsPresent(index.Columns, keptColumns) {
				continue
			}
			compositeIndexes = append(compositeIndexes, index)
		}

		resolved = append(resolved, ResolvedTable{
			Schema:           meta.Schema,
			Name:             meta.Name,
			ModelName:        modelName,
			AIPSQLBuilder:    aipsqlBuilder,
			GenerateAIPTable: generateAIPTable,
			TimestampMode:    timestampMode,
			Columns:          columns,
			CompositeIndexes: compositeIndexes,
		})
	}

	if len(resolved) == 0 {
		return nil, nil, fmt.Errorf("no tables selected after applying filters and overrides")
	}

	return resolved, warnings, nil
}

func resolveColumn(
	col ColumnMeta,
	ov ColumnOverride,
	tableTimestampMode string,
	opts Options,
) (ResolvedColumn, []string, error) {
	role := timestampRoleByColumnName(col.Name)
	goField := defaultGoFieldForRole(col, role)
	if strings.TrimSpace(ov.GoField) != "" {
		goField = strings.TrimSpace(ov.GoField)
	}

	jsonName := col.Name
	if strings.TrimSpace(ov.JSONName) != "" {
		jsonName = strings.TrimSpace(ov.JSONName)
	}

	fieldPath := col.Name
	if strings.TrimSpace(ov.FieldPath) != "" {
		fieldPath = strings.TrimSpace(ov.FieldPath)
	}

	columnTimestampMode := tableTimestampMode
	if strings.TrimSpace(ov.TimestampMode) != "" {
		columnTimestampMode = strings.ToLower(strings.TrimSpace(ov.TimestampMode))
	}
	if opts.TimestampModeSet {
		columnTimestampMode = strings.ToLower(strings.TrimSpace(opts.TimestampMode))
	}

	goType, useIntTimestamp, softDeleteKind, warning, err := resolveTimestampType(
		col,
		columnTimestampMode,
		role,
	)
	if err != nil {
		return ResolvedColumn{}, nil, err
	}
	if strings.TrimSpace(ov.GoType) != "" {
		goType = strings.TrimSpace(ov.GoType)
		useIntTimestamp = false
		switch baseGoType(goType) {
		case "gorm.DeletedAt":
			softDeleteKind = softDeleteKindGORM
		case "soft_delete.DeletedAt":
			softDeleteKind = softDeleteKindPlugin
			useIntTimestamp = true
		default:
			softDeleteKind = softDeleteKindNone
		}
	}

	filterable := isScalarGoType(goType)
	if ov.Filterable != nil {
		filterable = *ov.Filterable
	}

	sortable := col.IsPrimaryKey || col.IsIndexed
	if ov.Sortable != nil {
		sortable = *ov.Sortable
	}

	implicitFilter := false
	if ov.ImplicitFilter != nil {
		implicitFilter = *ov.ImplicitFilter
	}

	boolType := baseGoType(goType) == "bool"
	if ov.BoolType != nil {
		boolType = *ov.BoolType
	}

	keyValue := false
	if ov.KeyValue != nil {
		keyValue = *ov.KeyValue
	}

	matchModes := []string(nil)
	if isTextualColumn(col) {
		matchModes = []string{"exact"}
	}
	if ov.MatchModes != nil {
		matchModes = dedupeStrings(ov.MatchModes)
	}
	for _, mode := range matchModes {
		if !isSupportedMatchMode(mode) {
			return ResolvedColumn{}, nil, fmt.Errorf("unsupported match mode %q", mode)
		}
	}

	indexHint := strings.TrimSpace(ov.IndexHint)
	gormTagAppend := strings.TrimSpace(ov.GormTagAppend)
	hideFromAIPTable := false
	if role == timestampRoleDeleted {
		hideFromAIPTable = true
		if boolPtrValue(ov.Filterable) ||
			boolPtrValue(ov.Sortable) ||
			boolPtrValue(ov.ImplicitFilter) {
			hideFromAIPTable = false
		}
	}

	warnings := make([]string, 0, 1)
	if warning != "" {
		warnings = append(warnings, warning)
	}

	return ResolvedColumn{
		Name:             col.Name,
		GoField:          goField,
		GoType:           goType,
		JSONName:         jsonName,
		FieldPath:        fieldPath,
		Filterable:       filterable,
		Sortable:         sortable,
		ImplicitFilter:   implicitFilter,
		BoolType:         boolType,
		KeyValue:         keyValue,
		MatchModes:       matchModes,
		IndexHint:        indexHint,
		GormTagAppend:    gormTagAppend,
		TimestampRole:    role,
		TimestampMode:    columnTimestampMode,
		UseIntTimestamp:  useIntTimestamp,
		SoftDeleteKind:   softDeleteKind,
		HideFromAIPTable: hideFromAIPTable,
		IsPrimaryKey:     col.IsPrimaryKey,
		IsAutoIncrement:  col.IsAutoIncrement,
		DataType:         col.DataType,
		RawType:          col.RawType,
		Nullable:         col.Nullable,
	}, warnings, nil
}

func isSupportedMatchMode(mode string) bool {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "exact", "prefix", "fulltext", "contains":
		return true
	default:
		return false
	}
}

func makeStringSet(items []string) map[string]struct{} {
	set := make(map[string]struct{}, len(items))
	for _, item := range items {
		if item == "" {
			continue
		}
		set[item] = struct{}{}
	}
	return set
}

func containsKey(set map[string]struct{}, key string) bool {
	_, ok := set[key]
	return ok
}

func makeStringSetFromResolved(cols []ResolvedColumn) map[string]struct{} {
	set := make(map[string]struct{}, len(cols))
	for _, col := range cols {
		set[col.Name] = struct{}{}
	}
	return set
}

func allColumnsPresent(columns []string, set map[string]struct{}) bool {
	for _, col := range columns {
		if !containsKey(set, col) {
			return false
		}
	}
	return true
}

func boolPtrValue(v *bool) bool {
	return v != nil && *v
}
