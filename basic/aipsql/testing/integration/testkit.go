//go:build integration

package integration

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	aip "github.com/codesjoy/pkg/basic/aipsql"
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/testcontainers/testcontainers-go"
	mysqlc "github.com/testcontainers/testcontainers-go/modules/mysql"
	postgresc "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	seedRowCount = 10_000

	testDBName     = "aip_integration"
	testDBUser     = "aip"
	testDBPassword = "aip"
)

const (
	defaultPerfWarmupRuns   = 3
	defaultPerfSampleRuns   = 9
	defaultPerfMaxRatio     = 2.0
	mysqlAccessTypeRef      = "ref"
	mysqlAccessTypeRange    = "range"
	mysqlAccessTypeFulltext = "fulltext"
)

type perfScenario string

const (
	perfScenarioContainsVsPrefix perfScenario = "contains_vs_prefix"
	perfScenarioCompositeOnVsOff perfScenario = "composite_on_vs_off"
	perfScenarioSeekVsOffset     perfScenario = "seek_vs_offset"
)

type perfThreshold struct {
	MaxMedianRatio float64
}

var perfThresholds = map[perfScenario]perfThreshold{
	perfScenarioContainsVsPrefix: {MaxMedianRatio: defaultPerfMaxRatio},
	perfScenarioCompositeOnVsOff: {MaxMedianRatio: defaultPerfMaxRatio},
	perfScenarioSeekVsOffset:     {MaxMedianRatio: defaultPerfMaxRatio},
}

type perfMeasurementConfig struct {
	WarmupRuns int
	SampleRuns int
}

var defaultPerfMeasurementConfig = perfMeasurementConfig{
	WarmupRuns: defaultPerfWarmupRuns,
	SampleRuns: defaultPerfSampleRuns,
}

type durationStats struct {
	Samples []time.Duration
	Median  time.Duration
	P95     time.Duration
	Min     time.Duration
	Max     time.Duration
}

func (s durationStats) Summary() string {
	return fmt.Sprintf(
		"median=%s p95=%s min=%s max=%s n=%d",
		s.Median,
		s.P95,
		s.Min,
		s.Max,
		len(s.Samples),
	)
}

type explainSummary struct {
	Dialect       aip.SQLDialect
	RawPlan       string
	PostgresNodes []postgresPlanNodeSummary
	MySQLRows     []mysqlExplainRowSummary
}

type postgresPlanNodeSummary struct {
	NodeType     string
	RelationName string
	IndexName    string
}

type mysqlExplainRowSummary struct {
	Table      string
	AccessType string
	Key        string
	Rows       int64
	Extra      string
}

func (s explainSummary) HasIndex(indexName string) bool {
	needle := strings.ToLower(strings.TrimSpace(indexName))
	if needle == "" {
		return false
	}
	for _, node := range s.PostgresNodes {
		if strings.ToLower(strings.TrimSpace(node.IndexName)) == needle {
			return true
		}
	}
	for _, row := range s.MySQLRows {
		if strings.ToLower(strings.TrimSpace(row.Key)) == needle {
			return true
		}
	}
	return false
}

func (s explainSummary) PostgresHasIndexScan() bool {
	for _, node := range s.PostgresNodes {
		if strings.Contains(strings.ToLower(node.NodeType), "index") {
			return true
		}
	}
	return false
}

func (s explainSummary) MySQLHasAccessType(accessTypes ...string) bool {
	if len(accessTypes) == 0 {
		return false
	}
	allowed := make(map[string]struct{}, len(accessTypes))
	for _, value := range accessTypes {
		allowed[strings.ToLower(strings.TrimSpace(value))] = struct{}{}
	}
	for _, row := range s.MySQLRows {
		if _, ok := allowed[strings.ToLower(strings.TrimSpace(row.AccessType))]; ok {
			return true
		}
	}
	return false
}

func (s explainSummary) DebugString() string {
	switch s.Dialect {
	case aip.SQLDialectPostgres:
		parts := make([]string, 0, len(s.PostgresNodes))
		for _, node := range s.PostgresNodes {
			parts = append(
				parts,
				fmt.Sprintf(
					"node=%s index=%s relation=%s",
					node.NodeType,
					node.IndexName,
					node.RelationName,
				),
			)
		}
		return strings.Join(parts, " | ")
	case aip.SQLDialectMySQL:
		parts := make([]string, 0, len(s.MySQLRows))
		for _, row := range s.MySQLRows {
			parts = append(
				parts,
				fmt.Sprintf(
					"table=%s type=%s key=%s rows=%d extra=%s",
					row.Table,
					row.AccessType,
					row.Key,
					row.Rows,
					row.Extra,
				),
			)
		}
		return strings.Join(parts, " | ")
	default:
		return s.RawPlan
	}
}

type postgresExplainJSONRoot struct {
	Plan postgresExplainJSONNode `json:"Plan"`
}

type postgresExplainJSONNode struct {
	NodeType     string                    `json:"Node Type"`
	RelationName string                    `json:"Relation Name"`
	IndexName    string                    `json:"Index Name"`
	Plans        []postgresExplainJSONNode `json:"Plans"`
}

type dbHarness struct {
	dialect   aip.SQLDialect
	db        *sql.DB
	terminate func(context.Context) error
}

type itemRow struct {
	ID        int64
	CreatedAt string
}

func startDBHarness(ctx context.Context, dialect aip.SQLDialect) (*dbHarness, error) {
	switch dialect {
	case aip.SQLDialectPostgres:
		return startPostgresHarness(ctx)
	case aip.SQLDialectMySQL:
		return startMySQLHarness(ctx)
	default:
		return nil, fmt.Errorf("unsupported harness dialect %q", dialect)
	}
}

func startPostgresHarness(ctx context.Context) (*dbHarness, error) {
	container, err := postgresc.Run(ctx,
		"postgres:15-alpine",
		postgresc.WithDatabase(testDBName),
		postgresc.WithUsername(testDBUser),
		postgresc.WithPassword(testDBPassword),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(2*time.Minute),
		),
	)
	if err != nil {
		return nil, err
	}

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		_ = container.Terminate(ctx)
		return nil, err
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		_ = container.Terminate(ctx)
		return nil, err
	}
	if err := waitForDB(ctx, db); err != nil {
		_ = db.Close()
		_ = container.Terminate(ctx)
		return nil, err
	}

	return &dbHarness{
		dialect: aip.SQLDialectPostgres,
		db:      db,
		terminate: func(stopCtx context.Context) error {
			closeErr := db.Close()
			termErr := container.Terminate(stopCtx)
			return errors.Join(closeErr, termErr)
		},
	}, nil
}

func startMySQLHarness(ctx context.Context) (*dbHarness, error) {
	container, err := mysqlc.Run(ctx,
		"mysql:8.4",
		mysqlc.WithDatabase(testDBName),
		mysqlc.WithUsername(testDBUser),
		mysqlc.WithPassword(testDBPassword),
		testcontainers.WithWaitStrategy(
			wait.ForLog("ready for connections").
				WithOccurrence(1).
				WithStartupTimeout(2*time.Minute),
		),
	)
	if err != nil {
		return nil, err
	}

	dsn, err := container.ConnectionString(ctx, "parseTime=true", "multiStatements=true")
	if err != nil {
		_ = container.Terminate(ctx)
		return nil, err
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		_ = container.Terminate(ctx)
		return nil, err
	}
	if err := waitForDB(ctx, db); err != nil {
		_ = db.Close()
		_ = container.Terminate(ctx)
		return nil, err
	}

	return &dbHarness{
		dialect: aip.SQLDialectMySQL,
		db:      db,
		terminate: func(stopCtx context.Context) error {
			closeErr := db.Close()
			termErr := container.Terminate(stopCtx)
			return errors.Join(closeErr, termErr)
		},
	}, nil
}

func waitForDB(ctx context.Context, db *sql.DB) error {
	deadline := time.Now().Add(45 * time.Second)
	consecutiveSuccess := 0
	for {
		if err := db.PingContext(ctx); err == nil {
			consecutiveSuccess++
			if consecutiveSuccess >= 3 {
				return nil
			}
		} else {
			consecutiveSuccess = 0
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("database did not become ready before timeout")
		}
		time.Sleep(250 * time.Millisecond)
	}
}

func (h *dbHarness) close(ctx context.Context) error {
	if h == nil || h.terminate == nil {
		return nil
	}
	return h.terminate(ctx)
}

func prepareSchemaAndSeed(ctx context.Context, h *dbHarness) error {
	if err := createSchema(ctx, h); err != nil {
		return err
	}
	return seedRows(ctx, h, seedRowCount)
}

func createSchema(ctx context.Context, h *dbHarness) error {
	statements := postgresSchemaStatements
	if h.dialect == aip.SQLDialectMySQL {
		statements = mysqlSchemaStatements
	}
	for _, statement := range statements {
		if _, err := h.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("schema statement failed: %w", err)
		}
	}
	return nil
}

var postgresSchemaStatements = []string{
	`CREATE TABLE aip_items (
		id BIGINT PRIMARY KEY,
		status VARCHAR(32) NOT NULL,
		user_id VARCHAR(64) NOT NULL,
		title VARCHAR(255) NOT NULL,
		body TEXT NOT NULL,
		created_at VARCHAR(32) NOT NULL
	)`,
	`CREATE INDEX idx_items_status ON aip_items(status)`,
	`CREATE INDEX idx_items_title_prefix ON aip_items(title varchar_pattern_ops)`,
	`CREATE INDEX idx_items_created_at ON aip_items(created_at)`,
	`CREATE INDEX idx_items_status_user_created_id ON aip_items(status, user_id, created_at, id)`,
	`CREATE INDEX idx_items_status_created_id ON aip_items(status, created_at, id)`,
	`CREATE INDEX idx_items_title_fts ON aip_items USING GIN (to_tsvector('simple', title))`,
}

var mysqlSchemaStatements = []string{
	`CREATE TABLE aip_items (
		id BIGINT PRIMARY KEY,
		status VARCHAR(32) NOT NULL,
		user_id VARCHAR(64) NOT NULL,
		title VARCHAR(255) NOT NULL,
		body TEXT NOT NULL,
		created_at VARCHAR(32) NOT NULL,
		KEY idx_items_status (status),
		KEY idx_items_title_prefix (title),
		KEY idx_items_created_at (created_at),
		KEY idx_items_status_user_created_id (status, user_id, created_at, id),
		KEY idx_items_status_created_id (status, created_at, id),
		FULLTEXT KEY idx_items_title_fts (title)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
}

func seedRows(ctx context.Context, h *dbHarness, rows int) error {
	if h.dialect == aip.SQLDialectMySQL {
		return seedRowsMySQL(ctx, h.db, rows)
	}
	return seedRowsPostgres(ctx, h.db, rows)
}

func seedRowsPostgres(ctx context.Context, db *sql.DB, rows int) error {
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	insertSQL := `INSERT INTO aip_items (id, status, user_id, title, body, created_at) VALUES ($1, $2, $3, $4, $5, $6)`

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	stmt, err := tx.PrepareContext(ctx, insertSQL)
	if err != nil {
		_ = tx.Rollback()
		return err
	}

	for i := 1; i <= rows; i++ {
		status, userID, title, body, createdAt := buildSeedRow(base, i)

		if _, err := stmt.ExecContext(ctx, i, status, userID, title, body, createdAt); err != nil {
			_ = stmt.Close()
			_ = tx.Rollback()
			return err
		}
	}

	if err := stmt.Close(); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return nil
}

func seedRowsMySQL(ctx context.Context, db *sql.DB, rows int) error {
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	const chunkSize = 250
	for start := 1; start <= rows; start += chunkSize {
		end := start + chunkSize
		if end > rows+1 {
			end = rows + 1
		}

		var builder strings.Builder
		builder.WriteString(
			"INSERT INTO aip_items (id, status, user_id, title, body, created_at) VALUES ",
		)
		args := make([]any, 0, (end-start)*6)
		for i := start; i < end; i++ {
			if i > start {
				builder.WriteString(",")
			}
			builder.WriteString("(?, ?, ?, ?, ?, ?)")
			status, userID, title, body, createdAt := buildSeedRow(base, i)
			args = append(args, i, status, userID, title, body, createdAt)
		}

		execErr := retryExec(ctx, db, builder.String(), args...)
		if execErr != nil {
			return execErr
		}
	}
	return nil
}

func retryExec(ctx context.Context, db *sql.DB, sqlText string, args ...any) error {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if _, err := db.ExecContext(ctx, sqlText, args...); err == nil {
			return nil
		} else {
			lastErr = err
		}
		time.Sleep(200 * time.Millisecond)
	}
	return lastErr
}

func buildSeedRow(base time.Time, i int) (status, userID, title, body, createdAt string) {
	status = []string{"active", "pending", "closed"}[i%3]
	userID = fmt.Sprintf("user_%03d", i%200)

	title = fmt.Sprintf("record-%05d", i)
	switch {
	case i%15 == 0:
		title = fmt.Sprintf("distributed-systems-pattern-%05d", i)
	case i%10 == 0:
		title = fmt.Sprintf("go-distributed-systems-%05d", i)
	}

	createdAt = base.Add(time.Duration(i) * time.Minute).Format(time.RFC3339)
	body = fmt.Sprintf("seed row %d talking about distributed systems and query planning", i)
	return
}

func buildIntegrationTable() *aip.Table {
	table := aip.NewTable().WithColumns(
		aip.NewColumn().
			WithFieldPath("status").
			WithDatabaseName("status").
			Filterable().
			WithMatchModes(aip.MatchModeExact).
			Build(),
		aip.NewColumn().
			WithFieldPath("user_id").
			WithDatabaseName("user_id").
			Filterable().
			WithMatchModes(aip.MatchModeExact).
			Build(),
		aip.NewColumn().
			WithFieldPath("created_at").
			WithDatabaseName("created_at").
			Filterable().
			Sortable().
			Build(),
		aip.NewColumn().WithFieldPath("id").WithDatabaseName("id").Filterable().Sortable().Build(),
		aip.NewColumn().
			WithFieldPath("title_prefix").
			WithDatabaseName("title").
			Filterable().
			WithMatchModes(aip.MatchModePrefix).
			Build(),
		aip.NewColumn().
			WithFieldPath("title_contains").
			WithDatabaseName("title").
			Filterable().
			WithMatchModes(aip.MatchModeContains).
			Build(),
		aip.NewColumn().
			WithFieldPath("title_fulltext").
			WithDatabaseName("title").
			Filterable().
			WithMatchModes(aip.MatchModeFullText).
			Build(),
	).
		Build()
	table.CompositeIndexes = []aip.CompositeIndex{
		{
			Name:    "idx_items_status_user_created_id",
			Columns: []string{"status", "user_id", "created_at", "id"},
		},
		{Name: "idx_items_status_created_id", Columns: []string{"status", "created_at", "id"}},
	}
	return table
}

func queryIDs(
	ctx context.Context,
	h *dbHarness,
	sqlText string,
	params []aip.QueryParameter,
) ([]int64, error) {
	rewrittenSQL, args, err := rewriteNamedParameters(h.dialect, sqlText, params)
	if err != nil {
		return nil, err
	}

	rows, err := h.db.QueryContext(ctx, rewrittenSQL, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ids := make([]int64, 0, 64)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return ids, nil
}

func queryItemRows(
	ctx context.Context,
	h *dbHarness,
	sqlText string,
	params []aip.QueryParameter,
) ([]itemRow, error) {
	rewrittenSQL, args, err := rewriteNamedParameters(h.dialect, sqlText, params)
	if err != nil {
		return nil, err
	}

	rows, err := h.db.QueryContext(ctx, rewrittenSQL, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]itemRow, 0, 32)
	for rows.Next() {
		var item itemRow
		if err := rows.Scan(&item.ID, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func explainPlan(
	ctx context.Context,
	h *dbHarness,
	sqlText string,
	params []aip.QueryParameter,
) (string, error) {
	summary, err := explainPlanSummary(ctx, h, sqlText, params)
	if err != nil {
		return "", err
	}
	return summary.RawPlan, nil
}

func explainPlanSummary(
	ctx context.Context,
	h *dbHarness,
	sqlText string,
	params []aip.QueryParameter,
) (explainSummary, error) {
	rewrittenSQL, args, err := rewriteNamedParameters(h.dialect, sqlText, params)
	if err != nil {
		return explainSummary{}, err
	}
	if h.dialect == aip.SQLDialectPostgres {
		return explainPostgresPlanSummary(ctx, h, rewrittenSQL, args)
	}
	return explainRowsSummary(ctx, h, rewrittenSQL, args)
}

func explainPostgresPlanSummary(
	ctx context.Context,
	h *dbHarness,
	rewrittenSQL string,
	args []any,
) (explainSummary, error) {
	explainSQL := "EXPLAIN (FORMAT JSON) " + rewrittenSQL

	var rawJSON string
	if err := h.db.QueryRowContext(ctx, explainSQL, args...).Scan(&rawJSON); err != nil {
		return explainSummary{}, err
	}

	var roots []postgresExplainJSONRoot
	if err := json.Unmarshal([]byte(rawJSON), &roots); err != nil {
		return explainSummary{}, fmt.Errorf("parse postgres explain json: %w", err)
	}

	nodes := make([]postgresPlanNodeSummary, 0, 8)
	for _, root := range roots {
		collectPostgresPlanNodes(root.Plan, &nodes)
	}
	if len(nodes) == 0 {
		return explainSummary{}, fmt.Errorf("postgres explain returned no plan nodes")
	}

	return explainSummary{
		Dialect:       aip.SQLDialectPostgres,
		RawPlan:       rawJSON,
		PostgresNodes: nodes,
	}, nil
}

func collectPostgresPlanNodes(node postgresExplainJSONNode, out *[]postgresPlanNodeSummary) {
	*out = append(*out, postgresPlanNodeSummary{
		NodeType:     node.NodeType,
		RelationName: node.RelationName,
		IndexName:    node.IndexName,
	})
	for _, child := range node.Plans {
		collectPostgresPlanNodes(child, out)
	}
}

func explainRowsSummary(
	ctx context.Context,
	h *dbHarness,
	rewrittenSQL string,
	args []any,
) (explainSummary, error) {
	explainSQL := "EXPLAIN " + rewrittenSQL

	rows, err := h.db.QueryContext(ctx, explainSQL, args...)
	if err != nil {
		return explainSummary{}, err
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return explainSummary{}, err
	}
	values := make([]any, len(columns))
	valuePointers := make([]any, len(columns))
	for i := range values {
		valuePointers[i] = &values[i]
	}

	mysqlRows := make([]mysqlExplainRowSummary, 0, 4)
	lines := make([]string, 0, 4)
	for rows.Next() {
		if err := rows.Scan(valuePointers...); err != nil {
			return explainSummary{}, err
		}

		byColumn := make(map[string]string, len(columns))
		line := make([]string, 0, len(columns))
		for i, column := range columns {
			columnValue := cleanExplainValue(toExplainValue(values[i]))
			line = append(line, column+"="+columnValue)
			byColumn[strings.ToLower(column)] = columnValue
		}
		lines = append(lines, strings.Join(line, "; "))

		mysqlRows = append(mysqlRows, mysqlExplainRowSummary{
			Table:      byColumn["table"],
			AccessType: strings.ToLower(byColumn["type"]),
			Key:        byColumn["key"],
			Rows:       parseInt64Value(byColumn["rows"]),
			Extra:      strings.ToLower(byColumn["extra"]),
		})
	}
	if err := rows.Err(); err != nil {
		return explainSummary{}, err
	}
	return explainSummary{
		Dialect:   h.dialect,
		RawPlan:   strings.Join(lines, "\n"),
		MySQLRows: mysqlRows,
	}, nil
}

func cleanExplainValue(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "<nil>" {
		return ""
	}
	return trimmed
}

func parseInt64Value(value string) int64 {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0
	}
	if number, err := strconv.ParseInt(trimmed, 10, 64); err == nil {
		return number
	}
	if floatNumber, err := strconv.ParseFloat(trimmed, 64); err == nil {
		return int64(floatNumber)
	}
	return 0
}

func toExplainValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return "<nil>"
	case []byte:
		return string(typed)
	default:
		return fmt.Sprint(typed)
	}
}

func rewriteNamedParameters(
	dialect aip.SQLDialect,
	sqlText string,
	params []aip.QueryParameter,
) (string, []any, error) {
	byName := make(map[string]any, len(params))
	for _, param := range params {
		byName["@"+param.Name] = param.Value
	}

	var args []any
	var builder strings.Builder
	builder.Grow(len(sqlText) + 16)

	for i := 0; i < len(sqlText); {
		ch := sqlText[i]
		if ch != '@' {
			builder.WriteByte(ch)
			i++
			continue
		}

		if i+1 < len(sqlText) && sqlText[i+1] == '@' {
			builder.WriteString("@@")
			i += 2
			continue
		}

		j := i + 1
		for j < len(sqlText) && isNamedParameterChar(sqlText[j]) {
			j++
		}
		if j == i+1 {
			builder.WriteByte(sqlText[i])
			i++
			continue
		}

		token := sqlText[i:j]
		value, ok := byName[token]
		if !ok {
			return "", nil, fmt.Errorf("missing query parameter value for %s", token)
		}
		args = append(args, value)

		if dialect == aip.SQLDialectPostgres {
			builder.WriteString("$")
			builder.WriteString(strconv.Itoa(len(args)))
		} else {
			builder.WriteString("?")
		}
		i = j
	}

	return builder.String(), args, nil
}

func isNamedParameterChar(ch byte) bool {
	if ch >= 'a' && ch <= 'z' {
		return true
	}
	if ch >= 'A' && ch <= 'Z' {
		return true
	}
	if ch >= '0' && ch <= '9' {
		return true
	}
	return ch == '_'
}

func idsOverlap(a, b []itemRow) bool {
	seen := make(map[int64]struct{}, len(a))
	for _, row := range a {
		seen[row.ID] = struct{}{}
	}
	for _, row := range b {
		if _, ok := seen[row.ID]; ok {
			return true
		}
	}
	return false
}

func measureDurationStats(config perfMeasurementConfig, run func() error) (durationStats, error) {
	if config.WarmupRuns < 0 {
		return durationStats{}, fmt.Errorf("warmup runs must be >= 0")
	}
	if config.SampleRuns <= 0 {
		return durationStats{}, fmt.Errorf("sample runs must be > 0")
	}

	for i := 0; i < config.WarmupRuns; i++ {
		if err := run(); err != nil {
			return durationStats{}, err
		}
	}

	durations := make([]time.Duration, 0, config.SampleRuns)
	for i := 0; i < config.SampleRuns; i++ {
		start := time.Now()
		if err := run(); err != nil {
			return durationStats{}, err
		}
		durations = append(durations, time.Since(start))
	}
	return computeDurationStats(durations), nil
}

func computeDurationStats(durations []time.Duration) durationStats {
	if len(durations) == 0 {
		return durationStats{}
	}
	sorted := append([]time.Duration(nil), durations...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	return durationStats{
		Samples: sorted,
		Median:  sorted[len(sorted)/2],
		P95:     percentileDuration(sorted, 95),
		Min:     sorted[0],
		Max:     sorted[len(sorted)-1],
	}
}

func percentileDuration(sorted []time.Duration, percentile float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	if percentile <= 0 {
		return sorted[0]
	}
	if percentile >= 100 {
		return sorted[len(sorted)-1]
	}

	rank := int(math.Ceil(percentile/100*float64(len(sorted)))) - 1
	if rank < 0 {
		rank = 0
	}
	if rank >= len(sorted) {
		rank = len(sorted) - 1
	}
	return sorted[rank]
}

func durationRatio(target, baseline time.Duration) float64 {
	if baseline <= 0 {
		if target <= 0 {
			return 1
		}
		return math.Inf(1)
	}
	return float64(target) / float64(baseline)
}

func thresholdForScenario(scenario perfScenario) perfThreshold {
	threshold, ok := perfThresholds[scenario]
	if !ok {
		return perfThreshold{MaxMedianRatio: defaultPerfMaxRatio}
	}
	return threshold
}
