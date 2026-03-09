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

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	aip "github.com/codesjoy/pkg/basic/aipsql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMySQLIntegration_AIPSQLExecution(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	harness, err := startDBHarness(ctx, aip.SQLDialectMySQL)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, harness.close(context.Background()))
	})

	require.NoError(t, prepareSchemaAndSeed(ctx, harness))
	table := buildIntegrationTable()

	t.Run("exact and composite index", func(t *testing.T) {
		filter, err := aip.ParseFilter(
			`created_at>"2025-01-02T00:00:00Z" AND user_id="user_007" AND status="active"`,
		)
		require.NoError(t, err)

		whereClause, params, err := table.WhereClauseWithOptions(
			filter,
			"p_",
			aip.WhereClauseOptions{
				Dialect:                          aip.SQLDialectMySQL,
				StrictMode:                       true,
				EnableCompositeIndexOptimization: true,
			},
		)
		require.NoError(t, err)

		sqlText := "SELECT id FROM aip_items WHERE " + whereClause + " ORDER BY created_at DESC, id LIMIT 25"
		ids, err := queryIDs(ctx, harness, sqlText, params)
		require.NoError(t, err)
		require.NotEmpty(t, ids)

		plan, err := explainPlanSummary(ctx, harness, sqlText, params)
		require.NoError(t, err)
		assert.True(
			t,
			plan.HasIndex("idx_items_status_user_created_id"),
			"expected key idx_items_status_user_created_id, explain=%s raw=%s",
			plan.DebugString(),
			plan.RawPlan,
		)
		assert.True(
			t,
			plan.MySQLHasAccessType(mysqlAccessTypeRef, mysqlAccessTypeRange),
			"expected mysql access type ref/range, explain=%s raw=%s",
			plan.DebugString(),
			plan.RawPlan,
		)
	})

	t.Run("prefix and fulltext index", func(t *testing.T) {
		prefixFilter, err := aip.ParseFilter(`title_prefix:"go-distributed-systems-"`)
		require.NoError(t, err)
		prefixWhere, prefixParams, err := table.WhereClauseWithOptions(
			prefixFilter,
			"p_",
			aip.WhereClauseOptions{
				Dialect:    aip.SQLDialectMySQL,
				StrictMode: true,
			},
		)
		require.NoError(t, err)

		prefixSQL := "SELECT id FROM aip_items WHERE " + prefixWhere + " ORDER BY id LIMIT 50"
		ids, err := queryIDs(ctx, harness, prefixSQL, prefixParams)
		require.NoError(t, err)
		require.NotEmpty(t, ids)
		prefixPlan, err := explainPlanSummary(ctx, harness, prefixSQL, prefixParams)
		require.NoError(t, err)
		assert.True(
			t,
			prefixPlan.HasIndex("idx_items_title_prefix"),
			"expected key idx_items_title_prefix, explain=%s raw=%s",
			prefixPlan.DebugString(),
			prefixPlan.RawPlan,
		)
		assert.True(
			t,
			prefixPlan.MySQLHasAccessType(mysqlAccessTypeRange),
			"expected mysql range scan for prefix query, explain=%s raw=%s",
			prefixPlan.DebugString(),
			prefixPlan.RawPlan,
		)

		fulltextFilter, err := aip.ParseFilter(`title_fulltext:"distributed systems"`)
		require.NoError(t, err)
		fulltextWhere, fulltextParams, err := table.WhereClauseWithOptions(
			fulltextFilter,
			"p_",
			aip.WhereClauseOptions{
				Dialect:    aip.SQLDialectMySQL,
				StrictMode: true,
			},
		)
		require.NoError(t, err)

		fulltextSQL := "SELECT id FROM aip_items WHERE " + fulltextWhere + " ORDER BY id LIMIT 50"
		fulltextIDs, err := queryIDs(ctx, harness, fulltextSQL, fulltextParams)
		require.NoError(t, err)
		require.NotEmpty(t, fulltextIDs)
		fulltextPlan, err := explainPlanSummary(ctx, harness, fulltextSQL, fulltextParams)
		require.NoError(t, err)
		assert.True(
			t,
			fulltextPlan.HasIndex("idx_items_title_fts"),
			"expected key idx_items_title_fts, explain=%s raw=%s",
			fulltextPlan.DebugString(),
			fulltextPlan.RawPlan,
		)
		assert.True(
			t,
			fulltextPlan.MySQLHasAccessType(mysqlAccessTypeFulltext),
			"expected mysql fulltext access type, explain=%s raw=%s",
			fulltextPlan.DebugString(),
			fulltextPlan.RawPlan,
		)
	})

	t.Run("query planner seek pagination", func(t *testing.T) {
		planner, err := aip.NewQueryPlanner(aip.TableSpec{
			Table: table,
			DefaultOrder: []aip.OrderBy{
				{FieldPath: aip.NewFieldPath("created_at"), Descending: true},
			},
			TieBreakerFieldPath: aip.NewFieldPath("id"),
		}, aip.QueryPlannerOptions{
			Dialect:                          aip.SQLDialectMySQL,
			StrictMode:                       true,
			EnableCompositeIndexOptimization: true,
			DefaultPageSize:                  6,
			MaxPageSize:                      50,
		})
		require.NoError(t, err)

		firstPlan, err := planner.PlanList(ctx, aip.QueryRequest{
			Filter:   `status="active" AND user_id="user_007"`,
			PageSize: 6,
		})
		require.NoError(t, err)
		firstSQL := buildListSQL("id, created_at", "aip_items", firstPlan)

		firstRows, err := queryItemRows(ctx, harness, firstSQL, firstPlan.Parameters)
		require.NoError(t, err)
		require.Len(t, firstRows, 6)

		pageToken, err := firstPlan.NextPageToken(firstRows)
		require.NoError(t, err)

		nextPlan, err := planner.PlanList(ctx, aip.QueryRequest{
			Filter:    `status="active" AND user_id="user_007"`,
			PageSize:  6,
			PageToken: pageToken,
		})
		require.NoError(t, err)
		nextSQL := buildListSQL("id, created_at", "aip_items", nextPlan)
		nextRows, err := queryItemRows(ctx, harness, nextSQL, nextPlan.Parameters)
		require.NoError(t, err)
		require.NotEmpty(t, nextRows)
		assert.False(t, idsOverlap(firstRows, nextRows))
		planText, err := explainPlanSummary(ctx, harness, firstSQL, firstPlan.Parameters)
		require.NoError(t, err)
		assert.True(
			t,
			planText.HasIndex("idx_items_status_user_created_id"),
			"expected key idx_items_status_user_created_id for seek first page, explain=%s raw=%s",
			planText.DebugString(),
			planText.RawPlan,
		)
		assert.True(
			t,
			planText.MySQLHasAccessType(mysqlAccessTypeRef, mysqlAccessTypeRange),
			"expected mysql ref/range access for seek first page, explain=%s raw=%s",
			planText.DebugString(),
			planText.RawPlan,
		)
	})
}
