package oracle

import (
	"context"
	"database/sql"
	"dbms-to-document/internal/model"
	"fmt"
	"strings"
	"time"

	_ "github.com/sijms/go-ora/v2"
)

// Extractor implements Oracle database metadata extraction
type Extractor struct {
	db           *sql.DB
	config       Config
	schemaFilter []string
}

// Config holds Oracle-specific configuration
type Config struct {
	Host         string
	Port         int
	ServiceName  string
	Username     string
	Password     string
	SchemaFilter []string // Filter by OWNER
}

// NewExtractor creates a new Oracle extractor
func NewExtractor(cfg Config) (*Extractor, error) {
	// Build Oracle connection string (Pure Go driver - NO CGO)
	// Format: oracle://user:pass@host:port/serviceName
	connStr := fmt.Sprintf("oracle://%s:%s@%s:%d/%s",
		cfg.Username, cfg.Password, cfg.Host, cfg.Port, cfg.ServiceName)

	db, err := sql.Open("oracle", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open oracle connection: %w", err)
	}

	return &Extractor{
		db:           db,
		config:       cfg,
		schemaFilter: cfg.SchemaFilter,
	}, nil
}

// Connect establishes connection to Oracle
func (e *Extractor) Connect(ctx context.Context) error {
	return e.db.PingContext(ctx)
}

// Close releases database resources
func (e *Extractor) Close() error {
	if e.db != nil {
		return e.db.Close()
	}
	return nil
}

// GetDatabaseInfo retrieves basic database information
func (e *Extractor) GetDatabaseInfo(ctx context.Context) (name, version string, err error) {
	err = e.db.QueryRowContext(ctx, `
		SELECT 
			SYS_CONTEXT('USERENV', 'DB_NAME') as db_name,
			BANNER as version
		FROM V$VERSION
		WHERE ROWNUM = 1
	`).Scan(&name, &version)
	return
}

// GetTables extracts all table metadata with COMMENTS (CRITICAL RULE #1)
func (e *Extractor) GetTables(ctx context.Context) ([]model.Table, error) {
	query := `
		SELECT 
			t.OWNER,
			t.TABLE_NAME,
			t.TABLESPACE_NAME,
			t.NUM_ROWS,
			NVL(tc.COMMENTS, '') as TABLE_COMMENT,
			TO_CHAR(t.CREATED, 'YYYY-MM-DD HH24:MI:SS') as CREATED_AT,
			TO_CHAR(t.LAST_DDL_TIME, 'YYYY-MM-DD HH24:MI:SS') as MODIFIED_AT
		FROM ALL_TABLES t
		LEFT JOIN ALL_TAB_COMMENTS tc 
			ON t.OWNER = tc.OWNER AND t.TABLE_NAME = tc.TABLE_NAME
		WHERE 1=1
	`

	// CRITICAL RULE #2: Schema Filtering by OWNER
	if len(e.schemaFilter) > 0 {
		placeholders := make([]string, len(e.schemaFilter))
		for i := range e.schemaFilter {
			placeholders[i] = fmt.Sprintf(":%d", i+1)
		}
		query += fmt.Sprintf(" AND t.OWNER IN (%s)", strings.Join(placeholders, ","))
	}

	query += " ORDER BY t.OWNER, t.TABLE_NAME"

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
		var createdAt, modifiedAt sql.NullString

		err := rows.Scan(
			&t.Owner, &t.Name, &t.Type, &rowCount, &t.Comment,
			&createdAt, &modifiedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan table row: %w", err)
		}

		if rowCount.Valid {
			t.RowCount = rowCount.Int64
		}
		if createdAt.Valid {
			t.CreatedAt = createdAt.String
		}
		if modifiedAt.Valid {
			t.ModifiedAt = modifiedAt.String
		}

		// Fetch columns for this table
		t.Columns, err = e.getColumnsForTable(ctx, t.Owner, t.Name)
		if err != nil {
			return nil, fmt.Errorf("failed to get columns for %s.%s: %w", t.Owner, t.Name, err)
		}

		// Fetch indexes for this table
		t.Indexes, err = e.getIndexesForTable(ctx, t.Owner, t.Name)
		if err != nil {
			return nil, fmt.Errorf("failed to get indexes for %s.%s: %w", t.Owner, t.Name, err)
		}

		tables = append(tables, t)
	}

	return tables, rows.Err()
}

// getColumnsForTable retrieves columns with COMMENTS (CRITICAL RULE #1)
func (e *Extractor) getColumnsForTable(ctx context.Context, owner, tableName string) ([]model.Column, error) {
	query := `
		SELECT 
			c.COLUMN_NAME,
			c.COLUMN_ID as POSITION,
			c.DATA_TYPE,
			NVL(c.DATA_LENGTH, 0) as LENGTH,
			NVL(c.DATA_PRECISION, 0) as PRECISION,
			NVL(c.DATA_SCALE, 0) as SCALE,
			c.NULLABLE,
			NVL(c.DATA_DEFAULT, '') as DEFAULT_VALUE,
			NVL(cc.COMMENTS, '') as COLUMN_COMMENT,
			NVL(c.CHAR_COL_DECL_LENGTH, 0) as CHAR_LENGTH
		FROM ALL_TAB_COLUMNS c
		LEFT JOIN ALL_COL_COMMENTS cc 
			ON c.OWNER = cc.OWNER AND c.TABLE_NAME = cc.TABLE_NAME AND c.COLUMN_NAME = cc.COLUMN_NAME
		WHERE c.OWNER = :1 AND c.TABLE_NAME = :2
		ORDER BY c.COLUMN_ID
	`

	rows, err := e.db.QueryContext(ctx, query, owner, tableName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var columns []model.Column
	for rows.Next() {
		var col model.Column
		var nullable string
		var defaultVal, dataType sql.NullString

		err := rows.Scan(
			&col.Name, &col.Position, &dataType, &col.Length,
			&col.Precision, &col.Scale, &nullable, &defaultVal,
			&col.Comment, &col.Length,
		)
		if err != nil {
			return nil, err
		}

		if dataType.Valid {
			col.DataType = dataType.String
		}
		col.Nullable = (nullable == "Y")
		if defaultVal.Valid {
			col.DefaultValue = strings.TrimSpace(defaultVal.String)
		}

		columns = append(columns, col)
	}

	// Fetch constraint information (PK, FK, UK)
	if err := e.enrichColumnsWithConstraints(ctx, owner, tableName, columns); err != nil {
		return nil, err
	}

	return columns, rows.Err()
}

// enrichColumnsWithConstraints adds PK/FK/UK information
func (e *Extractor) enrichColumnsWithConstraints(ctx context.Context, owner, tableName string, columns []model.Column) error {
	query := `
		SELECT 
			cc.COLUMN_NAME,
			c.CONSTRAINT_TYPE,
			c.R_OWNER,
			rc.TABLE_NAME as R_TABLE_NAME,
			rcc.COLUMN_NAME as R_COLUMN_NAME
		FROM ALL_CONSTRAINTS c
		JOIN ALL_CONS_COLUMNS cc ON c.OWNER = cc.OWNER AND c.CONSTRAINT_NAME = cc.CONSTRAINT_NAME
		LEFT JOIN ALL_CONSTRAINTS rc ON c.R_OWNER = rc.OWNER AND c.R_CONSTRAINT_NAME = rc.CONSTRAINT_NAME
		LEFT JOIN ALL_CONS_COLUMNS rcc ON rc.OWNER = rcc.OWNER AND rc.CONSTRAINT_NAME = rcc.CONSTRAINT_NAME
		WHERE c.OWNER = :1 AND c.TABLE_NAME = :2
		AND c.CONSTRAINT_TYPE IN ('P', 'R', 'U')
	`

	rows, err := e.db.QueryContext(ctx, query, owner, tableName)
	if err != nil {
		return err
	}
	defer rows.Close()

	constraintMap := make(map[string]map[string]interface{})
	for rows.Next() {
		var colName, constraintType string
		var rOwner, rTable, rColumn sql.NullString

		err := rows.Scan(&colName, &constraintType, &rOwner, &rTable, &rColumn)
		if err != nil {
			return err
		}

		if constraintMap[colName] == nil {
			constraintMap[colName] = make(map[string]interface{})
		}

		switch constraintType {
		case "P":
			constraintMap[colName]["PK"] = true
		case "R":
			constraintMap[colName]["FK"] = true
			if rTable.Valid && rColumn.Valid {
				constraintMap[colName]["FK_TABLE"] = rTable.String
				constraintMap[colName]["FK_COLUMN"] = rColumn.String
			}
		case "U":
			constraintMap[colName]["UK"] = true
		}
	}

	// Apply constraints to columns
	for i := range columns {
		if constraints, ok := constraintMap[columns[i].Name]; ok {
			if _, isPK := constraints["PK"]; isPK {
				columns[i].IsPrimaryKey = true
			}
			if _, isFK := constraints["FK"]; isFK {
				columns[i].IsForeignKey = true
				if fkTable, ok := constraints["FK_TABLE"].(string); ok {
					columns[i].FKTargetTable = fkTable
				}
				if fkCol, ok := constraints["FK_COLUMN"].(string); ok {
					columns[i].FKTargetColumn = fkCol
				}
			}
			if _, isUK := constraints["UK"]; isUK {
				columns[i].IsUnique = true
			}
		}
	}

	return rows.Err()
}

// getIndexesForTable retrieves indexes with COMMENTS
func (e *Extractor) getIndexesForTable(ctx context.Context, owner, tableName string) ([]model.Index, error) {
	query := `
		SELECT DISTINCT
			i.INDEX_NAME,
			i.INDEX_TYPE,
			i.UNIQUENESS,
			NVL(ic.COMMENTS, '') as INDEX_COMMENT
		FROM ALL_INDEXES i
		LEFT JOIN ALL_IND_COMMENTS ic ON i.OWNER = ic.OWNER AND i.INDEX_NAME = ic.INDEX_NAME
		WHERE i.TABLE_OWNER = :1 AND i.TABLE_NAME = :2
		ORDER BY i.INDEX_NAME
	`

	rows, err := e.db.QueryContext(ctx, query, owner, tableName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var indexes []model.Index
	for rows.Next() {
		var idx model.Index
		var uniqueness string

		err := rows.Scan(&idx.Name, &idx.Type, &uniqueness, &idx.Comment)
		if err != nil {
			return nil, err
		}

		idx.TableName = tableName
		idx.Owner = owner
		idx.IsUnique = (uniqueness == "UNIQUE")
		idx.IsEnabled = true // Oracle doesn't have disabled indexes in same way

		// Fetch columns for this index
		idx.Columns, err = e.getIndexColumns(ctx, owner, idx.Name)
		if err != nil {
			return nil, err
		}

		indexes = append(indexes, idx)
	}

	return indexes, rows.Err()
}

// getIndexColumns retrieves columns for an index
func (e *Extractor) getIndexColumns(ctx context.Context, owner, indexName string) ([]string, error) {
	query := `
		SELECT COLUMN_NAME
		FROM ALL_IND_COLUMNS
		WHERE INDEX_OWNER = :1 AND INDEX_NAME = :2
		ORDER BY COLUMN_POSITION
	`

	rows, err := e.db.QueryContext(ctx, query, owner, indexName)
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

// GetViews extracts all view metadata with COMMENTS (NO SQL definition - security!)
func (e *Extractor) GetViews(ctx context.Context) ([]model.View, error) {
	query := `
		SELECT 
			v.OWNER,
			v.VIEW_NAME,
			'VIEW' as VIEW_TYPE,
			NVL(vc.COMMENTS, '') as VIEW_COMMENT,
			CASE WHEN v.READ_ONLY = 'Y' THEN 'N' ELSE 'Y' END as UPDATABLE
		FROM ALL_VIEWS v
		LEFT JOIN ALL_TAB_COMMENTS vc 
			ON v.OWNER = vc.OWNER AND v.VIEW_NAME = vc.TABLE_NAME
		WHERE 1=1
	`

	if len(e.schemaFilter) > 0 {
		placeholders := make([]string, len(e.schemaFilter))
		for i := range e.schemaFilter {
			placeholders[i] = fmt.Sprintf(":%d", i+1)
		}
		query += fmt.Sprintf(" AND v.OWNER IN (%s)", strings.Join(placeholders, ","))
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

		err := rows.Scan(&v.Owner, &v.Name, &v.Type, &v.Comment, &updatable)
		if err != nil {
			return nil, err
		}

		v.IsUpdatable = (updatable == "Y")

		// Fetch columns (NO TEXT definition - security!)
		v.Columns, err = e.getColumnsForTable(ctx, v.Owner, v.Name)
		if err != nil {
			return nil, err
		}

		views = append(views, v)
	}

	return views, rows.Err()
}

// GetRoutines extracts procedures/functions with COMMENTS (NO source code - security!)
func (e *Extractor) GetRoutines(ctx context.Context) ([]model.Routine, error) {
	query := `
		SELECT 
			p.OWNER,
			p.OBJECT_NAME,
			p.PROCEDURE_NAME,
			p.OBJECT_TYPE,
			NVL(oc.COMMENTS, '') as ROUTINE_COMMENT
		FROM ALL_PROCEDURES p
		LEFT JOIN ALL_TAB_COMMENTS oc 
			ON p.OWNER = oc.OWNER AND p.OBJECT_NAME = oc.TABLE_NAME
		WHERE p.OBJECT_TYPE IN ('PROCEDURE', 'FUNCTION')
	`

	if len(e.schemaFilter) > 0 {
		placeholders := make([]string, len(e.schemaFilter))
		for i := range e.schemaFilter {
			placeholders[i] = fmt.Sprintf(":%d", i+1)
		}
		query += fmt.Sprintf(" AND p.OWNER IN (%s)", strings.Join(placeholders, ","))
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
		var procName sql.NullString

		err := rows.Scan(&r.Owner, &r.Name, &procName, &r.Type, &r.Comment)
		if err != nil {
			return nil, err
		}

		// Oracle stores package procedures separately
		if procName.Valid && procName.String != "" {
			r.Name = r.Name + "." + procName.String
		}

		r.Language = "PL/SQL"

		// Fetch arguments (NO body - security!)
		r.Arguments, err = e.getRoutineArguments(ctx, r.Owner, r.Name)
		if err != nil {
			return nil, err
		}

		// Build signature from arguments
		r.Signature = e.buildSignature(r.Name, r.Arguments, r.Type)

		routines = append(routines, r)
	}

	return routines, rows.Err()
}

// getRoutineArguments retrieves parameters for a routine
func (e *Extractor) getRoutineArguments(ctx context.Context, owner, objectName string) ([]model.RoutineArgument, error) {
	query := `
		SELECT 
			ARGUMENT_NAME,
			POSITION,
			IN_OUT,
			DATA_TYPE,
			DEFAULT_VALUE
		FROM ALL_ARGUMENTS
		WHERE OWNER = :1 AND OBJECT_NAME = :2
		AND ARGUMENT_NAME IS NOT NULL
		ORDER BY POSITION
	`

	rows, err := e.db.QueryContext(ctx, query, owner, objectName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var args []model.RoutineArgument
	for rows.Next() {
		var arg model.RoutineArgument
		var defaultVal sql.NullString

		err := rows.Scan(&arg.Name, &arg.Position, &arg.Mode, &arg.DataType, &defaultVal)
		if err != nil {
			return nil, err
		}

		if defaultVal.Valid {
			arg.DefaultValue = defaultVal.String
		}

		args = append(args, arg)
	}

	return args, rows.Err()
}

// buildSignature creates routine signature (NO body!)
func (e *Extractor) buildSignature(name string, args []model.RoutineArgument, routineType string) string {
	argStrs := make([]string, len(args))
	for i, arg := range args {
		argStrs[i] = fmt.Sprintf("%s %s %s", arg.Name, arg.Mode, arg.DataType)
	}

	if routineType == "FUNCTION" {
		return fmt.Sprintf("FUNCTION %s(%s) RETURN <type>", name, strings.Join(argStrs, ", "))
	}
	return fmt.Sprintf("PROCEDURE %s(%s)", name, strings.Join(argStrs, ", "))
}

// GetSequences extracts sequence metadata with COMMENTS
func (e *Extractor) GetSequences(ctx context.Context) ([]model.Sequence, error) {
	query := `
		SELECT 
			SEQUENCE_OWNER,
			SEQUENCE_NAME,
			MIN_VALUE,
			MAX_VALUE,
			INCREMENT_BY,
			LAST_NUMBER,
			CACHE_SIZE,
			CYCLE_FLAG,
			ORDER_FLAG
		FROM ALL_SEQUENCES
		WHERE 1=1
	`

	if len(e.schemaFilter) > 0 {
		placeholders := make([]string, len(e.schemaFilter))
		for i := range e.schemaFilter {
			placeholders[i] = fmt.Sprintf(":%d", i+1)
		}
		query += fmt.Sprintf(" AND SEQUENCE_OWNER IN (%s)", strings.Join(placeholders, ","))
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
		var cycleFlag, orderFlag string

		err := rows.Scan(
			&seq.Owner, &seq.Name, &seq.MinValue, &seq.MaxValue,
			&seq.Increment, &seq.LastNumber, &seq.CacheSize,
			&cycleFlag, &orderFlag,
		)
		if err != nil {
			return nil, err
		}

		seq.IsCyclic = (cycleFlag == "Y")
		seq.IsOrdered = (orderFlag == "Y")
		seq.Comment = "" // Oracle doesn't have sequence comments by default

		sequences = append(sequences, seq)
	}

	return sequences, rows.Err()
}

// GetTriggers extracts trigger metadata with COMMENTS (NO trigger body - security!)
func (e *Extractor) GetTriggers(ctx context.Context) ([]model.Trigger, error) {
	query := `
		SELECT 
			OWNER,
			TRIGGER_NAME,
			TABLE_OWNER,
			TABLE_NAME,
			TRIGGER_TYPE,
			TRIGGERING_EVENT,
			STATUS
		FROM ALL_TRIGGERS
		WHERE 1=1
	`

	if len(e.schemaFilter) > 0 {
		placeholders := make([]string, len(e.schemaFilter))
		for i := range e.schemaFilter {
			placeholders[i] = fmt.Sprintf(":%d", i+1)
		}
		query += fmt.Sprintf(" AND OWNER IN (%s)", strings.Join(placeholders, ","))
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
		var tableOwner, triggerType string

		err := rows.Scan(
			&trg.Owner, &trg.Name, &tableOwner, &trg.TargetTable,
			&triggerType, &trg.Event, &trg.Status,
		)
		if err != nil {
			return nil, err
		}

		// Parse trigger type (e.g., "BEFORE EACH ROW")
		parts := strings.Fields(triggerType)
		if len(parts) >= 1 {
			trg.Timing = parts[0] // BEFORE, AFTER, INSTEAD OF
		}
		if strings.Contains(triggerType, "EACH ROW") {
			trg.Level = "ROW"
		} else {
			trg.Level = "STATEMENT"
		}

		trg.TargetType = "TABLE"
		trg.Comment = "" // Oracle doesn't have trigger comments by default

		triggers = append(triggers, trg)
	}

	return triggers, rows.Err()
}

// GetSynonyms extracts synonym metadata with COMMENTS
func (e *Extractor) GetSynonyms(ctx context.Context) ([]model.Synonym, error) {
	query := `
		SELECT 
			OWNER,
			SYNONYM_NAME,
			TABLE_OWNER,
			TABLE_NAME,
			DB_LINK
		FROM ALL_SYNONYMS
		WHERE 1=1
	`

	if len(e.schemaFilter) > 0 {
		placeholders := make([]string, len(e.schemaFilter))
		for i := range e.schemaFilter {
			placeholders[i] = fmt.Sprintf(":%d", i+1)
		}
		query += fmt.Sprintf(" AND OWNER IN (%s)", strings.Join(placeholders, ","))
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
		var dbLink sql.NullString

		err := rows.Scan(&syn.Owner, &syn.Name, &syn.TargetOwner, &syn.TargetObject, &dbLink)
		if err != nil {
			return nil, err
		}

		syn.IsPublic = (syn.Owner == "PUBLIC")
		syn.TargetType = "TABLE" // Simplified - could query actual type
		syn.Comment = ""         // Oracle doesn't have synonym comments

		synonyms = append(synonyms, syn)
	}

	return synonyms, rows.Err()
}

// ExtractSchema performs complete extraction
func (e *Extractor) ExtractSchema(ctx context.Context) (*model.Schema, error) {
	schema := &model.Schema{
		ExtractedAt: time.Now(),
	}

	// Get database info
	var err error
	schema.DatabaseName, schema.Version, err = e.GetDatabaseInfo(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get database info: %w", err)
	}
	schema.DatabaseType = "Oracle"

	// Extract all object types
	schema.Tables, err = e.GetTables(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get tables: %w", err)
	}

	schema.Views, err = e.GetViews(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get views: %w", err)
	}

	schema.Routines, err = e.GetRoutines(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get routines: %w", err)
	}

	schema.Sequences, err = e.GetSequences(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get sequences: %w", err)
	}

	schema.Triggers, err = e.GetTriggers(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get triggers: %w", err)
	}

	schema.Synonyms, err = e.GetSynonyms(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get synonyms: %w", err)
	}

	// Collect all indexes from tables
	for _, table := range schema.Tables {
		schema.Indexes = append(schema.Indexes, table.Indexes...)
	}

	return schema, nil
}
