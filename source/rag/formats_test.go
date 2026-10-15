package rag

import (
	"archive/zip"
	"bytes"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/xuri/excelize/v2"
	"golang.org/x/text/encoding/simplifiedchinese"
)

func TestPrepareDocumentTextEncodings(t *testing.T) {
	gbk, _ := simplifiedchinese.GBK.NewEncoder().Bytes([]byte("登录检查"))
	for _, tt := range []struct {
		name string
		data []byte
		want string
	}{
		{"gbk.txt", gbk, "登录检查"},
		{"bom.json", append([]byte{239, 187, 191}, []byte(`{"ok":true}`)...), `{"ok":true}`},
		{"utf16.txt", []byte{255, 254, 65, 0, 66, 0}, "AB"},
		{"nul.txt", []byte("safe\x00text"), "safetext"},
		{"readme.md", []byte("# Section\n\nDetails"), "# Section\n\nDetails"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			text, mime, err := PrepareDocument(tt.name, "", tt.data)
			if err != nil || text != tt.want || !utf8.ValidString(text) {
				t.Fatalf("text=%q err=%v", text, err)
			}
			blocks, err := ParseDocument(tt.name, mime, []byte(text))
			if err != nil || len(ChunkBlocks(blocks, 900, 120)) == 0 {
				t.Fatalf("ingestion failed: %v", err)
			}
			if tt.name == "readme.md" && blocks[0].Heading != "Section" {
				t.Fatal("lost markdown headings")
			}
		})
	}
}

func TestPrepareDocumentDOCXRoundTrip(t *testing.T) {
	raw := makeDOCX(t, `<w:document xmlns:w="urn:word"><w:body><w:p><w:pPr><w:pStyle w:val="Heading1"/></w:pPr><w:r><w:t>认证检查</w:t></w:r></w:p><w:p><w:r><w:t>登录需要令牌</w:t></w:r></w:p></w:body></w:document>`)
	text, mime, err := PrepareDocument("report.docx", "", raw)
	if err != nil || mime != ExtractedTextMIME || !strings.Contains(text, "登录需要令牌") {
		t.Fatalf("text=%q mime=%q err=%v", text, mime, err)
	}
	blocks, err := ParseDocument("report.docx", mime, []byte(text))
	if err != nil || len(blocks) != 2 || blocks[1].Heading != "认证检查" {
		t.Fatalf("reparse lost structure: %#v, %v", blocks, err)
	}
}

func TestParseExtractedDocument(t *testing.T) {
	blocks, err := ParseDocument("report.pdf", ExtractedTextMIME, []byte("# Evidence\n\nPreserved content"))
	if err != nil || len(blocks) == 0 || blocks[0].Heading != "Evidence" {
		t.Fatalf("extracted document: blocks=%#v err=%v", blocks, err)
	}
}

func TestPrepareDocumentXLSXSharedStrings(t *testing.T) {
	book := excelize.NewFile()
	defer book.Close()
	_ = book.SetCellValue("Sheet1", "A1", "资产")
	_ = book.SetCellValue("Sheet1", "B1", "状态")
	_ = book.SetCellValue("Sheet1", "A2", "example.test")
	_ = book.SetCellValue("Sheet1", "B2", "已验证")
	buffer, err := book.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	text, _, err := PrepareDocument("assets.xlsx", "", buffer.Bytes())
	if err != nil || !strings.Contains(text, "资产\t状态") || !strings.Contains(text, "example.test\t已验证") {
		t.Fatalf("text=%q err=%v", text, err)
	}
}

func TestPrepareDocumentPPTXSlideOrder(t *testing.T) {
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for _, num := range []int{10, 2, 1} {
		file, _ := writer.Create(fmt.Sprintf("ppt/slides/slide%d.xml", num))
		fmt.Fprintf(file, `<p:sld xmlns:p="urn:p" xmlns:a="urn:a"><a:p><a:r><a:t>Slide %d</a:t></a:r></a:p></p:sld>`, num)
	}
	_ = writer.Close()
	text, _, err := PrepareDocument("deck.pptx", "", buffer.Bytes())
	if err != nil || strings.Index(text, "Slide 2") > strings.Index(text, "Slide 10") || !strings.Contains(text, "Slide 1") {
		t.Fatalf("text=%q err=%v", text, err)
	}
}

// A complete PDF fixture exercises xref, font decoding, and text extraction.
func pdfFixture(text string) []byte {
	var buffer bytes.Buffer
	buffer.WriteString("%PDF-1.4\n%\xb5\xb6\xb7\xb8\n")
	stream := "BT /F1 12 Tf 72 720 Td (" + text + ") Tj ET"
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >>",
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(stream), stream),
	}
	offsets := make([]int, len(objects))
	for i, object := range objects {
		offsets[i] = buffer.Len()
		fmt.Fprintf(&buffer, "%d 0 obj\n%s\nendobj\n", i+1, object)
	}
	xref := buffer.Len()
	fmt.Fprintf(&buffer, "xref\n0 %d\n0000000000 65535 f \n", len(objects)+1)
	for _, offset := range offsets {
		fmt.Fprintf(&buffer, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&buffer, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xref)
	return buffer.Bytes()
}

func TestPrepareDocumentPDF(t *testing.T) {
	raw := pdfFixture("RAG PDF upload regression")
	text, mime, err := PrepareDocument("report.pdf", "", raw)
	if err != nil || mime != ExtractedTextMIME || !strings.Contains(text, "RAG PDF upload regression") || !utf8.ValidString(text) {
		t.Fatalf("text=%q mime=%q err=%v", text, mime, err)
	}
	blocks, err := ParseDocument("report.pdf", mime, []byte(text))
	if err != nil || len(ChunkBlocks(blocks, 900, 120)) == 0 {
		t.Fatalf("reparse failed: %v", err)
	}
	if _, _, err := PrepareDocument("empty.pdf", "", pdfFixture("")); err == nil {
		t.Fatal("empty/scanned PDF must not be accepted")
	}
	if _, _, err := PrepareDocument("bad.pdf", "", []byte("%PDF-1.4\nstream")); err == nil {
		t.Fatal("malformed PDF must not be accepted")
	}
}
