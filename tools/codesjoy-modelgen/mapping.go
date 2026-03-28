// Copyright 2022 The codesjoy Authors.
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

package main

import (
	"fmt"
	"strings"
)

const (
	timestampRoleNone    = "none"
	timestampRoleCreated = "created"
	timestampRoleUpdated = "updated"
	timestampRoleDeleted = "deleted"
)

func inferGoType(col ColumnMeta) string {
	dataType := strings.ToLower(strings.TrimSpace(col.DataType))
	rawType := strings.ToLower(strings.TrimSpace(col.RawType))

	unsigned := strings.Contains(rawType, "unsigned")
	switch {
	case dataType == "bool" || dataType == "boolean":
		return applyNullable("bool", col.Nullable)
	case dataType == "tinyint" && strings.HasPrefix(rawType, "tinyint(1"):
		return applyNullable("bool", col.Nullable)
	case strings.Contains(dataType, "bigint"):
		if unsigned {
			return applyNullable("uint64", col.Nullable)
		}
		return applyNullable("int64", col.Nullable)
	case dataType == "int" || dataType == "integer" || dataType == "mediumint":
		if unsigned {
			return applyNullable("uint32", col.Nullable)
		}
		return applyNullable("int32", col.Nullable)
	case dataType == "smallint":
		if unsigned {
			return applyNullable("uint16", col.Nullable)
		}
		return applyNullable("int16", col.Nullable)
	case dataType == "tinyint":
		if unsigned {
			return applyNullable("uint8", col.Nullable)
		}
		return applyNullable("int8", col.Nullable)
	case dataType == "float" || dataType == "double" || dataType == "real":
		return applyNullable("float64", col.Nullable)
	case dataType == "decimal" || dataType == "numeric":
		return applyNullable("float64", col.Nullable)
	case isDateTimeLikeColumnType(col):
		return applyNullable("time.Time", col.Nullable)
	case dataType == "bytea" || strings.Contains(dataType, "blob") ||
		strings.Contains(dataType, "binary"):
		return "[]byte"
	default:
		return applyNullable("string", col.Nullable)
	}
}

func applyNullable(goType string, nullable bool) string {
	if !nullable {
		return goType
	}
	if strings.HasPrefix(goType, "[]") || strings.HasPrefix(goType, "map[") {
		return goType
	}
	if strings.HasPrefix(goType, "*") {
		return goType
	}
	return "*" + goType
}

func baseGoType(goType string) string {
	return strings.TrimPrefix(goType, "*")
}

func isTextualColumn(col ColumnMeta) bool {
	dataType := strings.ToLower(strings.TrimSpace(col.DataType))
	rawType := strings.ToLower(strings.TrimSpace(col.RawType))

	if isKnownTextualType(dataType) || isKnownTextualType(rawType) {
		return true
	}
	if strings.HasPrefix(rawType, "varchar(") || strings.HasPrefix(rawType, "char(") {
		return true
	}
	return false
}

func isKnownTextualType(kind string) bool {
	switch kind {
	case "char",
		"character",
		"varchar",
		"character varying",
		"bpchar",
		"text",
		"tinytext",
		"mediumtext",
		"longtext",
		"citext",
		"uuid",
		"json",
		"jsonb",
		"name":
		return true
	default:
		return false
	}
}

func isScalarGoType(goType string) bool {
	base := baseGoType(goType)
	return base != "[]byte"
}

func timestampRoleByColumnName(name string) string {
	normalized := normalizeColumnName(name)
	switch normalized {
	case "createdat", "createat", "created", "create":
		return timestampRoleCreated
	case "updatedat", "updateat", "updated", "update":
		return timestampRoleUpdated
	case "deletedat", "deleteat", "deleted", "delete":
		return timestampRoleDeleted
	default:
		return timestampRoleNone
	}
}

func normalizeColumnName(name string) string {
	lower := strings.ToLower(strings.TrimSpace(name))
	replacer := strings.NewReplacer("_", "", "-", "")
	return replacer.Replace(lower)
}

func isIntegerLikeColumnType(col ColumnMeta) bool {
	dataType := strings.ToLower(strings.TrimSpace(col.DataType))
	rawType := strings.ToLower(strings.TrimSpace(col.RawType))
	joined := dataType + " " + rawType

	intMarkers := []string{
		"int",
		"serial",
		"bigserial",
		"smallserial",
		"int2",
		"int4",
		"int8",
	}
	for _, marker := range intMarkers {
		if strings.Contains(joined, marker) {
			return true
		}
	}
	return false
}

func isDateTimeLikeColumnType(col ColumnMeta) bool {
	dataType := strings.ToLower(strings.TrimSpace(col.DataType))
	rawType := strings.ToLower(strings.TrimSpace(col.RawType))
	joined := dataType + " " + rawType

	timeMarkers := []string{
		"date",
		"datetime",
		"timestamp",
		"timestamptz",
		"time",
	}
	for _, marker := range timeMarkers {
		if strings.Contains(joined, marker) {
			return true
		}
	}
	return false
}

func resolveTimestampType(
	col ColumnMeta,
	mode string,
	role string,
) (
	goType string,
	useInt bool,
	softDeleteKind string,
	warning string,
	err error,
) {
	if role == timestampRoleNone {
		return inferGoType(col), false, softDeleteKindNone, "", nil
	}

	normalizedMode := strings.ToLower(strings.TrimSpace(mode))
	if err := validateTimestampMode(normalizedMode); err != nil {
		return "", false, softDeleteKindNone, "", err
	}

	if role == timestampRoleDeleted {
		return resolveDeletedAtType(col)
	}

	if isDateTimeLikeColumnType(col) {
		return applyNullable("time.Time", col.Nullable), false, softDeleteKindNone, "", nil
	}

	if isIntegerLikeColumnType(col) {
		return applyNullable("int64", col.Nullable), true, softDeleteKindNone, "", nil
	}

	fallbackType := inferGoType(col)
	return fallbackType, false, softDeleteKindNone, fmt.Sprintf(
		"column %s physical type %q is neither datetime-like nor integer-like, fallback to %s",
		col.Name,
		col.DataType,
		fallbackType,
	), nil
}

func resolveDeletedAtType(
	col ColumnMeta,
) (goType string, useInt bool, softDeleteKind string, warning string, err error) {
	if isDateTimeLikeColumnType(col) {
		return "gorm.DeletedAt", false, softDeleteKindGORM, "", nil
	}
	if isIntegerLikeColumnType(col) {
		return "soft_delete.DeletedAt", true, softDeleteKindPlugin, "", nil
	}
	return "gorm.DeletedAt", false, softDeleteKindGORM, fmt.Sprintf(
		"column %s physical type %q is neither integer-like nor datetime-like, fallback to gorm.DeletedAt",
		col.Name,
		col.DataType,
	), nil
}

func defaultGoFieldForRole(col ColumnMeta, role string) string {
	switch role {
	case timestampRoleCreated:
		return "CreatedAt"
	case timestampRoleUpdated:
		return "UpdatedAt"
	case timestampRoleDeleted:
		return "DeletedAt"
	default:
		return ToPascalCase(col.Name)
	}
}

func timestampGormTag(col ResolvedColumn) string {
	switch col.TimestampRole {
	case timestampRoleCreated:
		return timestampTagByMode(
			col.UseIntTimestamp,
			col.TimestampMode,
			"autoCreateTime",
			"autoCreateTime:milli",
			"autoCreateTime:nano",
		)
	case timestampRoleUpdated:
		return timestampTagByMode(
			col.UseIntTimestamp,
			col.TimestampMode,
			"autoUpdateTime",
			"autoUpdateTime:milli",
			"autoUpdateTime:nano",
		)
	case timestampRoleDeleted:
		switch col.SoftDeleteKind {
		case softDeleteKindGORM:
			return "index"
		case softDeleteKindPlugin:
			switch strings.ToLower(strings.TrimSpace(col.TimestampMode)) {
			case timestampModeUnixMilli:
				return "softDelete:milli"
			case timestampModeUnixNano:
				return "softDelete:nano"
			default:
				return "softDelete"
			}
		default:
			return ""
		}
	default:
		return ""
	}
}

func timestampTagByMode(
	useIntTimestamp bool,
	mode string,
	defaultTag string,
	milliTag string,
	nanoTag string,
) string {
	if !useIntTimestamp {
		return defaultTag
	}
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case timestampModeUnixMilli:
		return milliTag
	case timestampModeUnixNano:
		return nanoTag
	default:
		return defaultTag
	}
}
