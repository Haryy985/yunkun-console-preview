package server

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"mime/multipart"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"yunkun/console/db"
	"yunkun/console/rag"
)

func ragTestZIP(t *testing.T, names []string, contents [][]byte) []byte {
	t.Helper()
	var out bytes.Buffer
	writer := zip.NewWriter(&out)
	for i, name := range names {
		header := &zip.FileHeader{Name: name, Method: zip.Deflate}
		if strings.Contains(name, "symlink") {
			header.SetMode(fs.ModeSymlink | 0777)
		}
		file, err := writer.CreateHeader(header)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write(contents[i]); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func TestRAGUploadArchivePathsAndLimits(t *testing.T) {
	for _, name := range []string{"../bad.txt", "/absolute.txt", `C:\bad.txt`, `folder\..\bad.txt`, "\x00bad"} {
		if _, err := normalizeRAGUploadPath(name); err == nil {
			t.Fatalf("accepted path %q", name)
		}
	}
	raw := ragTestZIP(t, []string{"../bad.txt", "/bad.txt", "symlink.txt", "folder/ok.txt", "image.png"},
		[][]byte{[]byte("bad"), []byte("bad"), []byte("target"), []byte("valid"), []byte("image")})
	var size int64
	count := 0
	entries, issues, err := expandRAGArchive("bundle.zip", raw, 0, &size, &count)
	if err != nil || len(entries) != 1 || len(issues) != 4 || entries[0].Name != "bundle.zip/folder/ok.txt" {
		t.Fatalf("entries=%v issues=%v err=%v", entries, issues, err)
	}
	count = maxRAGUploadFiles
	if _, _, err := expandRAGArchive("bundle.zip", raw, 0, &size, &count); err == nil {
		t.Fatal("file count limit ignored")
	}
	count, size = 0, maxRAGExpandedBytes
	if _, _, err := expandRAGArchive("bundle.zip", raw, 0, &size, &count); err == nil {
		t.Fatal("expansion limit ignored")
	}
	nested := ragTestZIP(t, []string{"inner.zip"}, [][]byte{raw})
	count, size = 0, 0
	entries, _, err = expandRAGArchive("outer.zip", nested, 0, &size, &count)
	if err != nil || len(entries) != 1 {
		t.Fatalf("two levels should work: %v", err)
	}
	count, size = 0, 0
	third := ragTestZIP(t, []string{"middle.zip"}, [][]byte{nested})
	entries, issues, err = expandRAGArchive("outer.zip", third, 0, &size, &count)
	if err != nil || len(entries) != 0 || len(issues) == 0 {
		t.Fatalf("third level must be rejected: %v %v", issues, err)
	}
}

func TestRAGUploadDatabaseRoundTrip(t *testing.T) {
	dsn, _, err := db.DSN()
	if err != nil {
		t.Skip("integration test requires isolated YUNKUN_PG_DSN")
	}
	pg, err := db.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pg.Close()
	kb, err := pg.CreateRAGKnowledgeBase("upload-test", "", "private", fmt.Sprintf("upload-%d", time.Now().UnixNano()), "", "", "", 900, 120)
	if err != nil {
		t.Fatal(err)
	}
	defer pg.Exec("DELETE FROM rag_knowledge_bases WHERE id=$1", kb.ID)
	s := &Server{m: &Manager{pg: pg}}
	docx := ragTestZIP(t, []string{"word/document.xml"}, [][]byte{[]byte(`<w:document xmlns:w="urn:w"><w:body><w:p><w:r><w:t>上传认证 fixture</w:t></w:r></w:p></w:body></w:document>`)})
	bundle := ragTestZIP(t, []string{"report.docx", "notes.md", "old.doc"}, [][]byte{docx, []byte("# Upload\n\nretrieval fixture"), {0xb5, 0, 1}})
	upload := func(names []string, contents [][]byte) *httptest.ResponseRecorder {
		t.Helper()
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		for i, name := range names {
			part, err := writer.CreateFormFile("file", name)
			if err != nil {
				t.Fatal(err)
			}
			_, _ = io.Copy(part, bytes.NewReader(contents[i]))
			_ = writer.WriteField("file_path", "folder/"+name)
		}
		_ = writer.Close()
		req := httptest.NewRequest("POST", "/upload?batch=1", &body)
		req.SetPathValue("id", strconv.FormatInt(kb.ID, 10))
		req.Header.Set("Content-Type", writer.FormDataContentType())
		rec := httptest.NewRecorder()
		s.uploadRAGDocument(rec, req)
		return rec
	}
	rec := upload([]string{"bundle.zip", "gbk.txt"}, [][]byte{bundle, {0xb5, 0xc7, 0xc2, 0xbc}})
	if rec.Code != 202 {
		t.Fatal(rec.Code, rec.Body)
	}
	var result struct {
		Uploaded  int               `json:"uploaded"`
		Skipped   int               `json:"skipped"`
		Documents []*db.RAGDocument `json:"documents"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Uploaded != 3 || result.Skipped != 1 {
		t.Fatalf("unexpected batch: %s", rec.Body)
	}
	for _, item := range result.Documents {
		deadline := time.Now().Add(15 * time.Second)
		for {
			doc, err := pg.GetRAGDocument(item.ID)
			if err != nil {
				t.Fatal(err)
			}
			if doc.Status == "failed" {
				t.Fatal(doc.Error)
			}
			if doc.Status == "ready" {
				var active int
				if err := pg.QueryRow("SELECT count(*) FROM rag_ingestion_jobs WHERE document_id=$1 AND status IN ('queued','running')", doc.ID).Scan(&active); err != nil {
					t.Fatal(err)
				}
				if active > 0 && time.Now().Before(deadline) {
					time.Sleep(20 * time.Millisecond)
					continue
				}
				if doc.ChunkCount == 0 || strings.ContainsRune(doc.Content, 0) {
					t.Fatalf("invalid ready document: %#v", doc)
				}
				if strings.HasSuffix(doc.Name, ".docx") && (doc.MIMEType != rag.ExtractedTextMIME || !strings.Contains(doc.Content, "上传认证")) {
					t.Fatalf("binary not extracted: %#v", doc)
				}
				if !strings.HasPrefix(doc.SourcePath, "upload/folder/") {
					t.Fatal(doc.SourcePath)
				}
				job, _, err := pg.QueueRAGDocumentJob(doc.ID, "document_reparse")
				if err != nil {
					t.Fatal(err)
				}
				s.ingestRAGDocument(doc.ID, job)
				doc, _ = pg.GetRAGDocument(doc.ID)
				if doc.Status != "ready" {
					t.Fatal("reparse failed", doc.Error)
				}
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("ingestion timeout", doc.Status)
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	rec = upload([]string{"bundle.zip"}, [][]byte{bundle})
	if rec.Code != 400 || !strings.Contains(rec.Body.String(), "相同文档") {
		t.Fatal("duplicate response", rec.Code, rec.Body)
	}
	found, err := pg.SearchRAGChunks(kb.ID, "retrieval", 5)
	if err != nil || len(found) == 0 {
		t.Fatal("uploaded text not searchable", err)
	}
	if !strings.Contains(found[0].SourceURI, "folder/bundle.zip/notes.md") {
		t.Fatal("retrieval lost original folder path", found[0].SourceURI)
	}
}
