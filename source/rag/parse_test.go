package rag

import (
	"archive/zip"
	"bytes"
	"testing"
)

func TestParseDOCX(t *testing.T) {
	content := makeDOCX(t, `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>
<w:p><w:pPr><w:pStyle w:val="Title"/></w:pPr><w:r><w:t>安全报告</w:t></w:r></w:p>
<w:p><w:pPr><w:pStyle w:val="Heading1"/></w:pPr><w:r><w:t>认证</w:t></w:r></w:p>
<w:p><w:r><w:t>接口需要登录</w:t></w:r><w:r><w:tab/><w:t>继续检查</w:t></w:r></w:p>
</w:body></w:document>`)

	blocks, err := ParseDocument("report.docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document", content)
	if err != nil {
		t.Fatalf("parse docx: %v", err)
	}
	if len(blocks) != 3 {
		t.Fatalf("expected 3 blocks, got %d", len(blocks))
	}
	if blocks[0].Heading != "安全报告" || len(blocks[1].HeadingPath) != 2 ||
		blocks[1].HeadingPath[0] != "安全报告" || blocks[1].HeadingPath[1] != "认证" {
		t.Fatalf("unexpected heading metadata: %#v %#v", blocks[0], blocks[1])
	}
	if blocks[2].Content != "接口需要登录\t继续检查" {
		t.Fatalf("unexpected paragraph text: %q", blocks[2].Content)
	}
}

func TestParseDOCXHeadingHierarchy(t *testing.T) {
	tests := []struct {
		name     string
		document string
		want     [][]string
	}{
		{
			name: "title with nested and sibling headings",
			document: `<w:p><w:pPr><w:pStyle w:val="Title"/></w:pPr><w:r><w:t>Report</w:t></w:r></w:p>
<w:p><w:pPr><w:pStyle w:val="Heading1"/></w:pPr><w:r><w:t>Login</w:t></w:r></w:p>
<w:p><w:pPr><w:pStyle w:val="Heading2"/></w:pPr><w:r><w:t>Session</w:t></w:r></w:p>
<w:p><w:pPr><w:pStyle w:val="Heading1"/></w:pPr><w:r><w:t>Payments</w:t></w:r></w:p>
<w:p><w:r><w:t>Details</w:t></w:r></w:p>`,
			want: [][]string{{"Report"}, {"Report", "Login"}, {"Report", "Login", "Session"}, {"Report", "Payments"}, {"Report", "Payments"}},
		},
		{
			name: "headings without title",
			document: `<w:p><w:pPr><w:pStyle w:val="Heading1"/></w:pPr><w:r><w:t>Login</w:t></w:r></w:p>
<w:p><w:pPr><w:pStyle w:val="Heading2"/></w:pPr><w:r><w:t>Session</w:t></w:r></w:p>
<w:p><w:pPr><w:pStyle w:val="Heading1"/></w:pPr><w:r><w:t>Payments</w:t></w:r></w:p>`,
			want: [][]string{{"Login"}, {"Login", "Session"}, {"Payments"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content := makeDOCX(t, `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>`+tt.document+`</w:body></w:document>`)
			blocks, err := ParseDocument("report.docx", "", content)
			if err != nil {
				t.Fatal(err)
			}
			if len(blocks) != len(tt.want) {
				t.Fatalf("got %d blocks, want %d", len(blocks), len(tt.want))
			}
			for i, want := range tt.want {
				if len(blocks[i].HeadingPath) != len(want) {
					t.Fatalf("block %d heading path = %q, want %q", i, blocks[i].HeadingPath, want)
				}
				for j, heading := range want {
					if blocks[i].HeadingPath[j] != heading {
						t.Fatalf("block %d heading path = %q, want %q", i, blocks[i].HeadingPath, want)
					}
				}
			}
		})
	}
}

func TestParseDOCXMissingDocument(t *testing.T) {
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	file, err := writer.Create("word/other.xml")
	if err != nil {
		t.Fatalf("create test zip entry: %v", err)
	}
	if _, err := file.Write([]byte("<root/>")); err != nil {
		t.Fatalf("write test zip entry: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close test zip: %v", err)
	}
	if _, err := ParseDocument("report.docx", "", buffer.Bytes()); err == nil {
		t.Fatal("expected missing document.xml error")
	}
}

func makeDOCX(t *testing.T, document string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	file, err := writer.Create("word/document.xml")
	if err != nil {
		t.Fatalf("create document.xml: %v", err)
	}
	if _, err := file.Write([]byte(document)); err != nil {
		t.Fatalf("write document.xml: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close docx zip: %v", err)
	}
	return buffer.Bytes()
}
