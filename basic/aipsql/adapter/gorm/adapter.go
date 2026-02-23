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

package aipsqlgorm

import (
	"database/sql"
	"strings"

	aip "github.com/codesjoy/pkg/basic/aipsql"
	"gorm.io/gorm"
)

// NamedArgs converts aipsql QueryParameters to gorm arguments backed by sql.NamedArg.
func NamedArgs(params []aip.QueryParameter) []any {
	if len(params) == 0 {
		return []any{}
	}

	args := make([]any, 0, len(params))
	for _, param := range params {
		args = append(args, sql.Named(param.Name, param.Value))
	}
	return args
}

// ApplyWhere applies one aipsql WHERE clause with named parameters onto a gorm query.
func ApplyWhere(db *gorm.DB, whereSQL string, params []aip.QueryParameter) *gorm.DB {
	if db == nil || strings.TrimSpace(whereSQL) == "" {
		return db
	}
	return db.Where(whereSQL, NamedArgs(params)...)
}

// ApplyPlan applies a QueryPlan's WHERE/ORDER/LIMIT clauses onto a gorm query.
func ApplyPlan(db *gorm.DB, plan *aip.QueryPlan) *gorm.DB {
	if db == nil || plan == nil {
		return db
	}

	if strings.TrimSpace(plan.WhereClause) != "" {
		db = db.Where(plan.WhereClause, NamedArgs(plan.Parameters)...)
	}
	if strings.TrimSpace(plan.OrderByClause) != "" {
		db = db.Order(plan.OrderByClause)
	}
	if plan.Limit > 0 {
		db = db.Limit(plan.Limit)
	}

	return db
}
