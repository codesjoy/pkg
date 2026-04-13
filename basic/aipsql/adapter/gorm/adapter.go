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

// ApplyPlan applies a QueryPlan's WHERE/ORDER/LIMIT/OFFSET clauses onto a gorm query.
//
// Clause application order:
//  1. WHERE — filter predicate with named parameters
//  2. ORDER BY — sort specification
//  3. LIMIT — maximum number of rows to return
//  4. OFFSET — number of rows to skip (only used for offset pagination mode)
//
// This order matches SQL semantics and ensures GORM chains the clauses correctly.
// Returns the original db if either argument is nil.
func ApplyPlan(db *gorm.DB, plan *aip.QueryPlan) *gorm.DB {
	if db == nil || plan == nil {
		return db
	}

	// Step 1: Apply WHERE clause with parameterized named args.
	if strings.TrimSpace(plan.WhereClause) != "" {
		db = db.Where(plan.WhereClause, NamedArgs(plan.Parameters)...)
	}
	// Step 2: Apply ORDER BY clause (already validated by the planner).
	if strings.TrimSpace(plan.OrderByClause) != "" {
		db = db.Order(plan.OrderByClause)
	}
	// Step 3: Apply LIMIT (page size, already capped by planner).
	if plan.Limit > 0 {
		db = db.Limit(plan.Limit)
	}
	// Step 4: Apply OFFSET (only non-zero for offset pagination mode).
	if plan.Offset > 0 {
		db = db.Offset(plan.Offset)
	}

	return db
}
