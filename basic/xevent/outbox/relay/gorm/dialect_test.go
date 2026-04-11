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

package outboxgorm

import (
	"errors"
	"testing"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/schema"
)

func TestNewGORMStoreAutoDetectsDialect(t *testing.T) {
	tests := []struct {
		name string
		db   *gorm.DB
		want GORMStoreDialect
	}{
		{
			name: "sqlite defaults to standard",
			db:   openTestDB(t),
			want: GORMStoreDialectStandard,
		},
		{
			name: "postgres dialector uses postgres strategy",
			db: &gorm.DB{
				Config: &gorm.Config{
					Dialector: stubDialector{name: "postgres"},
				},
			},
			want: GORMStoreDialectPostgres,
		},
		{
			name: "mysql dialector uses mysql strategy",
			db: &gorm.DB{
				Config: &gorm.Config{
					Dialector: stubDialector{name: "mysql"},
				},
			},
			want: GORMStoreDialectMySQL,
		},
		{
			name: "pgx dialector uses postgres strategy",
			db: &gorm.DB{
				Config: &gorm.Config{
					Dialector: stubDialector{name: "pgx"},
				},
			},
			want: GORMStoreDialectPostgres,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store, err := NewGORMStore(GORMStoreConfig{DB: tt.db})
			if err != nil {
				t.Fatalf("NewGORMStore returned error: %v", err)
			}
			if store.dialect != tt.want {
				t.Fatalf("unexpected dialect: got %q want %q", store.dialect, tt.want)
			}
		})
	}
}

func TestNewGORMStoreExplicitDialectOverride(t *testing.T) {
	store, err := NewGORMStore(GORMStoreConfig{
		DB:      openTestDB(t),
		Dialect: GORMStoreDialectStandard,
	})
	if err != nil {
		t.Fatalf("NewGORMStore returned error: %v", err)
	}
	if store.dialect != GORMStoreDialectStandard {
		t.Fatalf("unexpected dialect: got %q want %q", store.dialect, GORMStoreDialectStandard)
	}
}

func TestNewGORMStoreRejectsUnsupportedDialect(t *testing.T) {
	_, err := NewGORMStore(GORMStoreConfig{
		DB:      openTestDB(t),
		Dialect: GORMStoreDialect("oracle"),
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrUnsupportedGORMStoreDialect) {
		t.Fatalf("expected ErrUnsupportedGORMStoreDialect, got %v", err)
	}
}

func TestNewGORMStoreRejectsNilDB(t *testing.T) {
	_, err := NewGORMStore(GORMStoreConfig{})
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "xevent outbox gorm db is nil" {
		t.Fatalf("unexpected error: %v", err)
	}
}

type stubDialector struct {
	name string
}

func (d stubDialector) Name() string { return d.name }

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
