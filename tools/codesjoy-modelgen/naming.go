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
	"path/filepath"
	"strings"
	"unicode"

	"github.com/jinzhu/inflection"
)

var commonAcronyms = map[string]string{
	"api":   "API",
	"http":  "HTTP",
	"https": "HTTPS",
	"id":    "ID",
	"ip":    "IP",
	"json":  "JSON",
	"sql":   "SQL",
	"url":   "URL",
	"uuid":  "UUID",
}

// ToPascalCase converts snake_case-like names to PascalCase with common acronym rules.
func ToPascalCase(input string) string {
	parts := splitWords(input)
	if len(parts) == 0 {
		return "X"
	}

	var b strings.Builder
	for _, part := range parts {
		if part == "" {
			continue
		}
		lower := strings.ToLower(part)
		if acronym, ok := commonAcronyms[lower]; ok {
			b.WriteString(acronym)
			continue
		}
		runes := []rune(lower)
		runes[0] = unicode.ToUpper(runes[0])
		b.WriteString(string(runes))
	}

	out := b.String()
	if out == "" {
		return "X"
	}
	if unicode.IsDigit([]rune(out)[0]) {
		return "X" + out
	}
	return out
}

func splitWords(input string) []string {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return nil
	}

	var (
		parts []string
		curr  []rune
	)
	flush := func() {
		if len(curr) == 0 {
			return
		}
		parts = append(parts, string(curr))
		curr = curr[:0]
	}

	for _, r := range trimmed {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			curr = append(curr, r)
			continue
		}
		flush()
	}
	flush()

	return parts
}

func singularizeTableName(tableName string) string {
	singularTableName := strings.TrimSpace(inflection.Singular(tableName))
	if singularTableName == "" {
		singularTableName = strings.TrimSpace(tableName)
	}
	return singularTableName
}

func defaultModelName(tableName string) string {
	singularTableName := singularizeTableName(tableName)
	if singularTableName == "" {
		return "Model"
	}
	return ToPascalCase(singularTableName)
}

func defaultAIPSQLBuilderName(tableName string) string {
	return "New" + defaultModelName(tableName) + "AIPTable"
}

func generatedModelFileName(tableName string) string {
	singularTableName := singularizeTableName(tableName)
	if singularTableName == "" {
		singularTableName = "model"
	}
	return filepath.Base(singularTableName) + "_model_gen.go"
}

func dedupeFieldNames(cols []ResolvedColumn) ([]ResolvedColumn, error) {
	seen := make(map[string]int, len(cols))
	for i := range cols {
		if cols[i].GoField == "" {
			cols[i].GoField = ToPascalCase(cols[i].Name)
		}
		base := cols[i].GoField
		if base == "" {
			return nil, fmt.Errorf("empty field name for column %q", cols[i].Name)
		}

		count := seen[base]
		if count == 0 {
			seen[base] = 1
			continue
		}

		for {
			count++
			candidate := fmt.Sprintf("%s%d", base, count)
			if seen[candidate] == 0 {
				cols[i].GoField = candidate
				seen[base] = count
				seen[candidate] = 1
				break
			}
		}
	}
	return cols, nil
}
