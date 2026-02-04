package html

import (
	"dbms-to-document/internal/model"
	"html/template"
	"io"
)

// Config holds configuration for HTML export
type Config struct {
	Language string
	Title    string
}

// Exporter implements HTML export functionality
type Exporter struct {
	config Config
}

// NewExporter creates a new HTML exporter
func NewExporter(cfg Config) *Exporter {
	return &Exporter{config: cfg}
}

// Format returns the format name
func (e *Exporter) Format() string {
	return "html"
}

// MimeType returns the MIME type
func (e *Exporter) MimeType() string {
	return "text/html; charset=utf-8"
}

// FileExtension returns the file extension
func (e *Exporter) FileExtension() string {
	return ".html"
}

// Export generates an HTML document with print-optimized CSS
// CRITICAL RULE #3: Korean fonts FIRST + @media print rules
func (e *Exporter) Export(schema *model.Schema, w io.Writer) error {
	tmpl := template.Must(template.New("schema").Parse(htmlTemplate))
	return tmpl.Execute(w, schema)
}

// htmlTemplate with Korean font support and print CSS (CRITICAL RULES)
const htmlTemplate = `<!DOCTYPE html>
<html lang="ko">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{.DatabaseName}} - 스키마 문서</title>
    <style>
        /* CRITICAL RULE #3: Korean Fonts FIRST */
        * {
            font-family: 'Malgun Gothic', 'Apple SD Gothic Neo', 'Noto Sans KR', 
                         -apple-system, BlinkMacSystemFont, 'Segoe UI', 
                         Arial, sans-serif;
            box-sizing: border-box;
        }

        body {
            margin: 0;
            padding: 20px;
            background: #f5f5f5;
            color: #333;
            line-height: 1.6;
        }

        .container {
            max-width: 1200px;
            margin: 0 auto;
            background: white;
            padding: 40px;
            box-shadow: 0 2px 10px rgba(0,0,0,0.1);
        }

        h1 {
            color: #2c3e50;
            border-bottom: 3px solid #3498db;
            padding-bottom: 10px;
            margin-bottom: 30px;
        }

        h2 {
            color: #34495e;
            border-bottom: 2px solid #95a5a6;
            padding-bottom: 8px;
            margin-top: 40px;
            margin-bottom: 20px;
        }

        h3 {
            color: #7f8c8d;
            margin-top: 30px;
            margin-bottom: 15px;
        }

        table {
            width: 100%;
            border-collapse: collapse;
            margin-bottom: 30px;
            background: white;
        }

        th {
            background: #D9D9D9;
            color: #333;
            font-weight: bold;
            text-align: left;
            padding: 12px;
            border: 1px solid #bdc3c7;
        }

        td {
            padding: 10px 12px;
            border: 1px solid #ecf0f1;
        }

        tr:nth-child(even) {
            background: #f9f9f9;
        }

        tr:hover {
            background: #e8f4f8;
        }

        .badge {
            display: inline-block;
            padding: 3px 8px;
            border-radius: 3px;
            font-size: 11px;
            font-weight: bold;
        }

        .badge-pk { background: #2ecc71; color: white; }
        .badge-fk { background: #3498db; color: white; }
        .badge-uk { background: #f39c12; color: white; }

        .summary {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
            gap: 15px;
            margin-bottom: 30px;
        }

        .summary-card {
            background: #ecf0f1;
            padding: 15px;
            border-radius: 5px;
            border-left: 4px solid #3498db;
        }

        .summary-card h3 {
            margin: 0 0 5px 0;
            font-size: 14px;
            color: #7f8c8d;
        }

        .summary-card .value {
            font-size: 24px;
            font-weight: bold;
            color: #2c3e50;
        }

        /* CRITICAL RULE #3: @media print CSS */
        @media print {
            @page {
                size: A4;
                margin: 2cm;
            }

            body {
                background: white;
                padding: 0;
            }

            .container {
                box-shadow: none;
                padding: 0;
            }

            /* Page breaks for major sections */
            h1, h2 {
                page-break-before: always;
            }

            h1:first-of-type, h2:first-of-type {
                page-break-before: avoid;
            }

            /* Keep headings with content */
            h3, h4 {
                page-break-after: avoid;
            }

            /* Avoid breaking tables */
            table {
                page-break-inside: avoid;
            }

            tr {
                page-break-inside: avoid;
            }

            /* Hide interactive elements */
            .no-print {
                display: none;
            }
        }
    </style>
</head>
<body>
    <div class="container">
        <h1>{{.DatabaseName}} - 데이터베이스 스키마 문서</h1>

        <div class="summary">
            <div class="summary-card">
                <h3>데이터베이스 유형</h3>
                <div class="value">{{.DatabaseType}}</div>
            </div>
            <div class="summary-card">
                <h3>버전</h3>
                <div class="value">{{.Version}}</div>
            </div>
            <div class="summary-card">
                <h3>테이블 수</h3>
                <div class="value">{{len .Tables}}</div>
            </div>
            <div class="summary-card">
                <h3>뷰 수</h3>
                <div class="value">{{len .Views}}</div>
            </div>
            <div class="summary-card">
                <h3>프로시저/함수 수</h3>
                <div class="value">{{len .Routines}}</div>
            </div>
            <div class="summary-card">
                <h3>트리거 수</h3>
                <div class="value">{{len .Triggers}}</div>
            </div>
        </div>

        {{if .Tables}}
        <h2>📋 테이블 목록</h2>
        <table>
            <thead>
                <tr>
                    <th>이름</th>
                    <th>소유자</th>
                    <th>행 수</th>
                    <th>설명</th>
                </tr>
            </thead>
            <tbody>
                {{range .Tables}}
                <tr>
                    <td><strong>{{.Name}}</strong></td>
                    <td>{{.Owner}}</td>
                    <td>{{.RowCount}}</td>
                    <td>{{.Comment}}</td>
                </tr>
                {{end}}
            </tbody>
        </table>

        {{range .Tables}}
        <h3>테이블: {{.Name}}</h3>
        {{if .Comment}}<p><em>{{.Comment}}</em></p>{{end}}
        
        <table>
            <thead>
                <tr>
                    <th>컬럼명</th>
                    <th>데이터타입</th>
                    <th>NULL허용</th>
                    <th>제약조건</th>
                    <th>기본값</th>
                    <th>설명</th>
                </tr>
            </thead>
            <tbody>
                {{range .Columns}}
                <tr>
                    <td><strong>{{.Name}}</strong></td>
                    <td>{{.DataType}}</td>
                    <td>{{if .Nullable}}YES{{else}}NO{{end}}</td>
                    <td>
                        {{if .IsPrimaryKey}}<span class="badge badge-pk">PK</span>{{end}}
                        {{if .IsForeignKey}}<span class="badge badge-fk">FK</span>{{end}}
                        {{if .IsUnique}}<span class="badge badge-uk">UK</span>{{end}}
                    </td>
                    <td>{{.DefaultValue}}</td>
                    <td>{{.Comment}}</td>
                </tr>
                {{end}}
            </tbody>
        </table>
        {{end}}
        {{end}}

        {{if .Routines}}
        <h2>⚙️ 프로시저 / 함수</h2>
        <table>
            <thead>
                <tr>
                    <th>이름</th>
                    <th>유형</th>
                    <th>서명</th>
                    <th>설명</th>
                </tr>
            </thead>
            <tbody>
                {{range .Routines}}
                <tr>
                    <td><strong>{{.Name}}</strong></td>
                    <td>{{.Type}}</td>
                    <td><code>{{.Signature}}</code></td>
                    <td>{{.Comment}}</td>
                </tr>
                {{end}}
            </tbody>
        </table>
        <p style="color: #7f8c8d; font-size: 12px;">
            ⚠️ 보안: 프로시저 본문은 제외되었습니다 (서명만 표시)
        </p>
        {{end}}

        {{if .Triggers}}
        <h2>🔔 트리거</h2>
        <table>
            <thead>
                <tr>
                    <th>이름</th>
                    <th>대상 테이블</th>
                    <th>시점</th>
                    <th>이벤트</th>
                    <th>상태</th>
                    <th>설명</th>
                </tr>
            </thead>
            <tbody>
                {{range .Triggers}}
                <tr>
                    <td><strong>{{.Name}}</strong></td>
                    <td>{{.TargetTable}}</td>
                    <td>{{.Timing}}</td>
                    <td>{{.Event}}</td>
                    <td>{{.Status}}</td>
                    <td>{{.Comment}}</td>
                </tr>
                {{end}}
            </tbody>
        </table>
        <p style="color: #7f8c8d; font-size: 12px;">
            ⚠️ 보안: 트리거 정의는 제외되었습니다 (메타데이터만 표시)
        </p>
        {{end}}

        {{if .Sequences}}
        <h2>🔢 시퀀스</h2>
        <table>
            <thead>
                <tr>
                    <th>이름</th>
                    <th>최소값</th>
                    <th>최대값</th>
                    <th>증가값</th>
                    <th>현재값</th>
                    <th>설명</th>
                </tr>
            </thead>
            <tbody>
                {{range .Sequences}}
                <tr>
                    <td><strong>{{.Name}}</strong></td>
                    <td>{{.MinValue}}</td>
                    <td>{{.MaxValue}}</td>
                    <td>{{.Increment}}</td>
                    <td>{{.LastNumber}}</td>
                    <td>{{.Comment}}</td>
                </tr>
                {{end}}
            </tbody>
        </table>
        {{end}}

        <hr style="margin: 40px 0; border: none; border-top: 2px solid #ecf0f1;">
        <p style="text-align: center; color: #95a5a6; font-size: 12px;">
            생성 시간: {{.ExtractedAt.Format "2006-01-02 15:04:05"}} | 
            DBMS-to-Document Tool
        </p>
    </div>
</body>
</html>
`
