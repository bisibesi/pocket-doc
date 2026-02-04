package mssql

import (
	"context"
	"database/sql"
	"pocket-doc/internal/model"
	"fmt"
	"strings"
	"time"

	_ "github.com/microsoft/go-mssqldb"
)

// Extractor implements MSSQL database metadata extraction
type Extractor struct {
	db           *sql.DB
	config       Config
	schemaFilter []string
}

// Config holds MSSQL-specific configuration
type Config struct {
	Host         string
	Port         int
	Database     string
	Username     string
	Password     string
	Encrypt      string   // disable, false, true
	SchemaFilter []string // Filter by schema
}

// NewExtractor creates a new MSSQL extractor
func NewExtractor(cfg Config) (*Extractor, error) {
	// Build MSSQL connection string
	// Format: sqlserver://user:password@host:port?database=dbname&encrypt=disable
	connStr := fmt.Sprintf("sqlserver://%s:%s@%s:%d?database=%s&encrypt=%s",
		cfg.Username, cfg.Password, cfg.Host, cfg.Port, cfg.Database, cfg.Encrypt)

	db, err := sql.Open("sqlserver", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open mssql connection: %w", err)
	}

	schemas := cfg.SchemaFilter
	if len(schemas) == 0 {
		schemas = []string{"dbo"} // Default schema
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
	err = e.db.QueryRowContext(ctx, "SELECT DB_NAME(), @@VERSION").Scan(&name, &version)
	return
}

// GetTables extracts tables with COMMENTS from sys.extended_properties (CRITICAL RULE #1)
func (e *Extractor) GetTables(ctx context.Context) ([]model.Table, error) {
	query := `
		SELECT 
			s.name as schema_name,
			t.name as table_name,
			t.type_desc,
			ISNULL(ep.value, '') as table_comment,
			ISNULL(ps.row_count, 0) as row_count,
			t.create_date,
			t.modify_date
		FROM sys.tables t
		JOIN sys.schemas s ON s.schema_id = t.schema_id
		LEFT JOIN sys.extended_properties ep 
			ON ep.major_id = t.object_id 
			AND ep.minor_id = 0 
			AND ep.name = 'MS_Description'
		LEFT JOIN (
			SELECT object_id, SUM(rows) as row_count
			FROM sys.partitions
			WHERE index_id IN (0,1)
			GROUP BY object_id
		) ps ON ps.object_id = t.object_id
		WHERE 1=1
	`

	// CRITICAL RULE #2: Schema filtering
	if len(e.schemaFilter) > 0 {
		placeholders := make([]string, len(e.schemaFilter))
		for i := range e.schemaFilter {
			placeholders[i] = fmt.Sprintf("@p%d", i+1)
		}
		query += fmt.Sprintf(" AND s.name IN (%s)", strings.Join(placeholders, ","))
	}

	query += " ORDER BY s.name, t.name"

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
		var createDate, modifyDate sql.NullTime

		err := rows.Scan(
			&t.Owner, &t.Name, &t.Type, &t.Comment, &rowCount,
			&createDate, &modifyDate,
		)
		if err != nil {
			return nil, err
		}

		if rowCount.Valid {
			t.RowCount = rowCount.Int64
		}
		if createDate.Valid {
			t.CreatedAt = createDate.Time.Format("2006-01-02 15:04:05")
		}
		if modifyDate.Valid {
			t.ModifiedAt = modifyDate.Time.Format("2006-01-02 15:04:05")
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

// getColumnsForTable retrieves columns with MS_Description (CRITICAL RULE #1)
func (e *Extractor) getColumnsForTable(ctx context.Context, schema, tableName string) ([]model.Column, error) {
	query := `
		SELECT 
			c.column_id as position,
			c.name as column_name,
			ty.name as data_type,
			c.max_length,
			c.precision,
			c.scale,
			c.is_nullable,
			ISNULL(dc.definition, '') as default_value,
			ISNULL(ep.value, '') as column_comment,
			c.is_identity,
			ISNULL(ic.is_primary_key, 0) as is_primary,
			ISNULL(fk.is_foreign_key, 0) as is_foreign,
			ISNULL(uc.is_unique, 0) as is_unique
		FROM sys.columns c
		JOIN sys.tables t ON t.object_id = c.object_id
		JOIN sys.schemas s ON s.schema_id = t.schema_id
		JOIN sys.types ty ON ty.user_type_id = c.user_type_id
		LEFT JOIN sys.extended_properties ep 
			ON ep.major_id = c.object_id 
			AND ep.minor_id = c.column_id 
			AND ep.name = 'MS_Description'
		LEFT JOIN sys.default_constraints dc ON dc.parent_object_id = c.object_id AND dc.parent_column_id = c.column_id
		LEFT JOIN (
			SELECT ic.object_id, ic.column_id, 1 as is_primary_key
			FROM sys.index_columns ic
			JOIN sys.indexes i ON i.object_id = ic.object_id AND i.index_id = ic.index_id
			WHERE i.is_primary_key = 1
		) ic ON ic.object_id = c.object_id AND ic.column_id = c.column_id
		LEFT JOIN (
			SELECT fkc.parent_object_id, fkc.parent_column_id, 1 as is_foreign_key
			FROM sys.foreign_key_columns fkc
		) fk ON fk.parent_object_id = c.object_id AND fk.parent_column_id = c.column_id
		LEFT JOIN (
			SELECT ic.object_id, ic.column_id, 1 as is_unique
			FROM sys.index_columns ic
			JOIN sys.indexes i ON i.object_id = ic.object_id AND i.index_id = ic.index_id
			WHERE i.is_unique = 1 AND i.is_primary_key = 0
		) uc ON uc.object_id = c.object_id AND uc.column_id = c.column_id
		WHERE s.name = @p1 AND t.name = @p2
		ORDER BY c.column_id
	`

	rows, err := e.db.QueryContext(ctx, query, schema, tableName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var columns []model.Column
	for rows.Next() {
		var col model.Column
		var isNullable, isIdentity, isPrimary, isForeign, isUnique bool

		err := rows.Scan(
			&col.Position, &col.Name, &col.DataType, &col.Length,
			&col.Precision, &col.Scale, &isNullable, &col.DefaultValue,
			&col.Comment, &isIdentity,
			&isPrimary, &isForeign, &isUnique,
		)
		if err != nil {
			return nil, err
		}

		col.Nullable = isNullable
		col.IsPrimaryKey = isPrimary
		col.IsForeignKey = isForeign
		col.IsUnique = isUnique
		col.IsAutoIncrement = isIdentity

		// Get FK target if applicable
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

// getForeignKeyTarget retrieves FK information
func (e *Extractor) getForeignKeyTarget(ctx context.Context, schema, table, column string) (map[string]string, error) {
	query := `
		SELECT 
			OBJECT_SCHEMA_NAME(fk.referenced_object_id) + '.' + OBJECT_NAME(fk.referenced_object_id) as ref_table,
			COL_NAME(fk.referenced_object_id, fkc.referenced_column_id) as ref_column
		FROM sys.foreign_keys fk
		JOIN sys.foreign_key_columns fkc ON fk.object_id = fkc.constraint_object_id
		JOIN sys.tables t ON t.object_id = fk.parent_object_id
		JOIN sys.schemas s ON s.schema_id = t.schema_id
		JOIN sys.columns c ON c.object_id = t.object_id AND c.column_id = fkc.parent_column_id
		WHERE s.name = @p1 AND t.name = @p2 AND c.name = @p3
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
			i.name as index_name,
			i.type_desc as index_type,
			i.is_unique,
			i.is_primary_key,
			ISNULL(ep.value, '') as index_comment
		FROM sys.indexes i
		JOIN sys.tables t ON t.object_id = i.object_id
		JOIN sys.schemas s ON s.schema_id = t.schema_id
		LEFT JOIN sys.extended_properties ep 
			ON ep.major_id = i.object_id 
			AND ep.minor_id = i.index_id 
			AND ep.name = 'MS_Description'
		WHERE s.name = @p1 AND t.name = @p2
		AND i.name IS NOT NULL
		ORDER BY i.name
	`

	rows, err := e.db.QueryContext(ctx, query, schema, tableName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var indexes []model.Index
	for rows.Next() {
		var idx model.Index
		var isUnique, isPrimary bool

		err := rows.Scan(&idx.Name, &idx.Type, &isUnique, &isPrimary, &idx.Comment)
		if err != nil {
			return nil, err
		}

		idx.TableName = tableName
		idx.Owner = schema
		idx.IsUnique = isUnique
		idx.IsPrimary = isPrimary
		idx.IsEnabled = true

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
		SELECT c.name
		FROM sys.index_columns ic
		JOIN sys.indexes i ON i.object_id = ic.object_id AND i.index_id = ic.index_id
		JOIN sys.columns c ON c.object_id = ic.object_id AND c.column_id = ic.column_id
		JOIN sys.tables t ON t.object_id = i.object_id
		JOIN sys.schemas s ON s.schema_id = t.schema_id
		WHERE s.name = @p1 AND t.name = @p2 AND i.name = @p3
		ORDER BY ic.key_ordinal
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

// GetViews extracts views with MS_Description (NO definition - security!)
func (e *Extractor) GetViews(ctx context.Context) ([]model.View, error) {
	query := `
		SELECT 
			s.name as schema_name,
			v.name as view_name,
			ISNULL(ep.value, '') as view_comment,
			CASE WHEN EXISTS(
				SELECT 1 FROM sys.sql_modules m 
				WHERE m.object_id = v.object_id 
				AND m.definition LIKE '%INSTEAD OF%'
			) THEN 1 ELSE 0 END as is_updatable
		FROM sys.views v
		JOIN sys.schemas s ON s.schema_id = v.schema_id
		LEFT JOIN sys.extended_properties ep 
			ON ep.major_id = v.object_id 
			AND ep.minor_id = 0 
			AND ep.name = 'MS_Description'
		WHERE 1=1
	`

	if len(e.schemaFilter) > 0 {
		placeholders := make([]string, len(e.schemaFilter))
		for i := range e.schemaFilter {
			placeholders[i] = fmt.Sprintf("@p%d", i+1)
		}
		query += fmt.Sprintf(" AND s.name IN (%s)", strings.Join(placeholders, ","))
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
		var isUpdatable int

		err := rows.Scan(&v.Owner, &v.Name, &v.Comment, &isUpdatable)
		if err != nil {
			return nil, err
		}

		v.Type = "VIEW"
		v.IsUpdatable = (isUpdatable == 1)

		// Fetch columns (reuse table column query)
		v.Columns, err = e.getColumnsForView(ctx, v.Owner, v.Name)
		if err != nil {
			return nil, err
		}

		views = append(views, v)
	}

	return views, rows.Err()
}

// getColumnsForView retrieves columns for a view
func (e *Extractor) getColumnsForView(ctx context.Context, schema, viewName string) ([]model.Column, error) {
	query := `
		SELECT 
			c.column_id as position,
			c.name as column_name,
			ty.name as data_type,
			ISNULL(ep.value, '') as column_comment
		FROM sys.columns c
		JOIN sys.views v ON v.object_id = c.object_id
		JOIN sys.schemas s ON s.schema_id = v.schema_id
		JOIN sys.types ty ON ty.user_type_id = c.user_type_id
		LEFT JOIN sys.extended_properties ep 
			ON ep.major_id = c.object_id 
			AND ep.minor_id = c.column_id 
			AND ep.name = 'MS_Description'
		WHERE s.name = @p1 AND v.name = @p2
		ORDER BY c.column_id
	`

	rows, err := e.db.QueryContext(ctx, query, schema, viewName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var columns []model.Column
	for rows.Next() {
		var col model.Column

		err := rows.Scan(&col.Position, &col.Name, &col.DataType, &col.Comment)
		if err != nil {
			return nil, err
		}

		columns = append(columns, col)
	}

	return columns, rows.Err()
}

// GetRoutines extracts procedures/functions with MS_Description (NO source - security!)
func (e *Extractor) GetRoutines(ctx context.Context) ([]model.Routine, error) {
	query := `
		SELECT 
			s.name as schema_name,
			p.name as routine_name,
			p.type_desc as routine_type,
			ISNULL(ep.value, '') as routine_comment
		FROM sys.procedures p
		JOIN sys.schemas s ON s.schema_id = p.schema_id
		LEFT JOIN sys.extended_properties ep 
			ON ep.major_id = p.object_id 
			AND ep.minor_id = 0 
			AND ep.name = 'MS_Description'
		WHERE 1=1
	`

	if len(e.schemaFilter) > 0 {
		placeholders := make([]string, len(e.schemaFilter))
		for i := range e.schemaFilter {
			placeholders[i] = fmt.Sprintf("@p%d", i+1)
		}
		query += fmt.Sprintf(" AND s.name IN (%s)", strings.Join(placeholders, ","))
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

		err := rows.Scan(&r.Owner, &r.Name, &r.Type, &r.Comment)
		if err != nil {
			return nil, err
		}

		r.Language = "T-SQL"

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
			p.name as parameter_name,
			p.parameter_id as position,
			CASE WHEN p.is_output = 1 THEN 'OUT' ELSE 'IN' END as mode,
			ty.name as data_type
		FROM sys.parameters p
		JOIN sys.procedures proc ON proc.object_id = p.object_id
		JOIN sys.schemas s ON s.schema_id = proc.schema_id
		JOIN sys.types ty ON ty.user_type_id = p.user_type_id
		WHERE s.name = @p1 AND proc.name = @p2
		ORDER BY p.parameter_id
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
		argStrs[i] = fmt.Sprintf("%s %s %s", arg.Name, arg.Mode, arg.DataType)
	}

	return fmt.Sprintf("%s %s(%s)", routineType, name, strings.Join(argStrs, ", "))
}

// GetSequences extracts sequences with MS_Description
func (e *Extractor) GetSequences(ctx context.Context) ([]model.Sequence, error) {
	query := `
		SELECT 
			s.name as schema_name,
			seq.name as sequence_name,
			seq.minimum_value,
			seq.maximum_value,
			seq.increment,
			seq.current_value,
			seq.is_cycling,
			ISNULL(ep.value, '') as seq_comment
		FROM sys.sequences seq
		JOIN sys.schemas s ON s.schema_id = seq.schema_id
		LEFT JOIN sys.extended_properties ep 
			ON ep.major_id = seq.object_id 
			AND ep.minor_id = 0 
			AND ep.name = 'MS_Description'
		WHERE 1=1
	`

	if len(e.schemaFilter) > 0 {
		placeholders := make([]string, len(e.schemaFilter))
		for i := range e.schemaFilter {
			placeholders[i] = fmt.Sprintf("@p%d", i+1)
		}
		query += fmt.Sprintf(" AND s.name IN (%s)", strings.Join(placeholders, ","))
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

	var sequences []model.Sequence
	for rows.Next() {
		var seq model.Sequence
		var isCycling bool

		err := rows.Scan(
			&seq.Owner, &seq.Name, &seq.MinValue, &seq.MaxValue,
			&seq.Increment, &seq.LastNumber, &isCycling, &seq.Comment,
		)
		if err != nil {
			return nil, err
		}

		seq.IsCyclic = isCycling

		sequences = append(sequences, seq)
	}

	return sequences, rows.Err()
}

// GetTriggers extracts triggers with MS_Description (NO body - security!)
func (e *Extractor) GetTriggers(ctx context.Context) ([]model.Trigger, error) {
	query := `
		SELECT 
			s.name as schema_name,
			tr.name as trigger_name,
			OBJECT_NAME(tr.parent_id) as table_name,
			CASE 
				WHEN OBJECTPROPERTY(tr.object_id, 'ExecIsInsertTrigger') = 1 THEN 'INSERT'
				WHEN OBJECTPROPERTY(tr.object_id, 'ExecIsUpdateTrigger') = 1 THEN 'UPDATE'
				WHEN OBJECTPROPERTY(tr.object_id, 'ExecIsDeleteTrigger') = 1 THEN 'DELETE'
				ELSE 'UNKNOWN'
			END as event,
			CASE 
				WHEN OBJECTPROPERTY(tr.object_id, 'ExecIsAfterTrigger') = 1 THEN 'AFTER'
				WHEN OBJECTPROPERTY(tr.object_id, 'ExecIsInsteadOfTrigger') = 1 THEN 'INSTEAD OF'
				ELSE 'UNKNOWN'
			END as timing,
			CASE WHEN tr.is_disabled = 0 THEN 'ENABLED' ELSE 'DISABLED' END as status,
			ISNULL(ep.value, '') as trigger_comment
		FROM sys.triggers tr
		JOIN sys.tables t ON t.object_id = tr.parent_id
		JOIN sys.schemas s ON s.schema_id = t.schema_id
		LEFT JOIN sys.extended_properties ep 
			ON ep.major_id = tr.object_id 
			AND ep.minor_id = 0 
			AND ep.name = 'MS_Description'
		WHERE tr.is_ms_shipped = 0
	`

	if len(e.schemaFilter) > 0 {
		placeholders := make([]string, len(e.schemaFilter))
		for i := range e.schemaFilter {
			placeholders[i] = fmt.Sprintf("@p%d", i+1)
		}
		query += fmt.Sprintf(" AND s.name IN (%s)", strings.Join(placeholders, ","))
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

		err := rows.Scan(
			&trg.Owner, &trg.Name, &trg.TargetTable, &trg.Event,
			&trg.Timing, &trg.Status, &trg.Comment,
		)
		if err != nil {
			return nil, err
		}

		trg.TargetType = "TABLE"
		trg.Level = "ROW" // MSSQL triggers can be row or statement, simplified here

		triggers = append(triggers, trg)
	}

	return triggers, rows.Err()
}

// GetSynonyms extracts synonyms with MS_Description
func (e *Extractor) GetSynonyms(ctx context.Context) ([]model.Synonym, error) {
	query := `
		SELECT 
			s.name as schema_name,
			syn.name as synonym_name,
			syn.base_object_name as target_object,
			ISNULL(ep.value, '') as synonym_comment
		FROM sys.synonyms syn
		JOIN sys.schemas s ON s.schema_id = syn.schema_id
		LEFT JOIN sys.extended_properties ep 
			ON ep.major_id = syn.object_id 
			AND ep.minor_id = 0 
			AND ep.name = 'MS_Description'
		WHERE 1=1
	`

	if len(e.schemaFilter) > 0 {
		placeholders := make([]string, len(e.schemaFilter))
		for i := range e.schemaFilter {
			placeholders[i] = fmt.Sprintf("@p%d", i+1)
		}
		query += fmt.Sprintf(" AND s.name IN (%s)", strings.Join(placeholders, ","))
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

	var synonyms []model.Synonym
	for rows.Next() {
		var syn model.Synonym

		err := rows.Scan(&syn.Owner, &syn.Name, &syn.TargetObject, &syn.Comment)
		if err != nil {
			return nil, err
		}

		// Parse target (may include schema)
		parts := strings.Split(syn.TargetObject, ".")
		if len(parts) >= 2 {
			syn.TargetOwner = parts[0]
			syn.TargetObject = parts[1]
		}
		syn.TargetType = "TABLE" // Simplified

		synonyms = append(synonyms, syn)
	}

	return synonyms, rows.Err()
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
	schema.DatabaseType = "MSSQL"

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
