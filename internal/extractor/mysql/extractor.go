package mysql

import (
	"context"
	"database/sql"
	"pocket-doc/internal/model"
	"fmt"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// Extractor implements MySQL database metadata extraction
type Extractor struct {
	db           *sql.DB
	config       Config
	schemaFilter []string
}

// Config holds MySQL-specific configuration
type Config struct {
	Host         string
	Port         int
	Database     string
	Username     string
	Password     string
	SchemaFilter []string // Filter by SCHEMA
}

// NewExtractor creates a new MySQL extractor
func NewExtractor(cfg Config) (*Extractor, error) {
	// Build MySQL DSN: user:password@tcp(host:port)/dbname
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true",
		cfg.Username, cfg.Password, cfg.Host, cfg.Port, cfg.Database)

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open mysql connection: %w", err)
	}

	schemas := cfg.SchemaFilter
	if len(schemas) == 0 {
		schemas = []string{cfg.Database} // Default to connected database
	}

	return &Extractor{
		db:           db,
		config:       cfg,
		schemaFilter: schemas,
	}, nil
}

// Connect establishes connection
func (e *Extractor) Connect(ctx context.Context) error {
	return e.db.PingContext(ctx)
}

// Close releases resources
func (e *Extractor) Close() error {
	if e.db != nil {
		return e.db.Close()
	}
	return nil
}

// GetDatabaseInfo retrieves database information
func (e *Extractor) GetDatabaseInfo(ctx context.Context) (name, version string, err error) {
	err = e.db.QueryRowContext(ctx, "SELECT DATABASE(), VERSION()").Scan(&name, &version)
	return
}

// GetTables extracts tables with COMMENTS from INFORMATION_SCHEMA (CRITICAL RULE #1)
func (e *Extractor) GetTables(ctx context.Context) ([]model.Table, error) {
	query := `
		SELECT 
			TABLE_SCHEMA,
			TABLE_NAME,
			ENGINE,
			TABLE_ROWS,
			IFNULL(TABLE_COMMENT, '') as TABLE_COMMENT,
			CREATE_TIME,
			UPDATE_TIME
		FROM INFORMATION_SCHEMA.TABLES
		WHERE TABLE_TYPE = 'BASE TABLE'
	`

	// CRITICAL RULE #2: Schema filtering
	if len(e.schemaFilter) > 0 {
		placeholders := make([]string, len(e.schemaFilter))
		for i := range e.schemaFilter {
			placeholders[i] = "?"
		}
		query += fmt.Sprintf(" AND TABLE_SCHEMA IN (%s)", strings.Join(placeholders, ","))
	}

	query += " ORDER BY TABLE_SCHEMA, TABLE_NAME"

	var args []interface{}
	for _, schema := range e.schemaFilter {
		args = append(args, schema)
	}

	rows, err := e.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query tables: %w", err)
	}
	defer rows.Close()

	var tables []model.Table
	for rows.Next() {
		var t model.Table
		var rowCount sql.NullInt64
		var engine sql.NullString
		var createTime, updateTime sql.NullTime

		err := rows.Scan(
			&t.Owner, &t.Name, &engine, &rowCount, &t.Comment,
			&createTime, &updateTime,
		)
		if err != nil {
			return nil, err
		}

		if engine.Valid {
			t.Type = engine.String
		}
		if rowCount.Valid {
			t.RowCount = rowCount.Int64
		}
		if createTime.Valid {
			t.CreatedAt = createTime.Time.Format("2006-01-02 15:04:05")
		}
		if updateTime.Valid {
			t.ModifiedAt = updateTime.Time.Format("2006-01-02 15:04:05")
		}

		// Fetch columns
		t.Columns, err = e.getColumnsForTable(ctx, t.Owner, t.Name)
		if err != nil {
			return nil, err
		}

		// Fetch indexes
		t.Indexes, err = e.getIndexesForTable(ctx, t.Owner, t.Name)
		if err != nil {
			return nil, err
		}

		tables = append(tables, t)
	}

	return tables, rows.Err()
}

// getColumnsForTable retrieves columns with COLUMN_COMMENT (CRITICAL RULE #1)
func (e *Extractor) getColumnsForTable(ctx context.Context, schema, tableName string) ([]model.Column, error) {
	query := `
		SELECT 
			COLUMN_NAME,
			ORDINAL_POSITION,
			DATA_TYPE,
			IFNULL(CHARACTER_MAXIMUM_LENGTH, 0),
			IFNULL(NUMERIC_PRECISION, 0),
			IFNULL(NUMERIC_SCALE, 0),
			IS_NULLABLE,
			IFNULL(COLUMN_DEFAULT, ''),
			IFNULL(COLUMN_COMMENT, ''),
			COLUMN_KEY,
			EXTRA
		FROM INFORMATION_SCHEMA.COLUMNS
		WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ?
		ORDER BY ORDINAL_POSITION
	`

	rows, err := e.db.QueryContext(ctx, query, schema, tableName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var columns []model.Column
	for rows.Next() {
		var col model.Column
		var nullable, columnKey, extra string

		err := rows.Scan(
			&col.Name, &col.Position, &col.DataType, &col.Length,
			&col.Precision, &col.Scale, &nullable, &col.DefaultValue,
			&col.Comment, &columnKey, &extra,
		)
		if err != nil {
			return nil, err
		}

		col.Nullable = (nullable == "YES")
		col.IsPrimaryKey = (columnKey == "PRI")
		col.IsForeignKey = (columnKey == "MUL" || columnKey == "FOR")
		col.IsUnique = (columnKey == "UNI")
		col.IsAutoIncrement = strings.Contains(extra, "auto_increment")

		// Get FK target info if applicable
		if col.IsForeignKey {
			fkInfo, err := e.getForeignKeyTarget(ctx, schema, tableName, col.Name)
			if err == nil && fkInfo != nil {
				col.FKTargetTable = fkInfo["table"]
				col.FKTargetColumn = fkInfo["column"]
			}
		}

		columns = append(columns, col)
	}

	return columns, rows.Err()
}

// getForeignKeyTarget retrieves FK target information
func (e *Extractor) getForeignKeyTarget(ctx context.Context, schema, table, column string) (map[string]string, error) {
	query := `
		SELECT 
			REFERENCED_TABLE_NAME,
			REFERENCED_COLUMN_NAME
		FROM INFORMATION_SCHEMA.KEY_COLUMN_USAGE
		WHERE TABLE_SCHEMA = ? 
		AND TABLE_NAME = ? 
		AND COLUMN_NAME = ?
		AND REFERENCED_TABLE_NAME IS NOT NULL
		LIMIT 1
	`

	var refTable, refColumn sql.NullString
	err := e.db.QueryRowContext(ctx, query, schema, table, column).Scan(&refTable, &refColumn)
	if err != nil {
		return nil, err
	}

	if refTable.Valid && refColumn.Valid {
		return map[string]string{
			"table":  refTable.String,
			"column": refColumn.String,
		}, nil
	}

	return nil, nil
}

// getIndexesForTable retrieves indexes
func (e *Extractor) getIndexesForTable(ctx context.Context, schema, tableName string) ([]model.Index, error) {
	query := `
		SELECT DISTINCT
			INDEX_NAME,
			INDEX_TYPE,
			NON_UNIQUE
		FROM INFORMATION_SCHEMA.STATISTICS
		WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ?
		ORDER BY INDEX_NAME
	`

	rows, err := e.db.QueryContext(ctx, query, schema, tableName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var indexes []model.Index
	for rows.Next() {
		var idx model.Index
		var nonUnique int

		err := rows.Scan(&idx.Name, &idx.Type, &nonUnique)
		if err != nil {
			return nil, err
		}

		idx.TableName = tableName
		idx.Owner = schema
		idx.IsUnique = (nonUnique == 0)
		idx.IsPrimary = (idx.Name == "PRIMARY")
		idx.IsEnabled = true
		idx.Comment = ""

		// Fetch columns
		idx.Columns, err = e.getIndexColumns(ctx, schema, tableName, idx.Name)
		if err != nil {
			return nil, err
		}

		indexes = append(indexes, idx)
	}

	return indexes, rows.Err()
}

// getIndexColumns retrieves columns for an index
func (e *Extractor) getIndexColumns(ctx context.Context, schema, table, indexName string) ([]string, error) {
	query := `
		SELECT COLUMN_NAME
		FROM INFORMATION_SCHEMA.STATISTICS
		WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ? AND INDEX_NAME = ?
		ORDER BY SEQ_IN_INDEX
	`

	rows, err := e.db.QueryContext(ctx, query, schema, table, indexName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var columns []string
	for rows.Next() {
		var col string
		if err := rows.Scan(&col); err != nil {
			return nil, err
		}
		columns = append(columns, col)
	}

	return columns, rows.Err()
}

// GetViews extracts views with COMMENTS (NO definition - security!)
func (e *Extractor) GetViews(ctx context.Context) ([]model.View, error) {
	query := `
		SELECT 
			TABLE_SCHEMA,
			TABLE_NAME,
			IFNULL(TABLE_COMMENT, ''),
			IS_UPDATABLE
		FROM INFORMATION_SCHEMA.VIEWS
		WHERE 1=1
	`

	if len(e.schemaFilter) > 0 {
		placeholders := make([]string, len(e.schemaFilter))
		for i := range e.schemaFilter {
			placeholders[i] = "?"
		}
		query += fmt.Sprintf(" AND TABLE_SCHEMA IN (%s)", strings.Join(placeholders, ","))
	}

	var args []interface{}
	for _, schema := range e.schemaFilter {
		args = append(args, schema)
	}

	rows, err := e.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var views []model.View
	for rows.Next() {
		var v model.View
		var updatable string

		err := rows.Scan(&v.Owner, &v.Name, &v.Comment, &updatable)
		if err != nil {
			return nil, err
		}

		v.Type = "VIEW"
		v.IsUpdatable = (updatable == "YES")

		// Fetch columns
		v.Columns, err = e.getColumnsForTable(ctx, v.Owner, v.Name)
		if err != nil {
			return nil, err
		}

		views = append(views, v)
	}

	return views, rows.Err()
}

// GetRoutines extracts procedures/functions with COMMENTS (NO source - security!)
func (e *Extractor) GetRoutines(ctx context.Context) ([]model.Routine, error) {
	query := `
		SELECT 
			ROUTINE_SCHEMA,
			ROUTINE_NAME,
			ROUTINE_TYPE,
			IFNULL(ROUTINE_COMMENT, ''),
			IFNULL(DTD_IDENTIFIER, '')
		FROM INFORMATION_SCHEMA.ROUTINES
		WHERE 1=1
	`

	if len(e.schemaFilter) > 0 {
		placeholders := make([]string, len(e.schemaFilter))
		for i := range e.schemaFilter {
			placeholders[i] = "?"
		}
		query += fmt.Sprintf(" AND ROUTINE_SCHEMA IN (%s)", strings.Join(placeholders, ","))
	}

	var args []interface{}
	for _, schema := range e.schemaFilter {
		args = append(args, schema)
	}

	rows, err := e.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var routines []model.Routine
	for rows.Next() {
		var r model.Routine
		var returnType string

		err := rows.Scan(&r.Owner, &r.Name, &r.Type, &r.Comment, &returnType)
		if err != nil {
			return nil, err
		}

		if r.Type == "FUNCTION" {
			r.ReturnType = returnType
		}
		r.Language = "SQL"

		// Fetch parameters
		r.Arguments, err = e.getRoutineParameters(ctx, r.Owner, r.Name)
		if err != nil {
			return nil, err
		}

		// Build signature
		r.Signature = e.buildSignature(r.Name, r.Arguments, r.Type)

		routines = append(routines, r)
	}

	return routines, rows.Err()
}

// getRoutineParameters retrieves parameters
func (e *Extractor) getRoutineParameters(ctx context.Context, schema, routineName string) ([]model.RoutineArgument, error) {
	query := `
		SELECT 
			PARAMETER_NAME,
			ORDINAL_POSITION,
			PARAMETER_MODE,
			DATA_TYPE
		FROM INFORMATION_SCHEMA.PARAMETERS
		WHERE SPECIFIC_SCHEMA = ? AND SPECIFIC_NAME = ?
		AND PARAMETER_NAME IS NOT NULL
		ORDER BY ORDINAL_POSITION
	`

	rows, err := e.db.QueryContext(ctx, query, schema, routineName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var args []model.RoutineArgument
	for rows.Next() {
		var arg model.RoutineArgument

		err := rows.Scan(&arg.Name, &arg.Position, &arg.Mode, &arg.DataType)
		if err != nil {
			return nil, err
		}

		args = append(args, arg)
	}

	return args, rows.Err()
}

// buildSignature creates routine signature
func (e *Extractor) buildSignature(name string, args []model.RoutineArgument, routineType string) string {
	argStrs := make([]string, len(args))
	for i, arg := range args {
		argStrs[i] = fmt.Sprintf("%s %s %s", arg.Mode, arg.Name, arg.DataType)
	}

	return fmt.Sprintf("%s %s(%s)", routineType, name, strings.Join(argStrs, ", "))
}

// GetSequences - MySQL doesn't have sequences (use AUTO_INCREMENT)
func (e *Extractor) GetSequences(ctx context.Context) ([]model.Sequence, error) {
	return []model.Sequence{}, nil
}

// GetTriggers extracts triggers with COMMENTS (NO body - security!)
func (e *Extractor) GetTriggers(ctx context.Context) ([]model.Trigger, error) {
	query := `
		SELECT 
			TRIGGER_SCHEMA,
			TRIGGER_NAME,
			EVENT_OBJECT_SCHEMA,
			EVENT_OBJECT_TABLE,
			ACTION_TIMING,
			EVENT_MANIPULATION,
			'ENABLED' as STATUS
		FROM INFORMATION_SCHEMA.TRIGGERS
		WHERE 1=1
	`

	if len(e.schemaFilter) > 0 {
		placeholders := make([]string, len(e.schemaFilter))
		for i := range e.schemaFilter {
			placeholders[i] = "?"
		}
		query += fmt.Sprintf(" AND TRIGGER_SCHEMA IN (%s)", strings.Join(placeholders, ","))
	}

	var args []interface{}
	for _, schema := range e.schemaFilter {
		args = append(args, schema)
	}

	rows, err := e.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var triggers []model.Trigger
	for rows.Next() {
		var trg model.Trigger
		var objectSchema string

		err := rows.Scan(
			&trg.Owner, &trg.Name, &objectSchema, &trg.TargetTable,
			&trg.Timing, &trg.Event, &trg.Status,
		)
		if err != nil {
			return nil, err
		}

		trg.TargetType = "TABLE"
		trg.Level = "ROW" // MySQL triggers are row-level
		trg.Comment = ""

		triggers = append(triggers, trg)
	}

	return triggers, rows.Err()
}

// GetSynonyms - MySQL doesn't have synonyms
func (e *Extractor) GetSynonyms(ctx context.Context) ([]model.Synonym, error) {
	return []model.Synonym{}, nil
}

// ExtractSchema performs complete extraction
func (e *Extractor) ExtractSchema(ctx context.Context) (*model.Schema, error) {
	schema := &model.Schema{
		ExtractedAt: time.Now(),
	}

	var err error
	schema.DatabaseName, schema.Version, err = e.GetDatabaseInfo(ctx)
	if err != nil {
		return nil, err
	}
	schema.DatabaseType = "MySQL"

	schema.Tables, err = e.GetTables(ctx)
	if err != nil {
		return nil, err
	}

	schema.Views, err = e.GetViews(ctx)
	if err != nil {
		return nil, err
	}

	schema.Routines, err = e.GetRoutines(ctx)
	if err != nil {
		return nil, err
	}

	schema.Sequences, err = e.GetSequences(ctx)
	if err != nil {
		return nil, err
	}

	schema.Triggers, err = e.GetTriggers(ctx)
	if err != nil {
		return nil, err
	}

	schema.Synonyms, err = e.GetSynonyms(ctx)
	if err != nil {
		return nil, err
	}

	for _, table := range schema.Tables {
		schema.Indexes = append(schema.Indexes, table.Indexes...)
	}

	return schema, nil
}
