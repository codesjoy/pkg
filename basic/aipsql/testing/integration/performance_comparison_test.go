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

func TestIntegrationPerformanceComparison(t *testing.T) {
	testCases := []struct {
		name    string
		dialect aip.SQLDialect
	}{
		{name: "postgres", dialect: aip.SQLDialectPostgres},
		{name: "mysql", dialect: aip.SQLDialectMySQL},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()

			harness, err := startDBHarness(ctx, tc.dialect)
			require.NoError(t, err)
			t.Cleanup(func() {
				require.NoError(t, harness.close(context.Background()))
			})

			require.NoError(t, prepareSchemaAndSeed(ctx, harness))
			table := buildIntegrationTable()

			t.Run("contains vs prefix", func(t *testing.T) {
				containsFilter, err := aip.ParseFilter(`title_contains:"distributed-systems"`)
				require.NoError(t, err)
				containsWhere, containsParams, err := table.WhereClauseWithOptions(
					containsFilter,
					"p_",
					aip.WhereClauseOptions{
						Dialect:    tc.dialect,
						StrictMode: true,
					},
				)
				require.NoError(t, err)

				prefixFilter, err := aip.ParseFilter(`title_prefix:"go-distributed-systems-"`)
				require.NoError(t, err)
				prefixWhere, prefixParams, err := table.WhereClauseWithOptions(
					prefixFilter,
					"p_",
					aip.WhereClauseOptions{
						Dialect:    tc.dialect,
						StrictMode: true,
					},
				)
				require.NoError(t, err)

				containsSQL := "SELECT id FROM aip_items WHERE " + containsWhere + " ORDER BY id LIMIT 200"
				prefixSQL := "SELECT id FROM aip_items WHERE " + prefixWhere + " ORDER BY id LIMIT 200"

				containsStats, err := measureDurationStats(
					defaultPerfMeasurementConfig,
					func() error {
						_, runErr := queryIDs(ctx, harness, containsSQL, containsParams)
						return runErr
					},
				)
				require.NoError(t, err)
				prefixStats, err := measureDurationStats(
					defaultPerfMeasurementConfig,
					func() error {
						_, runErr := queryIDs(ctx, harness, prefixSQL, prefixParams)
						return runErr
					},
				)
				require.NoError(t, err)

				ratio := durationRatio(prefixStats.Median, containsStats.Median)
				threshold := thresholdForScenario(perfScenarioContainsVsPrefix)
				t.Logf(
					"dialect=%s scenario=%s ratio=%.2f contains={%s} prefix={%s}",
					tc.dialect,
					perfScenarioContainsVsPrefix,
					ratio,
					containsStats.Summary(),
					prefixStats.Summary(),
				)
				assert.LessOrEqual(
					t,
					ratio,
					threshold.MaxMedianRatio,
					"prefix vs contains regression ratio=%.2f threshold=%.2f contains={%s} prefix={%s}",
					ratio,
					threshold.MaxMedianRatio,
					containsStats.Summary(),
					prefixStats.Summary(),
				)
			})

			t.Run("composite optimization on vs off", func(t *testing.T) {
				filter, err := aip.ParseFilter(
					`created_at>"2025-01-02T00:00:00Z" AND user_id="user_007" AND status="active"`,
				)
				require.NoError(t, err)

				optimizedWhere, optimizedParams, err := table.WhereClauseWithOptions(
					filter,
					"p_",
					aip.WhereClauseOptions{
						Dialect:                          tc.dialect,
						StrictMode:                       true,
						EnableCompositeIndexOptimization: true,
					},
				)
				require.NoError(t, err)
				baselineWhere, baselineParams, err := table.WhereClauseWithOptions(
					filter,
					"p_",
					aip.WhereClauseOptions{
						Dialect:                          tc.dialect,
						StrictMode:                       true,
						EnableCompositeIndexOptimization: false,
					},
				)
				require.NoError(t, err)

				optimizedSQL := "SELECT id FROM aip_items WHERE " + optimizedWhere + " ORDER BY created_at DESC, id LIMIT 100"
				baselineSQL := "SELECT id FROM aip_items WHERE " + baselineWhere + " ORDER BY created_at DESC, id LIMIT 100"

				optimizedIDs, err := queryIDs(ctx, harness, optimizedSQL, optimizedParams)
				require.NoError(t, err)
				baselineIDs, err := queryIDs(ctx, harness, baselineSQL, baselineParams)
				require.NoError(t, err)
				assert.Equal(t, baselineIDs, optimizedIDs)

				optimizedStats, err := measureDurationStats(
					defaultPerfMeasurementConfig,
					func() error {
						_, runErr := queryIDs(ctx, harness, optimizedSQL, optimizedParams)
						return runErr
					},
				)
				require.NoError(t, err)
				baselineStats, err := measureDurationStats(
					defaultPerfMeasurementConfig,
					func() error {
						_, runErr := queryIDs(ctx, harness, baselineSQL, baselineParams)
						return runErr
					},
				)
				require.NoError(t, err)

				ratio := durationRatio(optimizedStats.Median, baselineStats.Median)
				threshold := thresholdForScenario(perfScenarioCompositeOnVsOff)
				t.Logf(
					"dialect=%s scenario=%s ratio=%.2f composite_on={%s} composite_off={%s}",
					tc.dialect,
					perfScenarioCompositeOnVsOff,
					ratio,
					optimizedStats.Summary(),
					baselineStats.Summary(),
				)
				assert.LessOrEqual(
					t,
					ratio,
					threshold.MaxMedianRatio,
					"composite on/off regression ratio=%.2f threshold=%.2f composite_on={%s} composite_off={%s}",
					ratio,
					threshold.MaxMedianRatio,
					optimizedStats.Summary(),
					baselineStats.Summary(),
				)
			})

			t.Run("seek vs offset", func(t *testing.T) {
				filter, err := aip.ParseFilter(`status="active"`)
				require.NoError(t, err)
				whereClause, whereParams, err := table.WhereClauseWithOptions(
					filter,
					"p_",
					aip.WhereClauseOptions{
						Dialect:    tc.dialect,
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

				planner, err := aip.NewQueryPlanner(aip.TableSpec{
					Table: table,
					DefaultOrder: []aip.OrderBy{
						{FieldPath: aip.NewFieldPath("created_at"), Descending: true},
					},
					TieBreakerFieldPath: aip.NewFieldPath("id"),
				}, aip.QueryPlannerOptions{
					Dialect:         tc.dialect,
					StrictMode:      true,
					DefaultPageSize: 20,
					MaxPageSize:     100,
				})
				require.NoError(t, err)

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

				offsetStats, err := measureDurationStats(
					defaultPerfMeasurementConfig,
					func() error {
						_, runErr := queryIDs(ctx, harness, offsetSQL, whereParams)
						return runErr
					},
				)
				require.NoError(t, err)
				seekStats, err := measureDurationStats(defaultPerfMeasurementConfig, func() error {
					_, runErr := queryItemRows(ctx, harness, seekSQL, seekPlan.Parameters)
					return runErr
				})
				require.NoError(t, err)

				ratio := durationRatio(seekStats.Median, offsetStats.Median)
				threshold := thresholdForScenario(perfScenarioSeekVsOffset)
				t.Logf(
					"dialect=%s scenario=%s ratio=%.2f offset={%s} seek={%s}",
					tc.dialect,
					perfScenarioSeekVsOffset,
					ratio,
					offsetStats.Summary(),
					seekStats.Summary(),
				)
				assert.LessOrEqual(
					t,
					ratio,
					threshold.MaxMedianRatio,
					"seek vs offset regression ratio=%.2f threshold=%.2f offset={%s} seek={%s}",
					ratio,
					threshold.MaxMedianRatio,
					offsetStats.Summary(),
					seekStats.Summary(),
				)
			})
		})
	}
}
