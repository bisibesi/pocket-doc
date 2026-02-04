package xlsx

import (
	"dbms-to-document/internal/model"
	"fmt"
	"io"
	"time"

	"github.com/xuri/excelize/v2"
)

// Config holds configuration for Excel export
type Config struct {
	Language     string
	ExcludeTypes []string
	ColorScheme  string
}

// Exporter implements Excel (.xlsx) export functionality
type Exporter struct {
	config Config
}

// NewExporter creates a new Excel exporter
func NewExporter(cfg Config) *Exporter {
	return &Exporter{config: cfg}
}

// Format returns the format name
func (e *Exporter) Format() string {
	return "xlsx"
}

// MimeType returns the MIME type
func (e *Exporter) MimeType() string {
	return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
}

// FileExtension returns the file extension
func (e *Exporter) FileExtension() string {
	return ".xlsx"
}

// Export generates an Excel file with 4 sheets (CRITICAL RULE #2)
func (e *Exporter) Export(schema *model.Schema, w io.Writer) error {
	f := excelize.NewFile()
	defer func() {
		if err := f.Close(); err != nil {
			fmt.Printf("Error closing Excel file: %v\n", err)
		}
	}()

	// CRITICAL RULE #2: 4 Sheets - Overview, Tables, Columns, Objects
	sheets := []string{"Overview", "Tables", "Columns", "Objects"}

	// Delete default Sheet1 and create our sheets
	f.DeleteSheet("Sheet1")
	for _, sheetName := range sheets {
		_, err := f.NewSheet(sheetName)
		if err != nil {
			return fmt.Errorf("failed to create sheet %s: %w", sheetName, err)
		}
	}

	// Set Overview as active sheet
	f.SetActiveSheet(0)

	// Generate content for each sheet
	if err := e.writeOverview(f, schema); err != nil {
		return fmt.Errorf("failed to write overview: %w", err)
	}

	if err := e.writeTables(f, schema); err != nil {
		return fmt.Errorf("failed to write tables: %w", err)
	}

	if err := e.writeColumns(f, schema); err != nil {
		return fmt.Errorf("failed to write columns: %w", err)
	}

	if err := e.writeObjects(f, schema); err != nil {
		return fmt.Errorf("failed to write objects: %w", err)
	}

	// Write to output
	return f.Write(w)
}

// writeOverview creates the database summary sheet
func (e *Exporter) writeOverview(f *excelize.File, schema *model.Schema) error {
	sheet := "Overview"

	// Headers
	headers := []string{"항목", "값"}
	if e.config.Language == "en" {
		headers = []string{"Item", "Value"}
	}

	// Write headers
	for i, header := range headers {
		cell := fmt.Sprintf("%c1", 'A'+i)
		f.SetCellValue(sheet, cell, header)
	}

	// Apply header style (CRITICAL RULE #2: Gray Header)
	headerStyle := e.getHeaderStyle(f)
	f.SetCellStyle(sheet, "A1", fmt.Sprintf("%c1", 'A'+len(headers)-1), headerStyle)

	// Write data
	row := 2
	data := [][]interface{}{
		{"데이터베이스 이름", schema.DatabaseName},
		{"데이터베이스 유형", schema.DatabaseType},
		{"버전", schema.Version},
		{"추출 시간", schema.ExtractedAt.Format(time.RFC3339)},
		{"총 테이블 수", len(schema.Tables)},
		{"총 뷰 수", len(schema.Views)},
		{"총 프로시저/함수 수", len(schema.Routines)},
		{"총 시퀀스 수", len(schema.Sequences)},
		{"총 트리거 수", len(schema.Triggers)},
		{"총 동의어 수", len(schema.Synonyms)},
		{"총 인덱스 수", len(schema.Indexes)},
	}

	if e.config.Language == "en" {
		data = [][]interface{}{
			{"Database Name", schema.DatabaseName},
			{"Database Type", schema.DatabaseType},
			{"Version", schema.Version},
			{"Extracted At", schema.ExtractedAt.Format(time.RFC3339)},
			{"Total Tables", len(schema.Tables)},
			{"Total Views", len(schema.Views)},
			{"Total Routines", len(schema.Routines)},
			{"Total Sequences", len(schema.Sequences)},
			{"Total Triggers", len(schema.Triggers)},
			{"Total Synonyms", len(schema.Synonyms)},
			{"Total Indexes", len(schema.Indexes)},
		}
	}

	for _, rowData := range data {
		f.SetCellValue(sheet, fmt.Sprintf("A%d", row), rowData[0])
		f.SetCellValue(sheet, fmt.Sprintf("B%d", row), rowData[1])
		row++
	}

	// Auto-fit columns
	f.SetColWidth(sheet, "A", "A", 25)
	f.SetColWidth(sheet, "B", "B", 30)

	return nil
}

// writeTables creates the tables sheet
func (e *Exporter) writeTables(f *excelize.File, schema *model.Schema) error {
	sheet := "Tables"

	// Headers
	headers := []string{"이름", "소유자", "유형", "컬럼 수", "인덱스 수", "행 수", "설명"}
	if e.config.Language == "en" {
		headers = []string{"Name", "Owner", "Type", "Column Count", "Index Count", "Row Count", "Comment"}
	}

	for i, header := range headers {
		cell := fmt.Sprintf("%c1", 'A'+i)
		f.SetCellValue(sheet, cell, header)
	}

	headerStyle := e.getHeaderStyle(f)
	f.SetCellStyle(sheet, "A1", fmt.Sprintf("%c1", 'A'+len(headers)-1), headerStyle)

	// Data
	row := 2
	for _, table := range schema.Tables {
		f.SetCellValue(sheet, fmt.Sprintf("A%d", row), table.Name)
		f.SetCellValue(sheet, fmt.Sprintf("B%d", row), table.Owner)
		f.SetCellValue(sheet, fmt.Sprintf("C%d", row), table.Type)
		f.SetCellValue(sheet, fmt.Sprintf("D%d", row), len(table.Columns))
		f.SetCellValue(sheet, fmt.Sprintf("E%d", row), len(table.Indexes))
		f.SetCellValue(sheet, fmt.Sprintf("F%d", row), table.RowCount)
		f.SetCellValue(sheet, fmt.Sprintf("G%d", row), table.Comment)
		row++
	}

	// Auto-fit
	f.SetColWidth(sheet, "A", "A", 25)
	f.SetColWidth(sheet, "B", "B", 15)
	f.SetColWidth(sheet, "C", "C", 15)
	f.SetColWidth(sheet, "D", "D", 12)
	f.SetColWidth(sheet, "E", "E", 12)
	f.SetColWidth(sheet, "F", "F", 12)
	f.SetColWidth(sheet, "G", "G", 40)

	return nil
}

// writeColumns creates the columns detail sheet
func (e *Exporter) writeColumns(f *excelize.File, schema *model.Schema) error {
	sheet := "Columns"

	headers := []string{"테이블", "컬럼명", "순서", "데이터타입", "NULL허용", "PK", "FK", "UK", "기본값", "설명"}
	if e.config.Language == "en" {
		headers = []string{"Table", "Column Name", "Position", "Data Type", "Nullable", "PK", "FK", "UK", "Default", "Comment"}
	}

	for i, header := range headers {
		cell := fmt.Sprintf("%c1", 'A'+i)
		f.SetCellValue(sheet, cell, header)
	}

	headerStyle := e.getHeaderStyle(f)
	f.SetCellStyle(sheet, "A1", "J1", headerStyle)

	row := 2
	for _, table := range schema.Tables {
		for _, col := range table.Columns {
			f.SetCellValue(sheet, fmt.Sprintf("A%d", row), table.Name)
			f.SetCellValue(sheet, fmt.Sprintf("B%d", row), col.Name)
			f.SetCellValue(sheet, fmt.Sprintf("C%d", row), col.Position)
			f.SetCellValue(sheet, fmt.Sprintf("D%d", row), col.DataType)
			f.SetCellValue(sheet, fmt.Sprintf("E%d", row), boolToYN(col.Nullable))
			f.SetCellValue(sheet, fmt.Sprintf("F%d", row), boolToYN(col.IsPrimaryKey))
			f.SetCellValue(sheet, fmt.Sprintf("G%d", row), boolToYN(col.IsForeignKey))
			f.SetCellValue(sheet, fmt.Sprintf("H%d", row), boolToYN(col.IsUnique))
			f.SetCellValue(sheet, fmt.Sprintf("I%d", row), col.DefaultValue)
			f.SetCellValue(sheet, fmt.Sprintf("J%d", row), col.Comment)
			row++
		}
	}

	// Auto-fit
	f.SetColWidth(sheet, "A", "A", 20)
	f.SetColWidth(sheet, "B", "B", 20)
	f.SetColWidth(sheet, "C", "C", 8)
	f.SetColWidth(sheet, "D", "D", 15)
	f.SetColWidth(sheet, "E", "E", 8)
	f.SetColWidth(sheet, "F", "F", 6)
	f.SetColWidth(sheet, "G", "G", 6)
	f.SetColWidth(sheet, "H", "H", 6)
	f.SetColWidth(sheet, "I", "I", 15)
	f.SetColWidth(sheet, "J", "J", 40)

	return nil
}

// writeObjects creates the combined objects sheet (Routines, Sequences, Triggers, Synonyms)
func (e *Exporter) writeObjects(f *excelize.File, schema *model.Schema) error {
	sheet := "Objects"
	row := 1

	// Routines section (NO source code - SECURITY)
	if len(schema.Routines) > 0 {
		// Section header
		f.SetCellValue(sheet, fmt.Sprintf("A%d", row), "프로시저/함수")
		if e.config.Language == "en" {
			f.SetCellValue(sheet, fmt.Sprintf("A%d", row), "ROUTINES")
		}
		f.MergeCell(sheet, fmt.Sprintf("A%d", row), fmt.Sprintf("G%d", row))
		row++

		headers := []string{"이름", "소유자", "유형", "서명", "반환타입", "언어", "설명"}
		if e.config.Language == "en" {
			headers = []string{"Name", "Owner", "Type", "Signature", "Return Type", "Language", "Comment"}
		}
		for i, h := range headers {
			f.SetCellValue(sheet, fmt.Sprintf("%c%d", 'A'+i, row), h)
		}
		headerStyle := e.getHeaderStyle(f)
		f.SetCellStyle(sheet, fmt.Sprintf("A%d", row), fmt.Sprintf("G%d", row), headerStyle)
		row++

		for _, routine := range schema.Routines {
			f.SetCellValue(sheet, fmt.Sprintf("A%d", row), routine.Name)
			f.SetCellValue(sheet, fmt.Sprintf("B%d", row), routine.Owner)
			f.SetCellValue(sheet, fmt.Sprintf("C%d", row), routine.Type)
			f.SetCellValue(sheet, fmt.Sprintf("D%d", row), routine.Signature)
			f.SetCellValue(sheet, fmt.Sprintf("E%d", row), routine.ReturnType)
			f.SetCellValue(sheet, fmt.Sprintf("F%d", row), routine.Language)
			f.SetCellValue(sheet, fmt.Sprintf("G%d", row), routine.Comment)
			row++
		}
		row++ // Blank row
	}

	// Sequences section
	if len(schema.Sequences) > 0 {
		f.SetCellValue(sheet, fmt.Sprintf("A%d", row), "시퀀스")
		if e.config.Language == "en" {
			f.SetCellValue(sheet, fmt.Sprintf("A%d", row), "SEQUENCES")
		}
		f.MergeCell(sheet, fmt.Sprintf("A%d", row), fmt.Sprintf("G%d", row))
		row++

		headers := []string{"이름", "최소값", "최대값", "증가값", "현재값", "순환", "설명"}
		if e.config.Language == "en" {
			headers = []string{"Name", "Min", "Max", "Increment", "Current", "Cyclic", "Comment"}
		}
		for i, h := range headers {
			f.SetCellValue(sheet, fmt.Sprintf("%c%d", 'A'+i, row), h)
		}
		headerStyle := e.getHeaderStyle(f)
		f.SetCellStyle(sheet, fmt.Sprintf("A%d", row), fmt.Sprintf("G%d", row), headerStyle)
		row++

		for _, seq := range schema.Sequences {
			f.SetCellValue(sheet, fmt.Sprintf("A%d", row), seq.Name)
			f.SetCellValue(sheet, fmt.Sprintf("B%d", row), seq.MinValue)
			f.SetCellValue(sheet, fmt.Sprintf("C%d", row), seq.MaxValue)
			f.SetCellValue(sheet, fmt.Sprintf("D%d", row), seq.Increment)
			f.SetCellValue(sheet, fmt.Sprintf("E%d", row), seq.LastNumber)
			f.SetCellValue(sheet, fmt.Sprintf("F%d", row), boolToYN(seq.IsCyclic))
			f.SetCellValue(sheet, fmt.Sprintf("G%d", row), seq.Comment)
			row++
		}
		row++
	}

	// Triggers section (NO trigger body - SECURITY)
	if len(schema.Triggers) > 0 {
		f.SetCellValue(sheet, fmt.Sprintf("A%d", row), "트리거")
		if e.config.Language == "en" {
			f.SetCellValue(sheet, fmt.Sprintf("A%d", row), "TRIGGERS")
		}
		f.MergeCell(sheet, fmt.Sprintf("A%d", row), fmt.Sprintf("G%d", row))
		row++

		headers := []string{"이름", "테이블", "시점", "이벤트", "레벨", "상태", "설명"}
		if e.config.Language == "en" {
			headers = []string{"Name", "Table", "Timing", "Event", "Level", "Status", "Comment"}
		}
		for i, h := range headers {
			f.SetCellValue(sheet, fmt.Sprintf("%c%d", 'A'+i, row), h)
		}
		headerStyle := e.getHeaderStyle(f)
		f.SetCellStyle(sheet, fmt.Sprintf("A%d", row), fmt.Sprintf("G%d", row), headerStyle)
		row++

		for _, trg := range schema.Triggers {
			f.SetCellValue(sheet, fmt.Sprintf("A%d", row), trg.Name)
			f.SetCellValue(sheet, fmt.Sprintf("B%d", row), trg.TargetTable)
			f.SetCellValue(sheet, fmt.Sprintf("C%d", row), trg.Timing)
			f.SetCellValue(sheet, fmt.Sprintf("D%d", row), trg.Event)
			f.SetCellValue(sheet, fmt.Sprintf("E%d", row), trg.Level)
			f.SetCellValue(sheet, fmt.Sprintf("F%d", row), trg.Status)
			f.SetCellValue(sheet, fmt.Sprintf("G%d", row), trg.Comment)
			row++
		}
		row++
	}

	// Synonyms section
	if len(schema.Synonyms) > 0 {
		f.SetCellValue(sheet, fmt.Sprintf("A%d", row), "동의어")
		if e.config.Language == "en" {
			f.SetCellValue(sheet, fmt.Sprintf("A%d", row), "SYNONYMS")
		}
		f.MergeCell(sheet, fmt.Sprintf("A%d", row), fmt.Sprintf("E%d", row))
		row++

		headers := []string{"이름", "대상", "소유자", "유형", "설명"}
		if e.config.Language == "en" {
			headers = []string{"Name", "Target", "Owner", "Type", "Comment"}
		}
		for i, h := range headers {
			f.SetCellValue(sheet, fmt.Sprintf("%c%d", 'A'+i, row), h)
		}
		headerStyle := e.getHeaderStyle(f)
		f.SetCellStyle(sheet, fmt.Sprintf("A%d", row), fmt.Sprintf("E%d", row), headerStyle)
		row++

		for _, syn := range schema.Synonyms {
			f.SetCellValue(sheet, fmt.Sprintf("A%d", row), syn.Name)
			f.SetCellValue(sheet, fmt.Sprintf("B%d", row), syn.TargetObject)
			f.SetCellValue(sheet, fmt.Sprintf("C%d", row), syn.TargetOwner)
			f.SetCellValue(sheet, fmt.Sprintf("D%d", row), syn.TargetType)
			f.SetCellValue(sheet, fmt.Sprintf("E%d", row), syn.Comment)
			row++
		}
	}

	// Auto-fit
	f.SetColWidth(sheet, "A", "A", 25)
	f.SetColWidth(sheet, "B", "B", 20)
	f.SetColWidth(sheet, "D", "D", 50)
	f.SetColWidth(sheet, "G", "G", 40)

	return nil
}

// getHeaderStyle returns the gray header style (CRITICAL RULE #2)
func (e *Exporter) getHeaderStyle(f *excelize.File) int {
	style, _ := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true, Size: 11},
		Fill: excelize.Fill{
			Type:    "pattern",
			Color:   []string{"#D9D9D9"}, // Gray background
			Pattern: 1,
		},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
		Border: []excelize.Border{
			{Type: "top", Color: "000000", Style: 1},
			{Type: "bottom", Color: "000000", Style: 1},
			{Type: "left", Color: "000000", Style: 1},
			{Type: "right", Color: "000000", Style: 1},
		},
	})
	return style
}

// boolToYN converts bool to Y/N string
func boolToYN(b bool) string {
	if b {
		return "Y"
	}
	return "N"
}
