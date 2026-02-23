// Copyright 2022 The codesjoy Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package aipsql

import (
	"fmt"
	"strings"
	"testing"
	"testing/quick"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// Dialect Validation Tests
// ============================================================================

func TestValidateDialect(t *testing.T) {
	tests := []struct {
		name      string
		dialect   string
		wantError bool
	}{
		{
			name:      "valid generic dialect",
			dialect:   "generic",
			wantError: false,
		},
		{
			name:      "valid postgres dialect",
			dialect:   "postgres",
			wantError: false,
		},
		{
			name:      "valid mysql dialect",
			dialect:   "mysql",
			wantError: false,
		},
		{
			name:      "empty string defaults to generic",
			dialect:   "",
			wantError: false,
		},
		{
			name:      "case insensitive - GENERIC",
			dialect:   "GENERIC",
			wantError: false,
		},
		{
			name:      "case insensitive - Postgres",
			dialect:   "Postgres",
			wantError: false,
		},
		{
			name:      "case insensitive - MySQL",
			dialect:   "MySQL",
			wantError: false,
		},
		{
			name:      "invalid dialect - sqlite",
			dialect:   "sqlite",
			wantError: true,
		},
		{
			name:      "invalid dialect - oracle",
			dialect:   "oracle",
			wantError: true,
		},
		{
			name:      "invalid dialect - random string",
			dialect:   "invalid",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDialect(tt.dialect)
			if tt.wantError {
				if err == nil {
					t.Errorf("validateDialect(%q) expected error, got nil", tt.dialect)
				} else {
					// Verify error message contains the invalid dialect
					if !strings.Contains(err.Error(), "invalid SQL dialect") {
						t.Errorf("validateDialect(%q) error message should contain 'invalid SQL dialect', got: %v", tt.dialect, err)
					}
					// Verify error message lists supported dialects
					if !strings.Contains(err.Error(), "generic") || !strings.Contains(err.Error(), "postgres") || !strings.Contains(err.Error(), "mysql") {
						t.Errorf("validateDialect(%q) error message should list supported dialects, got: %v", tt.dialect, err)
					}
				}
			} else {
				if err != nil {
					t.Errorf("validateDialect(%q) unexpected error: %v", tt.dialect, err)
				}
			}
		})
	}
}

// **Validates: Requirements 6.5, 6.6**
// Feature: aip-sql-execution-optimization, Property 13: SQL 方言验证
//
// For any invalid SQL_Dialect value, Filter_Generator and OrderBy_Generator should
// return descriptive error messages.
func TestProperty_SQLDialectValidation(t *testing.T) {
	// Test scenario 1: Invalid dialect values should return descriptive errors for Filter_Generator
	t.Run("Filter_Generator returns descriptive error for invalid dialect", func(t *testing.T) {
		property := func(invalidDialect string) bool {
			// Skip valid dialects and empty string
			normalizedDialect := strings.ToLower(invalidDialect)
			if normalizedDialect == "" ||
				normalizedDialect == "generic" ||
				normalizedDialect == "postgres" ||
				normalizedDialect == "mysql" {
				return true
			}

			// Create a simple table with a filterable column
			table := NewTable().WithColumns(
				NewColumn().
					WithFieldPath("name").
					WithDatabaseName("db_name").
					Filterable().
					WithMatchModes(MatchModeExact).
					Build(),
			).Build()

			filter, err := ParseFilter(`name="test"`)
			if err != nil {
				return true
			}

			// Try to generate WHERE clause with invalid dialect
			_, _, err = table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
				Dialect: SQLDialect(invalidDialect),
			})

			// Should return an error
			if err == nil {
				return false
			}

			// Error message should mention "dialect"
			if !strings.Contains(strings.ToLower(err.Error()), "dialect") {
				return false
			}

			// Error message should list supported dialects
			errMsg := err.Error()
			if !strings.Contains(errMsg, "generic") ||
				!strings.Contains(errMsg, "postgres") ||
				!strings.Contains(errMsg, "mysql") {
				return false
			}

			return true
		}

		config := &quick.Config{MaxCount: 100}
		if err := quick.Check(property, config); err != nil {
			t.Error(err)
		}
	})

	// Test scenario 2: Invalid dialect values should return descriptive errors for OrderBy_Generator
	t.Run("OrderBy_Generator returns descriptive error for invalid dialect", func(t *testing.T) {
		property := func(invalidDialect string) bool {
			// Skip valid dialects and empty string
			normalizedDialect := strings.ToLower(invalidDialect)
			if normalizedDialect == "" ||
				normalizedDialect == "generic" ||
				normalizedDialect == "postgres" ||
				normalizedDialect == "mysql" {
				return true
			}

			// Create a simple table with a sortable column
			table := NewTable().WithColumns(
				NewColumn().
					WithFieldPath("created_at").
					WithDatabaseName("db_created_at").
					Sortable().
					Build(),
			).Build()

			orderBy, err := ParseOrderBy("created_at")
			if err != nil {
				return true
			}

			// Try to generate ORDER BY clause with invalid dialect
			_, err = table.OrderByClauseWithDialect(orderBy, SQLDialect(invalidDialect))

			// Should return an error
			if err == nil {
				return false
			}

			// Error message should mention "dialect"
			if !strings.Contains(strings.ToLower(err.Error()), "dialect") {
				return false
			}

			// Error message should list supported dialects
			errMsg := err.Error()
			if !strings.Contains(errMsg, "generic") ||
				!strings.Contains(errMsg, "postgres") ||
				!strings.Contains(errMsg, "mysql") {
				return false
			}

			return true
		}

		config := &quick.Config{MaxCount: 100}
		if err := quick.Check(property, config); err != nil {
			t.Error(err)
		}
	})

	// Test scenario 3: Valid dialects should not return errors for Filter_Generator
	t.Run("Filter_Generator accepts valid dialects", func(t *testing.T) {
		validDialects := []SQLDialect{
			SQLDialectGeneric,
			SQLDialectPostgres,
			SQLDialectMySQL,
			"", // Empty string defaults to generic
		}

		table := NewTable().WithColumns(
			NewColumn().
				WithFieldPath("name").
				WithDatabaseName("db_name").
				Filterable().
				WithMatchModes(MatchModeExact).
				Build(),
		).Build()

		filter, err := ParseFilter(`name="test"`)
		require.NoError(t, err)

		for _, dialect := range validDialects {
			t.Run(fmt.Sprintf("dialect=%s", dialect), func(t *testing.T) {
				sql, params, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
					Dialect: dialect,
				})

				// Should not return an error
				assert.NoError(t, err, "Valid dialect %q should not return error", dialect)
				assert.NotEmpty(t, sql, "SQL should be generated")
				assert.NotEmpty(t, params, "Parameters should be generated")
			})
		}
	})

	// Test scenario 4: Valid dialects should not return errors for OrderBy_Generator
	t.Run("OrderBy_Generator accepts valid dialects", func(t *testing.T) {
		validDialects := []SQLDialect{
			SQLDialectGeneric,
			SQLDialectPostgres,
			SQLDialectMySQL,
		}

		table := NewTable().WithColumns(
			NewColumn().
				WithFieldPath("created_at").
				WithDatabaseName("db_created_at").
				Sortable().
				Build(),
		).Build()

		orderBy, err := ParseOrderBy("created_at")
		require.NoError(t, err)

		for _, dialect := range validDialects {
			t.Run(fmt.Sprintf("dialect=%s", dialect), func(t *testing.T) {
				sql, err := table.OrderByClauseWithDialect(orderBy, dialect)

				// Should not return an error
				assert.NoError(t, err, "Valid dialect %q should not return error", dialect)
				assert.NotEmpty(t, sql, "SQL should be generated")
			})
		}
	})

	// Test scenario 5: Case-insensitive dialect validation
	t.Run("dialect validation is case-insensitive", func(t *testing.T) {
		caseVariations := []string{
			"GENERIC", "Generic", "gEnErIc",
			"POSTGRES", "Postgres", "pOsTgReS",
			"MYSQL", "MySQL", "mYsQl",
		}

		table := NewTable().WithColumns(
			NewColumn().
				WithFieldPath("name").
				WithDatabaseName("db_name").
				Filterable().
				WithMatchModes(MatchModeExact).
				Build(),
		).Build()

		filter, err := ParseFilter(`name="test"`)
		require.NoError(t, err)

		for _, dialectVariation := range caseVariations {
			t.Run(fmt.Sprintf("dialect=%s", dialectVariation), func(t *testing.T) {
				sql, params, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
					Dialect: SQLDialect(dialectVariation),
				})

				// Should not return an error (case-insensitive)
				assert.NoError(
					t,
					err,
					"Dialect %q should be accepted (case-insensitive)",
					dialectVariation,
				)
				assert.NotEmpty(t, sql, "SQL should be generated")
				assert.NotEmpty(t, params, "Parameters should be generated")
			})
		}
	})

	// Test scenario 6: Error message quality - should be actionable
	t.Run("error messages are actionable", func(t *testing.T) {
		invalidDialects := []string{
			"sqlite",
			"oracle",
			"mssql",
			"invalid",
			"random_string",
		}

		table := NewTable().WithColumns(
			NewColumn().
				WithFieldPath("name").
				WithDatabaseName("db_name").
				Filterable().
				WithMatchModes(MatchModeExact).
				Build(),
		).Build()

		filter, err := ParseFilter(`name="test"`)
		require.NoError(t, err)

		for _, invalidDialect := range invalidDialects {
			t.Run(fmt.Sprintf("dialect=%s", invalidDialect), func(t *testing.T) {
				_, _, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
					Dialect: SQLDialect(invalidDialect),
				})

				// Should return an error
				require.Error(t, err, "Invalid dialect %q should return error", invalidDialect)

				errMsg := err.Error()

				// Error message should contain the invalid dialect value
				assert.Contains(
					t,
					errMsg,
					invalidDialect,
					"Error should mention the invalid dialect",
				)

				// Error message should list all valid options
				assert.Contains(t, errMsg, "generic", "Error should list 'generic' as valid option")
				assert.Contains(
					t,
					errMsg,
					"postgres",
					"Error should list 'postgres' as valid option",
				)
				assert.Contains(t, errMsg, "mysql", "Error should list 'mysql' as valid option")
			})
		}
	})

	// Test scenario 7: Dialect validation happens before other processing
	t.Run("dialect validation happens early", func(t *testing.T) {
		// Create a table with an invalid filter to test that dialect validation
		// happens before filter processing
		table := NewTable().WithColumns(
			NewColumn().
				WithFieldPath("name").
				WithDatabaseName("db_name").
				Filterable().
				WithMatchModes(MatchModeExact).
				Build(),
		).Build()

		// Use a valid filter
		filter, err := ParseFilter(`name="test"`)
		require.NoError(t, err)

		// Try with invalid dialect
		_, _, err = table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
			Dialect: "invalid_dialect",
		})

		// Should get dialect error, not filter processing error
		require.Error(t, err)
		assert.Contains(t, strings.ToLower(err.Error()), "dialect",
			"Should get dialect validation error before other processing")
	})

	// Test scenario 8: Both generators validate dialect consistently
	t.Run("both generators validate dialect consistently", func(t *testing.T) {
		property := func(dialectStr string) bool {
			// Skip empty string (valid for Filter_Generator but not tested here)
			if dialectStr == "" {
				return true
			}

			dialect := SQLDialect(dialectStr)

			// Test Filter_Generator
			filterTable := NewTable().WithColumns(
				NewColumn().
					WithFieldPath("name").
					WithDatabaseName("db_name").
					Filterable().
					WithMatchModes(MatchModeExact).
					Build(),
			).Build()

			filter, err := ParseFilter(`name="test"`)
			if err != nil {
				return true
			}

			_, _, filterErr := filterTable.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
				Dialect: dialect,
			})

			// Test OrderBy_Generator
			orderByTable := NewTable().WithColumns(
				NewColumn().
					WithFieldPath("created_at").
					WithDatabaseName("db_created_at").
					Sortable().
					Build(),
			).Build()

			orderBy, err := ParseOrderBy("created_at")
			if err != nil {
				return true
			}

			_, orderByErr := orderByTable.OrderByClauseWithDialect(orderBy, dialect)

			// Both should have the same error status (both error or both success)
			if (filterErr == nil) != (orderByErr == nil) {
				return false
			}

			// If both errored, both should mention dialect
			if filterErr != nil && orderByErr != nil {
				filterErrLower := strings.ToLower(filterErr.Error())
				orderByErrLower := strings.ToLower(orderByErr.Error())

				if !strings.Contains(filterErrLower, "dialect") ||
					!strings.Contains(orderByErrLower, "dialect") {
					return false
				}
			}

			return true
		}

		config := &quick.Config{MaxCount: 100}
		if err := quick.Check(property, config); err != nil {
			t.Error(err)
		}
	})

	// Test scenario 9: Dialect validation with complex filters
	t.Run("dialect validation with complex filters", func(t *testing.T) {
		table := NewTable().WithColumns(
			NewColumn().
				WithFieldPath("name").
				WithDatabaseName("db_name").
				Filterable().
				WithMatchModes(MatchModePrefix).
				Build(),
			NewColumn().
				WithFieldPath("status").
				WithDatabaseName("db_status").
				Filterable().
				WithMatchModes(MatchModeExact).
				Build(),
			NewColumn().
				WithFieldPath("age").
				WithDatabaseName("db_age").
				Filterable().
				Build(),
		).Build()

		// Complex filter with multiple conditions
		filter, err := ParseFilter(`name:"test" AND status="active" AND age>18`)
		require.NoError(t, err)

		// Test with invalid dialect
		_, _, err = table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
			Dialect: "invalid_dialect",
		})

		// Should still get dialect error even with complex filter
		require.Error(t, err)
		assert.Contains(t, strings.ToLower(err.Error()), "dialect",
			"Should validate dialect even with complex filters")
	})

	// Test scenario 10: Dialect validation with composite index optimization
	t.Run("dialect validation with composite index optimization", func(t *testing.T) {
		table := NewTable().WithColumns(
			NewColumn().
				WithFieldPath("status").
				WithDatabaseName("db_status").
				Filterable().
				WithMatchModes(MatchModeExact).
				Build(),
			NewColumn().
				WithFieldPath("user_id").
				WithDatabaseName("db_user_id").
				Filterable().
				Build(),
		).Build()

		// Add composite index
		table.CompositeIndexes = []CompositeIndex{
			{
				Name:    "idx_status_user",
				Columns: []string{"db_status", "db_user_id"},
			},
		}

		filter, err := ParseFilter(`status="active" AND user_id=123`)
		require.NoError(t, err)

		// Test with invalid dialect and composite index optimization enabled
		_, _, err = table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
			Dialect:                          "invalid_dialect",
			EnableCompositeIndexOptimization: true,
		})

		// Should still get dialect error
		require.Error(t, err)
		assert.Contains(t, strings.ToLower(err.Error()), "dialect",
			"Should validate dialect even with composite index optimization")
	})
}

// ============================================================================
// Operator Validation Tests
// ============================================================================

func TestValidateHasOperator(t *testing.T) {
	tests := []struct {
		name      string
		column    *Column
		wantError bool
		errorMsg  string
	}{
		{
			name: "valid string column without argSubstitute",
			column: &Column{
				fieldPath:     NewFieldPath("name"),
				databaseName:  "name",
				columnType:    ColumnTypeString,
				argSubstitute: nil,
			},
			wantError: false,
		},
		{
			name: "invalid non-string column (BOOL)",
			column: &Column{
				fieldPath:     NewFieldPath("active"),
				databaseName:  "active",
				columnType:    ColumnTypeBool,
				argSubstitute: nil,
			},
			wantError: true,
			errorMsg:  "has (:) operator can only be used on STRING columns",
		},
		{
			name: "invalid string column with argSubstitute",
			column: &Column{
				fieldPath:     NewFieldPath("name"),
				databaseName:  "name",
				columnType:    ColumnTypeString,
				argSubstitute: func(s string) string { return s },
			},
			wantError: true,
			errorMsg:  "cannot use has (:) operator on a field that have argSubstitute function",
		},
		{
			name: "invalid non-string column with argSubstitute",
			column: &Column{
				fieldPath:     NewFieldPath("count"),
				databaseName:  "count",
				columnType:    ColumnTypeBool,
				argSubstitute: func(s string) string { return s },
			},
			wantError: true,
			errorMsg:  "has (:) operator can only be used on STRING columns",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateHasOperator(tt.column)
			if tt.wantError {
				if err == nil {
					t.Errorf("validateHasOperator() expected error, got nil")
					return
				}
				// Verify error message contains expected text
				if !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf(
						"validateHasOperator() error message should contain %q, got: %v",
						tt.errorMsg,
						err,
					)
				}
			} else if err != nil {
				t.Errorf("validateHasOperator() unexpected error: %v", err)
			}
		})
	}
}

// ============================================================================
// Match Mode Validation Tests
// ============================================================================

func TestValidateMatchMode(t *testing.T) {
	tests := []struct {
		name      string
		mode      MatchMode
		wantError bool
	}{
		{
			name:      "valid exact mode",
			mode:      MatchModeExact,
			wantError: false,
		},
		{
			name:      "valid prefix mode",
			mode:      MatchModePrefix,
			wantError: false,
		},
		{
			name:      "valid fulltext mode",
			mode:      MatchModeFullText,
			wantError: false,
		},
		{
			name:      "valid contains mode",
			mode:      MatchModeContains,
			wantError: false,
		},
		{
			name:      "invalid mode - fuzzy",
			mode:      MatchMode("fuzzy"),
			wantError: true,
		},
		{
			name:      "invalid mode - regex",
			mode:      MatchMode("regex"),
			wantError: true,
		},
		{
			name:      "invalid mode - empty string",
			mode:      MatchMode(""),
			wantError: true,
		},
		{
			name:      "invalid mode - random string",
			mode:      MatchMode("invalid"),
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateMatchMode(tt.mode)
			if tt.wantError {
				if err == nil {
					t.Errorf("validateMatchMode(%q) expected error, got nil", tt.mode)
				} else {
					// Verify error message contains "invalid match mode"
					if !strings.Contains(err.Error(), "invalid match mode") {
						t.Errorf("validateMatchMode(%q) error message should contain 'invalid match mode', got: %v", tt.mode, err)
					}
					// Verify error message lists all valid match modes
					if !strings.Contains(err.Error(), string(MatchModeExact)) ||
						!strings.Contains(err.Error(), string(MatchModePrefix)) ||
						!strings.Contains(err.Error(), string(MatchModeFullText)) ||
						!strings.Contains(err.Error(), string(MatchModeContains)) {
						t.Errorf("validateMatchMode(%q) error message should list all valid match modes, got: %v", tt.mode, err)
					}
				}
			} else {
				if err != nil {
					t.Errorf("validateMatchMode(%q) unexpected error: %v", tt.mode, err)
				}
			}
		})
	}
}
