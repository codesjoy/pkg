// Package tracer provides a GORM plugin for distributed tracing using OpenTelemetry.
//
// The tracer plugin creates OpenTelemetry spans for each database operation,
// recording SQL queries, table names, operation types, and errors as span attributes.
package tracer

import (
	"context"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"gorm.io/gorm"
)

// Plugin is the GORM tracer plugin.
type Plugin struct {
	tracer trace.Tracer
}

// New creates a new tracer plugin.
//
// The tracer parameter should be an OpenTelemetry trace.Tracer.
// If nil, a default tracer from the global provider will be used.
//
// Example:
//
//	import (
//	    "go.opentelemetry.io/otel"
//	    tracerplugin "github.com/codesjoy/pkg/basic/xgorm/plugin/tracer"
//	)
//
//	tracer := otel.Tracer("gorm")
//	tracerPlugin := tracerplugin.New(tracer)
//	db.Use(tracerPlugin)
func New(tracer trace.Tracer) *Plugin {
	if tracer == nil {
		tracer = otel.Tracer("gorm")
	}
	return &Plugin{
		tracer: tracer,
	}
}

// Name returns the name of the plugin.
func (p *Plugin) Name() string {
	return "otel-tracer"
}

// Initialize registers the plugin callbacks with GORM.
func (p *Plugin) Initialize(db *gorm.DB) error {
	// Register callbacks for all operations
	if err := db.Callback().Create().Before("gorm:create").Register("tracer:before_create", p.beforeCreate); err != nil {
		return err
	}
	if err := db.Callback().Create().After("gorm:create").Register("tracer:after_create", p.afterCreate); err != nil {
		return err
	}

	if err := db.Callback().Query().Before("gorm:query").Register("tracer:before_query", p.beforeQuery); err != nil {
		return err
	}
	if err := db.Callback().Query().After("gorm:query").Register("tracer:after_query", p.afterQuery); err != nil {
		return err
	}

	if err := db.Callback().Update().Before("gorm:update").Register("tracer:before_update", p.beforeUpdate); err != nil {
		return err
	}
	if err := db.Callback().Update().After("gorm:update").Register("tracer:after_update", p.afterUpdate); err != nil {
		return err
	}

	if err := db.Callback().Delete().Before("gorm:delete").Register("tracer:before_delete", p.beforeDelete); err != nil {
		return err
	}
	if err := db.Callback().Delete().After("gorm:delete").Register("tracer:after_delete", p.afterDelete); err != nil {
		return err
	}

	if err := db.Callback().Raw().Before("gorm:raw").Register("tracer:before_raw", p.beforeRaw); err != nil {
		return err
	}
	if err := db.Callback().Raw().After("gorm:raw").Register("tracer:after_raw", p.afterRaw); err != nil {
		return err
	}

	if err := db.Callback().Row().Before("gorm:row").Register("tracer:before_row", p.beforeRow); err != nil {
		return err
	}
	if err := db.Callback().Row().After("gorm:row").Register("tracer:after_row", p.afterRow); err != nil {
		return err
	}

	return nil
}

// Callback functions

func (p *Plugin) beforeCreate(db *gorm.DB) {
	p.startSpan(db, "CREATE")
}

func (p *Plugin) afterCreate(db *gorm.DB) {
	p.endSpan(db, "CREATE")
}

func (p *Plugin) beforeQuery(db *gorm.DB) {
	p.startSpan(db, "QUERY")
}

func (p *Plugin) afterQuery(db *gorm.DB) {
	p.endSpan(db, "QUERY")
}

func (p *Plugin) beforeUpdate(db *gorm.DB) {
	p.startSpan(db, "UPDATE")
}

func (p *Plugin) afterUpdate(db *gorm.DB) {
	p.endSpan(db, "UPDATE")
}

func (p *Plugin) beforeDelete(db *gorm.DB) {
	p.startSpan(db, "DELETE")
}

func (p *Plugin) afterDelete(db *gorm.DB) {
	p.endSpan(db, "DELETE")
}

func (p *Plugin) beforeRaw(db *gorm.DB) {
	p.startSpan(db, "RAW")
}

func (p *Plugin) afterRaw(db *gorm.DB) {
	p.endSpan(db, "RAW")
}

func (p *Plugin) beforeRow(db *gorm.DB) {
	p.startSpan(db, "ROW")
}

func (p *Plugin) afterRow(db *gorm.DB) {
	p.endSpan(db, "ROW")
}

// startSpan begins a new trace span for the database operation.
func (p *Plugin) startSpan(db *gorm.DB, operation string) {
	if db == nil || db.Statement == nil {
		return
	}

	ctx := db.Statement.Context
	if ctx == nil {
		ctx = context.Background()
	}

	// Build span name
	spanName := buildSpanName(operation, db.Statement.Table)

	// Create span with attributes
	attrs := p.buildAttributes(db, operation)
	ctx, _ = p.tracer.Start(ctx, spanName, trace.WithAttributes(attrs...))

	// Store span and context for use in after callback
	db.Statement.Context = ctx
}

// endSpan completes the trace span for the database operation.
func (p *Plugin) endSpan(db *gorm.DB, _ string) {
	if db == nil || db.Statement == nil {
		return
	}

	// Retrieve span from context
	span := trace.SpanFromContext(db.Statement.Context)
	if !span.SpanContext().IsValid() {
		return
	}

	// Record error if operation failed
	if db.Error != nil {
		span.SetStatus(codes.Error, db.Error.Error())
		span.SetAttributes(attribute.String("db.error", db.Error.Error()))
	} else {
		span.SetStatus(codes.Ok, "")
	}

	// End the span
	span.End()
}

// buildAttributes creates OpenTelemetry attributes from the GORM statement.
func (p *Plugin) buildAttributes(db *gorm.DB, operation string) []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		attribute.String("db.operation", operation),
		attribute.String("db.system", dbSystem(db)),
	}

	// Add table name
	if db.Statement.Table != "" {
		attrs = append(attrs, attribute.String("db.sql.table", db.Statement.Table))
	}

	// Add SQL statement
	if sql := db.Statement.SQL.String(); sql != "" {
		attrs = append(attrs, attribute.String("db.statement", sql))
	}

	// Add row count if available
	if db.RowsAffected >= 0 {
		attrs = append(attrs, attribute.Int64("db.rows_affected", db.RowsAffected))
	}

	return attrs
}

func dbSystem(db *gorm.DB) string {
	if db == nil || db.Config == nil || db.Dialector == nil {
		return "unknown_sql"
	}
	dialectorName := db.Name()

	switch strings.ToLower(dialectorName) {
	case "sqlite", "sqlite3":
		return "sqlite"
	case "postgres", "postgresql", "pgx":
		return "postgresql"
	case "mysql", "mariadb":
		return "mysql"
	case "sqlserver", "mssql":
		return "mssql"
	default:
		name := strings.TrimSpace(dialectorName)
		if name == "" {
			return "unknown_sql"
		}
		return strings.ToLower(name)
	}
}

// buildSpanName creates a descriptive name for the span.
func buildSpanName(operation, table string) string {
	if table != "" {
		return "gorm " + operation + " " + table
	}
	return "gorm " + operation
}
