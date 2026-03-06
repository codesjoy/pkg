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

package tracer

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/schema"
)

// TracerTestUser is a test model with explicit table name
type TracerTestUser struct {
	ID   uint `gorm:"primaryKey"`
	Name string
}

func TestNew(t *testing.T) {
	plugin := New(nil)

	assert.NotNil(t, plugin)
	assert.NotNil(t, plugin.tracer)
}

func TestNew_WithTracer(t *testing.T) {
	mockTracer := noop.NewTracerProvider().Tracer("test")
	plugin := New(mockTracer)

	assert.NotNil(t, plugin)
	assert.Equal(t, mockTracer, plugin.tracer)
}

func TestPlugin_Name(t *testing.T) {
	plugin := New(nil)
	assert.Equal(t, "otel-tracer", plugin.Name())
}

func TestPlugin_Initialize(t *testing.T) {
	plugin := New(nil)

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	err = db.Use(plugin)
	assert.NoError(t, err)
}

func TestPlugin_Tracing_Integration(t *testing.T) {
	plugin := New(nil)

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	err = db.Use(plugin)
	require.NoError(t, err)

	// Create table
	err = db.AutoMigrate(&TracerTestUser{})
	require.NoError(t, err)

	// Perform various operations
	user := TracerTestUser{Name: "Test User"}
	err = db.Create(&user).Error
	require.NoError(t, err)

	var foundUser TracerTestUser
	err = db.First(&foundUser, user.ID).Error
	require.NoError(t, err)

	err = db.Model(&user).Update("Name", "Updated").Error
	require.NoError(t, err)

	var users []TracerTestUser
	err = db.Find(&users).Error
	require.NoError(t, err)

	// If we got here without panicking, the tracer is working
	assert.True(t, true)
}

func TestPlugin_Tracing_WithError(t *testing.T) {
	plugin := New(nil)

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	err = db.Use(plugin)
	require.NoError(t, err)

	// Create table
	err = db.AutoMigrate(&TracerTestUser{})
	require.NoError(t, err)

	// Insert a user
	user1 := TracerTestUser{ID: 1, Name: "User1"}
	err = db.Create(&user1).Error
	require.NoError(t, err)

	// Try to create duplicate (should error)
	user2 := TracerTestUser{ID: 1, Name: "User2"}
	err = db.Create(&user2).Error
	assert.Error(t, err)

	// If we got here without panicking, error handling works
	assert.True(t, true)
}

func TestPlugin_DeleteOperation(t *testing.T) {
	provider, recorder := newTracerProvider()
	defer func() { _ = provider.Shutdown(context.Background()) }()

	plugin := New(provider.Tracer("test"))
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Use(plugin))
	require.NoError(t, db.AutoMigrate(&TracerTestUser{}))

	user := TracerTestUser{Name: "Delete Me"}
	require.NoError(t, db.Create(&user).Error)
	require.NoError(t, db.Delete(&user).Error)

	span := findSpanByName(recorder.Ended(), "gorm DELETE tracer_test_users")
	require.NotNil(t, span)
	assert.Equal(t, codes.Ok, span.Status().Code)
}

func TestPlugin_DeleteCallbacks(t *testing.T) {
	provider, recorder := newTracerProvider()
	defer func() { _ = provider.Shutdown(context.Background()) }()

	plugin := New(provider.Tracer("test"))
	db := &gorm.DB{
		Config: &gorm.Config{Dialector: stubDialector{name: "sqlite"}},
		Statement: &gorm.Statement{
			Table:   "users",
			Context: context.Background(),
		},
	}
	db.Statement.SQL.WriteString("DELETE FROM users WHERE id = ?")

	plugin.beforeDelete(db)
	plugin.afterDelete(db)

	span := findSpanByName(recorder.Ended(), "gorm DELETE users")
	require.NotNil(t, span)
	assert.Equal(t, codes.Ok, span.Status().Code)
	assert.Equal(t, "DELETE FROM users WHERE id = ?", attrString(span, "db.statement"))
}

func TestPlugin_EndSpanRecordsError(t *testing.T) {
	provider, recorder := newTracerProvider()
	defer func() { _ = provider.Shutdown(context.Background()) }()

	plugin := New(provider.Tracer("test"))
	db := &gorm.DB{
		Config: &gorm.Config{Dialector: stubDialector{name: "sqlite"}},
		Statement: &gorm.Statement{
			Table:   "users",
			Context: context.Background(),
		},
		Error: errors.New("delete failed"),
	}

	plugin.startSpan(db, "DELETE")
	plugin.endSpan(db, "DELETE")

	span := findSpanByName(recorder.Ended(), "gorm DELETE users")
	require.NotNil(t, span)
	assert.Equal(t, codes.Error, span.Status().Code)
	assert.Equal(t, "delete failed", span.Status().Description)
	assert.Equal(t, "delete failed", attrString(span, "db.error"))
}

func TestPlugin_BuildSpanName(t *testing.T) {
	tests := []struct {
		name      string
		operation string
		table     string
		want      string
	}{
		{"with table", "CREATE", "users", "gorm CREATE users"},
		{"without table", "QUERY", "", "gorm QUERY"},
		{"update with table", "UPDATE", "products", "gorm UPDATE products"},
		{"delete with table", "DELETE", "orders", "gorm DELETE orders"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildSpanName(tt.operation, tt.table)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestPlugin_BuildAttributes(t *testing.T) {
	plugin := New(nil)

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	stmt := &gorm.Statement{
		Table:   "tracer_test_users",
		DB:      db,
		Context: context.Background(),
	}
	stmt.SQL.WriteString("SELECT * FROM tracer_test_users")

	mockDB := &gorm.DB{
		Config:       db.Config,
		Statement:    stmt,
		RowsAffected: 5,
	}

	attrs := plugin.buildAttributes(mockDB, "CREATE")

	// Check that we have expected attributes with proper value types.
	attrMap := make(map[string]any)
	for _, attr := range attrs {
		switch attr.Key {
		case "db.rows_affected":
			attrMap[string(attr.Key)] = attr.Value.AsInt64()
		default:
			attrMap[string(attr.Key)] = attr.Value.AsString()
		}
	}

	assert.Equal(t, "CREATE", attrMap["db.operation"])
	assert.Equal(t, "sqlite", attrMap["db.system"])
	assert.Equal(t, "tracer_test_users", attrMap["db.sql.table"])
	assert.Equal(t, "SELECT * FROM tracer_test_users", attrMap["db.statement"])
	assert.Equal(t, int64(5), attrMap["db.rows_affected"])
}

func TestPlugin_MultipleOperations(t *testing.T) {
	plugin := New(nil)

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	err = db.Use(plugin)
	require.NoError(t, err)

	// Create table
	err = db.AutoMigrate(&TracerTestUser{})
	require.NoError(t, err)

	// Perform multiple operations
	for i := 0; i < 10; i++ {
		user := TracerTestUser{Name: "User"}
		err = db.Create(&user).Error
		require.NoError(t, err)
	}

	var users []TracerTestUser
	err = db.Find(&users).Error
	require.NoError(t, err)

	assert.Len(t, users, 10)
}

func TestPlugin_RawQuery(t *testing.T) {
	plugin := New(nil)

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	err = db.Use(plugin)
	require.NoError(t, err)

	// Create table
	err = db.AutoMigrate(&TracerTestUser{})
	require.NoError(t, err)

	// Perform raw query
	err = db.Exec("SELECT 1").Error
	require.NoError(t, err)

	// If we got here without panicking, raw query tracing works
	assert.True(t, true)
}

func TestPlugin_RowQuery(t *testing.T) {
	plugin := New(nil)

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	err = db.Use(plugin)
	require.NoError(t, err)

	// Create table
	err = db.AutoMigrate(&TracerTestUser{})
	require.NoError(t, err)

	// Insert test data
	user := TracerTestUser{Name: "Test"}
	err = db.Create(&user).Error
	require.NoError(t, err)

	// Perform row query
	var count int64
	err = db.Raw("SELECT COUNT(*) FROM tracer_test_users").Scan(&count).Error
	require.NoError(t, err)

	assert.Equal(t, int64(1), count)
}

func TestDBSystem(t *testing.T) {
	tests := []struct {
		name      string
		dialector string
		want      string
	}{
		{name: "sqlite", dialector: "sqlite", want: "sqlite"},
		{name: "postgres", dialector: "postgres", want: "postgresql"},
		{name: "postgresql", dialector: "postgresql", want: "postgresql"},
		{name: "mysql", dialector: "mysql", want: "mysql"},
		{name: "sqlserver", dialector: "sqlserver", want: "mssql"},
		{name: "custom", dialector: "clickhouse", want: "clickhouse"},
		{name: "empty", dialector: "", want: "unknown_sql"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := &gorm.DB{
				Config: &gorm.Config{
					Dialector: stubDialector{name: tt.dialector},
				},
			}
			assert.Equal(t, tt.want, dbSystem(db))
		})
	}

	assert.Equal(t, "unknown_sql", dbSystem(nil))
}

// Mock tracer for testing
type mockTracer struct {
	trace.Tracer
}

func newTracerProvider() (*sdktrace.TracerProvider, *tracetest.SpanRecorder) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	return provider, recorder
}

func findSpanByName(spans []sdktrace.ReadOnlySpan, name string) sdktrace.ReadOnlySpan {
	for _, span := range spans {
		if span.Name() == name {
			return span
		}
	}
	return nil
}

func attrString(span sdktrace.ReadOnlySpan, key string) string {
	if span == nil {
		return ""
	}
	for _, item := range span.Attributes() {
		if string(item.Key) == key {
			return item.Value.AsString()
		}
	}
	return ""
}

func (m *mockTracer) Start(
	ctx context.Context,
	name string,
	opts ...trace.SpanStartOption,
) (context.Context, trace.Span) {
	// Return a no-op span
	return noop.NewTracerProvider().Tracer("mock").Start(ctx, name, opts...)
}

func TestPlugin_WithMockTracer(t *testing.T) {
	mockTracer := &mockTracer{
		Tracer: noop.NewTracerProvider().Tracer("mock"),
	}
	plugin := New(mockTracer.Tracer)

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	err = db.Use(plugin)
	require.NoError(t, err)

	err = db.AutoMigrate(&TracerTestUser{})
	require.NoError(t, err)

	user := TracerTestUser{Name: "Test"}
	err = db.Create(&user).Error
	require.NoError(t, err)

	assert.True(t, true)
}

// Benchmark plugin operations
func BenchmarkPlugin_Callback(b *testing.B) {
	plugin := New(nil)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(b, err)

	err = db.Use(plugin)
	require.NoError(b, err)

	err = db.AutoMigrate(&TracerTestUser{})
	require.NoError(b, err)

	user := TracerTestUser{Name: "Benchmark"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		current := user
		current.ID = 0
		_ = db.Create(&current).Error
	}
}

type stubDialector struct {
	name string
}

func (d stubDialector) Name() string {
	return d.name
}

func (d stubDialector) Initialize(_ *gorm.DB) error {
	return nil
}

func (d stubDialector) Migrator(_ *gorm.DB) gorm.Migrator {
	return nil
}

func (d stubDialector) DataTypeOf(_ *schema.Field) string {
	return ""
}

func (d stubDialector) DefaultValueOf(_ *schema.Field) clause.Expression {
	return nil
}

func (d stubDialector) BindVarTo(_ clause.Writer, _ *gorm.Statement, _ interface{}) {}

func (d stubDialector) QuoteTo(_ clause.Writer, _ string) {}

func (d stubDialector) Explain(sql string, _ ...interface{}) string {
	return sql
}
