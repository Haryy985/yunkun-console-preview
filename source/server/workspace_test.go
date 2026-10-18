package server

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkspaceResolveRejectsLinkedPath(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	s := &Server{m: &Manager{dir: root}}
	if err := os.Symlink(outside, filepath.Join(root, "linked")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	for _, path := range []string{"linked", "linked/secret.txt", "linked/new/secret.txt"} {
		if resolved, ok := s.wsResolve(path); ok {
			t.Errorf("linked path %q resolved to %q", path, resolved)
		}
	}
	if err := os.Mkdir(filepath.Join(root, "normal"), 0o755); err != nil {
		t.Fatal(err)
	}
	if resolved, ok := s.wsResolve("normal/new/file.txt"); !ok || resolved != filepath.Join(root, "normal", "new", "file.txt") {
		t.Fatalf("normal workspace path rejected: %q, %v", resolved, ok)
	}
}

func TestWorkspaceRootedFileOperations(t *testing.T) {
	root := t.TempDir()
	s := &Server{m: &Manager{dir: root}}
	call := func(method, path, body string, handler http.HandlerFunc) *httptest.ResponseRecorder {
		t.Helper()
		w := httptest.NewRecorder()
		handler(w, httptest.NewRequest(method, path, strings.NewReader(body)))
		return w
	}
	if w := call(http.MethodPost, "/api/workspace/mkdir", `{"path":"docs"}`, s.wsMkdir); w.Code != 200 {
		t.Fatalf("mkdir: %d %s", w.Code, w.Body.String())
	}
	if w := call(http.MethodPost, "/api/workspace/write", `{"path":"docs/note.txt","content":"first"}`, s.wsWrite); w.Code != 200 {
		t.Fatalf("write: %d %s", w.Code, w.Body.String())
	}
	if w := call(http.MethodPost, "/api/workspace/write", `{"path":"docs/note.txt","content":"updated"}`, s.wsWrite); w.Code != 200 {
		t.Fatalf("overwrite: %d %s", w.Code, w.Body.String())
	}
	if w := call(http.MethodGet, "/api/workspace/read?path=docs/note.txt", "", s.wsRead); w.Code != 200 || !strings.Contains(w.Body.String(), "updated") {
		t.Fatalf("read: %d %s", w.Code, w.Body.String())
	}
	if w := call(http.MethodGet, "/api/workspace/download?path=docs/note.txt", "", s.wsDownload); w.Code != 200 || w.Body.String() != "updated" {
		t.Fatalf("download: %d %s", w.Code, w.Body.String())
	}
	if w := call(http.MethodGet, "/api/workspace/list?path=docs", "", s.wsList); w.Code != 200 || !strings.Contains(w.Body.String(), "note.txt") {
		t.Fatalf("list: %d %s", w.Code, w.Body.String())
	}
	if w := call(http.MethodDelete, "/api/workspace/delete?path=docs/note.txt", "", s.wsDelete); w.Code != 200 {
		t.Fatalf("delete: %d %s", w.Code, w.Body.String())
	}
	if _, err := os.Stat(filepath.Join(root, "docs", "note.txt")); !os.IsNotExist(err) {
		t.Fatalf("delete did not remove file: %v", err)
	}
}

func TestWorkspaceUploadRejectsLinkedDestination(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	s := &Server{m: &Manager{dir: root}}
	if err := os.Symlink(outside, filepath.Join(root, "linked")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	part, err := form.CreateFormFile("file", "secret.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte("secret")); err != nil {
		t.Fatal(err)
	}
	if err := form.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/workspace/upload?path=linked", &body)
	req.Header.Set("Content-Type", form.FormDataContentType())
	w := httptest.NewRecorder()
	s.wsUpload(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("linked upload status=%d body=%s", w.Code, w.Body.String())
	}
	if _, err := os.Stat(filepath.Join(outside, "secret.txt")); !os.IsNotExist(err) {
		t.Fatalf("upload escaped workspace: %v", err)
	}
}

func TestWorkspaceUploadSavesFileWithinRoot(t *testing.T) {
	root := t.TempDir()
	s := &Server{m: &Manager{dir: root}}
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	part, err := form.CreateFormFile("file", "note.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte("uploaded")); err != nil {
		t.Fatal(err)
	}
	if err := form.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/workspace/upload", &body)
	req.Header.Set("Content-Type", form.FormDataContentType())
	w := httptest.NewRecorder()
	s.wsUpload(w, req)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"uploaded":1`) {
		t.Fatalf("upload: %d %s", w.Code, w.Body.String())
	}
	got, err := os.ReadFile(filepath.Join(root, "note.txt"))
	if err != nil || string(got) != "uploaded" {
		t.Fatalf("saved content=%q err=%v", got, err)
	}
}

func TestChatUploadDoesNotOverwriteExistingAttachment(t *testing.T) {
	dir := t.TempDir()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err := os.WriteFile(filepath.Join(dir, "note.txt"), []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	part, err := form.CreateFormFile("file", "note.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte("new")); err != nil {
		t.Fatal(err)
	}
	if err := form.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/", &body)
	req.Header.Set("Content-Type", form.FormDataContentType())
	if err := req.ParseMultipartForm(1 << 20); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = req.MultipartForm.RemoveAll() })
	name, err := saveUniqueChatUpload(root, "note.txt", req.MultipartForm.File["file"][0])
	if err != nil || name != "note-1.txt" {
		t.Fatalf("new attachment=%q err=%v", name, err)
	}
	for file, want := range map[string]string{"note.txt": "original", "note-1.txt": "new"} {
		got, err := os.ReadFile(filepath.Join(dir, file))
		if err != nil || string(got) != want {
			t.Errorf("%s=%q err=%v, want %q", file, got, err, want)
		}
	}
}
