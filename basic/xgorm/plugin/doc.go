// Package plugin provides GORM plugins for tracing.
//
// The plugins implement the gorm.Plugin interface and can be registered
// with a GORM instance using db.Use(plugin).
//
// Subpackages:
//   - tracer: OpenTelemetry distributed tracing for database operations
//
// Note: Logging and metrics have been moved to the parent package.
// Use basic/xgorm.Logger (implements gorm/logger.Interface) and
// RegisterConnectionPoolMetrics() for OpenTelemetry metrics instead.
package plugin
