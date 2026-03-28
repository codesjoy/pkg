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

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strconv"
	"strings"

	mysql "github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5"
	_ "github.com/jackc/pgx/v5/stdlib"
)

// SQLIntrospector inspects metadata from MySQL/PostgreSQL catalogs.
type SQLIntrospector struct{}

// Inspect loads table/column/index metadata from the target database.
func (i *SQLIntrospector) Inspect(
	ctx context.Context,
	dsn string,
	schema string,
	requestedTables []string,
) ([]TableMeta, error) {
	normalizedDialect, err := inferDialectFromDSN(dsn)
	if err != nil {
		return nil, fmt.Errorf("infer database dialect from DSN: %w", err)
	}

	driverName := mysqlDriverName
	if normalizedDialect == dialectPostgres {
		driverName = postgresDriverName
	}

	db, err := sql.Open(driverName, dsn)
	if err != nil {
		return nil, fmt.Errorf("open %s connection: %w", normalizedDialect, err)
	}
	defer func() {
		_ = db.Close()
	}()

	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("ping %s database: %w", normalizedDialect, err)
	}

	schemaName := strings.TrimSpace(schema)
	if schemaName == "" {
		schemaName, err = detectDefaultSchema(ctx, db, normalizedDialect)
		if err != nil {
			return nil, err
		}
	}

	tableNames, err := listTables(ctx, db, normalizedDialect, schemaName, requestedTables)
	if err != nil {
		return nil, err
	}
	if len(tableNames) == 0 {
		return nil, fmt.Errorf("no tables found in schema %q", schemaName)
	}

	tables := make([]TableMeta, 0, len(tableNames))
	for _, tableName := range tableNames {
		columns, err := inspectColumns(ctx, db, normalizedDialect, schemaName, tableName)
		if err != nil {
			return nil, err
		}
		indexes, indexedCols, err := inspectIndexes(
			ctx,
			db,
			normalizedDialect,
			schemaName,
			tableName,
		)
		if err != nil {
			return nil, err
		}
		for idx := range columns {
			if _, ok := indexedCols[columns[idx].Name]; ok {
				columns[idx].IsIndexed = true
			}
		}

		tables = append(tables, TableMeta{
			Schema:  schemaName,
			Name:    tableName,
			Columns: columns,
			Indexes: indexes,
		})
	}

	return tables, nil
}

const (
	mysqlDriverName    = "mysql"
	postgresDriverName = "pgx"
)

func inferDialectFromDSN(dsn string) (string, error) {
	trimmedDSN := strings.TrimSpace(dsn)
	if trimmedDSN == "" {
		return "", fmt.Errorf("--dsn is required")
	}

	lowerDSN := strings.ToLower(trimmedDSN)
	switch {
	case strings.HasPrefix(lowerDSN, "postgres://"), strings.HasPrefix(lowerDSN, "postgresql://"):
		return dialectPostgres, nil
	case strings.Contains(trimmedDSN, "@tcp("), strings.Contains(trimmedDSN, "@unix("):
		return dialectMySQL, nil
	}

	if _, err := pgx.ParseConfig(trimmedDSN); err == nil {
		return dialectPostgres, nil
	}
	if _, err := mysql.ParseDSN(trimmedDSN); err == nil {
		return dialectMySQL, nil
	}

	return "", fmt.Errorf("unsupported DSN format; expected MySQL or PostgreSQL DSN")
}

func detectDefaultSchema(ctx context.Context, db *sql.DB, dialect string) (string, error) {
	switch dialect {
	case dialectMySQL:
		var schema sql.NullString
		if err := db.QueryRowContext(ctx, "SELECT DATABASE()").Scan(&schema); err != nil {
			return "", fmt.Errorf("detect mysql default schema: %w", err)
		}
		if !schema.Valid || strings.TrimSpace(schema.String) == "" {
			return "", fmt.Errorf("mysql default schema is empty; please pass --schema")
		}
		return schema.String, nil
	case dialectPostgres:
		var schema string
		if err := db.QueryRowContext(ctx, "SELECT current_schema()").Scan(&schema); err != nil {
			return "", fmt.Errorf("detect postgres default schema: %w", err)
		}
		if strings.TrimSpace(schema) == "" {
			return "", fmt.Errorf("postgres default schema is empty; please pass --schema")
		}
		return schema, nil
	default:
		return "", fmt.Errorf("unsupported dialect %q", dialect)
	}
}

func listTables(
	ctx context.Context,
	db *sql.DB,
	dialect string,
	schema string,
	requested []string,
) ([]string, error) {
	tables := dedupeStrings(requested)
	var (
		query string
		args  []any
	)

	switch dialect {
	case dialectMySQL:
		query = `
SELECT table_name
FROM information_schema.tables
WHERE table_schema = ?
  AND table_type = 'BASE TABLE'`
		args = append(args, schema)
		if len(tables) > 0 {
			query += "\n  AND table_name IN (" + strings.TrimRight(
				strings.Repeat("?,", len(tables)),
				",",
			) + ")"
			for _, t := range tables {
				args = append(args, t)
			}
		}
		query += "\nORDER BY table_name"
	case dialectPostgres:
		query = `
SELECT table_name
FROM information_schema.tables
WHERE table_schema = $1
  AND table_type = 'BASE TABLE'`
		args = append(args, schema)
		if len(tables) > 0 {
			placeholders := make([]string, 0, len(tables))
			for idx, t := range tables {
				placeholders = append(placeholders, "$"+strconv.Itoa(idx+2))
				args = append(args, t)
			}
			query += "\n  AND table_name IN (" + strings.Join(placeholders, ",") + ")"
		}
		query += "\nORDER BY table_name"
	default:
		return nil, fmt.Errorf("unsupported dialect %q", dialect)
	}

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list tables from schema %q: %w", schema, err)
	}
	defer func() {
		_ = rows.Close()
	}()

	found := make([]string, 0, 32)
	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			return nil, fmt.Errorf("scan table name: %w", err)
		}
		found = append(found, tableName)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read table rows: %w", err)
	}

	if len(tables) > 0 {
		set := makeStringSet(found)
		missing := make([]string, 0, len(tables))
		for _, t := range tables {
			if !containsKey(set, t) {
				missing = append(missing, t)
			}
		}
		if len(missing) > 0 {
			sort.Strings(missing)
			return nil, fmt.Errorf(
				"tables not found in schema %q: %s",
				schema,
				strings.Join(missing, ", "),
			)
		}
	}

	return found, nil
}

func inspectColumns(
	ctx context.Context,
	db *sql.DB,
	dialect string,
	schema string,
	table string,
) ([]ColumnMeta, error) {
	switch dialect {
	case dialectMySQL:
		return inspectMySQLColumns(ctx, db, schema, table)
	case dialectPostgres:
		return inspectPostgresColumns(ctx, db, schema, table)
	default:
		return nil, fmt.Errorf("unsupported dialect %q", dialect)
	}
}

func inspectMySQLColumns(
	ctx context.Context,
	db *sql.DB,
	schema string,
	table string,
) ([]ColumnMeta, error) {
	const query = `
SELECT
  c.column_name,
  c.data_type,
  c.column_type,
  c.is_nullable,
  c.column_default,
  c.ordinal_position,
  CASE WHEN k.column_name IS NOT NULL THEN 1 ELSE 0 END AS is_primary,
  c.extra
FROM information_schema.columns c
LEFT JOIN information_schema.key_column_usage k
  ON k.table_schema = c.table_schema
 AND k.table_name = c.table_name
 AND k.column_name = c.column_name
 AND k.constraint_name = 'PRIMARY'
WHERE c.table_schema = ?
  AND c.table_name = ?
ORDER BY c.ordinal_position`

	rows, err := db.QueryContext(ctx, query, schema, table)
	if err != nil {
		return nil, fmt.Errorf("inspect mysql columns for %s.%s: %w", schema, table, err)
	}
	defer func() {
		_ = rows.Close()
	}()

	columns := make([]ColumnMeta, 0, 32)
	for rows.Next() {
		var (
			name          string
			dataType      string
			columnType    string
			isNullable    string
			columnDefault sql.NullString
			ordinal       int
			isPrimary     int
			extra         string
		)
		if err := rows.Scan(
			&name,
			&dataType,
			&columnType,
			&isNullable,
			&columnDefault,
			&ordinal,
			&isPrimary,
			&extra,
		); err != nil {
			return nil, fmt.Errorf("scan mysql column %s.%s: %w", schema, table, err)
		}

		var defaultValue *string
		if columnDefault.Valid {
			value := columnDefault.String
			defaultValue = &value
		}
		columns = append(columns, ColumnMeta{
			Name:            name,
			DataType:        strings.ToLower(dataType),
			RawType:         strings.ToLower(columnType),
			Nullable:        strings.EqualFold(isNullable, "YES"),
			DefaultValue:    defaultValue,
			OrdinalPosition: ordinal,
			IsPrimaryKey:    isPrimary == 1,
			IsAutoIncrement: strings.Contains(strings.ToLower(extra), "auto_increment"),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate mysql columns for %s.%s: %w", schema, table, err)
	}
	return columns, nil
}

func inspectPostgresColumns(
	ctx context.Context,
	db *sql.DB,
	schema string,
	table string,
) ([]ColumnMeta, error) {
	const query = `
SELECT
  c.column_name,
  c.data_type,
  c.udt_name,
  c.is_nullable,
  c.column_default,
  c.ordinal_position,
  c.is_identity,
  EXISTS (
    SELECT 1
    FROM information_schema.table_constraints tc
    JOIN information_schema.key_column_usage kcu
      ON tc.constraint_name = kcu.constraint_name
     AND tc.table_schema = kcu.table_schema
     AND tc.table_name = kcu.table_name
    WHERE tc.table_schema = c.table_schema
      AND tc.table_name = c.table_name
      AND tc.constraint_type = 'PRIMARY KEY'
      AND kcu.column_name = c.column_name
  ) AS is_primary
FROM information_schema.columns c
WHERE c.table_schema = $1
  AND c.table_name = $2
ORDER BY c.ordinal_position`

	rows, err := db.QueryContext(ctx, query, schema, table)
	if err != nil {
		return nil, fmt.Errorf("inspect postgres columns for %s.%s: %w", schema, table, err)
	}
	defer func() {
		_ = rows.Close()
	}()

	columns := make([]ColumnMeta, 0, 32)
	for rows.Next() {
		var (
			name          string
			dataType      string
			udtName       string
			isNullable    string
			columnDefault sql.NullString
			ordinal       int
			isIdentity    string
			isPrimary     bool
		)
		if err := rows.Scan(
			&name,
			&dataType,
			&udtName,
			&isNullable,
			&columnDefault,
			&ordinal,
			&isIdentity,
			&isPrimary,
		); err != nil {
			return nil, fmt.Errorf("scan postgres column %s.%s: %w", schema, table, err)
		}

		var defaultValue *string
		if columnDefault.Valid {
			value := columnDefault.String
			defaultValue = &value
		}

		rawType := strings.ToLower(udtName)
		isAutoIncrement := strings.EqualFold(isIdentity, "YES")
		if columnDefault.Valid &&
			strings.Contains(strings.ToLower(columnDefault.String), "nextval(") {
			isAutoIncrement = true
		}

		columns = append(columns, ColumnMeta{
			Name:            name,
			DataType:        strings.ToLower(dataType),
			RawType:         rawType,
			Nullable:        strings.EqualFold(isNullable, "YES"),
			DefaultValue:    defaultValue,
			OrdinalPosition: ordinal,
			IsPrimaryKey:    isPrimary,
			IsAutoIncrement: isAutoIncrement,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate postgres columns for %s.%s: %w", schema, table, err)
	}
	return columns, nil
}

func inspectIndexes(
	ctx context.Context,
	db *sql.DB,
	dialect string,
	schema string,
	table string,
) ([]IndexMeta, map[string]struct{}, error) {
	switch dialect {
	case dialectMySQL:
		return inspectMySQLIndexes(ctx, db, schema, table)
	case dialectPostgres:
		return inspectPostgresIndexes(ctx, db, schema, table)
	default:
		return nil, nil, fmt.Errorf("unsupported dialect %q", dialect)
	}
}

func inspectMySQLIndexes(
	ctx context.Context,
	db *sql.DB,
	schema string,
	table string,
) ([]IndexMeta, map[string]struct{}, error) {
	const query = `
SELECT index_name, column_name, seq_in_index
FROM information_schema.statistics
WHERE table_schema = ?
  AND table_name = ?
ORDER BY index_name, seq_in_index`
	rows, err := db.QueryContext(ctx, query, schema, table)
	if err != nil {
		return nil, nil, fmt.Errorf("inspect mysql indexes for %s.%s: %w", schema, table, err)
	}
	defer func() {
		_ = rows.Close()
	}()

	rowsData := make([]indexRow, 0, 32)
	for rows.Next() {
		var (
			indexName string
			colName   sql.NullString
			seq       int
		)
		if err := rows.Scan(&indexName, &colName, &seq); err != nil {
			return nil, nil, fmt.Errorf("scan mysql index row for %s.%s: %w", schema, table, err)
		}
		if !colName.Valid || strings.TrimSpace(colName.String) == "" {
			continue
		}
		rowsData = append(rowsData, indexRow{
			Name:   indexName,
			Column: colName.String,
			Seq:    seq,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("iterate mysql indexes for %s.%s: %w", schema, table, err)
	}

	return collapseIndexRows(rowsData), indexColumns(rowsData), nil
}

func inspectPostgresIndexes(
	ctx context.Context,
	db *sql.DB,
	schema string,
	table string,
) ([]IndexMeta, map[string]struct{}, error) {
	const query = `
SELECT
  idx.relname AS index_name,
  att.attname AS column_name,
  ord.n AS seq_in_index
FROM pg_class tbl
JOIN pg_namespace ns ON ns.oid = tbl.relnamespace
JOIN pg_index i ON i.indrelid = tbl.oid
JOIN pg_class idx ON idx.oid = i.indexrelid
JOIN LATERAL unnest(i.indkey) WITH ORDINALITY AS ord(attnum, n) ON true
JOIN pg_attribute att ON att.attrelid = tbl.oid AND att.attnum = ord.attnum
WHERE ns.nspname = $1
  AND tbl.relname = $2
  AND ord.attnum > 0
ORDER BY idx.relname, ord.n`
	rows, err := db.QueryContext(ctx, query, schema, table)
	if err != nil {
		return nil, nil, fmt.Errorf("inspect postgres indexes for %s.%s: %w", schema, table, err)
	}
	defer func() {
		_ = rows.Close()
	}()

	rowsData := make([]indexRow, 0, 32)
	for rows.Next() {
		var (
			indexName string
			colName   string
			seq       int
		)
		if err := rows.Scan(&indexName, &colName, &seq); err != nil {
			return nil, nil, fmt.Errorf("scan postgres index row for %s.%s: %w", schema, table, err)
		}
		if strings.TrimSpace(colName) == "" {
			continue
		}
		rowsData = append(rowsData, indexRow{
			Name:   indexName,
			Column: colName,
			Seq:    seq,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("iterate postgres indexes for %s.%s: %w", schema, table, err)
	}

	return collapseIndexRows(rowsData), indexColumns(rowsData), nil
}

type indexRow struct {
	Name   string
	Column string
	Seq    int
}

func collapseIndexRows(rows []indexRow) []IndexMeta {
	if len(rows) == 0 {
		return nil
	}
	indexMap := make(map[string][]indexRow)
	order := make([]string, 0, len(rows))
	for _, row := range rows {
		if _, ok := indexMap[row.Name]; !ok {
			order = append(order, row.Name)
		}
		indexMap[row.Name] = append(indexMap[row.Name], row)
	}

	indexes := make([]IndexMeta, 0, len(indexMap))
	for _, indexName := range order {
		items := indexMap[indexName]
		sort.SliceStable(items, func(i, j int) bool {
			return items[i].Seq < items[j].Seq
		})
		columns := make([]string, 0, len(items))
		for _, item := range items {
			columns = append(columns, item.Column)
		}
		indexes = append(indexes, IndexMeta{
			Name:    indexName,
			Columns: columns,
		})
	}
	return indexes
}

func indexColumns(rows []indexRow) map[string]struct{} {
	set := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		set[row.Column] = struct{}{}
	}
	return set
}
