package model

import "time"

// Schema represents the complete database schema metadata
// It serves as the root container for all database objects
type Schema struct {
	DatabaseName string     `json:"databaseName"`
	DatabaseType string     `json:"databaseType"` // e.g., "oracle", "postgresql", "mysql"
	Version      string     `json:"version"`
	ExtractedAt  time.Time  `json:"extractedAt"`
	Comment      string     `json:"comment,omitempty"`
	Tables       []Table    `json:"tables,omitempty"`
	Views        []View     `json:"views,omitempty"`
	Routines     []Routine  `json:"routines,omitempty"`
	Sequences    []Sequence `json:"sequences,omitempty"`
	Triggers     []Trigger  `json:"triggers,omitempty"`
	Synonyms     []Synonym  `json:"synonyms,omitempty"`
	Indexes      []Index    `json:"indexes,omitempty"`
}

// Table represents a database table with its metadata
type Table struct {
	Name       string   `json:"name"`
	Owner      string   `json:"owner,omitempty"`
	Type       string   `json:"type"` // e.g., "TABLE", "PARTITIONED"
	Comment    string   `json:"comment,omitempty"`
	Columns    []Column `json:"columns"`
	Indexes    []Index  `json:"indexes,omitempty"`
	RowCount   int64    `json:"rowCount,omitempty"`
	CreatedAt  string   `json:"createdAt,omitempty"`
	ModifiedAt string   `json:"modifiedAt,omitempty"`
}

// View represents a database view with its metadata
type View struct {
	Name       string   `json:"name"`
	Owner      string   `json:"owner,omitempty"`
	Type       string   `json:"type"` // e.g., "VIEW", "MATERIALIZED VIEW"
	Comment    string   `json:"comment,omitempty"`
	Columns    []Column `json:"columns"`
	IsUpdatable bool    `json:"isUpdatable"`
	CreatedAt  string   `json:"createdAt,omitempty"`
	ModifiedAt string   `json:"modifiedAt,omitempty"`
}

// Column represents a table or view column with comprehensive metadata
type Column struct {
	Name         string `json:"name"`
	Position     int    `json:"position"`
	DataType     string `json:"dataType"`
	Length       int    `json:"length,omitempty"`
	Precision    int    `json:"precision,omitempty"`
	Scale        int    `json:"scale,omitempty"`
	Nullable     bool   `json:"nullable"`
	DefaultValue string `json:"defaultValue,omitempty"`
	Comment      string `json:"comment,omitempty"`
	
	// Constraints
	IsPrimaryKey   bool   `json:"isPrimaryKey"`
	IsForeignKey   bool   `json:"isForeignKey"`
	IsUnique       bool   `json:"isUnique"`
	FKTargetTable  string `json:"fkTargetTable,omitempty"`
	FKTargetColumn string `json:"fkTargetColumn,omitempty"`
	
	// Additional metadata
	IsAutoIncrement bool   `json:"isAutoIncrement"`
	CharacterSet    string `json:"characterSet,omitempty"`
	Collation       string `json:"collation,omitempty"`
}

// Routine represents a stored procedure or function
// CRITICAL: NO source code/definition field - metadata only
type Routine struct {
	Name       string           `json:"name"`
	Owner      string           `json:"owner,omitempty"`
	Type       string           `json:"type"` // "PROCEDURE" or "FUNCTION"
	Comment    string           `json:"comment,omitempty"`
	Signature  string           `json:"signature"` // Full signature without body
	Arguments  []RoutineArgument `json:"arguments,omitempty"`
	ReturnType string           `json:"returnType,omitempty"` // For functions
	Language   string           `json:"language,omitempty"`  // e.g., "SQL", "PLSQL"
	IsDeterministic bool        `json:"isDeterministic"`
	SecurityType    string      `json:"securityType,omitempty"` // DEFINER/INVOKER
	CreatedAt  string           `json:"createdAt,omitempty"`
	ModifiedAt string           `json:"modifiedAt,omitempty"`
}

// RoutineArgument represents a parameter of a stored procedure or function
type RoutineArgument struct {
	Name         string `json:"name"`
	Position     int    `json:"position"`
	Mode         string `json:"mode"` // IN, OUT, INOUT
	DataType     string `json:"dataType"`
	DefaultValue string `json:"defaultValue,omitempty"`
	Comment      string `json:"comment,omitempty"`
}

// Index represents a database index
type Index struct {
	Name       string   `json:"name"`
	TableName  string   `json:"tableName"`
	Owner      string   `json:"owner,omitempty"`
	Type       string   `json:"type"` // e.g., "BTREE", "HASH", "BITMAP"
	Columns    []string `json:"columns"`
	IsUnique   bool     `json:"isUnique"`
	IsPrimary  bool     `json:"isPrimary"`
	IsEnabled  bool     `json:"isEnabled"`
	Comment    string   `json:"comment,omitempty"`
	CreatedAt  string   `json:"createdAt,omitempty"`
}

// Sequence represents a database sequence
type Sequence struct {
	Name        string `json:"name"`
	Owner       string `json:"owner,omitempty"`
	MinValue    int64  `json:"minValue"`
	MaxValue    int64  `json:"maxValue"`
	Increment   int64  `json:"increment"`
	LastNumber  int64  `json:"lastNumber"`
	CacheSize   int    `json:"cacheSize,omitempty"`
	IsCyclic    bool   `json:"isCyclic"`
	IsOrdered   bool   `json:"isOrdered"`
	Comment     string `json:"comment,omitempty"`
	CreatedAt   string `json:"createdAt,omitempty"`
}

// Trigger represents a database trigger
// CRITICAL: NO trigger body/source code - metadata only
type Trigger struct {
	Name        string `json:"name"`
	Owner       string `json:"owner,omitempty"`
	TargetTable string `json:"targetTable"`
	TargetType  string `json:"targetType"` // TABLE, VIEW
	Timing      string `json:"timing"`     // BEFORE, AFTER, INSTEAD OF
	Event       string `json:"event"`      // INSERT, UPDATE, DELETE
	Level       string `json:"level"`      // ROW, STATEMENT
	Status      string `json:"status"`     // ENABLED, DISABLED
	Comment     string `json:"comment,omitempty"`
	CreatedAt   string `json:"createdAt,omitempty"`
	ModifiedAt  string `json:"modifiedAt,omitempty"`
}

// Synonym represents a database synonym (alias)
type Synonym struct {
	Name         string `json:"name"`
	Owner        string `json:"owner,omitempty"`
	TargetObject string `json:"targetObject"`
	TargetOwner  string `json:"targetOwner,omitempty"`
	TargetType   string `json:"targetType,omitempty"` // TABLE, VIEW, PROCEDURE, etc.
	IsPublic     bool   `json:"isPublic"`
	Comment      string `json:"comment,omitempty"`
	CreatedAt    string `json:"createdAt,omitempty"`
}
