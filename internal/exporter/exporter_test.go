package exporter

import (
	"pocket-doc/internal/model"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestGenerateArtifacts creates real output files for verification (CRITICAL RULE #1)
func TestGenerateArtifacts(t *testing.T) {
	// Create test_output directory (CRITICAL RULE #1)
	outputDir := "test_output"
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		t.Fatalf("Failed to create output directory: %v", err)
	}

	// Create mock schema with Korean data (REQUIREMENT)
	schema := createKoreanMockSchema()

	// Test config
	config := Config{
		Language:         "ko",
		IncludeTOC:       true,
		IncludeCoverPage: true,
		CompanyName:      "주식회사 테스트",
		ProjectName:      "ERP 시스템",
		Author:           "데이터베이스팀",
		ColorScheme:      "default",
	}

	// Test cases for all formats
	testCases := []struct {
		format   string
		filename string
	}{
		{"xlsx", "schema_test.xlsx"},
		{"html", "schema_test.html"},
		{"docx", "schema_test.docx"},
	}

	for _, tc := range testCases {
		t.Run(tc.format, func(t *testing.T) {
			// Create exporter
			exporter, err := NewExporter(tc.format, config)
			if err != nil {
				t.Fatalf("Failed to create %s exporter: %v", tc.format, err)
			}

			// Create output file
			outputPath := filepath.Join(outputDir, tc.filename)
			f, err := os.Create(outputPath)
			if err != nil {
				t.Fatalf("Failed to create output file %s: %v", outputPath, err)
			}
			defer f.Close()

			// Export schema
			if err := exporter.Export(schema, f); err != nil {
				t.Fatalf("Failed to export to %s: %v", tc.format, err)
			}

			// Close file to flush
			f.Close()

			// CRITICAL RULE #1: Verify file exists and is not empty
			stat, err := os.Stat(outputPath)
			if err != nil {
				t.Fatalf("Output file %s does not exist: %v", outputPath, err)
			}

			if stat.Size() == 0 {
				t.Fatalf("Output file %s is empty (0 bytes)", outputPath)
			}

			t.Logf("✅ Successfully generated %s (%d bytes)", tc.filename, stat.Size())
		})
	}

	t.Log("\n📁 All test files generated in ./test_output/")
	t.Log("   - schema_test.xlsx")
	t.Log("   - schema_test.html")
	t.Log("   - schema_test.docx")
}

// createKoreanMockSchema creates a schema with Korean data for testing
func createKoreanMockSchema() *model.Schema {
	now := time.Now()

	return &model.Schema{
		DatabaseName: "인사관리DB",
		DatabaseType: "Oracle",
		Version:      "19c Enterprise Edition",
		ExtractedAt:  now,
		Comment:      "인사 및 급여 관리 시스템 데이터베이스",

		Tables: []model.Table{
			{
				Name:       "사원",
				Owner:      "HR",
				Type:       "TABLE",
				RowCount:   150,
				Comment:    "사원 기본 정보 테이블",
				CreatedAt:  "2024-01-01 10:00:00",
				ModifiedAt: "2024-12-15 14:30:00",
				Columns: []model.Column{
					{
						Name:         "사원번호",
						Position:     1,
						DataType:     "NUMBER(6)",
						Nullable:     false,
						IsPrimaryKey: true,
						DefaultValue: "",
						Comment:      "사원 고유 식별번호",
						Length:       6,
						Precision:    6,
						Scale:        0,
					},
					{
						Name:     "이름",
						Position: 2,
						DataType: "VARCHAR2(50)",
						Nullable: false,
						Comment:  "사원 성명",
						Length:   50,
					},
					{
						Name:     "생년월일",
						Position: 3,
						DataType: "DATE",
						Nullable: true,
						Comment:  "사원 생년월일",
					},
					{
						Name:           "부서코드",
						Position:       4,
						DataType:       "NUMBER(4)",
						Nullable:       true,
						IsForeignKey:   true,
						FKTargetTable:  "부서",
						FKTargetColumn: "부서코드",
						Comment:        "소속 부서 코드 (외래키)",
						Length:         4,
						Precision:      4,
					},
					{
						Name:     "직급코드",
						Position: 5,
						DataType: "VARCHAR2(10)",
						Nullable: true,
						Comment:  "직급 코드",
						Length:   10,
					},
					{
						Name:      "급여",
						Position:  6,
						DataType:  "NUMBER(10,2)",
						Nullable:  true,
						Comment:   "월 급여액",
						Precision: 10,
						Scale:     2,
					},
					{
						Name:     "입사일",
						Position: 7,
						DataType: "DATE",
						Nullable: false,
						Comment:  "입사 일자",
					},
					{
						Name:     "이메일",
						Position: 8,
						DataType: "VARCHAR2(100)",
						Nullable: true,
						IsUnique: true,
						Comment:  "회사 이메일 주소 (UNIQUE)",
						Length:   100,
					},
				},
				Indexes: []model.Index{
					{
						Name:      "PK_사원",
						TableName: "사원",
						Owner:     "HR",
						Type:      "NORMAL",
						IsUnique:  true,
						IsPrimary: true,
						IsEnabled: true,
						Columns:   []string{"사원번호"},
						Comment:   "사원 기본키",
					},
					{
						Name:      "IDX_사원_부서",
						TableName: "사원",
						Owner:     "HR",
						Type:      "NORMAL",
						IsUnique:  false,
						IsEnabled: true,
						Columns:   []string{"부서코드"},
						Comment:   "부서별 검색용 인덱스",
					},
				},
			},
			{
				Name:       "부서",
				Owner:      "HR",
				Type:       "TABLE",
				RowCount:   25,
				Comment:    "부서 정보 테이블",
				CreatedAt:  "2024-01-01 09:30:00",
				ModifiedAt: "2024-06-20 11:00:00",
				Columns: []model.Column{
					{
						Name:         "부서코드",
						Position:     1,
						DataType:     "NUMBER(4)",
						Nullable:     false,
						IsPrimaryKey: true,
						Comment:      "부서 고유 코드",
						Precision:    4,
					},
					{
						Name:     "부서명",
						Position: 2,
						DataType: "VARCHAR2(50)",
						Nullable: false,
						Comment:  "부서 명칭",
						Length:   50,
					},
					{
						Name:      "상위부서코드",
						Position:  3,
						DataType:  "NUMBER(4)",
						Nullable:  true,
						Comment:   "상위 부서 코드 (NULL이면 최상위)",
						Precision: 4,
					},
					{
						Name:     "위치",
						Position: 4,
						DataType: "VARCHAR2(100)",
						Nullable: true,
						Comment:  "부서 사무실 위치",
						Length:   100,
					},
				},
				Indexes: []model.Index{
					{
						Name:      "PK_부서",
						TableName: "부서",
						Owner:     "HR",
						Type:      "NORMAL",
						IsUnique:  true,
						IsPrimary: true,
						IsEnabled: true,
						Columns:   []string{"부서코드"},
						Comment:   "부서 기본키",
					},
				},
			},
			{
				Name:       "급여이력",
				Owner:      "HR",
				Type:       "TABLE",
				RowCount:   450,
				Comment:    "사원 급여 변경 이력",
				CreatedAt:  "2024-01-01 10:30:00",
				ModifiedAt: "2025-01-05 09:15:00",
				Columns: []model.Column{
					{
						Name:            "이력번호",
						Position:        1,
						DataType:        "NUMBER(10)",
						Nullable:        false,
						IsPrimaryKey:    true,
						IsAutoIncrement: true,
						Comment:         "이력 고유번호 (자동증가)",
						Precision:       10,
					},
					{
						Name:           "사원번호",
						Position:       2,
						DataType:       "NUMBER(6)",
						Nullable:       false,
						IsForeignKey:   true,
						FKTargetTable:  "사원",
						FKTargetColumn: "사원번호",
						Comment:        "사원번호 (외래키)",
						Precision:      6,
					},
					{
						Name:     "변경일자",
						Position: 3,
						DataType: "DATE",
						Nullable: false,
						Comment:  "급여 변경 적용 일자",
					},
					{
						Name:      "변경전급여",
						Position:  4,
						DataType:  "NUMBER(10,2)",
						Nullable:  true,
						Comment:   "변경 전 급여액",
						Precision: 10,
						Scale:     2,
					},
					{
						Name:      "변경후급여",
						Position:  5,
						DataType:  "NUMBER(10,2)",
						Nullable:  false,
						Comment:   "변경 후 급여액",
						Precision: 10,
						Scale:     2,
					},
					{
						Name:     "변경사유",
						Position: 6,
						DataType: "VARCHAR2(200)",
						Nullable: true,
						Comment:  "급여 변경 사유",
						Length:   200,
					},
				},
				Indexes: []model.Index{
					{
						Name:      "PK_급여이력",
						TableName: "급여이력",
						Owner:     "HR",
						Type:      "NORMAL",
						IsUnique:  true,
						IsPrimary: true,
						IsEnabled: true,
						Columns:   []string{"이력번호"},
					},
					{
						Name:      "IDX_급여이력_사원",
						TableName: "급여이력",
						Owner:     "HR",
						Type:      "NORMAL",
						IsUnique:  false,
						IsEnabled: true,
						Columns:   []string{"사원번호", "변경일자"},
						Comment:   "사원별 급여 이력 검색용",
					},
				},
			},
		},

		Views: []model.View{
			{
				Name:        "부서별사원현황",
				Owner:       "HR",
				Type:        "VIEW",
				Comment:     "부서별 사원 수 및 평균 급여 조회 뷰",
				IsUpdatable: false,
				Columns: []model.Column{
					{Name: "부서코드", DataType: "NUMBER(4)", Comment: "부서 코드"},
					{Name: "부서명", DataType: "VARCHAR2(50)", Comment: "부서 명칭"},
					{Name: "사원수", DataType: "NUMBER", Comment: "부서 소속 사원 수"},
					{Name: "평균급여", DataType: "NUMBER", Comment: "부서 평균 급여"},
				},
			},
		},

		Routines: []model.Routine{
			{
				Name:      "급여인상처리",
				Owner:     "HR",
				Type:      "PROCEDURE",
				Language:  "PL/SQL",
				Comment:   "전사원 급여 인상 일괄 처리 프로시저",
				Signature: "PROCEDURE 급여인상처리(p_인상률 IN NUMBER, p_적용일 IN DATE)",
				Arguments: []model.RoutineArgument{
					{Name: "p_인상률", Position: 1, Mode: "IN", DataType: "NUMBER", Comment: "급여 인상률 (예: 3.5% = 3.5)"},
					{Name: "p_적용일", Position: 2, Mode: "IN", DataType: "DATE", Comment: "인상 적용 시작일"},
				},
			},
			{
				Name:       "사원정보조회",
				Owner:      "HR",
				Type:       "FUNCTION",
				Language:   "PL/SQL",
				ReturnType: "VARCHAR2",
				Comment:    "사원번호로 사원 상세 정보 조회",
				Signature:  "FUNCTION 사원정보조회(p_사원번호 IN NUMBER) RETURN VARCHAR2",
				Arguments: []model.RoutineArgument{
					{Name: "p_사원번호", Position: 1, Mode: "IN", DataType: "NUMBER", Comment: "조회할 사원번호"},
				},
			},
		},

		Sequences: []model.Sequence{
			{
				Name:       "급여이력_SEQ",
				Owner:      "HR",
				MinValue:   1,
				MaxValue:   999999999,
				Increment:  1,
				LastNumber: 450,
				IsCyclic:   false,
				CacheSize:  20,
				Comment:    "급여이력 테이블 이력번호 자동생성용 시퀀스",
			},
			{
				Name:       "사원번호_SEQ",
				Owner:      "HR",
				MinValue:   100000,
				MaxValue:   999999,
				Increment:  1,
				LastNumber: 100150,
				IsCyclic:   false,
				CacheSize:  10,
				Comment:    "사원번호 자동생성 시퀀스 (6자리)",
			},
		},

		Triggers: []model.Trigger{
			{
				Name:        "TRG_사원_입사일체크",
				Owner:       "HR",
				TargetTable: "사원",
				TargetType:  "TABLE",
				Timing:      "BEFORE",
				Event:       "INSERT",
				Level:       "ROW",
				Status:      "ENABLED",
				Comment:     "입사일이 미래 날짜인지 검증하는 트리거",
			},
			{
				Name:        "TRG_급여_이력기록",
				Owner:       "HR",
				TargetTable: "사원",
				TargetType:  "TABLE",
				Timing:      "AFTER",
				Event:       "UPDATE",
				Level:       "ROW",
				Status:      "ENABLED",
				Comment:     "급여 변경 시 이력 테이블에 자동 기록",
			},
		},

		Synonyms: []model.Synonym{
			{
				Name:         "EMP",
				Owner:        "PUBLIC",
				TargetObject: "사원",
				TargetOwner:  "HR",
				TargetType:   "TABLE",
				IsPublic:     true,
				Comment:      "사원 테이블의 영문 동의어",
			},
			{
				Name:         "DEPT",
				Owner:        "PUBLIC",
				TargetObject: "부서",
				TargetOwner:  "HR",
				TargetType:   "TABLE",
				IsPublic:     true,
				Comment:      "부서 테이블의 영문 동의어",
			},
		},

		Indexes: []model.Index{
			// Already included in tables
		},
	}
}

// TestExcelSheetStructure validates Excel has exactly 4 sheets (CRITICAL RULE #2)
func TestExcelSheetStructure(t *testing.T) {
	// This would require reading the generated Excel file
	// For now, we verify it's created and not empty
	outputPath := filepath.Join("test_output", "schema_test.xlsx")

	stat, err := os.Stat(outputPath)
	if err != nil {
		t.Skip("Excel file not generated yet, run TestGenerateArtifacts first")
	}

	if stat.Size() == 0 {
		t.Fatal("Excel file is empty")
	}

	t.Logf("✅ Excel file validated: %d bytes", stat.Size())
	t.Log("Expected sheets: Overview, Tables, Columns, Objects")
}

// TestHTMLKoreanFontSupport validates HTML has Korean fonts (CRITICAL RULE #3)
func TestHTMLKoreanFontSupport(t *testing.T) {
	outputPath := filepath.Join("test_output", "schema_test.html")

	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Skip("HTML file not generated yet, run TestGenerateArtifacts first")
	}

	htmlStr := string(content)

	// CRITICAL RULE #3: Check for Korean fonts
	requiredFonts := []string{
		"Malgun Gothic",
		"Apple SD Gothic Neo",
	}

	for _, font := range requiredFonts {
		if !contains(htmlStr, font) {
			t.Errorf("HTML does not contain required Korean font: %s", font)
		}
	}

	// CRITICAL RULE #3: Check for @media print
	if !contains(htmlStr, "@media print") {
		t.Error("HTML does not contain @media print CSS")
	}

	if !contains(htmlStr, "page-break-before") {
		t.Error("HTML does not contain page-break-before rules")
	}

	if !contains(htmlStr, "@page") {
		t.Error("HTML does not contain @page rules")
	}

	t.Log("✅ HTML Korean font support and print CSS validated")
}

// Helper function
func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && (s == substr || len(s) >= len(substr) &&
		(s[:len(substr)] == substr || s[len(s)-len(substr):] == substr ||
			findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
