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
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// NextPageToken returns the next page token after the caller has executed the
// current plan and collected the current page rows.
func (p *QueryPlan) NextPageToken(rows any) (string, error) {
	if p == nil {
		return "", fmt.Errorf("query plan is required")
	}

	rowsValue := reflect.ValueOf(rows)
	if !rowsValue.IsValid() ||
		(rowsValue.Kind() != reflect.Slice && rowsValue.Kind() != reflect.Array) {
		return "", fmt.Errorf("rows must be a slice or array")
	}

	rowCount := rowsValue.Len()
	if rowCount == 0 {
		return "", nil
	}
	if p.Limit > 0 && rowCount < p.Limit {
		return "", nil
	}

	switch p.paginationMode {
	case PaginationModeOffset:
		return EncodeOffsetPageToken(p.Offset + rowCount), nil
	case "", PaginationModeSeek:
		return p.nextSeekPageToken(rowsValue.Index(rowCount - 1))
	default:
		return "", fmt.Errorf("unsupported pagination mode %q", p.paginationMode)
	}
}

func (p *QueryPlan) nextSeekPageToken(lastRow reflect.Value) (string, error) {
	if p.tieBreakerFieldPath.String() == "" {
		return "", fmt.Errorf("seek pagination metadata is unavailable")
	}

	structValue, err := dereferenceRowValue(lastRow)
	if err != nil {
		return "", err
	}

	sortValues := make([]string, 0, len(p.sortOrder))
	for _, entry := range p.sortOrder {
		value, err := p.seekTokenValue(structValue, entry.FieldPath)
		if err != nil {
			return "", err
		}
		sortValues = append(sortValues, value)
	}

	tieBreakerValue, err := p.seekTokenValue(structValue, p.tieBreakerFieldPath)
	if err != nil {
		return "", err
	}

	return EncodeSeekPageToken(SeekPageToken{
		SortValues:      sortValues,
		TieBreakerValue: tieBreakerValue,
	})
}

func (p *QueryPlan) seekTokenValue(row reflect.Value, fieldPath FieldPath) (string, error) {
	value, err := p.resolveFieldPathValue(row, fieldPath)
	if err != nil {
		return "", err
	}
	text, err := stringifySeekTokenValue(value)
	if err != nil {
		return "", fmt.Errorf("field %q: %w", fieldPath.String(), err)
	}
	return text, nil
}

// resolveFieldPathValue resolves a field path against a struct value using a 3-tier
// resolution strategy for each segment:
//
//  1. GORM tag: Check if any struct field has gorm:"column:<name>" matching the segment
//     (or the leaf database column name). This handles GORM model mappings.
//
//  2. JSON tag: Check if any struct field has json:"<name>" matching the segment.
//     This handles the common case where API field names differ from Go field names.
//
//  3. Go field name: Convert the segment from snake_case/camelCase to Go exported name
//     (e.g., "created_at" → "CreatedAt", "user_id" → "UserID") using initialism-aware
//     conversion via fieldPathSegmentToGoName.
func (p *QueryPlan) resolveFieldPathValue(
	row reflect.Value,
	fieldPath FieldPath,
) (reflect.Value, error) {
	if len(fieldPath.segments) == 0 {
		return reflect.Value{}, fmt.Errorf("field path is required")
	}

	current := row
	leafDatabaseName := p.databaseColumnName(fieldPath)
	for i, segment := range fieldPath.segments {
		last := i == len(fieldPath.segments)-1

		structValue, err := dereferenceRowValue(current)
		if err != nil {
			return reflect.Value{}, fmt.Errorf("field %q: %w", fieldPath.String(), err)
		}

		gormCandidates := []string{segment}
		if last && leafDatabaseName != "" && leafDatabaseName != segment {
			gormCandidates = append([]string{leafDatabaseName}, gormCandidates...)
		}

		current, err = selectStructField(structValue, segment, gormCandidates)
		if err != nil {
			return reflect.Value{}, fmt.Errorf("field %q: %w", fieldPath.String(), err)
		}
	}

	return current, nil
}

func (p *QueryPlan) databaseColumnName(fieldPath FieldPath) string {
	if p.table == nil {
		return ""
	}
	column, err := p.table.SortableColumnByFieldPath(fieldPath)
	if err != nil || column == nil {
		return ""
	}
	return column.databaseName
}

// dereferenceRowValue unwraps pointer layers to reach the underlying struct value.
// Returns an error if a nil pointer is encountered or the final value is not a struct.
func dereferenceRowValue(value reflect.Value) (reflect.Value, error) {
	current := value
	for current.Kind() == reflect.Pointer {
		if current.IsNil() {
			return reflect.Value{}, fmt.Errorf("row element must not be nil")
		}
		current = current.Elem()
	}
	if current.Kind() != reflect.Struct {
		return reflect.Value{}, fmt.Errorf("row elements must be structs or pointers to structs")
	}
	return current, nil
}

// selectStructField finds a struct field matching the given segment using the
// 3-tier resolution strategy: GORM tag → JSON tag → Go field name.
// The gormCandidates list is checked first to support GORM column name mapping.
func selectStructField(
	structValue reflect.Value,
	segment string,
	gormCandidates []string,
) (reflect.Value, error) {
	structType := structValue.Type()

	for i := 0; i < structType.NumField(); i++ {
		field := structType.Field(i)
		if !field.IsExported() {
			continue
		}
		if matchesCandidate(parseGORMColumnName(field.Tag.Get("gorm")), gormCandidates) {
			return structValue.Field(i), nil
		}
	}

	for i := 0; i < structType.NumField(); i++ {
		field := structType.Field(i)
		if !field.IsExported() {
			continue
		}
		if jsonName := parseJSONFieldName(field.Tag.Get("json")); jsonName == segment {
			return structValue.Field(i), nil
		}
	}

	goFieldName := fieldPathSegmentToGoName(segment)
	for i := 0; i < structType.NumField(); i++ {
		field := structType.Field(i)
		if !field.IsExported() {
			continue
		}
		if field.Name == goFieldName {
			return structValue.Field(i), nil
		}
	}

	return reflect.Value{}, fmt.Errorf("no matching field for segment %q", segment)
}

func matchesCandidate(name string, candidates []string) bool {
	if name == "" {
		return false
	}
	for _, candidate := range candidates {
		if candidate == name {
			return true
		}
	}
	return false
}

// parseGORMColumnName extracts the column name from a GORM struct tag.
// For example, parseGORMColumnName(`gorm:"column:user_id;type:varchar"`) returns "user_id".
func parseGORMColumnName(tag string) string {
	for _, part := range strings.Split(tag, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		key, value, ok := strings.Cut(part, ":")
		if !ok {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(key), "column") {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

// parseJSONFieldName extracts the field name from a JSON struct tag.
// Returns "" for ignored fields (json:"-") and empty tags.
func parseJSONFieldName(tag string) string {
	if tag == "" {
		return ""
	}
	name := strings.TrimSpace(strings.Split(tag, ",")[0])
	if name == "-" {
		return ""
	}
	return name
}

// fieldPathSegmentToGoName converts a snake_case or kebab-case segment to a Go exported
// field name using title-case conversion. For example:
//
//	"created_at" → "CreatedAt"
//	"user_id"    → "UserID"        (ID is a common initialism)
//	"html_content" → "HTMLContent" (HTML is a common initialism)
//	"my-field"   → "MyField"
//
// Common initialisms (ID, URL, HTTP, etc.) are kept fully uppercase as per Go convention.
func fieldPathSegmentToGoName(segment string) string {
	parts := strings.FieldsFunc(segment, func(r rune) bool {
		return r == '_' || r == '-'
	})
	if len(parts) == 0 {
		return ""
	}

	var builder strings.Builder
	for _, part := range parts {
		if part == "" {
			continue
		}
		upper := strings.ToUpper(part)
		if isCommonInitialism(upper) {
			builder.WriteString(upper)
			continue
		}

		first, size := utf8.DecodeRuneInString(part)
		if first == utf8.RuneError && size == 0 {
			continue
		}
		builder.WriteRune(unicode.ToUpper(first))
		builder.WriteString(part[size:])
	}
	return builder.String()
}

func isCommonInitialism(value string) bool {
	switch value {
	case "ACL", "API", "ASCII", "CPU", "CSS", "DNS", "EOF", "GUID", "HTML", "HTTP", "HTTPS", "ID",
		"IP", "JSON", "QPS", "RAM", "RPC", "SLA", "SMTP", "SQL", "SSH", "TCP", "TLS", "TTL", "UDP",
		"UI", "UID", "UUID", "URI", "URL", "UTF8", "VM", "XML":
		return true
	default:
		return false
	}
}

// stringifySeekTokenValue converts a reflected value to its string representation for
// inclusion in a seek page token. Supports:
//   - string, bool, int/uint variants, float32/float64, []byte
//   - time.Time → RFC3339Nano format
//   - Other types → fmt.Sprint fallback
func stringifySeekTokenValue(value reflect.Value) (string, error) {
	current := value
	for current.Kind() == reflect.Pointer {
		if current.IsNil() {
			return "", fmt.Errorf("value must not be nil")
		}
		current = current.Elem()
	}

	switch current.Kind() {
	case reflect.String:
		return current.String(), nil
	case reflect.Bool:
		return strconv.FormatBool(current.Bool()), nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(current.Int(), 10), nil
	case reflect.Uint,
		reflect.Uint8,
		reflect.Uint16,
		reflect.Uint32,
		reflect.Uint64,
		reflect.Uintptr:
		return strconv.FormatUint(current.Uint(), 10), nil
	case reflect.Float32:
		return strconv.FormatFloat(current.Float(), 'g', -1, 32), nil
	case reflect.Float64:
		return strconv.FormatFloat(current.Float(), 'g', -1, 64), nil
	case reflect.Slice:
		if current.Type().Elem().Kind() == reflect.Uint8 {
			return string(current.Bytes()), nil
		}
	}

	if current.CanInterface() {
		if timeValue, ok := current.Interface().(time.Time); ok {
			return timeValue.Format(time.RFC3339Nano), nil
		}
		return fmt.Sprint(current.Interface()), nil
	}

	return "", fmt.Errorf("unsupported value kind %s", current.Kind())
}
