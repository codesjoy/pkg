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
