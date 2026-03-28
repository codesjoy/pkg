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

package main

import "testing"

func TestInferGoType(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		col  ColumnMeta
		want string
	}{
		{
			name: "mysql tinyint bool",
			col:  ColumnMeta{DataType: "tinyint", RawType: "tinyint(1)", Nullable: false},
			want: "bool",
		},
		{
			name: "mysql bigint unsigned nullable",
			col:  ColumnMeta{DataType: "bigint", RawType: "bigint unsigned", Nullable: true},
			want: "*uint64",
		},
		{
			name: "postgres timestamptz nullable",
			col: ColumnMeta{
				DataType: "timestamp with time zone",
				RawType:  "timestamptz",
				Nullable: true,
			},
			want: "*time.Time",
		},
		{
			name: "decimal fallback",
			col:  ColumnMeta{DataType: "decimal", RawType: "decimal(10,2)", Nullable: false},
			want: "float64",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := inferGoType(tc.col)
			if got != tc.want {
				t.Fatalf("inferGoType(%+v) = %q, want %q", tc.col, got, tc.want)
			}
		})
	}
}

func TestTimestampRoleByColumnName(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want string
	}{
		{name: "created snake", in: "created_at", want: timestampRoleCreated},
		{name: "created camel", in: "createdAt", want: timestampRoleCreated},
		{name: "created bare", in: "create", want: timestampRoleCreated},
		{name: "updated snake", in: "update_at", want: timestampRoleUpdated},
		{name: "updated bare", in: "updated", want: timestampRoleUpdated},
		{name: "deleted camel", in: "deletedAt", want: timestampRoleDeleted},
		{name: "deleted bare", in: "delete", want: timestampRoleDeleted},
		{name: "none", in: "name", want: timestampRoleNone},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := timestampRoleByColumnName(tc.in)
			if got != tc.want {
				t.Fatalf("timestampRoleByColumnName(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestResolveTimestampType(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		col        ColumnMeta
		mode       string
		role       string
		wantType   string
		wantUseInt bool
		wantSoft   string
		wantWarn   bool
	}{
		{
			name:       "unix sec integer",
			col:        ColumnMeta{DataType: "bigint", RawType: "bigint", Nullable: false},
			mode:       timestampModeUnixSec,
			role:       timestampRoleCreated,
			wantType:   "int64",
			wantUseInt: true,
			wantSoft:   softDeleteKindNone,
			wantWarn:   false,
		},
		{
			name:       "unix milli datetime uses time",
			col:        ColumnMeta{DataType: "timestamp", RawType: "timestamp", Nullable: false},
			mode:       timestampModeUnixMilli,
			role:       timestampRoleUpdated,
			wantType:   "time.Time",
			wantUseInt: false,
			wantSoft:   softDeleteKindNone,
			wantWarn:   false,
		},
		{
			name:       "deleted datetime uses gorm type",
			col:        ColumnMeta{DataType: "timestamp", RawType: "timestamp", Nullable: true},
			mode:       timestampModeUnixMilli,
			role:       timestampRoleDeleted,
			wantType:   "gorm.DeletedAt",
			wantUseInt: false,
			wantSoft:   softDeleteKindGORM,
			wantWarn:   false,
		},
		{
			name:       "deleted integer uses plugin type",
			col:        ColumnMeta{DataType: "bigint", RawType: "bigint", Nullable: false},
			mode:       timestampModeUnixNano,
			role:       timestampRoleDeleted,
			wantType:   "soft_delete.DeletedAt",
			wantUseInt: true,
			wantSoft:   softDeleteKindPlugin,
			wantWarn:   false,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			gotType, gotUseInt, gotSoftDeleteKind, warning, err := resolveTimestampType(
				tc.col,
				tc.mode,
				tc.role,
			)
			if err != nil {
				t.Fatalf("resolveTimestampType() error = %v", err)
			}
			if gotType != tc.wantType {
				t.Fatalf("type = %q, want %q", gotType, tc.wantType)
			}
			if gotUseInt != tc.wantUseInt {
				t.Fatalf("useInt = %v, want %v", gotUseInt, tc.wantUseInt)
			}
			if gotSoftDeleteKind != tc.wantSoft {
				t.Fatalf("softDeleteKind = %q, want %q", gotSoftDeleteKind, tc.wantSoft)
			}
			if tc.wantWarn && warning == "" {
				t.Fatalf("warning expected, got empty")
			}
			if !tc.wantWarn && warning != "" {
				t.Fatalf("warning not expected, got %q", warning)
			}
		})
	}
}

func TestTimestampGormTag(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		col  ResolvedColumn
		want string
	}{
		{
			name: "updated unix nano",
			col: ResolvedColumn{
				TimestampRole:   timestampRoleUpdated,
				TimestampMode:   timestampModeUnixNano,
				UseIntTimestamp: true,
			},
			want: "autoUpdateTime:nano",
		},
		{
			name: "deleted plugin milli",
			col: ResolvedColumn{
				TimestampRole:  timestampRoleDeleted,
				TimestampMode:  timestampModeUnixMilli,
				SoftDeleteKind: softDeleteKindPlugin,
			},
			want: "softDelete:milli",
		},
		{
			name: "deleted plugin nano",
			col: ResolvedColumn{
				TimestampRole:  timestampRoleDeleted,
				TimestampMode:  timestampModeUnixNano,
				SoftDeleteKind: softDeleteKindPlugin,
			},
			want: "softDelete:nano",
		},
		{
			name: "deleted gorm",
			col: ResolvedColumn{
				TimestampRole:  timestampRoleDeleted,
				SoftDeleteKind: softDeleteKindGORM,
			},
			want: "index",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := timestampGormTag(tc.col)
			if got != tc.want {
				t.Fatalf("timestampGormTag(%+v) = %q, want %q", tc.col, got, tc.want)
			}
		})
	}
}

func TestIsTextualColumn_CrossDialect(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		col  ColumnMeta
		want bool
	}{
		{
			name: "mysql varchar",
			col:  ColumnMeta{DataType: "varchar", RawType: "varchar(255)"},
			want: true,
		},
		{
			name: "postgres character varying",
			col:  ColumnMeta{DataType: "character varying", RawType: "varchar"},
			want: true,
		},
		{
			name: "postgres character bpchar",
			col:  ColumnMeta{DataType: "character", RawType: "bpchar"},
			want: true,
		},
		{
			name: "jsonb",
			col:  ColumnMeta{DataType: "jsonb", RawType: "jsonb"},
			want: true,
		},
		{
			name: "bytea",
			col:  ColumnMeta{DataType: "bytea", RawType: "bytea"},
			want: false,
		},
		{
			name: "blob",
			col:  ColumnMeta{DataType: "blob", RawType: "blob"},
			want: false,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := isTextualColumn(tc.col)
			if got != tc.want {
				t.Fatalf("isTextualColumn(%+v) = %v, want %v", tc.col, got, tc.want)
			}
		})
	}
}
