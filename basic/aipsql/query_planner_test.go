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
	"reflect"
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

func TestQueryPlannerPlanList_PostgresSeek(t *testing.T) {
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
	).Build()
	table.CompositeIndexes = []CompositeIndex{{
		Name:    "idx_status_user_created_id",
		Columns: []string{"status", "user_id", "created_at", "id"},
	}}

	planner, err := NewQueryPlanner(TableSpec{
		Table:               table,
		DefaultOrder:        []OrderBy{{FieldPath: NewFieldPath("created_at"), Descending: true}},
		TieBreakerFieldPath: NewFieldPath("id"),
	}, QueryPlannerOptions{
		Dialect:                          SQLDialectPostgres,
		StrictMode:                       true,
		EnableCompositeIndexOptimization: true,
		ParameterPrefix:                  "p_",
		DefaultPageSize:                  20,
		MaxPageSize:                      100,
	})
	require.NoError(t, err)

	pageToken, err := EncodeSeekPageToken(SeekPageToken{
		SortValues:      []string{"2025-01-01T00:00:00Z"},
		TieBreakerValue: "100",
	})
	require.NoError(t, err)

	plan, err := planner.PlanList(context.Background(), QueryRequest{
		Filter:    `created_at>"2024-01-01" AND (status="active" AND user_id="u-1")`,
		PageSize:  30,
		PageToken: pageToken,
	})
	require.NoError(t, err)

	assert.Equal(t, "created_at DESC, id", plan.OrderByClause)
	assert.Equal(t, 30, plan.Limit)
	assert.Equal(t, 0, plan.Offset)
	assert.Contains(t, plan.WhereClause, "created_at < @p_seek_0")
	assert.Contains(t, plan.WhereClause, "id > @p_seek_1")
	assert.Len(t, plan.Parameters, 5)
	assert.Equal(
		t,
		[]string{"p_0", "p_1", "p_2", "p_seek_0", "p_seek_1"},
		parameterNames(plan.Parameters),
	)

	nextToken := mustDecodeNextSeekToken(t, plan, []plannerResultRow{{
		CreatedAt: "2025-01-01T00:00:00Z",
		ID:        100,
	}})
	assert.Equal(t, []string{"2025-01-01T00:00:00Z"}, nextToken.SortValues)
	assert.Equal(t, "100", nextToken.TieBreakerValue)
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
	).Build()
	table.CompositeIndexes = []CompositeIndex{{
		Name:    "idx_created_id",
		Columns: []string{"created_at", "id"},
	}}

	planner, err := NewQueryPlanner(TableSpec{
		Table:               table,
		DefaultOrder:        []OrderBy{{FieldPath: NewFieldPath("created_at"), Descending: true}},
		TieBreakerFieldPath: NewFieldPath("id"),
	}, QueryPlannerOptions{
		DefaultPageSize: 25,
		MaxPageSize:     100,
	})
	require.NoError(t, err)

	plan, err := planner.PlanList(context.Background(), QueryRequest{
		Filter:  `title:"distributed systems"`,
		Dialect: SQLDialectMySQL,
	})
	require.NoError(t, err)

	assert.Equal(t, 25, plan.Limit)
	assert.Contains(t, plan.WhereClause, "MATCH(title) AGAINST")
	assert.Equal(t, "created_at DESC, id", plan.OrderByClause)

	nextToken := mustDecodeNextSeekToken(t, plan, []plannerResultRow{{
		CreatedAt: "2025-01-01T00:00:00Z",
		ID:        42,
	}})
	assert.Equal(t, []string{"2025-01-01T00:00:00Z"}, nextToken.SortValues)
	assert.Equal(t, "42", nextToken.TieBreakerValue)
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

	planner, err := NewQueryPlanner(TableSpec{
		Table:               table,
		TieBreakerFieldPath: NewFieldPath("id"),
	}, QueryPlannerOptions{})
	require.NoError(t, err)

	plan, err := planner.PlanList(context.Background(), QueryRequest{})
	require.NoError(t, err)
	assert.Equal(t, "status, created_at, id", plan.OrderByClause)
}

func TestQueryPlannerRegisterValidation(t *testing.T) {
	table := NewTable().WithColumns(
		NewColumn().WithFieldPath("id").WithDatabaseName("id").Sortable().Build(),
	).Build()

	_, err := NewQueryPlanner(TableSpec{
		Table:               table,
		DefaultOrder:        []OrderBy{{FieldPath: NewFieldPath("id")}},
		TieBreakerFieldPath: NewFieldPath("id"),
	}, QueryPlannerOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "default order must not include tie breaker")
}

func TestQueryPlannerRequiresTableMetadata(t *testing.T) {
	_, err := NewQueryPlanner(TableSpec{
		TieBreakerFieldPath: NewFieldPath("id"),
	}, QueryPlannerOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "table metadata is required")
}

func TestQueryPlannerValidatesPageSizeBoundsAtInitialization(t *testing.T) {
	table := NewTable().WithColumns(
		NewColumn().WithFieldPath("id").WithDatabaseName("id").Sortable().Build(),
	).Build()

	_, err := NewQueryPlanner(TableSpec{
		Table:               table,
		TieBreakerFieldPath: NewFieldPath("id"),
		DefaultPageSize:     100,
		MaxPageSize:         10,
	}, QueryPlannerOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "max page size")
}

func TestQueryPlannerRejectsTieBreakerInRequestOrder(t *testing.T) {
	table := NewTable().WithColumns(
		NewColumn().WithFieldPath("created_at").WithDatabaseName("created_at").Sortable().Build(),
		NewColumn().WithFieldPath("id").WithDatabaseName("id").Sortable().Build(),
	).Build()

	planner, err := NewQueryPlanner(TableSpec{
		Table:               table,
		DefaultOrder:        []OrderBy{{FieldPath: NewFieldPath("created_at"), Descending: true}},
		TieBreakerFieldPath: NewFieldPath("id"),
	}, QueryPlannerOptions{})
	require.NoError(t, err)

	_, err = planner.PlanList(context.Background(), QueryRequest{OrderBy: "id desc"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must not contain tie breaker")
}

func TestQueryPlannerSeekWithTieBreakerOnlyOrder(t *testing.T) {
	table := NewTable().WithColumns(
		NewColumn().WithFieldPath("id").WithDatabaseName("id").Sortable().Build(),
	).Build()

	planner, err := NewQueryPlanner(TableSpec{
		Table:               table,
		TieBreakerFieldPath: NewFieldPath("id"),
	}, QueryPlannerOptions{})
	require.NoError(t, err)

	pageToken, err := EncodeSeekPageToken(SeekPageToken{
		TieBreakerValue: "10",
	})
	require.NoError(t, err)

	plan, err := planner.PlanList(context.Background(), QueryRequest{PageToken: pageToken})
	require.NoError(t, err)
	assert.Equal(t, "id", plan.OrderByClause)
	assert.Equal(t, "(id > @p_seek_0)", plan.WhereClause)
	assert.Len(t, plan.Parameters, 1)
	assert.Equal(t, "p_seek_0", plan.Parameters[0].Name)
	assert.Equal(t, "10", plan.Parameters[0].Value)

	nextToken := mustDecodeNextSeekToken(t, plan, []plannerTieBreakerOnlyRow{{ID: 10}})
	assert.Empty(t, nextToken.SortValues)
	assert.Equal(t, "10", nextToken.TieBreakerValue)
}

func TestQueryPlannerPlanListReturnsFinalClauses(t *testing.T) {
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

	planner, err := NewQueryPlanner(TableSpec{
		Table:               table,
		DefaultOrder:        []OrderBy{{FieldPath: NewFieldPath("created_at"), Descending: true}},
		TieBreakerFieldPath: NewFieldPath("id"),
	}, QueryPlannerOptions{})
	require.NoError(t, err)

	pageToken, err := EncodeSeekPageToken(SeekPageToken{
		SortValues:      []string{"2025-01-01T00:00:00Z"},
		TieBreakerValue: "15",
	})
	require.NoError(t, err)

	plan, err := planner.PlanList(context.Background(), QueryRequest{
		Filter:    `status="active"`,
		PageSize:  10,
		PageToken: pageToken,
	})
	require.NoError(t, err)

	assert.Equal(t, "created_at DESC, id", plan.OrderByClause)
	assert.Equal(t, 10, plan.Limit)
	assert.Equal(t, 0, plan.Offset)
	assert.Contains(t, plan.WhereClause, "(status = @p_0)")
	assert.Contains(t, plan.WhereClause, "created_at < @p_seek_0")
	assert.Contains(t, plan.WhereClause, "id > @p_seek_1")

	nextToken := mustDecodeNextSeekToken(t, plan, []plannerResultRow{{
		CreatedAt: "2025-01-01T00:00:00Z",
		ID:        15,
	}})
	assert.Equal(t, []string{"2025-01-01T00:00:00Z"}, nextToken.SortValues)
	assert.Equal(t, "15", nextToken.TieBreakerValue)
}

func TestQueryPlannerPlanListOffsetMode(t *testing.T) {
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

	planner, err := NewQueryPlanner(TableSpec{
		Table:               table,
		DefaultOrder:        []OrderBy{{FieldPath: NewFieldPath("created_at"), Descending: true}},
		TieBreakerFieldPath: NewFieldPath("id"),
		PaginationMode:      PaginationModeOffset,
	}, QueryPlannerOptions{})
	require.NoError(t, err)

	t.Run("empty token uses zero offset", func(t *testing.T) {
		plan, err := planner.PlanList(context.Background(), QueryRequest{
			Filter:   `status="active"`,
			PageSize: 10,
		})
		require.NoError(t, err)

		assert.Equal(t, 0, plan.Offset)
		assert.Equal(t, "(status = @p_0)", plan.WhereClause)
		assert.Equal(t, "created_at DESC, id", plan.OrderByClause)

		nextToken, err := plan.NextPageToken(make([]int, 10))
		require.NoError(t, err)
		assert.Equal(t, EncodeOffsetPageToken(10), nextToken)
	})

	t.Run("offset token is decoded into plan", func(t *testing.T) {
		plan, err := planner.PlanList(context.Background(), QueryRequest{
			Filter:    `status="active"`,
			PageSize:  10,
			PageToken: EncodeOffsetPageToken(40),
		})
		require.NoError(t, err)

		assert.Equal(t, 40, plan.Offset)
		assert.Equal(t, "(status = @p_0)", plan.WhereClause)
		assert.Equal(t, "created_at DESC, id", plan.OrderByClause)

		nextToken, err := plan.NextPageToken(make([]int, 10))
		require.NoError(t, err)
		assert.Equal(t, EncodeOffsetPageToken(50), nextToken)
	})

	t.Run("invalid token returns error", func(t *testing.T) {
		_, err := planner.PlanList(context.Background(), QueryRequest{
			Filter:    `status="active"`,
			PageToken: "bad-token",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid page token")
	})

	t.Run("negative token falls back to zero", func(t *testing.T) {
		plan, err := planner.PlanList(context.Background(), QueryRequest{
			Filter:    `status="active"`,
			PageToken: "-50",
		})
		require.NoError(t, err)
		assert.Equal(t, 0, plan.Offset)
	})
}

func TestQueryPlannerRequestPaginationModeOverridesTableSpec(t *testing.T) {
	table := NewTable().WithColumns(
		NewColumn().WithFieldPath("created_at").WithDatabaseName("created_at").Sortable().Build(),
		NewColumn().WithFieldPath("id").WithDatabaseName("id").Sortable().Build(),
	).Build()

	planner, err := NewQueryPlanner(TableSpec{
		Table:               table,
		DefaultOrder:        []OrderBy{{FieldPath: NewFieldPath("created_at"), Descending: true}},
		TieBreakerFieldPath: NewFieldPath("id"),
		PaginationMode:      PaginationModeOffset,
	}, QueryPlannerOptions{})
	require.NoError(t, err)

	plan, err := planner.PlanList(context.Background(), QueryRequest{
		PaginationMode: PaginationModeSeek,
	})
	require.NoError(t, err)

	assert.Equal(t, 0, plan.Offset)

	nextToken := mustDecodeNextSeekToken(t, plan, []plannerResultRow{{
		CreatedAt: "2025-01-01T00:00:00Z",
		ID:        7,
	}})
	assert.Equal(t, []string{"2025-01-01T00:00:00Z"}, nextToken.SortValues)
	assert.Equal(t, "7", nextToken.TieBreakerValue)
}

func TestQueryPlannerValidatesTieBreakerAtInitialization(t *testing.T) {
	table := NewTable().WithColumns(
		NewColumn().WithFieldPath("created_at").WithDatabaseName("created_at").Sortable().Build(),
	).Build()

	_, err := NewQueryPlanner(TableSpec{
		Table:               table,
		TieBreakerFieldPath: NewFieldPath("id"),
	}, QueryPlannerOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid tie breaker field")
}

func TestQueryPlanNextPageToken_SeekUsesLastRow(t *testing.T) {
	plan := &QueryPlan{
		Limit:               2,
		paginationMode:      PaginationModeSeek,
		sortOrder:           []OrderBy{{FieldPath: NewFieldPath("created_at"), Descending: true}},
		tieBreakerFieldPath: NewFieldPath("id"),
	}

	rows := []plannerResultRow{
		{CreatedAt: "2025-01-02T00:00:00Z", ID: 20},
		{CreatedAt: "2025-01-01T00:00:00Z", ID: 21},
	}

	nextToken := mustDecodeNextSeekToken(t, plan, rows)
	assert.Equal(t, []string{"2025-01-01T00:00:00Z"}, nextToken.SortValues)
	assert.Equal(t, "21", nextToken.TieBreakerValue)
}

func TestQueryPlanNextPageToken_OffsetUsesRowCount(t *testing.T) {
	plan := &QueryPlan{
		Limit:          3,
		Offset:         40,
		paginationMode: PaginationModeOffset,
	}

	token, err := plan.NextPageToken([]int{1, 2, 3})
	require.NoError(t, err)
	assert.Equal(t, EncodeOffsetPageToken(43), token)
}

func TestQueryPlanNextPageToken_EmptyRowsReturnEmptyToken(t *testing.T) {
	plan := &QueryPlan{
		Limit:               2,
		paginationMode:      PaginationModeSeek,
		tieBreakerFieldPath: NewFieldPath("id"),
	}

	token, err := plan.NextPageToken([]plannerTieBreakerOnlyRow{})
	require.NoError(t, err)
	assert.Empty(t, token)
}

func TestQueryPlanNextPageToken_ShortPageReturnsEmptyToken(t *testing.T) {
	plan := &QueryPlan{
		Limit:          3,
		Offset:         40,
		paginationMode: PaginationModeOffset,
	}

	token, err := plan.NextPageToken([]int{1, 2})
	require.NoError(t, err)
	assert.Empty(t, token)
}

func TestQueryPlanNextPageToken_RequiresSliceOrArray(t *testing.T) {
	plan := &QueryPlan{paginationMode: PaginationModeOffset}

	_, err := plan.NextPageToken(struct{}{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "slice or array")
}

func TestQueryPlanNextPageToken_RequiresStructRowsForSeek(t *testing.T) {
	plan := &QueryPlan{
		Limit:               1,
		paginationMode:      PaginationModeSeek,
		tieBreakerFieldPath: NewFieldPath("id"),
	}

	_, err := plan.NextPageToken([]int{1})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "structs or pointers to structs")
}

func TestQueryPlanNextPageToken_ResolvesGORMTags(t *testing.T) {
	plan := &QueryPlan{
		Limit:               1,
		paginationMode:      PaginationModeSeek,
		sortOrder:           []OrderBy{{FieldPath: NewFieldPath("created_at")}},
		tieBreakerFieldPath: NewFieldPath("id"),
	}

	rows := []struct {
		Created string `gorm:"column:created_at"`
		Key     int64  `gorm:"column:id"`
	}{
		{Created: "2025-01-01T00:00:00Z", Key: 12},
	}

	nextToken := mustDecodeNextSeekToken(t, plan, rows)
	assert.Equal(t, []string{"2025-01-01T00:00:00Z"}, nextToken.SortValues)
	assert.Equal(t, "12", nextToken.TieBreakerValue)
}

func TestQueryPlanNextPageToken_ResolvesJSONTags(t *testing.T) {
	plan := &QueryPlan{
		Limit:               1,
		paginationMode:      PaginationModeSeek,
		sortOrder:           []OrderBy{{FieldPath: NewFieldPath("created_at")}},
		tieBreakerFieldPath: NewFieldPath("id"),
	}

	rows := []*struct {
		Created string `json:"created_at"`
		Key     int64  `json:"id"`
	}{
		{Created: "2025-01-01T00:00:00Z", Key: 33},
	}

	nextToken := mustDecodeNextSeekToken(t, plan, rows)
	assert.Equal(t, []string{"2025-01-01T00:00:00Z"}, nextToken.SortValues)
	assert.Equal(t, "33", nextToken.TieBreakerValue)
}

func TestQueryPlanNextPageToken_ResolvesGoFieldNames(t *testing.T) {
	plan := &QueryPlan{
		Limit:               1,
		paginationMode:      PaginationModeSeek,
		sortOrder:           []OrderBy{{FieldPath: NewFieldPath("created_at")}},
		tieBreakerFieldPath: NewFieldPath("id"),
	}

	rows := []plannerResultRow{{CreatedAt: "2025-01-01T00:00:00Z", ID: 77}}

	nextToken := mustDecodeNextSeekToken(t, plan, rows)
	assert.Equal(t, []string{"2025-01-01T00:00:00Z"}, nextToken.SortValues)
	assert.Equal(t, "77", nextToken.TieBreakerValue)
}

func TestQueryPlanNextPageToken_PrefersGORMOverJSONOverFieldName(t *testing.T) {
	plan := &QueryPlan{
		Limit:               1,
		paginationMode:      PaginationModeSeek,
		sortOrder:           []OrderBy{{FieldPath: NewFieldPath("created_at")}},
		tieBreakerFieldPath: NewFieldPath("id"),
	}

	rows := []struct {
		GORMValue string `gorm:"column:created_at"`
		JSONValue string `json:"created_at"`
		CreatedAt string
		GORMID    int64 `gorm:"column:id"`
		JSONID    int64 `json:"id"`
		ID        int64
	}{
		{
			GORMValue: "gorm-value",
			JSONValue: "json-value",
			CreatedAt: "field-value",
			GORMID:    101,
			JSONID:    102,
			ID:        103,
		},
	}

	nextToken := mustDecodeNextSeekToken(t, plan, rows)
	assert.Equal(t, []string{"gorm-value"}, nextToken.SortValues)
	assert.Equal(t, "101", nextToken.TieBreakerValue)
}

func parameterNames(params []QueryParameter) []string {
	result := make([]string, 0, len(params))
	for _, param := range params {
		result = append(result, param.Name)
	}
	return result
}

type plannerResultRow struct {
	CreatedAt string
	ID        int64
}

type plannerTieBreakerOnlyRow struct {
	ID int64
}

func mustDecodeNextSeekToken(t *testing.T, plan *QueryPlan, rows any) SeekPageToken {
	t.Helper()

	paddedRows := rows
	rowsValue := reflect.ValueOf(rows)
	if plan != nil && plan.Limit > 0 && rowsValue.IsValid() && rowsValue.Kind() == reflect.Slice &&
		rowsValue.Len() < plan.Limit {
		padded := reflect.MakeSlice(rowsValue.Type(), plan.Limit, plan.Limit)
		reflect.Copy(padded.Slice(plan.Limit-rowsValue.Len(), plan.Limit), rowsValue)
		paddedRows = padded.Interface()
	}

	token, err := plan.NextPageToken(paddedRows)
	require.NoError(t, err)
	require.NotEmpty(t, token)

	decoded, err := DecodeSeekPageToken(token)
	require.NoError(t, err)
	return decoded
}
