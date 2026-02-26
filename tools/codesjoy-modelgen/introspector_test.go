package main

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func newMockDB(t *testing.T) (*sql.DB, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})
	return db, mock
}

func TestNormalizeDialect(t *testing.T) {
	t.Parallel()
	if got, err := normalizeDialect(" MySQL "); err != nil || got != dialectMySQL {
		t.Fatalf("normalizeDialect(mysql) = %q, %v", got, err)
	}
	if got, err := normalizeDialect("postgres"); err != nil || got != dialectPostgres {
		t.Fatalf("normalizeDialect(postgres) = %q, %v", got, err)
	}
	if _, err := normalizeDialect("sqlite"); err == nil {
		t.Fatal("normalizeDialect(sqlite) error = nil, want error")
	}
}

func TestDetectDefaultSchema(t *testing.T) {
	t.Parallel()

	t.Run("mysql success", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectQuery(regexp.QuoteMeta("SELECT DATABASE()")).
			WillReturnRows(sqlmock.NewRows([]string{"database"}).AddRow("demo"))
		got, err := detectDefaultSchema(context.Background(), db, dialectMySQL)
		if err != nil || got != "demo" {
			t.Fatalf("detectDefaultSchema(mysql) = %q, %v", got, err)
		}
	})

	t.Run("mysql empty", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectQuery(regexp.QuoteMeta("SELECT DATABASE()")).
			WillReturnRows(sqlmock.NewRows([]string{"database"}).AddRow(nil))
		if _, err := detectDefaultSchema(context.Background(), db, dialectMySQL); err == nil {
			t.Fatal("detectDefaultSchema(mysql empty) error = nil, want error")
		}
	})

	t.Run("postgres success", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectQuery(regexp.QuoteMeta("SELECT current_schema()")).
			WillReturnRows(sqlmock.NewRows([]string{"current_schema"}).AddRow("public"))
		got, err := detectDefaultSchema(context.Background(), db, dialectPostgres)
		if err != nil || got != "public" {
			t.Fatalf("detectDefaultSchema(postgres) = %q, %v", got, err)
		}
	})

	t.Run("postgres query error", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectQuery(regexp.QuoteMeta("SELECT current_schema()")).
			WillReturnError(errors.New("boom"))
		if _, err := detectDefaultSchema(context.Background(), db, dialectPostgres); err == nil {
			t.Fatal("detectDefaultSchema(postgres error) = nil, want error")
		}
	})

	t.Run("unsupported", func(t *testing.T) {
		db, _ := newMockDB(t)
		if _, err := detectDefaultSchema(context.Background(), db, "sqlite"); err == nil {
			t.Fatal("detectDefaultSchema(unsupported) error = nil, want error")
		}
	})
}

func TestListTables(t *testing.T) {
	t.Parallel()

	t.Run("mysql all tables", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectQuery("FROM information_schema\\.tables").
			WithArgs("demo").
			WillReturnRows(sqlmock.NewRows([]string{"table_name"}).AddRow("users").AddRow("events"))
		got, err := listTables(context.Background(), db, dialectMySQL, "demo", nil)
		if err != nil {
			t.Fatalf("listTables(mysql) error = %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("listTables(mysql) len = %d, want 2", len(got))
		}
	})

	t.Run("postgres requested and missing", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectQuery("FROM information_schema\\.tables").
			WithArgs("public", "users", "missing").
			WillReturnRows(sqlmock.NewRows([]string{"table_name"}).AddRow("users"))
		_, err := listTables(
			context.Background(),
			db,
			dialectPostgres,
			"public",
			[]string{"users", "missing"},
		)
		if err == nil {
			t.Fatal("listTables(postgres missing) error = nil, want error")
		}
	})

	t.Run("query error", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectQuery("FROM information_schema\\.tables").
			WithArgs("demo").
			WillReturnError(errors.New("boom"))
		if _, err := listTables(context.Background(), db, dialectMySQL, "demo", nil); err == nil {
			t.Fatal("listTables(query error) error = nil, want error")
		}
	})
}

func TestInspectColumnsAndIndexes(t *testing.T) {
	t.Parallel()

	t.Run("mysql columns and indexes", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectQuery("FROM information_schema\\.columns c").
			WithArgs("demo", "users").
			WillReturnRows(
				sqlmock.NewRows(
					[]string{
						"column_name",
						"data_type",
						"column_type",
						"is_nullable",
						"column_default",
						"ordinal_position",
						"is_primary",
						"extra",
					},
				).
					AddRow("id", "bigint", "bigint", "NO", nil, 1, 1, "auto_increment").
					AddRow("name", "varchar", "varchar(255)", "YES", nil, 2, 0, ""),
			)
		cols, err := inspectMySQLColumns(context.Background(), db, "demo", "users")
		if err != nil {
			t.Fatalf("inspectMySQLColumns() error = %v", err)
		}
		if len(cols) != 2 ||
			!cols[0].IsPrimaryKey ||
			!cols[0].IsAutoIncrement ||
			!cols[1].Nullable {
			t.Fatalf("unexpected mysql columns: %#v", cols)
		}

		mock.ExpectQuery("FROM information_schema\\.statistics").
			WithArgs("demo", "users").
			WillReturnRows(
				sqlmock.NewRows([]string{"index_name", "column_name", "seq_in_index"}).
					AddRow("idx_users_created_id", "created_at", 1).
					AddRow("idx_users_created_id", "id", 2),
			)
		indexes, indexSet, err := inspectMySQLIndexes(context.Background(), db, "demo", "users")
		if err != nil {
			t.Fatalf("inspectMySQLIndexes() error = %v", err)
		}
		if len(indexes) != 1 || len(indexes[0].Columns) != 2 || indexes[0].Columns[1] != "id" {
			t.Fatalf("unexpected mysql indexes: %#v", indexes)
		}
		if _, ok := indexSet["created_at"]; !ok {
			t.Fatalf("index set missing created_at: %#v", indexSet)
		}
	})

	t.Run("postgres columns and indexes", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectQuery("FROM information_schema\\.columns c").
			WithArgs("public", "users").
			WillReturnRows(
				sqlmock.NewRows(
					[]string{
						"column_name",
						"data_type",
						"udt_name",
						"is_nullable",
						"column_default",
						"ordinal_position",
						"is_identity",
						"is_primary",
					},
				).
					AddRow("id", "bigint", "int8", "NO", "nextval('users_id_seq'::regclass)", 1, "NO", true).
					AddRow("name", "character varying", "varchar", "YES", nil, 2, "NO", false),
			)
		cols, err := inspectPostgresColumns(context.Background(), db, "public", "users")
		if err != nil {
			t.Fatalf("inspectPostgresColumns() error = %v", err)
		}
		if len(cols) != 2 ||
			!cols[0].IsAutoIncrement ||
			!cols[0].IsPrimaryKey ||
			!cols[1].Nullable {
			t.Fatalf("unexpected postgres columns: %#v", cols)
		}

		mock.ExpectQuery("FROM pg_class tbl").
			WithArgs("public", "users").
			WillReturnRows(
				sqlmock.NewRows([]string{"index_name", "column_name", "seq_in_index"}).
					AddRow("idx_users_created_id", "created_at", 1).
					AddRow("idx_users_created_id", "id", 2),
			)
		indexes, indexSet, err := inspectPostgresIndexes(
			context.Background(),
			db,
			"public",
			"users",
		)
		if err != nil {
			t.Fatalf("inspectPostgresIndexes() error = %v", err)
		}
		if len(indexes) != 1 || indexes[0].Columns[0] != "created_at" {
			t.Fatalf("unexpected postgres indexes: %#v", indexes)
		}
		if _, ok := indexSet["id"]; !ok {
			t.Fatalf("index set missing id: %#v", indexSet)
		}
	})

	t.Run("dispatcher unsupported", func(t *testing.T) {
		db, _ := newMockDB(t)
		if _, err := inspectColumns(
			context.Background(),
			db,
			"sqlite",
			"demo",
			"users",
		); err == nil {
			t.Fatal("inspectColumns(unsupported) error = nil")
		}
		if _, _, err := inspectIndexes(
			context.Background(),
			db,
			"sqlite",
			"demo",
			"users",
		); err == nil {
			t.Fatal("inspectIndexes(unsupported) error = nil")
		}
	})
}

func TestCollapseIndexRowsAndIndexColumns(t *testing.T) {
	t.Parallel()
	rows := []indexRow{
		{Name: "idx_ab", Column: "b", Seq: 2},
		{Name: "idx_ab", Column: "a", Seq: 1},
		{Name: "idx_c", Column: "c", Seq: 1},
	}
	indexes := collapseIndexRows(rows)
	if len(indexes) != 2 || indexes[0].Name != "idx_ab" || indexes[0].Columns[0] != "a" {
		t.Fatalf("collapseIndexRows() = %#v", indexes)
	}

	set := indexColumns(rows)
	if len(set) != 3 {
		t.Fatalf("indexColumns() len = %d, want 3", len(set))
	}

	if got := collapseIndexRows(nil); got != nil {
		t.Fatalf("collapseIndexRows(nil) = %#v, want nil", got)
	}
}

func TestListTablesUnsupportedDialect(t *testing.T) {
	t.Parallel()
	db, _ := newMockDB(t)
	if _, err := listTables(context.Background(), db, "sqlite", "demo", nil); err == nil {
		t.Fatal("listTables(unsupported) error = nil")
	}
}
