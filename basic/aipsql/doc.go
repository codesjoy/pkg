// Package aipsql contains utilities used to comply with API Improvement
// Proposals (AIPs) from https://google.aip.dev/. This includes
// an AIP-160 filter parser and SQL generator and AIP-132 order by
// clause parser and SQL generator.
//
// Performance Characteristics:
//
// Filter Generation (AIP-160):
//   - WhereClause generation: O(n) where n is number of filter conditions
//   - Composite index optimization: O(m * k * log(k)) where m is indexes, k is conditions
//   - Memory allocations: ~10-20 allocations per condition (reduced with sync.Pool)
//   - Thread safety: Table objects are safe for concurrent read-only operations
//
// Order By Generation (AIP-132):
//   - OrderByClause generation: O(n) where n is number of order by fields
//   - Memory allocations: ~5-10 allocations per field
//
// Best Practices:
//   - Enable EnableCompositeIndexOptimization for large tables (>10K rows)
//   - Use MatchModeExact or MatchModePrefix for indexed columns
//   - Reserve MatchModeContains for non-indexed full-text search
//   - Pre-allocate parameter capacity for known query patterns
//   - Use benchmark tests (filter_performance_bench_test.go) to validate performance
//
// Example Usage:
//
//	// Create a table with columns
//	table := NewTable().WithColumns(
//	    NewColumn().WithFieldPath("status").WithDatabaseName("status").
//	        WithMatchModes(MatchModeExact).Filterable().Build(),
//	    NewColumn().WithFieldPath("created_at").WithDatabaseName("created_at").
//	        Filterable().Sortable().Build(),
//	).Build()
//
//	// Add composite indexes for optimization
//	table.CompositeIndexes = []CompositeIndex{
//	    {Name: "idx_status_created", Columns: []string{"status", "created_at"}},
//	}
//
//	// Parse and generate SQL
//	filter, _ := ParseFilter("status=\"active\" AND created_at>\"2024-01-01\"")
//	sql, params, _ := table.WhereClauseWithOptions(filter, "p", WhereClauseOptions{
//	    EnableCompositeIndexOptimization: true,
//	})
package aipsql
