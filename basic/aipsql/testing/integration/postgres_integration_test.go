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
	"fmt"
	"testing"
	"time"

	aip "github.com/codesjoy/pkg/basic/aipsql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPostgresIntegration_AIPSQLExecution(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	harness, err := startDBHarness(ctx, aip.SQLDialectPostgres)
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
				Dialect:                          aip.SQLDialectPostgres,
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
			"expected index idx_items_status_user_created_id, plan=%s raw=%s",
			plan.DebugString(),
			plan.RawPlan,
		)
		assert.True(
			t,
			plan.PostgresHasIndexScan(),
			"expected postgres index scan node, plan=%s raw=%s",
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
				Dialect:    aip.SQLDialectPostgres,
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
			"expected prefix index idx_items_title_prefix, plan=%s raw=%s",
			prefixPlan.DebugString(),
			prefixPlan.RawPlan,
		)
		assert.True(
			t,
			prefixPlan.PostgresHasIndexScan(),
			"expected postgres index scan node for prefix query, plan=%s raw=%s",
			prefixPlan.DebugString(),
			prefixPlan.RawPlan,
		)

		fulltextFilter, err := aip.ParseFilter(`title_fulltext:"distributed systems"`)
		require.NoError(t, err)
		fulltextWhere, fulltextParams, err := table.WhereClauseWithOptions(
			fulltextFilter,
			"p_",
			aip.WhereClauseOptions{
				Dialect:    aip.SQLDialectPostgres,
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
			"expected fulltext index idx_items_title_fts, plan=%s raw=%s",
			fulltextPlan.DebugString(),
			fulltextPlan.RawPlan,
		)
		assert.True(
			t,
			fulltextPlan.PostgresHasIndexScan(),
			"expected postgres index scan node for fulltext query, plan=%s raw=%s",
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
			Dialect:                          aip.SQLDialectPostgres,
			StrictMode:                       true,
			EnableCompositeIndexOptimization: true,
			DefaultPageSize:                  5,
			MaxPageSize:                      50,
		})
		require.NoError(t, err)

		baseFilter := `status="active" AND user_id="user_007"`

		firstPlan, err := planner.PlanList(ctx, aip.QueryRequest{
			Filter:   baseFilter,
			PageSize: 5,
		})
		require.NoError(t, err)
		firstSQL := buildListSQL("id, created_at", "aip_items", firstPlan)

		firstRows, err := queryItemRows(ctx, harness, firstSQL, firstPlan.Parameters)
		require.NoError(t, err)
		require.Len(t, firstRows, 5)

		last := firstRows[len(firstRows)-1]
		pageToken, err := firstPlan.NextPageToken(firstRows)
		require.NoError(t, err)

		nextPlan, err := planner.PlanList(ctx, aip.QueryRequest{
			Filter:    baseFilter,
			PageSize:  5,
			PageToken: pageToken,
		})
		require.NoError(t, err)
		assert.Contains(t, nextPlan.WhereClause, "created_at < @p_seek_0")
		nextSQL := buildListSQL("id, created_at", "aip_items", nextPlan)

		nextRows, err := queryItemRows(ctx, harness, nextSQL, nextPlan.Parameters)
		require.NoError(t, err)
		require.NotEmpty(t, nextRows)

		assert.False(t, idsOverlap(firstRows, nextRows))
		for _, row := range nextRows {
			if row.CreatedAt == last.CreatedAt {
				assert.Greater(t, row.ID, last.ID)
			} else {
				assert.Less(t, row.CreatedAt, last.CreatedAt)
			}
		}

		firstPlanExplain, err := explainPlanSummary(
			ctx,
			harness,
			firstSQL,
			firstPlan.Parameters,
		)
		require.NoError(t, err)
		assert.True(
			t,
			firstPlanExplain.HasIndex("idx_items_status_user_created_id"),
			"expected index idx_items_status_user_created_id for seek first page, plan=%s raw=%s",
			firstPlanExplain.DebugString(),
			firstPlanExplain.RawPlan,
		)
		assert.True(
			t,
			firstPlanExplain.PostgresHasIndexScan(),
			"expected postgres index scan node for seek first page, plan=%s raw=%s",
			firstPlanExplain.DebugString(),
			firstPlanExplain.RawPlan,
		)
	})

	t.Run("offset and seek return the same page", func(t *testing.T) {
		planner, err := aip.NewQueryPlanner(aip.TableSpec{
			Table: table,
			DefaultOrder: []aip.OrderBy{
				{FieldPath: aip.NewFieldPath("created_at"), Descending: true},
			},
			TieBreakerFieldPath: aip.NewFieldPath("id"),
		}, aip.QueryPlannerOptions{
			Dialect:         aip.SQLDialectPostgres,
			StrictMode:      true,
			DefaultPageSize: 20,
			MaxPageSize:     100,
		})
		require.NoError(t, err)

		filter, err := aip.ParseFilter(`status="active"`)
		require.NoError(t, err)
		whereClause, whereParams, err := table.WhereClauseWithOptions(
			filter,
			"p_",
			aip.WhereClauseOptions{
				Dialect:    aip.SQLDialectPostgres,
				StrictMode: true,
			},
		)
		require.NoError(t, err)

		cursorSQL := fmt.Sprintf(
			"SELECT id, created_at FROM aip_items WHERE %s ORDER BY created_at DESC, id LIMIT 1 OFFSET 999",
			whereClause,
		)
		cursorRows, err := queryItemRows(ctx, harness, cursorSQL, whereParams)
		require.NoError(t, err)
		require.Len(t, cursorRows, 1)

		offsetSQL := fmt.Sprintf(
			"SELECT id FROM aip_items WHERE %s ORDER BY created_at DESC, id LIMIT 20 OFFSET 1000",
			whereClause,
		)
		offsetIDs, err := queryIDs(ctx, harness, offsetSQL, whereParams)
		require.NoError(t, err)
		require.NotEmpty(t, offsetIDs)

		cursorPlan, err := planner.PlanList(ctx, aip.QueryRequest{
			Filter:   `status="active"`,
			PageSize: 1,
		})
		require.NoError(t, err)
		cursorToken, err := cursorPlan.NextPageToken(cursorRows)
		require.NoError(t, err)

		seekPlan, err := planner.PlanList(ctx, aip.QueryRequest{
			Filter:    `status="active"`,
			PageSize:  20,
			PageToken: cursorToken,
		})
		require.NoError(t, err)
		seekSQL := buildListSQL("id, created_at", "aip_items", seekPlan)
		seekRows, err := queryItemRows(ctx, harness, seekSQL, seekPlan.Parameters)
		require.NoError(t, err)

		seekIDs := make([]int64, 0, len(seekRows))
		for _, row := range seekRows {
			seekIDs = append(seekIDs, row.ID)
		}
		assert.Equal(t, offsetIDs, seekIDs)
	})

	t.Run("query planner offset pagination matches handwritten offset", func(t *testing.T) {
		planner, err := aip.NewQueryPlanner(aip.TableSpec{
			Table: table,
			DefaultOrder: []aip.OrderBy{
				{FieldPath: aip.NewFieldPath("created_at"), Descending: true},
			},
			TieBreakerFieldPath: aip.NewFieldPath("id"),
			PaginationMode:      aip.PaginationModeOffset,
		}, aip.QueryPlannerOptions{
			Dialect:         aip.SQLDialectPostgres,
			StrictMode:      true,
			DefaultPageSize: 20,
			MaxPageSize:     100,
		})
		require.NoError(t, err)

		filter, err := aip.ParseFilter(`status="active"`)
		require.NoError(t, err)
		whereClause, whereParams, err := table.WhereClauseWithOptions(
			filter,
			"p_",
			aip.WhereClauseOptions{
				Dialect:    aip.SQLDialectPostgres,
				StrictMode: true,
			},
		)
		require.NoError(t, err)

		offsetSQL := fmt.Sprintf(
			"SELECT id, created_at FROM aip_items WHERE %s ORDER BY created_at DESC, id LIMIT 20 OFFSET 1000",
			whereClause,
		)
		expectedRows, err := queryItemRows(ctx, harness, offsetSQL, whereParams)
		require.NoError(t, err)
		require.NotEmpty(t, expectedRows)

		plan, err := planner.PlanList(ctx, aip.QueryRequest{
			Filter:    `status="active"`,
			PageSize:  20,
			PageToken: aip.EncodeOffsetPageToken(1000),
		})
		require.NoError(t, err)
		assert.Equal(t, 1000, plan.Offset)
		assert.Equal(t, "created_at DESC, id", plan.OrderByClause)

		actualSQL := buildListSQL("id, created_at", "aip_items", plan)
		assert.Contains(t, actualSQL, "LIMIT 20 OFFSET 1000")
		actualRows, err := queryItemRows(ctx, harness, actualSQL, plan.Parameters)
		require.NoError(t, err)
		assert.Equal(t, expectedRows, actualRows)
	})
}
