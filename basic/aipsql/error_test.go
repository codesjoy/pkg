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
	"strings"
	"testing"
)

// TestTable_InvalidFieldPathErrors tests error handling for invalid field paths
func TestTable_InvalidFieldPathErrors(t *testing.T) {
	table := NewTable().WithColumns(
		NewColumn().WithFieldPath("id").
			WithDatabaseName("id").
			Sortable().
			Build(), // Only sortable, not filterable
		NewColumn().WithFieldPath("status").
			WithDatabaseName("status").
			Filterable().
			Build(), // Only filterable, not sortable
		NewColumn().WithFieldPath("created_at").
			WithDatabaseName("created_at").
			Filterable().
			Sortable().
			Build(),
	).
		Build()

	tests := []struct {
		name      string
		fieldPath string
		wantErr   string
		testFunc  func(*Table, FieldPath) (*Column, error)
	}{
		{
			name:      "nonexistent filterable field",
			fieldPath: "unknown_field",
			wantErr:   "no filterable field",
			testFunc:  (*Table).FilterableColumnByFieldPath,
		},
		{
			name:      "nonexistent sortable field",
			fieldPath: "unknown_field",
			wantErr:   "no sortable field",
			testFunc:  (*Table).SortableColumnByFieldPath,
		},
		{
			name:      "empty field path filterable",
			fieldPath: "",
			wantErr:   "no filterable field",
			testFunc:  (*Table).FilterableColumnByFieldPath,
		},
		{
			name:      "empty field path sortable",
			fieldPath: "",
			wantErr:   "no sortable field",
			testFunc:  (*Table).SortableColumnByFieldPath,
		},
		{
			name:      "sortable field used as filterable",
			fieldPath: "id", // id is only sortable, not filterable
			wantErr:   "no filterable field",
			testFunc:  (*Table).FilterableColumnByFieldPath,
		},
		{
			name:      "filterable field used as sortable",
			fieldPath: "status", // status is only filterable, not sortable
			wantErr:   "no sortable field",
			testFunc:  (*Table).SortableColumnByFieldPath,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := NewFieldPath(tt.fieldPath)
			_, err := tt.testFunc(table, path)
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error should contain %q, got %q", tt.wantErr, err.Error())
			}
		})
	}
}

// TestFilterParser_MalformedInput tests error handling for malformed filter strings
func TestFilterParser_MalformedInput(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantError bool
	}{
		{
			name:      "empty filter returns nil expression",
			input:     "",
			wantError: false, // Empty filter is valid, returns nil expression
		},
		{
			name:      "unbalanced parentheses - opening",
			input:     "((((",
			wantError: true,
		},
		{
			name:      "unbalanced parentheses - closing",
			input:     "))))",
			wantError: true,
		},
		{
			name:      "missing value after equals",
			input:     "field = ",
			wantError: true,
		},
		{
			name:      "missing comparator",
			input:     "field value",
			wantError: false, // Parser accepts this as implicit filter
		},
		{
			name:      "invalid comparator",
			input:     "field == value",
			wantError: true,
		},
		{
			name:      "missing field name",
			input:     "= value",
			wantError: true,
		},
		{
			name:      "trailing operator",
			input:     "field =",
			wantError: true,
		},
		{
			name:      "multiple operators",
			input:     "field = = value",
			wantError: true,
		},
		{
			name:      "invalid has operator on non-string",
			input:     "user_id:123",
			wantError: false, // Parser accepts this, validation happens later
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseFilter(tt.input)
			if tt.wantError && err == nil {
				t.Fatalf("expected error for input %q, got nil", tt.input)
			}
			if !tt.wantError && err != nil {
				t.Fatalf("expected no error for input %q, got %v", tt.input, err)
			}
		})
	}
}

// TestFilterWhereClause_ErrorHandling tests error handling in WhereClause generation
func TestFilterWhereClause_ErrorHandling(t *testing.T) {
	table := NewTable().WithColumns(
		NewColumn().WithFieldPath("id").WithDatabaseName("id").Sortable().Build(),
		NewColumn().WithFieldPath("status").WithDatabaseName("status").Filterable().Build(),
	).Build()

	tests := []struct {
		name    string
		filter  string
		wantErr string
	}{
		{
			name:    "unknown field in filter",
			filter:  "unknown_field=\"value\"",
			wantErr: "no filterable field",
		},
		{
			name:    "field not filterable",
			filter:  "id=123",
			wantErr: "no filterable field",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter, err := ParseFilter(tt.filter)
			if err != nil {
				t.Fatalf("failed to parse filter: %v", err)
			}

			_, _, err = table.WhereClause(filter, "p")
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error should contain %q, got %q", tt.wantErr, err.Error())
			}
		})
	}
}

// TestOrderByParser_ErrorHandling tests error handling in order by parsing
func TestOrderByParser_ErrorHandling(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantError bool
	}{
		{
			name:      "empty order by",
			input:     "",
			wantError: false, // Empty order by is valid
		},
		{
			name:      "invalid direction",
			input:     "created_at invalid",
			wantError: true,
		},
		{
			name:      "missing field",
			input:     " desc",
			wantError: false, // Parser accepts empty order by
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseOrderBy(tt.input)
			if tt.wantError && err == nil {
				t.Fatalf("expected error for input %q, got nil", tt.input)
			}
			if !tt.wantError && err != nil {
				t.Fatalf("expected no error for input %q, got %v", tt.input, err)
			}
		})
	}
}

// TestTableOrderByClause_ErrorHandling tests error handling in order by clause generation
func TestTableOrderByClause_ErrorHandling(t *testing.T) {
	table := NewTable().WithColumns(
		NewColumn().WithFieldPath("id").WithDatabaseName("id").Sortable().Build(),
		NewColumn().WithFieldPath("status").WithDatabaseName("status").Filterable().Build(),
	).Build()

	tests := []struct {
		name    string
		orderBy string
		wantErr string
	}{
		{
			name:    "field not sortable",
			orderBy: "status desc",
			wantErr: "no sortable field",
		},
		{
			name:    "unknown field",
			orderBy: "unknown_field desc",
			wantErr: "no sortable field",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orderBy, err := ParseOrderBy(tt.orderBy)
			if err != nil {
				t.Fatalf("failed to parse order by: %v", err)
			}

			_, err = table.OrderByClause(orderBy)
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error should contain %q, got %q", tt.wantErr, err.Error())
			}
		})
	}
}
