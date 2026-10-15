package server

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"mime/multipart"
	"net/http"
	"path"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"yunkun/console/db"
	"yunkun/console/rag"
)

var ragUploadSlots = make(chan struct{}, 2)

const (
	maxRAGUploadBody     = 264 << 20
	maxRAGUploadFile     = 32 << 20
	maxRAGExpandedBytes  = 256 << 20
	maxRAGUploadFiles    = 500
	maxRAGArchiveDepth   = 2
	ragUploadMemoryLimit = 8 << 20
)

type ragUploadEntry struct {
	Name     string
	MIMEType string
	Content  []byte
}

type ragUploadIssue struct {
	Name  string `json:"name"`
	Error string `json:"error"`
}

func (s *Server) uploadRAGDocument(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	knowledgeBaseID, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad knowledge base id")
		return
	}
	base, err := pg.GetRAGKnowledgeBase(knowledgeBaseID)
	if err != nil {
		writeRAGKnowledgeBaseError(w, err)
		return
	}
	if base == nil {
		writeRAGKnowledgeBaseError(w, db.ErrRAGKnowledgeBaseNotFound)
		return
	}
	select {
	case ragUploadSlots <- struct{}{}:
		defer func() { <-ragUploadSlots }()
	default:
		writeErr(w, http.StatusTooManyRequests, "正在处理其他上传，请稍后重试")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxRAGUploadBody)
	if err := r.ParseMultipartForm(ragUploadMemoryLimit); err != nil {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
		var sizeErr *http.MaxBytesError
		if errors.As(err, &sizeErr) {
			writeErr(w, http.StatusRequestEntityTooLarge, "上传请求过大，文件总大小不能超过 256 MiB")
			return
		}
		writeErr(w, http.StatusBadRequest, "请使用 multipart/form-data 上传文档")
		return
	}
	defer r.MultipartForm.RemoveAll()
	headers := r.MultipartForm.File["file"]
	if len(headers) == 0 {
		writeErr(w, http.StatusBadRequest, "缺少 file 文档字段")
		return
	}

	entries, issues, err := collectRAGUploadEntries(headers, r.MultipartForm.Value["file_path"])
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	documents := make([]*db.RAGDocument, 0, len(entries))
	var jobs [][2]int64
	defer func() {
		if len(jobs) > 0 {
			go func() {
				for _, job := range jobs {
					s.ingestRAGDocument(job[0], job[1])
				}
			}()
		}
	}()
	for _, entry := range entries {
		if r.Context().Err() != nil {
			return
		}
		text, storedMIME, extractErr := rag.PrepareDocument(entry.Name, entry.MIMEType, entry.Content)
		if extractErr != nil {
			issues = append(issues, ragUploadIssue{Name: entry.Name, Error: extractErr.Error()})
			continue
		}
		sum := sha256.Sum256(entry.Content)
		hash := hex.EncodeToString(sum[:])
		sourceType := strings.TrimPrefix(strings.ToLower(filepath.Ext(entry.Name)), ".")
		if sourceType == "" {
			sourceType = "text"
		}
		item, jobID, createErr := pg.CreateRAGDocument(
			knowledgeBaseID,
			path.Base(entry.Name),
			sourceType,
			"upload/"+entry.Name,
			storedMIME,
			hash,
			hash,
			text,
		)
		if errors.Is(createErr, db.ErrRAGDocumentDuplicate) {
			issues = append(issues, ragUploadIssue{Name: entry.Name, Error: "相同文档已存在"})
			continue
		}
		if createErr != nil {
			if len(documents) == 0 {
				writeRAGKnowledgeBaseError(w, createErr)
				return
			}
			issues = append(issues, ragUploadIssue{Name: entry.Name, Error: createErr.Error()})
			continue
		}
		documents = append(documents, item)
		jobs = append(jobs, [2]int64{item.ID, jobID})
	}

	if len(documents) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error":     "没有可导入的文档，请查看各文件的失败原因",
			"uploaded":  0,
			"skipped":   len(issues),
			"documents": []*db.RAGDocument{},
			"errors":    issues,
		})
		return
	}
	response := map[string]any{
		"uploaded":  len(documents),
		"skipped":   len(issues),
		"documents": documents,
		"errors":    issues,
	}
	if len(documents) == 1 {
		response["document"] = documents[0]
		if r.URL.Query().Get("batch") != "1" && len(headers) == 1 && len(issues) == 0 && !isRAGArchive(headers[0].Filename) {
			writeJSON(w, http.StatusAccepted, documents[0])
			return
		}
	}
	writeJSON(w, http.StatusAccepted, response)
}

func collectRAGUploadEntries(headers []*multipart.FileHeader, paths []string) ([]ragUploadEntry, []ragUploadIssue, error) {
	var entries []ragUploadEntry
	issues := make([]ragUploadIssue, 0)
	var expandedBytes int64
	var uploadedBytes int64
	seenFiles := 0
	if len(headers) > maxRAGUploadFiles || (len(paths) != 0 && len(paths) != len(headers)) {
		return nil, nil, fmt.Errorf("文件数不能超过 500，且相对路径必须与文件一一对应")
	}
	for index, header := range headers {
		if header == nil {
			continue
		}
		seenFiles++
		if seenFiles > maxRAGUploadFiles {
			return nil, nil, fmt.Errorf("单次最多上传 %d 个文件", maxRAGUploadFiles)
		}
		file, err := header.Open()
		if err != nil {
			issues = append(issues, ragUploadIssue{Name: header.Filename, Error: "打开上传文件失败: " + err.Error()})
			continue
		}
		rawName := header.Filename
		if index < len(paths) && strings.TrimSpace(paths[index]) != "" {
			rawName = paths[index]
		}
		limit := int64(maxRAGUploadFile)
		if isRAGArchive(rawName) {
			limit = maxRAGExpandedBytes
		}
		content, readErr := io.ReadAll(io.LimitReader(file, limit+1))
		closeErr := file.Close()
		if readErr != nil {
			issues = append(issues, ragUploadIssue{Name: header.Filename, Error: "读取上传文件失败: " + readErr.Error()})
			continue
		}
		if closeErr != nil {
			issues = append(issues, ragUploadIssue{Name: header.Filename, Error: "关闭上传文件失败: " + closeErr.Error()})
			continue
		}
		if int64(len(content)) > limit {
			issues = append(issues, ragUploadIssue{Name: header.Filename, Error: fmt.Sprintf("文件不能超过 %d MiB", limit>>20)})
			continue
		}
		uploadedBytes += int64(len(content))
		if uploadedBytes > maxRAGExpandedBytes {
			return nil, nil, fmt.Errorf("上传文件总大小不能超过 256 MiB")
		}
		name, err := normalizeRAGUploadPath(rawName)
		if err != nil {
			issues = append(issues, ragUploadIssue{Name: rawName, Error: err.Error()})
			continue
		}
		mimeType := header.Header.Get("Content-Type")
		if mimeType == "" || mimeType == "application/octet-stream" {
			mimeType = mime.TypeByExtension(filepath.Ext(name))
		}
		if mimeType == "" {
			mimeType = "application/octet-stream"
		}
		if isRAGArchive(name) {
			archiveEntries, archiveIssues, archiveErr := expandRAGArchive(name, content, 0, &expandedBytes, &seenFiles)
			issues = append(issues, archiveIssues...)
			if archiveErr != nil {
				if errors.Is(archiveErr, errRAGArchiveLimit) {
					return nil, nil, archiveErr
				}
				issues = append(issues, ragUploadIssue{Name: name, Error: archiveErr.Error()})
				continue
			}
			if len(archiveEntries) == 0 && len(archiveIssues) == 0 {
				issues = append(issues, ragUploadIssue{Name: name, Error: "ZIP 内没有可导入的文档"})
			}
			entries = append(entries, archiveEntries...)
			continue
		}
		if !isRAGSupportedDocument(name) {
			issues = append(issues, ragUploadIssue{Name: name, Error: unsupportedRAGFormat(name)})
			continue
		}
		if expandedBytes+int64(len(content)) > maxRAGExpandedBytes {
			return nil, nil, fmt.Errorf("展开后的上传内容不能超过 256 MiB")
		}
		expandedBytes += int64(len(content))
		entries = append(entries, ragUploadEntry{Name: name, MIMEType: mimeType, Content: content})
	}
	return entries, issues, nil
}

var errRAGArchiveLimit = errors.New("压缩包展开超过总大小或文件数量限制")

func expandRAGArchive(name string, content []byte, depth int, expandedBytes *int64, seenFiles *int) ([]ragUploadEntry, []ragUploadIssue, error) {
	if depth >= maxRAGArchiveDepth {
		return nil, nil, fmt.Errorf("压缩包嵌套层数不能超过 %d 层", maxRAGArchiveDepth)
	}
	reader, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
	if err != nil {
		return nil, nil, fmt.Errorf("读取 ZIP「%s」失败: %w", name, err)
	}
	var entries []ragUploadEntry
	var issues []ragUploadIssue
	for _, file := range reader.File {
		if file.FileInfo().IsDir() {
			continue
		}
		(*seenFiles)++
		if *seenFiles > maxRAGUploadFiles {
			return nil, nil, errRAGArchiveLimit
		}
		if file.Mode()&fs.ModeSymlink != 0 {
			issues = append(issues, ragUploadIssue{Name: file.Name, Error: "压缩包内的符号链接已拒绝"})
			continue
		}
		memberName := file.Name
		if !utf8.ValidString(memberName) {
			issues = append(issues, ragUploadIssue{Name: name, Error: "ZIP 文件名不是 UTF-8，请重新压缩"})
			continue
		}
		memberName, pathErr := normalizeRAGUploadPath(memberName)
		if pathErr != nil {
			issues = append(issues, ragUploadIssue{Name: file.Name, Error: "压缩包路径无效: " + pathErr.Error()})
			continue
		}
		childName := path.Join(name, memberName)
		if file.UncompressedSize64 > maxRAGUploadFile {
			issues = append(issues, ragUploadIssue{Name: childName, Error: "压缩包内单个文件不能超过 32 MiB"})
			continue
		}
		stream, openErr := file.Open()
		if openErr != nil {
			issues = append(issues, ragUploadIssue{Name: childName, Error: "打开压缩包文件失败: " + openErr.Error()})
			continue
		}
		child, readErr := io.ReadAll(io.LimitReader(stream, maxRAGUploadFile+1))
		closeErr := stream.Close()
		if readErr != nil || closeErr != nil {
			if readErr == nil {
				readErr = closeErr
			}
			issues = append(issues, ragUploadIssue{Name: childName, Error: "读取压缩包文件失败: " + readErr.Error()})
			continue
		}
		if int64(len(child)) > maxRAGUploadFile {
			issues = append(issues, ragUploadIssue{Name: childName, Error: "压缩包内单个文件不能超过 32 MiB"})
			continue
		}
		if *expandedBytes+int64(len(child)) > maxRAGExpandedBytes {
			return nil, nil, errRAGArchiveLimit
		}
		*expandedBytes += int64(len(child))
		if isRAGArchive(childName) {
			nested, nestedIssues, nestedErr := expandRAGArchive(childName, child, depth+1, expandedBytes, seenFiles)
			entries = append(entries, nested...)
			issues = append(issues, nestedIssues...)
			if nestedErr != nil {
				if errors.Is(nestedErr, errRAGArchiveLimit) {
					return nil, nil, nestedErr
				}
				issues = append(issues, ragUploadIssue{Name: childName, Error: nestedErr.Error()})
			}
			continue
		}
		if !isRAGSupportedDocument(childName) {
			issues = append(issues, ragUploadIssue{Name: childName, Error: unsupportedRAGFormat(childName)})
			continue
		}
		entries = append(entries, ragUploadEntry{
			Name:     childName,
			MIMEType: mime.TypeByExtension(filepath.Ext(childName)),
			Content:  child,
		})
	}
	return entries, issues, nil
}

func normalizeRAGUploadPath(value string) (string, error) {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	if value == "" || !utf8.ValidString(value) || strings.ContainsRune(value, 0) || strings.HasPrefix(value, "/") {
		return "", fmt.Errorf("文档路径无效")
	}
	parts := strings.Split(value, "/")
	for _, part := range parts {
		if part == ".." {
			return "", fmt.Errorf("文档路径不能包含 ..")
		}
	}
	cleaned := path.Clean(value)
	if cleaned == "." || cleaned == "" || strings.HasPrefix(cleaned, "../") || strings.Contains(cleaned, ":") {
		return "", fmt.Errorf("文档路径无效")
	}
	return cleaned, nil
}

func isRAGArchive(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".zip":
		return true
	default:
		return false
	}
}

func isRAGSupportedDocument(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".md", ".markdown", ".txt", ".text", ".json", ".csv", ".html", ".htm",
		".docx", ".pdf", ".xlsx", ".pptx", ".xml", ".yaml", ".yml", ".log",
		".sql", ".ini", ".conf", ".properties", ".rst", ".go", ".py", ".js", ".ts",
		".tsx", ".jsx", ".sh":
		return true
	default:
		return false
	}
}

func unsupportedRAGFormat(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".doc", ".xls", ".ppt":
		return "不支持旧版二进制 Office 格式，请另存为 DOCX/XLSX/PPTX"
	default:
		return "不支持的文档格式"
	}
}
