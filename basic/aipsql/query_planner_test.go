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

package aipsql

import (
	"context"
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSeekPageTokenCodec(t *testing.T) {
	token := SeekPageToken{
		SortValues:      []string{"2025-01-01T00:00:00Z", "HIGH"},
		TieBreakerValue: "101",
	}

	encoded, err := EncodeSeekPageToken(token)
	require.NoError(t, err)
	require.NotEmpty(t, encoded)

	decoded, err := DecodeSeekPageToken(encoded)
	require.NoError(t, err)
	assert.Equal(t, token, decoded)

	_, err = DecodeSeekPageToken("!")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid page token encoding")

	garbagePayload := base64.RawURLEncoding.EncodeToString([]byte("not-json"))
	_, err = DecodeSeekPageToken(garbagePayload)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid page token payload")
}

func TestQueryPlannerPlanList_PostgresSeekAndDebug(t *testing.T) {
	table := NewTable().WithColumns(
		NewColumn().WithFieldPath("status").
			WithDatabaseName("status").
			Filterable().
			WithMatchModes(MatchModeExact).
			Build(),
		NewColumn().WithFieldPath("user_id").WithDatabaseName("user_id").Filterable().Build(),
		NewColumn().WithFieldPath("created_at").
			WithDatabaseName("created_at").
			Filterable().
			Sortable().
			Build(),
		NewColumn().WithFieldPath("id").WithDatabaseName("id").Filterable().Sortable().Build(),
	).
		Build()
	table.CompositeIndexes = []CompositeIndex{{
		Name:    "idx_status_user_created_id",
		Columns: []string{"status", "user_id", "created_at", "id"},
	}}

	planner, err := NewQueryPlanner(QueryPlannerOptions{
		Dialect:                          SQLDialectPostgres,
		StrictMode:                       true,
		EnableCompositeIndexOptimization: true,
		ParameterPrefix:                  "p_",
		DefaultPageSize:                  20,
		MaxPageSize:                      100,
	})
	require.NoError(t, err)

	err = planner.RegisterTableSpec(TableSpec{
		Name:                "orders",
		Table:               table,
		FromClause:          "orders",
		SelectClause:        "id, status, user_id, created_at",
		DefaultOrder:        []OrderBy{{FieldPath: NewFieldPath("created_at"), Descending: true}},
		TieBreakerFieldPath: NewFieldPath("id"),
	})
	require.NoError(t, err)

	pageToken, err := EncodeSeekPageToken(SeekPageToken{
		SortValues:      []string{"2025-01-01T00:00:00Z"},
		TieBreakerValue: "100",
	})
	require.NoError(t, err)

	plan, err := planner.PlanList(context.Background(), "orders", QueryRequest{
		Filter:      `created_at>"2024-01-01" AND (status="active" AND user_id="u-1")`,
		PageSize:    30,
		PageToken:   pageToken,
		EnableDebug: true,
	})
	require.NoError(t, err)

	assert.Equal(t, 30, plan.Limit)
	assert.Equal(t, PaginationModeSeek, plan.PaginationMode)
	assert.Equal(t, "id, status, user_id, created_at", plan.SelectClause)
	assert.Equal(t, "orders", plan.FromClause)
	assert.Equal(t, 0, plan.Offset)
	assert.Contains(t, plan.SQL, "SELECT id, status, user_id, created_at FROM orders")
	assert.Contains(t, plan.SQL, "ORDER BY created_at DESC, id")
	assert.Contains(t, plan.SQL, "LIMIT 30")
	assert.Equal(t, plan.SeekClause, plan.PaginationClause)
	assert.Contains(t, plan.SeekClause, "created_at < @p_seek_0")
	assert.Contains(t, plan.SeekClause, "id > @p_seek_1")
	assert.Len(t, plan.Parameters, 5)
	assert.Equal(
		t,
		[]string{"p_0", "p_1", "p_2", "p_seek_0", "p_seek_1"},
		parameterNames(plan.Parameters),
	)

	require.Len(t, plan.TokenDescriptor.SortOrder, 1)
	assert.Equal(
		t,
		NewFieldPath("created_at").String(),
		plan.TokenDescriptor.SortOrder[0].FieldPath.String(),
	)
	assert.Equal(t, NewFieldPath("id").String(), plan.TokenDescriptor.TieBreakerFieldPath.String())

	require.NotNil(t, plan.Debug)
	assert.Equal(t, SQLDialectPostgres, plan.Debug.Dialect)
	assert.True(t, plan.Debug.StrictMode)
	assert.True(t, plan.Debug.CompositeIndexOptimization)
	assert.True(t, plan.Debug.SeekPaginationEnabled)
	assert.Equal(t, "idx_status_user_created_id", plan.Debug.FilterCompositeIndex)
	assert.True(t, plan.Debug.FilterCompositeReordered)
	assert.Equal(t, "p_", plan.Debug.ParameterPrefix)
}

func TestQueryPlannerPlanList_MySQLFullText(t *testing.T) {
	table := NewTable().WithColumns(
		NewColumn().WithFieldPath("title").
			WithDatabaseName("title").
			Filterable().
			WithMatchModes(MatchModeFullText, MatchModeContains).
			Build(),
		NewColumn().WithFieldPath("created_at").WithDatabaseName("created_at").Sortable().Build(),
		NewColumn().WithFieldPath("id").WithDatabaseName("id").Sortable().Build(),
	).
		Build()
	table.CompositeIndexes = []CompositeIndex{{
		Name:    "idx_created_id",
		Columns: []string{"created_at", "id"},
	}}

	planner, err := NewQueryPlanner(QueryPlannerOptions{
		DefaultPageSize: 25,
		MaxPageSize:     100,
	})
	require.NoError(t, err)

	err = planner.RegisterTableSpec(TableSpec{
		Name:                "articles",
		Table:               table,
		FromClause:          "articles",
		DefaultOrder:        []OrderBy{{FieldPath: NewFieldPath("created_at"), Descending: true}},
		TieBreakerFieldPath: NewFieldPath("id"),
	})
	require.NoError(t, err)

	plan, err := planner.PlanList(context.Background(), "articles", QueryRequest{
		Filter:  `title:"distributed systems"`,
		Dialect: SQLDialectMySQL,
	})
	require.NoError(t, err)

	assert.Equal(t, 25, plan.Limit)
	assert.Contains(t, plan.FilterClause, "MATCH(title) AGAINST")
	assert.Empty(t, plan.SeekClause)
	assert.Equal(t, "created_at DESC, id", plan.OrderByClause)
	assert.Contains(t, plan.SQL, "ORDER BY created_at DESC, id")
	assert.Contains(t, plan.SQL, "LIMIT 25")
	assert.Nil(t, plan.Debug)
}

func TestQueryPlannerDerivesOrderFromCompositeIndex(t *testing.T) {
	table := NewTable().WithColumns(
		NewColumn().WithFieldPath("status").WithDatabaseName("status").Sortable().Build(),
		NewColumn().WithFieldPath("created_at").WithDatabaseName("created_at").Sortable().Build(),
		NewColumn().WithFieldPath("id").WithDatabaseName("id").Sortable().Build(),
	).Build()
	table.CompositeIndexes = []CompositeIndex{{
		Name:    "idx_status_created_id",
		Columns: []string{"status", "created_at", "id"},
	}}

	planner, err := NewQueryPlanner(QueryPlannerOptions{})
	require.NoError(t, err)

	err = planner.RegisterTableSpec(TableSpec{
		Name:                "tickets",
		Table:               table,
		TieBreakerFieldPath: NewFieldPath("id"),
	})
	require.NoError(t, err)

	plan, err := planner.PlanList(context.Background(), "tickets", QueryRequest{})
	require.NoError(t, err)
	assert.Equal(t, "status, created_at, id", plan.OrderByClause)
}

func TestQueryPlannerRegisterValidation(t *testing.T) {
	table := NewTable().WithColumns(
		NewColumn().WithFieldPath("id").WithDatabaseName("id").Sortable().Build(),
	).Build()

	planner, err := NewQueryPlanner(QueryPlannerOptions{})
	require.NoError(t, err)

	err = planner.RegisterTableSpec(TableSpec{
		Name:                "invalid",
		Table:               table,
		DefaultOrder:        []OrderBy{{FieldPath: NewFieldPath("id")}},
		TieBreakerFieldPath: NewFieldPath("id"),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "default order must not include tie breaker")
}

func TestQueryPlannerRejectsTieBreakerInRequestOrder(t *testing.T) {
	table := NewTable().WithColumns(
		NewColumn().WithFieldPath("created_at").WithDatabaseName("created_at").Sortable().Build(),
		NewColumn().WithFieldPath("id").WithDatabaseName("id").Sortable().Build(),
	).Build()

	planner, err := NewQueryPlanner(QueryPlannerOptions{})
	require.NoError(t, err)

	err = planner.RegisterTableSpec(TableSpec{
		Name:                "events",
		Table:               table,
		DefaultOrder:        []OrderBy{{FieldPath: NewFieldPath("created_at"), Descending: true}},
		TieBreakerFieldPath: NewFieldPath("id"),
	})
	require.NoError(t, err)

	_, err = planner.PlanList(context.Background(), "events", QueryRequest{OrderBy: "id desc"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must not contain tie breaker")
}

func TestQueryPlannerSeekWithTieBreakerOnlyOrder(t *testing.T) {
	table := NewTable().WithColumns(
		NewColumn().WithFieldPath("id").WithDatabaseName("id").Sortable().Build(),
	).Build()

	planner, err := NewQueryPlanner(QueryPlannerOptions{})
	require.NoError(t, err)

	err = planner.RegisterTableSpec(TableSpec{
		Name:                "logs",
		Table:               table,
		TieBreakerFieldPath: NewFieldPath("id"),
	})
	require.NoError(t, err)

	pageToken, err := EncodeSeekPageToken(SeekPageToken{
		SortValues:      nil,
		TieBreakerValue: "10",
	})
	require.NoError(t, err)

	plan, err := planner.PlanList(context.Background(), "logs", QueryRequest{PageToken: pageToken})
	require.NoError(t, err)
	assert.Equal(t, "id", plan.OrderByClause)
	assert.Equal(t, "(id > @p_seek_0)", plan.SeekClause)
	assert.Len(t, plan.Parameters, 1)
	assert.Equal(t, "p_seek_0", plan.Parameters[0].Name)
	assert.Equal(t, "10", plan.Parameters[0].Value)
}

func TestQueryPlannerPlanListPartsMatchesSeekPlan(t *testing.T) {
	table := NewTable().WithColumns(
		NewColumn().WithFieldPath("status").
			WithDatabaseName("status").
			Filterable().
			WithMatchModes(MatchModeExact).
			Build(),
		NewColumn().WithFieldPath("created_at").
			WithDatabaseName("created_at").
			Sortable().
			Build(),
		NewColumn().WithFieldPath("id").WithDatabaseName("id").Sortable().Build(),
	).Build()

	planner, err := NewQueryPlanner(QueryPlannerOptions{})
	require.NoError(t, err)
	require.NoError(t, planner.RegisterTableSpec(TableSpec{
		Name:                "events",
		Table:               table,
		FromClause:          "events",
		SelectClause:        "id, created_at",
		DefaultOrder:        []OrderBy{{FieldPath: NewFieldPath("created_at"), Descending: true}},
		TieBreakerFieldPath: NewFieldPath("id"),
	}))

	pageToken, err := EncodeSeekPageToken(SeekPageToken{
		SortValues:      []string{"2025-01-01T00:00:00Z"},
		TieBreakerValue: "15",
	})
	require.NoError(t, err)

	req := QueryRequest{
		Filter:    `status="active"`,
		PageSize:  10,
		PageToken: pageToken,
	}

	parts, err := planner.PlanListParts(context.Background(), "events", req)
	require.NoError(t, err)
	plan, err := planner.PlanList(context.Background(), "events", req)
	require.NoError(t, err)

	assert.Equal(t, PaginationModeSeek, parts.PaginationMode)
	assert.Equal(t, "id, created_at", parts.SelectClause)
	assert.Equal(t, "events", parts.FromClause)
	assert.Equal(t, plan.FilterClause, parts.FilterClause)
	assert.Equal(t, plan.PaginationClause, parts.PaginationClause)
	assert.Equal(t, plan.WhereClause, parts.WhereClause)
	assert.Equal(t, plan.OrderByClause, parts.OrderByClause)
	assert.Equal(t, plan.Parameters, parts.Parameters)
	assert.Equal(t, plan.Limit, parts.Limit)
	assert.Equal(t, 0, parts.Offset)
	assert.Equal(t, plan.TokenDescriptor, parts.TokenDescriptor)
	assert.NotEmpty(t, parts.PaginationClause)
}

func TestQueryPlannerPlanListPartsOffsetMode(t *testing.T) {
	table := NewTable().WithColumns(
		NewColumn().WithFieldPath("status").
			WithDatabaseName("status").
			Filterable().
			WithMatchModes(MatchModeExact).
			Build(),
		NewColumn().WithFieldPath("created_at").
			WithDatabaseName("created_at").
			Sortable().
			Build(),
		NewColumn().WithFieldPath("id").WithDatabaseName("id").Sortable().Build(),
	).Build()

	planner, err := NewQueryPlanner(QueryPlannerOptions{})
	require.NoError(t, err)
	require.NoError(t, planner.RegisterTableSpec(TableSpec{
		Name:                "orders",
		Table:               table,
		FromClause:          "orders",
		SelectClause:        "id, created_at",
		DefaultOrder:        []OrderBy{{FieldPath: NewFieldPath("created_at"), Descending: true}},
		TieBreakerFieldPath: NewFieldPath("id"),
		PaginationMode:      PaginationModeOffset,
	}))

	t.Run("empty token uses zero offset", func(t *testing.T) {
		parts, err := planner.PlanListParts(context.Background(), "orders", QueryRequest{
			Filter:   `status="active"`,
			PageSize: 10,
		})
		require.NoError(t, err)

		assert.Equal(t, PaginationModeOffset, parts.PaginationMode)
		assert.Equal(t, 0, parts.Offset)
		assert.Empty(t, parts.PaginationClause)
		assert.Equal(t, "(status = @p_0)", parts.WhereClause)
		assert.Empty(t, parts.TokenDescriptor.SortOrder)
		assert.Equal(t, "", parts.TokenDescriptor.TieBreakerFieldPath.String())
	})

	t.Run("offset token is decoded into parts and sql", func(t *testing.T) {
		parts, err := planner.PlanListParts(context.Background(), "orders", QueryRequest{
			Filter:    `status="active"`,
			PageSize:  10,
			PageToken: EncodeOffsetPageToken(40),
		})
		require.NoError(t, err)

		assert.Equal(t, PaginationModeOffset, parts.PaginationMode)
		assert.Equal(t, 40, parts.Offset)
		assert.Empty(t, parts.PaginationClause)
		assert.Equal(t, "(status = @p_0)", parts.WhereClause)
		assert.Equal(t, "created_at DESC, id", parts.OrderByClause)

		plan, err := planner.PlanList(context.Background(), "orders", QueryRequest{
			Filter:         `status="active"`,
			PageSize:       10,
			PageToken:      EncodeOffsetPageToken(40),
			PaginationMode: PaginationModeOffset,
		})
		require.NoError(t, err)

		assert.Equal(t, PaginationModeOffset, plan.PaginationMode)
		assert.Equal(t, 40, plan.Offset)
		assert.Empty(t, plan.PaginationClause)
		assert.Empty(t, plan.SeekClause)
		assert.Contains(t, plan.SQL, "LIMIT 10 OFFSET 40")
		assert.Equal(t, parts.WhereClause, plan.WhereClause)
	})

	t.Run("invalid token returns error", func(t *testing.T) {
		_, err := planner.PlanListParts(context.Background(), "orders", QueryRequest{
			Filter:    `status="active"`,
			PageToken: "bad-token",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid page token")
	})

	t.Run("negative token falls back to zero", func(t *testing.T) {
		parts, err := planner.PlanListParts(context.Background(), "orders", QueryRequest{
			Filter:    `status="active"`,
			PageToken: "-50",
		})
		require.NoError(t, err)
		assert.Equal(t, 0, parts.Offset)
	})
}

func TestQueryPlannerRequestPaginationModeOverridesTableSpec(t *testing.T) {
	table := NewTable().WithColumns(
		NewColumn().WithFieldPath("created_at").WithDatabaseName("created_at").Sortable().Build(),
		NewColumn().WithFieldPath("id").WithDatabaseName("id").Sortable().Build(),
	).Build()

	planner, err := NewQueryPlanner(QueryPlannerOptions{})
	require.NoError(t, err)
	require.NoError(t, planner.RegisterTableSpec(TableSpec{
		Name:                "logs",
		Table:               table,
		DefaultOrder:        []OrderBy{{FieldPath: NewFieldPath("created_at"), Descending: true}},
		TieBreakerFieldPath: NewFieldPath("id"),
		PaginationMode:      PaginationModeOffset,
	}))

	parts, err := planner.PlanListParts(context.Background(), "logs", QueryRequest{
		PaginationMode: PaginationModeSeek,
	})
	require.NoError(t, err)

	assert.Equal(t, PaginationModeSeek, parts.PaginationMode)
	require.Len(t, parts.TokenDescriptor.SortOrder, 1)
	assert.Equal(t, NewFieldPath("created_at").String(), parts.TokenDescriptor.SortOrder[0].FieldPath.String())
	assert.Equal(t, NewFieldPath("id").String(), parts.TokenDescriptor.TieBreakerFieldPath.String())
	assert.Equal(t, 0, parts.Offset)
}

func TestBuildSQLWithOffset(t *testing.T) {
	t.Run("omits offset when zero", func(t *testing.T) {
		sql := buildSQL("*", "orders", "(status = @p_0)", "id", 20, 0)
		assert.Equal(t, "SELECT * FROM orders WHERE (status = @p_0) ORDER BY id LIMIT 20", sql)
	})

	t.Run("appends offset when positive", func(t *testing.T) {
		sql := buildSQL("*", "orders", "(status = @p_0)", "id", 20, 40)
		assert.Equal(
			t,
			"SELECT * FROM orders WHERE (status = @p_0) ORDER BY id LIMIT 20 OFFSET 40",
			sql,
		)
	})
}

func parameterNames(params []QueryParameter) []string {
	result := make([]string, 0, len(params))
	for _, param := range params {
		result = append(result, param.Name)
	}
	return result
}
