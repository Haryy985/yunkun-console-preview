package server

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/klauspost/compress/zstd"
	"golang.org/x/text/encoding/simplifiedchinese"
)

func TestValidSkillName(t *testing.T) {
	ok := []string{"web-recon", "a", "nuclei2", "中文技能", "端口扫描-x", "日本語スキル"}
	bad := []string{
		"", "Web-Recon", "-lead", "trail-", "dou--ble", "has space", "中文 技能",
		"dot.name", "a/b", `a\b`, "..", ".", "中文/技能", "sk\x00ill", "中‮文",
		strings.Repeat("a", 65), "1abc", "中文技能!",
	}
	for _, n := range ok {
		if !validSkillName(n) {
			t.Errorf("validSkillName(%q) = false, want true", n)
		}
	}
	for _, n := range bad {
		if validSkillName(n) {
			t.Errorf("validSkillName(%q) = true, want false", n)
		}
	}
}

func TestSkillRelPath(t *testing.T) {
	ok := map[string]string{
		"SKILL.md":           "SKILL.md",
		"scripts/run.py":     "scripts/run.py",
		"参考/中文说明.md":         "参考/中文说明.md",
		"references/a b.txt": "references/a b.txt",
		"./SKILL.md":         "SKILL.md",
		"assets/图片-1_v2.png": "assets/图片-1_v2.png",
	}
	for in, want := range ok {
		got, msg := skillRelPath(in)
		if msg != "" || got != want {
			t.Errorf("skillRelPath(%q) = (%q, %q), want (%q, \"\")", in, got, msg, want)
		}
	}
	bad := []string{
		"", "../etc/passwd", "a/../../b", "/abs/path", "a//b", `..\..\x`,
		"%2e%2e/x", "a\x00b", "中文 名.md", "中‮文.md", "a#b.md", "a?b.md",
		"a:b.md", string([]byte{0xd6, 0xd0}) + ".md", // 裸 GBK 字节：非法 UTF-8
		strings.Repeat("a", maxSkillPathLen+1),
	}
	for _, in := range bad {
		if got, msg := skillRelPath(in); msg == "" {
			t.Errorf("skillRelPath(%q) = (%q, \"\"), want rejection", in, got)
		}
	}
}

func TestSkillFilePathRejectsSymlinkComponents(t *testing.T) {
	root := t.TempDir()
	skill := filepath.Join(root, "sample")
	outside := t.TempDir()
	if err := os.MkdirAll(skill, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(skill, "linked")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := skillFilePath(root, "sample", "linked/secret.txt"); err == nil {
		t.Fatal("path traversed a symlinked directory")
	}
	if _, err := skillFilePath(root, "sample", "new/file.txt"); err != nil {
		t.Fatalf("normal new file rejected: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "alias")); err != nil {
		t.Fatal(err)
	}
	if _, err := skillFilePath(root, "alias", "SKILL.md"); err == nil {
		t.Fatal("symlinked skill root was accepted")
	}
}

func TestSkillFileEndpointsRejectSymlinkRoot(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "SKILL.md"), []byte("private"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "alias")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	s := &Server{skillDir: root}
	for _, tc := range []struct {
		method, path string
		handle       func(http.ResponseWriter, *http.Request)
	}{
		{http.MethodGet, "/api/skills/alias/files", s.fsListFiles},
		{http.MethodPut, "/api/skills/alias/meta", s.fsUpdateSkillMeta},
		{http.MethodGet, "/api/skills/alias/files/SKILL.md", s.fsReadFile},
	} {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(`{}`))
		req.SetPathValue("name", "alias")
		req.SetPathValue("file", "SKILL.md")
		rr := httptest.NewRecorder()
		tc.handle(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("%s %s: status=%d body=%s", tc.method, tc.path, rr.Code, rr.Body.String())
		}
	}
}

func TestCreateSkillDoesNotOverwriteExistingDirectory(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "demo")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	original := []byte("original skill")
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), original, 0o644); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/skills", strings.NewReader(`{"name":"demo","description":"replacement"}`))
	rr := httptest.NewRecorder()
	(&Server{skillDir: root}).fsCreateSkill(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	got, err := os.ReadFile(filepath.Join(dir, "SKILL.md"))
	if err != nil || !bytes.Equal(got, original) {
		t.Fatalf("existing skill changed: %q, %v", got, err)
	}
}

func TestUploadSkillOverwritePreservesExistingOnInvalidArchive(t *testing.T) {
	root := t.TempDir()
	old := filepath.Join(root, "demo")
	if err := os.Mkdir(old, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(old, "SKILL.md"), []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	data := buildZip(t,
		zipFile{name: "SKILL.md", body: "---\nname: demo\n---\n"},
		zipFile{name: "../escape", body: "bad"},
	)
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	f, err := mw.CreateFormFile("file", "demo.zip")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/skills/upload?overwrite=true", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rr := httptest.NewRecorder()
	(&Server{skillDir: root}).fsUploadSkill(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	got, err := os.ReadFile(filepath.Join(old, "SKILL.md"))
	if err != nil || string(got) != "original" {
		t.Fatalf("existing skill changed: %q, %v", got, err)
	}
}

type nopWriteCloser struct{ io.Writer }

func (nopWriteCloser) Close() error { return nil }

// zipFile describes one entry for buildZip.
type zipFile struct {
	name    string
	body    string
	method  uint16
	nonUTF8 bool // write the name bytes as-is (GBK 包)
	mode    os.FileMode
}

func buildZip(t *testing.T, files ...zipFile) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	zw.RegisterCompressor(zipMethodZstd, zstd.ZipCompressor())
	// Deflate64 没有纯 Go 编码器；这里原样写入，只是为了给条目打上 method 9 的标记 ——
	// 断言的是「方法不支持时给什么提示」，不会真去解压它。
	zw.RegisterCompressor(zipMethodDeflate64, func(w io.Writer) (io.WriteCloser, error) {
		return nopWriteCloser{w}, nil
	})
	for _, f := range files {
		h := &zip.FileHeader{Name: f.name, Method: f.method, NonUTF8: f.nonUTF8}
		if f.mode != 0 {
			h.SetMode(f.mode)
		}
		w, err := zw.CreateHeader(h)
		if err != nil {
			t.Fatalf("CreateHeader(%q): %v", f.name, err)
		}
		if _, err := io.WriteString(w, f.body); err != nil {
			t.Fatalf("write %q: %v", f.name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	return buf.Bytes()
}

// uploadZip posts raw zip bytes to fsUploadSkill and returns the response.
func uploadZip(t *testing.T, skillDir string, filename string, data []byte) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, err := mw.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write(data); err != nil {
		t.Fatal(err)
	}
	mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/skills/upload", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rr := httptest.NewRecorder()
	(&Server{skillDir: skillDir}).fsUploadSkill(rr, req)

	var out map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &out)
	return rr, out
}

const zhSkillMD = "---\nname: 中文技能\ndescription: 测试\n---\n正文\n"

// A zstd-compressed archive (WinZip 的可选压缩方式) used to blow up with
// "zip: unsupported compression"; it now installs like any Deflate archive.
func TestUploadSkillZstdAndChineseNames(t *testing.T) {
	dir := t.TempDir()
	data := buildZip(t,
		zipFile{name: "中文技能/SKILL.md", body: zhSkillMD, method: zipMethodZstd},
		zipFile{name: "中文技能/参考/说明 文档.md", body: "参考", method: zipMethodZstd},
		zipFile{name: "中文技能/scripts/run.py", body: "print(1)", method: zip.Deflate},
	)
	rr, out := uploadZip(t, dir, "中文技能.zip", data)
	if rr.Code != 201 {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body)
	}
	if out["name"] != "中文技能" {
		t.Fatalf("name = %v, want 中文技能", out["name"])
	}
	for _, rel := range []string{"SKILL.md", "参考/说明 文档.md", "scripts/run.py"} {
		if _, err := os.Stat(filepath.Join(dir, "中文技能", rel)); err != nil {
			t.Errorf("missing extracted file %q: %v", rel, err)
		}
	}
}

// GBK-named entries (7-Zip / 资源管理器 on Chinese Windows) must be decoded rather
// than rejected as invalid UTF-8 paths.
func TestUploadSkillGBKNames(t *testing.T) {
	gbk := func(s string) string {
		b, err := simplifiedchinese.GBK.NewEncoder().String(s)
		if err != nil {
			t.Fatalf("gbk encode %q: %v", s, err)
		}
		return b
	}
	dir := t.TempDir()
	data := buildZip(t,
		zipFile{name: gbk("中文技能/SKILL.md"), body: zhSkillMD, method: zip.Deflate, nonUTF8: true},
		zipFile{name: gbk("中文技能/参考资料.md"), body: "内容", method: zip.Deflate, nonUTF8: true},
	)
	rr, out := uploadZip(t, dir, "skill.zip", data)
	if rr.Code != 201 {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body)
	}
	if out["name"] != "中文技能" {
		t.Fatalf("name = %v, want 中文技能", out["name"])
	}
	if _, err := os.Stat(filepath.Join(dir, "中文技能", "参考资料.md")); err != nil {
		t.Errorf("GBK-named entry not extracted: %v", err)
	}
}

// An archive we genuinely cannot decode should name the method in Chinese instead of
// surfacing "zip: unsupported compression algorithm".
func TestUploadSkillUnsupportedMethod(t *testing.T) {
	data := buildZip(t,
		zipFile{name: "demo/SKILL.md", body: "---\nname: demo\n---\n", method: zipMethodDeflate64},
	)
	rr, out := uploadZip(t, t.TempDir(), "demo.zip", data)
	if rr.Code != 400 {
		t.Fatalf("status = %d, want 400 (body %s)", rr.Code, rr.Body)
	}
	msg, _ := out["error"].(string)
	if !strings.Contains(msg, "Deflate64") || !strings.Contains(msg, "不支持的压缩方式") {
		t.Fatalf("error = %q, want a Chinese message naming Deflate64", msg)
	}
}

func TestUploadSkillEncrypted(t *testing.T) {
	data := buildZip(t, zipFile{name: "demo/SKILL.md", body: "---\nname: demo\n---\n", method: zip.Deflate})
	// flip the "encrypted" general-purpose flag bit in the local file header
	// (offset 6) and in the central directory copy (offset 8).
	local := bytes.Index(data, []byte("PK\x03\x04"))
	central := bytes.Index(data, []byte("PK\x01\x02"))
	if local < 0 || central < 0 {
		t.Fatal("could not locate zip headers")
	}
	data[local+6] |= 1
	data[central+8] |= 1

	rr, out := uploadZip(t, t.TempDir(), "demo.zip", data)
	if rr.Code != 400 {
		t.Fatalf("status = %d, want 400 (body %s)", rr.Code, rr.Body)
	}
	if msg, _ := out["error"].(string); !strings.Contains(msg, "已加密") {
		t.Fatalf("error = %q, want 加密 hint", msg)
	}
}

// Zip-slip must still be refused now that the path check accepts Unicode.
func TestUploadSkillRejectsTraversal(t *testing.T) {
	data := buildZip(t,
		zipFile{name: "demo/SKILL.md", body: "---\nname: demo\n---\n", method: zip.Deflate},
		zipFile{name: "demo/../../evil.sh", body: "rm -rf /", method: zip.Deflate},
	)
	dir := t.TempDir()
	rr, out := uploadZip(t, dir, "demo.zip", data)
	if rr.Code != 400 {
		t.Fatalf("status = %d, want 400 (body %s)", rr.Code, rr.Body)
	}
	if msg, _ := out["error"].(string); !strings.Contains(msg, "非法路径") {
		t.Fatalf("error = %q, want 非法路径", msg)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("upload left files behind: %v", entries)
	}
}

func TestUploadSkillWindowsPaths(t *testing.T) {
	t.Setenv("GODEBUG", "zipinsecurepath=0")
	for _, tc := range []struct {
		name string
		root string
		file string
	}{
		{"root", "SKILL.md", `agents\openai.yaml`},
		{"wrapped", `demo\SKILL.md`, `demo\agents\openai.yaml`},
		{"mixed", `demo\SKILL.md`, `demo/agents\openai.yaml`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			data := buildZip(t,
				zipFile{name: tc.root, body: "---\nname: demo\n---\n", method: zip.Deflate},
				zipFile{name: tc.file, body: "display_name: Demo\n", method: zip.Deflate},
			)
			rr, _ := uploadZip(t, dir, "demo.zip", data)
			if rr.Code != http.StatusCreated {
				t.Fatalf("status = %d, body = %s", rr.Code, rr.Body)
			}
			got, err := os.ReadFile(filepath.Join(dir, "demo", "agents", "openai.yaml"))
			if err != nil || string(got) != "display_name: Demo\n" {
				t.Fatalf("extracted YAML = %q, error = %v", got, err)
			}
		})
	}
}

func TestUploadSkillWindowsGBKPaths(t *testing.T) {
	dir := t.TempDir()
	files := []zipFile{
		{name: `中文技能\SKILL.md`, body: zhSkillMD, method: zip.Deflate, nonUTF8: true},
		{name: `中文技能\参考\说明.md`, body: "内容", method: zip.Deflate, nonUTF8: true},
	}
	for i := range files {
		encoded, err := simplifiedchinese.GBK.NewEncoder().String(files[i].name)
		if err != nil {
			t.Fatal(err)
		}
		files[i].name = encoded
	}
	rr, _ := uploadZip(t, dir, "demo.zip", buildZip(t, files...))
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body)
	}
	got, err := os.ReadFile(filepath.Join(dir, "中文技能", "参考", "说明.md"))
	if err != nil || string(got) != "内容" {
		t.Fatalf("extracted content = %q, error = %v", got, err)
	}
}

func TestUploadSkillRejectsUnsafeWindowsPaths(t *testing.T) {
	t.Setenv("GODEBUG", "zipinsecurepath=0")
	for _, name := range []string{
		`..\evil.txt`, `demo\..\evil.txt`, `demo\..\..\evil.txt`,
		`C:\demo\SKILL.md`, `\\server\demo\SKILL.md`,
		`/abs/demo/SKILL.md`, `\abs\demo\SKILL.md`,
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			data := buildZip(t,
				zipFile{name: "demo/SKILL.md", body: "---\nname: demo\n---\n"},
				zipFile{name: name, body: "---\nname: demo\n---\n"},
			)
			rr, out := uploadZip(t, dir, "demo.zip", data)
			if rr.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, body = %s", rr.Code, rr.Body)
			}
			if msg, _ := out["error"].(string); !strings.Contains(msg, "非法路径") {
				t.Fatalf("error = %q, want path rejection", msg)
			}
			entries, err := os.ReadDir(dir)
			if err != nil || len(entries) != 0 {
				t.Fatalf("upload left files: %v, error = %v", entries, err)
			}
		})
	}
}

func TestUploadSkillRejectsNormalizedDuplicates(t *testing.T) {
	for _, duplicate := range []string{"agents/openai.yaml", `Agents\OpenAI.yaml`} {
		t.Run(duplicate, func(t *testing.T) {
			dir := t.TempDir()
			data := buildZip(t,
				zipFile{name: "SKILL.md", body: "---\nname: demo\n---\n"},
				zipFile{name: `agents\openai.yaml`, body: "first"},
				zipFile{name: duplicate, body: "second"},
			)
			rr, out := uploadZip(t, dir, "demo.zip", data)
			if rr.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, body = %s", rr.Code, rr.Body)
			}
			if msg, _ := out["error"].(string); !strings.Contains(msg, "重复路径") {
				t.Fatalf("error = %q, want duplicate rejection", msg)
			}
			entries, err := os.ReadDir(dir)
			if err != nil || len(entries) != 0 {
				t.Fatalf("upload left files: %v, error = %v", entries, err)
			}
		})
	}
}

func TestUploadSkillRejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	data := buildZip(t,
		zipFile{name: "SKILL.md", body: "---\nname: demo\n---\n"},
		zipFile{name: `agents\openai.yaml`, body: "../../outside", mode: os.ModeSymlink | 0o777},
	)
	rr, out := uploadZip(t, dir, "demo.zip", data)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body)
	}
	if msg, _ := out["error"].(string); !strings.Contains(msg, "符号链接") {
		t.Fatalf("error = %q, want symlink rejection", msg)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 0 {
		t.Fatalf("upload left files: %v, error = %v", entries, err)
	}
}

func TestSkillNameFromFrontmatterQuoted(t *testing.T) {
	got := skillNameFromFrontmatter([]byte("---\nname: \"中文技能\"\ndescription: x\n---\n"))
	if got != "中文技能" {
		t.Fatalf("name = %q, want 中文技能", got)
	}
}
