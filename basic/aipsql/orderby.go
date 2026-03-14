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

	participle "github.com/alecthomas/participle/v2"
	"github.com/alecthomas/participle/v2/lexer"
)

var (
	orderByLexer = lexer.MustSimple([]lexer.SimpleRule{
		{Name: "Spaces", Pattern: `[ ]+`},
		{Name: "String", Pattern: `[a-zA-Z_][a-zA-Z_0-9]*`},
		{Name: "QuotedString", Pattern: "`(``|[^`])*`"},
		{Name: "Operators", Pattern: "[.,]"},
	})

	orderByParser = participle.MustBuild[orderByList](participle.Lexer(orderByLexer))
)

// OrderBy represents a part of an AIP-132 order_by clause.
type OrderBy struct {
	// The field path. This is the path of the field in the
	// resource message that the AIP-132 List RPC is listing.
	FieldPath FieldPath
	// Whether the field should be sorted in descending order.
	Descending bool
}

// FieldPath represents the path to a field in a message.
//
// For example, for the given message:
//
//	message MyThing {
//	   message Bar {
//	       string foobar = 2;
//	   }
//	   string foo = 1;
//	   Bar bar = 2;
//	   map<string, Bar> named_bars = 3;
//	}
//
// Some valid paths would be: foo, bar.foobar and
// named_bars.`bar-key`.foobar.
type FieldPath struct {
	// The field path as its segments.
	segments []string

	// The canonical representation of the field path.
	canonical string
}

// NewFieldPath initializes a new field path with the given segments.
func NewFieldPath(segments ...string) FieldPath {
	var builder strings.Builder
	for _, segment := range segments {
		if builder.Len() > 0 {
			builder.WriteString(".")
		}
		if isStringLiteral(segment) {
			builder.WriteString(segment)
			continue
		}
		builder.WriteString("`")
		builder.WriteString(strings.ReplaceAll(segment, "`", "``"))
		builder.WriteString("`")
	}
	return FieldPath{
		segments:  segments,
		canonical: builder.String(),
	}
}

// Equals returns iff two field paths refer to exactly the
// same field.
func (f FieldPath) Equals(other FieldPath) bool {
	return f.canonical == other.canonical
}

// String returns a canonical representation of the field path,
// following AIP-132 / AIP-161 syntax.
func (f FieldPath) String() string {
	return f.canonical
}

// ParseOrderBy parses an AIP-132 order_by list. The method validates the
// syntax is correct and each identifier appears at most once, but
// it does not validate the identifiers themselves are valid.
func ParseOrderBy(text string) ([]OrderBy, error) {
	if strings.Trim(text, " ") == "" {
		return nil, nil
	}

	expr, err := parseOrderByList(text)
	if err != nil {
		return nil, err
	}

	result := make([]OrderBy, 0, len(expr.SortOrder))
	uniqueFieldPaths := make(map[string]struct{}, len(expr.SortOrder))
	for _, clause := range expr.SortOrder {
		orderBy := OrderBy{
			FieldPath:  NewFieldPath(clause.FieldPath.Path()...),
			Descending: clause.Order.Desc,
		}
		if err := trackUniqueOrderByField(uniqueFieldPaths, orderBy.FieldPath); err != nil {
			return nil, err
		}
		result = append(result, orderBy)
	}

	return result, nil
}

func parseOrderByList(text string) (*orderByList, error) {
	expr, err := orderByParser.ParseString("", text)
	if err == nil {
		return expr, nil
	}

	// participle includes "lexer: " in some syntax errors. Normalize the
	// prefix so callers get stable error messages across parser versions.
	message := strings.Replace(err.Error(), "lexer: ", "", 1)
	return nil, fmt.Errorf("syntax error: %s", message)
}

func trackUniqueOrderByField(seen map[string]struct{}, fieldPath FieldPath) error {
	key := fieldPath.String()
	if _, ok := seen[key]; ok {
		return fmt.Errorf("field appears multiple times: %q", fieldPath)
	}
	seen[key] = struct{}{}
	return nil
}

// MergeWithDefaultOrder merges the specified order with the given
// defaultOrder. The merge occurs as follows:
//   - Ordering specified in `order` takes precedence.
//   - For columns not specified in the `order` that appear in `defaultOrder`,
//     ordering is applied in the order they apply in defaultOrder.
func MergeWithDefaultOrder(defaultOrder []OrderBy, order []OrderBy) []OrderBy {
	result := make([]OrderBy, 0, len(order)+len(defaultOrder))
	seenColumns := make(map[string]struct{})
	for _, entry := range order {
		result = append(result, entry)
		seenColumns[entry.FieldPath.String()] = struct{}{}
	}
	for _, entry := range defaultOrder {
		if _, ok := seenColumns[entry.FieldPath.String()]; ok {
			continue
		}
		result = append(result, entry)
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
	for i, entry := range order {
		if i > 0 {
			result.WriteString(", ")
		}
		clause, err := t.buildOrderByFieldClause(entry, seenColumns)
		if err != nil {
			return "", err
		}
		result.WriteString(clause)
	}
	return result.String(), nil
}

func (t *Table) buildOrderByFieldClause(
	order OrderBy,
	seenColumns map[string]struct{},
) (string, error) {
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
func findMatchingOrderByIndex(fields []OrderByField, indexes []CompositeIndex) *CompositeIndex {
	for i := range indexes {
		if indexMatchesOrderBy(fields, indexes[i]) {
			return &indexes[i]
		}
	}
	return nil
}

func indexMatchesOrderBy(fields []OrderByField, index CompositeIndex) bool {
	if len(fields) > len(index.Columns) {
		return false
	}
	for i, field := range fields {
		if field.Column.databaseName != index.Columns[i] {
			return false
		}
	}
	return true
}

func isStringLiteral(segment string) bool {
	if len(segment) == 0 {
		return false
	}
	if !isStringLiteralFirstChar(segment[0]) {
		return false
	}
	for i := 1; i < len(segment); i++ {
		if !isStringLiteralChar(segment[i]) {
			return false
		}
	}
	return true
}

func isStringLiteralFirstChar(ch byte) bool {
	return ch == '_' || (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z')
}

func isStringLiteralChar(ch byte) bool {
	return isStringLiteralFirstChar(ch) || (ch >= '0' && ch <= '9')
}

type orderByList struct {
	SortOrder []*orderByClause `parser:"@@ ( Spaces? ',' @@ )* Spaces?"`
}

type orderByClause struct {
	FieldPath *fieldPath `parser:"@@"`
	Order     *order     `parser:"@@"`
}

type order struct {
	Desc bool `parser:"@( Spaces 'desc' )?"`
}

type fieldPath struct {
	Segments []*segment `parser:"Spaces? @@ ( '.' @@ )*"`
}

// Path returns the field path as a list of path segments.
func (f *fieldPath) Path() []string {
	result := make([]string, 0, len(f.Segments))
	for _, segment := range f.Segments {
		result = append(result, segment.Value())
	}
	return result
}

type segment struct {
	StringValue  *string `parser:"@String"`
	QuotedString *string `parser:"| @QuotedString"`
}

func (s *segment) Value() string {
	if s.QuotedString != nil {
		unquotedString := (*s.QuotedString)[1 : len(*s.QuotedString)-1]
		return strings.ReplaceAll(unquotedString, "``", "`")
	}
	if s.StringValue != nil {
		return *s.StringValue
	}
	panic("invalid syntax")
}
