package docx

import (
	"archive/zip"
	"dbms-to-document/internal/model"
	"fmt"
	"io"
	"strings"
	"time"
)

// Config holds configuration for Word export
type Config struct {
	Language         string
	IncludeTOC       bool
	IncludeCoverPage bool
	CompanyName      string
	ProjectName      string
	Author           string
	ExcludeTypes     []string
	ColorScheme      string
}

// Exporter implements Word (.docx) export functionality
type Exporter struct {
	config Config
}

// NewExporter creates a new Word exporter
func NewExporter(cfg Config) *Exporter {
	return &Exporter{config: cfg}
}

// Format returns the format name
func (e *Exporter) Format() string {
	return "docx"
}

// MimeType returns the MIME type
func (e *Exporter) MimeType() string {
	return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
}

// FileExtension returns the file extension
func (e *Exporter) FileExtension() string {
	return ".docx"
}

// Export generates a valid .docx file (OOXML format)
// Creates a minimal but valid ZIP-based Word document
func (e *Exporter) Export(schema *model.Schema, w io.Writer) error {
	// Create ZIP writer
	zipWriter := zip.NewWriter(w)
	defer zipWriter.Close()

	// 1. [Content_Types].xml
	if err := e.writeContentTypes(zipWriter); err != nil {
		return err
	}

	// 2. _rels/.rels
	if err := e.writeRels(zipWriter); err != nil {
		return err
	}

	// 3. word/_rels/document.xml.rels
	if err := e.writeDocumentRels(zipWriter); err != nil {
		return err
	}

	// 4. word/document.xml (main content)
	if err := e.writeDocument(zipWriter, schema); err != nil {
		return err
	}

	// 5. word/styles.xml
	if err := e.writeStyles(zipWriter); err != nil {
		return err
	}

	return nil
}

// writeContentTypes creates [Content_Types].xml
func (e *Exporter) writeContentTypes(zw *zip.Writer) error {
	f, err := zw.Create("[Content_Types].xml")
	if err != nil {
		return err
	}

	content := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
	<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
	<Default Extension="xml" ContentType="application/xml"/>
	<Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>
	<Override PartName="/word/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.styles+xml"/>
</Types>`

	_, err = f.Write([]byte(content))
	return err
}

// writeRels creates _rels/.rels
func (e *Exporter) writeRels(zw *zip.Writer) error {
	f, err := zw.Create("_rels/.rels")
	if err != nil {
		return err
	}

	content := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
	<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>
</Relationships>`

	_, err = f.Write([]byte(content))
	return err
}

// writeDocumentRels creates word/_rels/document.xml.rels
func (e *Exporter) writeDocumentRels(zw *zip.Writer) error {
	f, err := zw.Create("word/_rels/document.xml.rels")
	if err != nil {
		return err
	}

	content := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
	<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/>
</Relationships>`

	_, err = f.Write([]byte(content))
	return err
}

// writeDocument creates word/document.xml with schema content
func (e *Exporter) writeDocument(zw *zip.Writer, schema *model.Schema) error {
	f, err := zw.Create("word/document.xml")
	if err != nil {
		return err
	}

	var body strings.Builder

	// Title
	body.WriteString(e.paragraph(fmt.Sprintf("%s - 데이터베이스 스키마 문서", schema.DatabaseName), "Title"))
	body.WriteString(e.paragraph("", "Normal"))

	// Overview
	body.WriteString(e.paragraph("개요", "Heading1"))
	body.WriteString(e.paragraph(fmt.Sprintf("데이터베이스 유형: %s", schema.DatabaseType), "Normal"))
	body.WriteString(e.paragraph(fmt.Sprintf("버전: %s", schema.Version), "Normal"))
	body.WriteString(e.paragraph(fmt.Sprintf("추출 시간: %s", schema.ExtractedAt.Format(time.RFC3339)), "Normal"))
	body.WriteString(e.paragraph("", "Normal"))

	// Summary
	body.WriteString(e.paragraph("객체 통계", "Heading2"))
	body.WriteString(e.paragraph(fmt.Sprintf("• 테이블: %d", len(schema.Tables)), "Normal"))
	body.WriteString(e.paragraph(fmt.Sprintf("• 뷰: %d", len(schema.Views)), "Normal"))
	body.WriteString(e.paragraph(fmt.Sprintf("• 프로시저/함수: %d", len(schema.Routines)), "Normal"))
	body.WriteString(e.paragraph(fmt.Sprintf("• 시퀀스: %d", len(schema.Sequences)), "Normal"))
	body.WriteString(e.paragraph(fmt.Sprintf("• 트리거: %d", len(schema.Triggers)), "Normal"))
	body.WriteString(e.paragraph(fmt.Sprintf("• 동의어: %d", len(schema.Synonyms)), "Normal"))
	body.WriteString(e.paragraph("", "Normal"))

	// Tables
	if len(schema.Tables) > 0 {
		body.WriteString(e.paragraph("테이블 목록", "Heading1"))
		for _, table := range schema.Tables {
			body.WriteString(e.paragraph(fmt.Sprintf("테이블: %s", table.Name), "Heading2"))
			if table.Comment != "" {
				body.WriteString(e.paragraph(table.Comment, "Normal"))
			}
			body.WriteString(e.paragraph(fmt.Sprintf("소유자: %s, 행 수: %d", table.Owner, table.RowCount), "Normal"))

			// Columns
			if len(table.Columns) > 0 {
				body.WriteString(e.paragraph("컬럼:", "Heading3"))
				for _, col := range table.Columns {
					constraints := ""
					if col.IsPrimaryKey {
						constraints += "[PK] "
					}
					if col.IsForeignKey {
						constraints += "[FK] "
					}
					if col.IsUnique {
						constraints += "[UK] "
					}

					colInfo := fmt.Sprintf("  • %s (%s) %s", col.Name, col.DataType, constraints)
					if col.Comment != "" {
						colInfo += fmt.Sprintf(" - %s", col.Comment)
					}
					body.WriteString(e.paragraph(colInfo, "Normal"))
				}
			}
			body.WriteString(e.paragraph("", "Normal"))
		}
	}

	// Routines (NO source code - SECURITY)
	if len(schema.Routines) > 0 {
		body.WriteString(e.paragraph("프로시저 / 함수", "Heading1"))
		body.WriteString(e.paragraph("⚠️ 보안: 프로시저 본문은 제외되었습니다 (서명만 표시)", "Normal"))
		body.WriteString(e.paragraph("", "Normal"))

		for _, routine := range schema.Routines {
			body.WriteString(e.paragraph(fmt.Sprintf("%s: %s", routine.Type, routine.Name), "Heading2"))
			body.WriteString(e.paragraph(routine.Signature, "Normal"))
			if routine.Comment != "" {
				body.WriteString(e.paragraph(routine.Comment, "Normal"))
			}
			body.WriteString(e.paragraph("", "Normal"))
		}
	}

	// Triggers (NO definition - SECURITY)
	if len(schema.Triggers) > 0 {
		body.WriteString(e.paragraph("트리거", "Heading1"))
		body.WriteString(e.paragraph("⚠️ 보안: 트리거 정의는 제외되었습니다 (메타데이터만 표시)", "Normal"))
		body.WriteString(e.paragraph("", "Normal"))

		for _, trg := range schema.Triggers {
			body.WriteString(e.paragraph(fmt.Sprintf("트리거: %s", trg.Name), "Heading2"))
			body.WriteString(e.paragraph(fmt.Sprintf("대상 테이블: %s", trg.TargetTable), "Normal"))
			body.WriteString(e.paragraph(fmt.Sprintf("시점: %s, 이벤트: %s, 상태: %s", trg.Timing, trg.Event, trg.Status), "Normal"))
			if trg.Comment != "" {
				body.WriteString(e.paragraph(trg.Comment, "Normal"))
			}
			body.WriteString(e.paragraph("", "Normal"))
		}
	}

	// Sequences
	if len(schema.Sequences) > 0 {
		body.WriteString(e.paragraph("시퀀스", "Heading1"))
		for _, seq := range schema.Sequences {
			body.WriteString(e.paragraph(fmt.Sprintf("시퀀스: %s", seq.Name), "Heading2"))
			body.WriteString(e.paragraph(fmt.Sprintf("범위: %d ~ %d, 증가: %d, 현재: %d",
				seq.MinValue, seq.MaxValue, seq.Increment, seq.LastNumber), "Normal"))
			if seq.Comment != "" {
				body.WriteString(e.paragraph(seq.Comment, "Normal"))
			}
			body.WriteString(e.paragraph("", "Normal"))
		}
	}

	// Footer
	body.WriteString(e.paragraph("", "Normal"))
	body.WriteString(e.paragraph("──────────────────────────────────────", "Normal"))
	body.WriteString(e.paragraph("생성: DBMS-to-Document Tool", "Normal"))

	content := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
	<w:body>
%s
		<w:sectPr>
			<w:pgSz w:w="11906" w:h="16838"/>
			<w:pgMar w:top="1440" w:right="1440" w:bottom="1440" w:left="1440"/>
		</w:sectPr>
	</w:body>
</w:document>`, body.String())

	_, err = f.Write([]byte(content))
	return err
}

// paragraph creates a Word paragraph with specified style
func (e *Exporter) paragraph(text, style string) string {
	// Escape XML special characters
	text = strings.ReplaceAll(text, "&", "&amp;")
	text = strings.ReplaceAll(text, "<", "&lt;")
	text = strings.ReplaceAll(text, ">", "&gt;")
	text = strings.ReplaceAll(text, "\"", "&quot;")

	return fmt.Sprintf(`		<w:p>
			<w:pPr>
				<w:pStyle w:val="%s"/>
			</w:pPr>
			<w:r>
				<w:t xml:space="preserve">%s</w:t>
			</w:r>
		</w:p>
`, style, text)
}

// writeStyles creates word/styles.xml with Korean font support
func (e *Exporter) writeStyles(zw *zip.Writer) error {
	f, err := zw.Create("word/styles.xml")
	if err != nil {
		return err
	}

	// CRITICAL: Korean font support - Malgun Gothic
	content := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:styles xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
	<w:docDefaults>
		<w:rPrDefault>
			<w:rPr>
				<w:rFonts w:ascii="Malgun Gothic" w:hAnsi="Malgun Gothic" w:eastAsia="Malgun Gothic" w:cs="Malgun Gothic"/>
				<w:sz w:val="22"/>
				<w:szCs w:val="22"/>
			</w:rPr>
		</w:rPrDefault>
	</w:docDefaults>
	<w:style w:type="paragraph" w:styleId="Normal">
		<w:name w:val="Normal"/>
		<w:qFormat/>
		<w:rPr>
			<w:rFonts w:ascii="Malgun Gothic" w:hAnsi="Malgun Gothic" w:eastAsia="Malgun Gothic"/>
			<w:sz w:val="22"/>
		</w:rPr>
	</w:style>
	<w:style w:type="paragraph" w:styleId="Title">
		<w:name w:val="Title"/>
		<w:basedOn w:val="Normal"/>
		<w:qFormat/>
		<w:rPr>
			<w:rFonts w:ascii="Malgun Gothic" w:hAnsi="Malgun Gothic" w:eastAsia="Malgun Gothic"/>
			<w:b/>
			<w:sz w:val="56"/>
			<w:color w:val="2E74B5"/>
		</w:rPr>
	</w:style>
	<w:style w:type="paragraph" w:styleId="Heading1">
		<w:name w:val="Heading 1"/>
		<w:basedOn w:val="Normal"/>
		<w:qFormat/>
		<w:rPr>
			<w:rFonts w:ascii="Malgun Gothic" w:hAnsi="Malgun Gothic" w:eastAsia="Malgun Gothic"/>
			<w:b/>
			<w:sz w:val="32"/>
			<w:color w:val="2E74B5"/>
		</w:rPr>
		<w:pPr>
			<w:spacing w:before="480" w:after="240"/>
		</w:pPr>
	</w:style>
	<w:style w:type="paragraph" w:styleId="Heading2">
		<w:name w:val="Heading 2"/>
		<w:basedOn w:val="Normal"/>
		<w:qFormat/>
		<w:rPr>
			<w:rFonts w:ascii="Malgun Gothic" w:hAnsi="Malgun Gothic" w:eastAsia="Malgun Gothic"/>
			<w:b/>
			<w:sz w:val="28"/>
			<w:color w:val="2E74B5"/>
		</w:rPr>
		<w:pPr>
			<w:spacing w:before="360" w:after="180"/>
		</w:pPr>
	</w:style>
	<w:style w:type="paragraph" w:styleId="Heading3">
		<w:name w:val="Heading 3"/>
		<w:basedOn w:val="Normal"/>
		<w:qFormat/>
		<w:rPr>
			<w:rFonts w:ascii="Malgun Gothic" w:hAnsi="Malgun Gothic" w:eastAsia="Malgun Gothic"/>
			<w:b/>
			<w:sz w:val="24"/>
			<w:color w:val="1F4D78"/>
		</w:rPr>
		<w:pPr>
			<w:spacing w:before="240" w:after="120"/>
		</w:pPr>
	</w:style>
</w:styles>`

	_, err = f.Write([]byte(content))
	return err
}
