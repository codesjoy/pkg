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
	assert.Contains(t, plan.SQL, "SELECT id, status, user_id, created_at FROM orders")
	assert.Contains(t, plan.SQL, "ORDER BY created_at DESC, id")
	assert.Contains(t, plan.SQL, "LIMIT 30")
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

func parameterNames(params []QueryParameter) []string {
	result := make([]string, 0, len(params))
	for _, param := range params {
		result = append(result, param.Name)
	}
	return result
}
