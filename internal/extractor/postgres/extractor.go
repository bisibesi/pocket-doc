package postgres

import (
	"context"
	"database/sql"
	"pocket-doc/internal/model"
	"fmt"
	"strings"
	"time"

	_ "github.com/lib/pq"
)

// Extractor implements PostgreSQL database metadata extraction
type Extractor struct {
	db           *sql.DB
	config       Config
	schemaFilter []string
}

// Config holds PostgreSQL-specific configuration
type Config struct {
	Host         string
	Port         int
	Database     string
	Username     string
	Password     string
	SSLMode      string   // disable, require, verify-ca, verify-full
	SchemaFilter []string // Filter by schema/namespace
}

// NewExtractor creates a new PostgreSQL extractor
func NewExtractor(cfg Config) (*Extractor, error) {
	// Build PostgreSQL connection string
	connStr := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.Host, cfg.Port, cfg.Username, cfg.Password, cfg.Database, cfg.SSLMode)

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open postgres connection: %w", err)
	}

	schemas := cfg.SchemaFilter
	if len(schemas) == 0 {
		schemas = []string{"public"} // Default schema
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
	err = e.db.QueryRowContext(ctx, "SELECT current_database(), version()").Scan(&name, &version)
	return
}

// GetTables extracts tables with COMMENTS using obj_description (CRITICAL RULE #1)
func (e *Extractor) GetTables(ctx context.Context) ([]model.Table, error) {
	query := `
		SELECT 
			n.nspname as schema_name,
			c.relname as table_name,
			COALESCE(obj_description(c.oid, 'pg_class'), '') as table_comment,
			COALESCE(pg_stat_get_live_tuples(c.oid), 0) as row_count,
			c.relkind as kind
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE c.relkind = 'r' -- regular tables only
	`

	// CRITICAL RULE #2: Schema filtering
	if len(e.schemaFilter) > 0 {
		placeholders := make([]string, len(e.schemaFilter))
		for i := range e.schemaFilter {
			placeholders[i] = fmt.Sprintf("$%d", i+1)
		}
		query += fmt.Sprintf(" AND n.nspname IN (%s)", strings.Join(placeholders, ","))
	}

	query += " ORDER BY n.nspname, c.relname"

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
		var kind string

		err := rows.Scan(&t.Owner, &t.Name, &t.Comment, &t.RowCount, &kind)
		if err != nil {
			return nil, err
		}

		t.Type = "TABLE"

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

// getColumnsForTable retrieves columns with pg_description comments (CRITICAL RULE #1)
func (e *Extractor) getColumnsForTable(ctx context.Context, schema, tableName string) ([]model.Column, error) {
	query := `
		SELECT 
			a.attnum as position,
			a.attname as column_name,
			format_type(a.atttypid, a.atttypmod) as data_type,
			NOT a.attnotnull as nullable,
			COALESCE(pg_get_expr(d.adbin, d.adrelid), '') as default_value,
			COALESCE(col_description(a.attrelid, a.attnum), '') as column_comment,
			EXISTS(
				SELECT 1 FROM pg_index i 
				WHERE i.indrelid = a.attrelid 
				AND a.attnum = ANY(i.indkey) 
				AND i.indisprimary
			) as is_primary,
			EXISTS(
				SELECT 1 FROM pg_constraint con
				WHERE con.conrelid = a.attrelid
				AND a.attnum = ANY(con.conkey)
				AND con.contype = 'f'
			) as is_foreign,
			EXISTS(
				SELECT 1 FROM pg_index i 
				WHERE i.indrelid = a.attrelid 
				AND a.attnum = ANY(i.indkey) 
				AND i.indisunique
				AND NOT i.indisprimary
			) as is_unique
		FROM pg_attribute a
		JOIN pg_class c ON c.oid = a.attrelid
		JOIN pg_namespace n ON n.oid = c.relnamespace
		LEFT JOIN pg_attrdef d ON d.adrelid = a.attrelid AND d.adnum = a.attnum
		WHERE n.nspname = $1 
		AND c.relname = $2
		AND a.attnum > 0
		AND NOT a.attisdropped
		ORDER BY a.attnum
	`

	rows, err := e.db.QueryContext(ctx, query, schema, tableName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var columns []model.Column
	for rows.Next() {
		var col model.Column

		err := rows.Scan(
			&col.Position, &col.Name, &col.DataType, &col.Nullable,
			&col.DefaultValue, &col.Comment,
			&col.IsPrimaryKey, &col.IsForeignKey, &col.IsUnique,
		)
		if err != nil {
			return nil, err
		}

		// Check for serial/identity (auto-increment)
		col.IsAutoIncrement = strings.Contains(col.DefaultValue, "nextval")

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
			pn.nspname || '.' || pc.relname as ref_table,
			pa.attname as ref_column
		FROM pg_constraint con
		JOIN pg_class c ON con.conrelid = c.oid
		JOIN pg_namespace n ON n.oid = c.relnamespace
		JOIN pg_attribute a ON a.attrelid = c.oid AND a.attnum = ANY(con.conkey)
		JOIN pg_class pc ON con.confrelid = pc.oid
		JOIN pg_namespace pn ON pn.oid = pc.relnamespace
		JOIN pg_attribute pa ON pa.attrelid = pc.oid AND pa.attnum = ANY(con.confkey)
		WHERE n.nspname = $1 
		AND c.relname = $2
		AND a.attname = $3
		AND con.contype = 'f'
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
		SELECT 
			i.indexname as index_name,
			am.amname as index_type,
			ix.indisunique as is_unique,
			ix.indisprimary as is_primary,
			COALESCE(obj_description(ix.indexrelid, 'pg_class'), '') as index_comment
		FROM pg_indexes i
		JOIN pg_class c ON c.relname = i.tablename
		JOIN pg_namespace n ON n.nspname = i.schemaname
		JOIN pg_index ix ON ix.indexrelid = (i.schemaname || '.' || i.indexname)::regclass
		JOIN pg_class ic ON ic.oid = ix.indexrelid
		JOIN pg_am am ON am.oid = ic.relam
		WHERE i.schemaname = $1 AND i.tablename = $2
		ORDER BY i.indexname
	`

	rows, err := e.db.QueryContext(ctx, query, schema, tableName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var indexes []model.Index
	for rows.Next() {
		var idx model.Index
		var isPrimary bool

		err := rows.Scan(&idx.Name, &idx.Type, &idx.IsUnique, &isPrimary, &idx.Comment)
		if err != nil {
			return nil, err
		}

		idx.TableName = tableName
		idx.Owner = schema
		idx.IsPrimary = isPrimary
		idx.IsEnabled = true

		// Fetch columns
		idx.Columns, err = e.getIndexColumns(ctx, schema, idx.Name)
		if err != nil {
			return nil, err
		}

		indexes = append(indexes, idx)
	}

	return indexes, rows.Err()
}

// getIndexColumns retrieves columns for an index
func (e *Extractor) getIndexColumns(ctx context.Context, schema, indexName string) ([]string, error) {
	query := `
		SELECT a.attname
		FROM pg_index ix
		JOIN pg_class c ON c.oid = ix.indexrelid
		JOIN pg_namespace n ON n.oid = c.relnamespace
		JOIN pg_attribute a ON a.attrelid = ix.indrelid AND a.attnum = ANY(ix.indkey)
		WHERE n.nspname = $1 AND c.relname = $2
		ORDER BY array_position(ix.indkey, a.attnum)
	`

	rows, err := e.db.QueryContext(ctx, query, schema, indexName)
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

// GetViews extracts views with obj_description (NO definition - security!)
func (e *Extractor) GetViews(ctx context.Context) ([]model.View, error) {
	query := `
		SELECT 
			n.nspname as schema_name,
			c.relname as view_name,
			COALESCE(obj_description(c.oid, 'pg_class'), '') as view_comment,
			CASE WHEN v.is_updatable = 'YES' THEN true ELSE false END as is_updatable
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		LEFT JOIN information_schema.views v ON v.table_schema = n.nspname AND v.table_name = c.relname
		WHERE c.relkind = 'v'
	`

	if len(e.schemaFilter) > 0 {
		placeholders := make([]string, len(e.schemaFilter))
		for i := range e.schemaFilter {
			placeholders[i] = fmt.Sprintf("$%d", i+1)
		}
		query += fmt.Sprintf(" AND n.nspname IN (%s)", strings.Join(placeholders, ","))
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

		err := rows.Scan(&v.Owner, &v.Name, &v.Comment, &v.IsUpdatable)
		if err != nil {
			return nil, err
		}

		v.Type = "VIEW"

		// Fetch columns
		v.Columns, err = e.getColumnsForTable(ctx, v.Owner, v.Name)
		if err != nil {
			return nil, err
		}

		views = append(views, v)
	}

	return views, rows.Err()
}

// GetRoutines extracts functions with obj_description (NO source - security!)
func (e *Extractor) GetRoutines(ctx context.Context) ([]model.Routine, error) {
	query := `
		SELECT 
			n.nspname as schema_name,
			p.proname as routine_name,
			CASE WHEN p.prokind = 'f' THEN 'FUNCTION' 
			     WHEN p.prokind = 'p' THEN 'PROCEDURE'
			     ELSE 'FUNCTION' END as routine_type,
			COALESCE(obj_description(p.oid, 'pg_proc'), '') as routine_comment,
			pg_get_function_identity_arguments(p.oid) as arguments,
			format_type(p.prorettype, NULL) as return_type,
			l.lanname as language
		FROM pg_proc p
		JOIN pg_namespace n ON n.oid = p.pronamespace
		JOIN pg_language l ON l.oid = p.prolang
		WHERE p.prokind IN ('f', 'p')
	`

	if len(e.schemaFilter) > 0 {
		placeholders := make([]string, len(e.schemaFilter))
		for i := range e.schemaFilter {
			placeholders[i] = fmt.Sprintf("$%d", i+1)
		}
		query += fmt.Sprintf(" AND n.nspname IN (%s)", strings.Join(placeholders, ","))
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
		var argStr, returnType string

		err := rows.Scan(&r.Owner, &r.Name, &r.Type, &r.Comment, &argStr, &returnType, &r.Language)
		if err != nil {
			return nil, err
		}

		if r.Type == "FUNCTION" {
			r.ReturnType = returnType
		}

		// Build signature (PostgreSQL provides formatted arguments)
		r.Signature = fmt.Sprintf("%s %s(%s)", r.Type, r.Name, argStr)
		if r.Type == "FUNCTION" {
			r.Signature += " RETURNS " + returnType
		}

		routines = append(routines, r)
	}

	return routines, rows.Err()
}

// GetSequences extracts sequences with obj_description
func (e *Extractor) GetSequences(ctx context.Context) ([]model.Sequence, error) {
	query := `
		SELECT 
			n.nspname as schema_name,
			c.relname as sequence_name,
			s.seqmin as min_value,
			s.seqmax as max_value,
			s.seqincrement as increment,
			s.last_value as last_number,
			s.seqcycle as is_cyclic,
			COALESCE(obj_description(c.oid, 'pg_class'), '') as seq_comment
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		JOIN pg_sequence s ON s.seqrelid = c.oid
		WHERE c.relkind = 'S'
	`

	if len(e.schemaFilter) > 0 {
		placeholders := make([]string, len(e.schemaFilter))
		for i := range e.schemaFilter {
			placeholders[i] = fmt.Sprintf("$%d", i+1)
		}
		query += fmt.Sprintf(" AND n.nspname IN (%s)", strings.Join(placeholders, ","))
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

		err := rows.Scan(
			&seq.Owner, &seq.Name, &seq.MinValue, &seq.MaxValue,
			&seq.Increment, &seq.LastNumber, &seq.IsCyclic, &seq.Comment,
		)
		if err != nil {
			return nil, err
		}

		sequences = append(sequences, seq)
	}

	return sequences, rows.Err()
}

// GetTriggers extracts triggers with obj_description (NO body - security!)
func (e *Extractor) GetTriggers(ctx context.Context) ([]model.Trigger, error) {
	query := `
		SELECT 
			n.nspname as schema_name,
			t.tgname as trigger_name,
			c.relname as table_name,
			CASE t.tgtype & 1 WHEN 1 THEN 'ROW' ELSE 'STATEMENT' END as level,
			CASE 
				WHEN t.tgtype & 2 = 2 THEN 'BEFORE'
				WHEN t.tgtype & 64 = 64 THEN 'INSTEAD OF'
				ELSE 'AFTER'
			END as timing,
			CASE 
				WHEN t.tgtype & 4 = 4 THEN 'INSERT'
				WHEN t.tgtype & 8 = 8 THEN 'DELETE'
				WHEN t.tgtype & 16 = 16 THEN 'UPDATE'
				ELSE 'TRUNCATE'
			END as event,
			CASE WHEN t.tgenabled = 'O' THEN 'ENABLED' ELSE 'DISABLED' END as status,
			COALESCE(obj_description(t.oid, 'pg_trigger'), '') as trigger_comment
		FROM pg_trigger t
		JOIN pg_class c ON c.oid = t.tgrelid
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE NOT t.tgisinternal
	`

	if len(e.schemaFilter) > 0 {
		placeholders := make([]string, len(e.schemaFilter))
		for i := range e.schemaFilter {
			placeholders[i] = fmt.Sprintf("$%d", i+1)
		}
		query += fmt.Sprintf(" AND n.nspname IN (%s)", strings.Join(placeholders, ","))
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
			&trg.Owner, &trg.Name, &trg.TargetTable, &trg.Level,
			&trg.Timing, &trg.Event, &trg.Status, &trg.Comment,
		)
		if err != nil {
			return nil, err
		}

		trg.TargetType = "TABLE"

		triggers = append(triggers, trg)
	}

	return triggers, rows.Err()
}

// GetSynonyms - PostgreSQL doesn't have synonyms (but has schemas/search_path)
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
	schema.DatabaseType = "PostgreSQL"

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
