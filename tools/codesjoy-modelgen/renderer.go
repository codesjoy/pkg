package main

import (
	"bytes"
	"fmt"
	"go/format"
	"sort"
	"strings"
)

// GoRenderer renders Go source files.
type GoRenderer struct{}

// Render renders single-file output for each table.
func (r *GoRenderer) Render(packageName string, tables []ResolvedTable) ([]RenderedFile, error) {
	files := make([]RenderedFile, 0, len(tables))
	for _, table := range tables {
		content, err := r.renderTableFile(packageName, table)
		if err != nil {
			return nil, fmt.Errorf("render table file for %s: %w", table.Name, err)
		}
		files = append(files, RenderedFile{
			Name:    fmt.Sprintf("%s_gen.go", table.Name),
			Content: content,
		})
	}
	return files, nil
}

func (r *GoRenderer) renderTableFile(packageName string, table ResolvedTable) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString(generatedHeader)
	buf.WriteString("\n\n")
	buf.WriteString("package ")
	buf.WriteString(packageName)
	buf.WriteString("\n\n")

	imports := collectImports(table)
	if len(imports) > 0 {
		buf.WriteString("import (\n")
		for _, imp := range imports {
			buf.WriteString("\t")
			buf.WriteString(imp)
			buf.WriteString("\n")
		}
		buf.WriteString(")\n\n")
	}

	renderModelBlock(&buf, table)
	if table.GenerateAIPTable {
		buf.WriteString("\n")
		renderAIPTableMethodBlock(&buf, table)
		buf.WriteString("\n")
		renderAIPTableWrapperBlock(&buf, table)
	}

	return formatSource(buf.Bytes())
}

func collectImports(table ResolvedTable) []string {
	imports := make([]string, 0, 4)
	if needsTimeImport(table.Columns) {
		imports = append(imports, "\"time\"")
	}
	if needsGORMImport(table.Columns) {
		imports = append(imports, "\"gorm.io/gorm\"")
	}
	if needsSoftDeleteImport(table.Columns) {
		imports = append(imports, "soft_delete \"gorm.io/plugin/soft_delete\"")
	}
	if table.GenerateAIPTable {
		imports = append(imports, "aipsql \"github.com/codesjoy/pkg/basic/aipsql\"")
	}
	sort.Strings(imports)
	return imports
}

func renderModelBlock(buf *bytes.Buffer, table ResolvedTable) {
	buf.WriteString("// ")
	buf.WriteString(table.ModelName)
	buf.WriteString(" is a generated GORM model for table ")
	_, _ = fmt.Fprintf(buf, "%q", table.Name)
	buf.WriteString(".\n")
	buf.WriteString("type ")
	buf.WriteString(table.ModelName)
	buf.WriteString(" struct {\n")
	for _, col := range table.Columns {
		gormTag := buildGormTag(col)
		buf.WriteString("\t")
		buf.WriteString(col.GoField)
		buf.WriteString(" ")
		buf.WriteString(col.GoType)
		buf.WriteString(" `gorm:\"")
		buf.WriteString(gormTag)
		buf.WriteString("\" json:\"")
		buf.WriteString(col.JSONName)
		buf.WriteString("\"`\n")
	}
	buf.WriteString("}\n\n")
	buf.WriteString("// TableName returns the database table name for ")
	buf.WriteString(table.ModelName)
	buf.WriteString(".\n")
	buf.WriteString("func (")
	buf.WriteString(table.ModelName)
	buf.WriteString(") TableName() string {\n")
	buf.WriteString("\treturn ")
	_, _ = fmt.Fprintf(buf, "%q", table.Name)
	buf.WriteString("\n}\n")
}

func renderAIPTableMethodBlock(buf *bytes.Buffer, table ResolvedTable) {
	visibleColumns := aipVisibleColumns(table.Columns)
	visibleColumnSet := makeStringSetFromResolved(visibleColumns)

	buf.WriteString("// AIPTable builds the aipsql.Table mapped to ")
	_, _ = fmt.Fprintf(buf, "%q", table.Name)
	buf.WriteString(".\n")
	buf.WriteString("func (")
	buf.WriteString(table.ModelName)
	buf.WriteString(") AIPTable() *aipsql.Table {\n")
	buf.WriteString("\ttable := aipsql.NewTable().WithColumns(\n")
	for _, col := range visibleColumns {
		buf.WriteString(renderAIPSQLColumn(col))
	}
	buf.WriteString("\t).Build()\n")

	if len(table.CompositeIndexes) > 0 {
		visibleIndexes := make([]IndexMeta, 0, len(table.CompositeIndexes))
		for _, index := range table.CompositeIndexes {
			if allColumnsPresent(index.Columns, visibleColumnSet) {
				visibleIndexes = append(visibleIndexes, index)
			}
		}
		if len(visibleIndexes) > 0 {
			buf.WriteString("\ttable.CompositeIndexes = []aipsql.CompositeIndex{\n")
			for _, index := range visibleIndexes {
				buf.WriteString("\t\t{\n")
				buf.WriteString("\t\t\tName: ")
				_, _ = fmt.Fprintf(buf, "%q", index.Name)
				buf.WriteString(",\n")
				buf.WriteString("\t\t\tColumns: []string{")
				for idx, col := range index.Columns {
					if idx > 0 {
						buf.WriteString(", ")
					}
					_, _ = fmt.Fprintf(buf, "%q", col)
				}
				buf.WriteString("},\n")
				buf.WriteString("\t\t},\n")
			}
			buf.WriteString("\t}\n")
		}
	}

	buf.WriteString("\treturn table\n")
	buf.WriteString("}\n")
}

func renderAIPTableWrapperBlock(buf *bytes.Buffer, table ResolvedTable) {
	buf.WriteString("// ")
	buf.WriteString(table.AIPSQLBuilder)
	buf.WriteString(" keeps compatibility with package-level AIP table builders.\n")
	buf.WriteString("func ")
	buf.WriteString(table.AIPSQLBuilder)
	buf.WriteString("() *aipsql.Table {\n")
	buf.WriteString("\treturn ")
	buf.WriteString(table.ModelName)
	buf.WriteString("{}.AIPTable()\n")
	buf.WriteString("}\n")
}

func renderAIPSQLColumn(col ResolvedColumn) string {
	var buf bytes.Buffer
	buf.WriteString("\t\taipsql.NewColumn().\n")
	buf.WriteString("\t\t\tWithFieldPath(")
	buf.WriteString(fmt.Sprintf("%q", col.FieldPath))
	buf.WriteString(").\n")
	buf.WriteString("\t\t\tWithDatabaseName(")
	buf.WriteString(fmt.Sprintf("%q", col.Name))
	buf.WriteString(").\n")

	if col.BoolType {
		buf.WriteString("\t\t\tBool().\n")
	}
	if col.KeyValue {
		buf.WriteString("\t\t\tKeyValue().\n")
	}
	if col.ImplicitFilter {
		buf.WriteString("\t\t\tFilterableImplicitly().\n")
	} else if col.Filterable {
		buf.WriteString("\t\t\tFilterable().\n")
	}
	if col.Sortable {
		buf.WriteString("\t\t\tSortable().\n")
	}
	if len(col.MatchModes) > 0 {
		buf.WriteString("\t\t\tWithMatchModes(")
		modes := make([]string, 0, len(col.MatchModes))
		for _, mode := range col.MatchModes {
			modes = append(modes, toAIPSQLMatchMode(mode))
		}
		buf.WriteString(strings.Join(modes, ", "))
		buf.WriteString(").\n")
	}
	if strings.TrimSpace(col.IndexHint) != "" {
		buf.WriteString("\t\t\tWithIndexHint(")
		buf.WriteString(fmt.Sprintf("%q", col.IndexHint))
		buf.WriteString(").\n")
	}
	buf.WriteString("\t\t\tBuild(),\n")

	return buf.String()
}

func toAIPSQLMatchMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "exact":
		return "aipsql.MatchModeExact"
	case "prefix":
		return "aipsql.MatchModePrefix"
	case "fulltext":
		return "aipsql.MatchModeFullText"
	case "contains":
		return "aipsql.MatchModeContains"
	default:
		return "aipsql.MatchModeExact"
	}
}

func buildGormTag(col ResolvedColumn) string {
	parts := []string{"column:" + col.Name}
	if col.IsPrimaryKey {
		parts = append(parts, "primaryKey")
	}
	if col.IsAutoIncrement {
		parts = append(parts, "autoIncrement")
	}
	if tsTag := timestampGormTag(col); tsTag != "" {
		parts = append(parts, tsTag)
	}
	if strings.TrimSpace(col.GormTagAppend) != "" {
		parts = append(parts, strings.TrimSpace(col.GormTagAppend))
	}
	return strings.Join(parts, ";")
}

func needsTimeImport(cols []ResolvedColumn) bool {
	for _, col := range cols {
		if strings.Contains(baseGoType(col.GoType), "time.Time") {
			return true
		}
	}
	return false
}

func needsGORMImport(cols []ResolvedColumn) bool {
	for _, col := range cols {
		if baseGoType(col.GoType) == "gorm.DeletedAt" {
			return true
		}
	}
	return false
}

func needsSoftDeleteImport(cols []ResolvedColumn) bool {
	for _, col := range cols {
		if baseGoType(col.GoType) == "soft_delete.DeletedAt" {
			return true
		}
	}
	return false
}

func aipVisibleColumns(cols []ResolvedColumn) []ResolvedColumn {
	visible := make([]ResolvedColumn, 0, len(cols))
	for _, col := range cols {
		if col.HideFromAIPTable {
			continue
		}
		visible = append(visible, col)
	}
	return visible
}

func formatSource(src []byte) ([]byte, error) {
	formatted, err := format.Source(src)
	if err != nil {
		return nil, fmt.Errorf("format generated source: %w", err)
	}
	return formatted, nil
}
