package rag

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/ledongthuc/pdf"
	"github.com/xuri/excelize/v2"
	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/simplifiedchinese"
	textunicode "golang.org/x/text/encoding/unicode"
)

// Only extracted UTF-8 text, never the original binary, is stored with this MIME.
const ExtractedTextMIME = "application/x-yunkun-extracted-text"
const maxExtractedBytes = 32 << 20

// Preserve textual source structure; binary documents become Markdown sections.
func PrepareDocument(name, mimeType string, content []byte) (text, storedMIME string, err error) {
	defer func() {
		if recover() != nil {
			text, storedMIME, err = "", "", fmt.Errorf("文档损坏或包含无法解析的结构")
		}
	}()
	ext := strings.ToLower(filepath.Ext(name))
	if ext != ".pdf" && ext != ".docx" && ext != ".xlsx" && ext != ".pptx" {
		for _, magic := range [][]byte{[]byte("%PDF-"), {'P', 'K', 3, 4}, {0xd0, 0xcf, 0x11, 0xe0}} {
			if bytes.HasPrefix(content, magic) {
				return "", "", fmt.Errorf("文档扩展名与二进制内容不匹配，请使用正确的文件扩展名")
			}
		}
		text, err = decodeText(content)
		if err != nil {
			return "", "", err
		}
		if len(text) > maxExtractedBytes {
			return "", "", fmt.Errorf("解码后文本不能超过 32 MiB")
		}
		blocks, parseErr := ParseDocument(name, mimeType, []byte(text))
		if parseErr != nil {
			return "", "", parseErr
		}
		if len(blocks) == 0 {
			return "", "", fmt.Errorf("文档没有可提取的文本内容")
		}
		return text, mimeType, nil
	}
	blocks, err := ParseDocument(name, mimeType, content)
	if err != nil {
		return "", "", err
	}
	var output strings.Builder
	var previous []string
	for _, block := range blocks {
		value := cleanText(block.Content)
		if strings.TrimSpace(value) == "" {
			continue
		}
		changed := false
		for level, heading := range block.HeadingPath {
			changed = changed || level >= len(previous) || previous[level] != heading
			if changed {
				output.WriteString(strings.Repeat("#", min(level+1, 6)) + " " + cleanText(heading) + "\n\n")
			}
		}
		previous = append([]string(nil), block.HeadingPath...)
		if value != block.Heading || len(block.HeadingPath) == 0 {
			output.WriteString(value + "\n\n")
		}
		if output.Len() > maxExtractedBytes {
			return "", "", fmt.Errorf("提取文本不能超过 32 MiB")
		}
	}
	text = strings.TrimSpace(output.String())
	if text == "" {
		return "", "", fmt.Errorf("文档没有可提取的文本内容")
	}
	return text, ExtractedTextMIME, nil
}

func cleanText(value string) string {
	return strings.ReplaceAll(strings.ToValidUTF8(value, "\uFFFD"), "\x00", "")
}

func decodeText(content []byte) (string, error) {
	content = bytes.TrimPrefix(content, []byte{0xef, 0xbb, 0xbf})
	var enc encoding.Encoding
	switch {
	case bytes.HasPrefix(content, []byte{0xff, 0xfe}):
		enc, content = textunicode.UTF16(textunicode.LittleEndian, textunicode.IgnoreBOM), content[2:]
	case bytes.HasPrefix(content, []byte{0xfe, 0xff}):
		enc, content = textunicode.UTF16(textunicode.BigEndian, textunicode.IgnoreBOM), content[2:]
	case utf8.Valid(content):
		return cleanText(string(content)), nil
	default:
		enc = simplifiedchinese.GB18030
	}
	decoded, err := enc.NewDecoder().Bytes(content)
	if err != nil || bytes.Contains(decoded, []byte("\uFFFD")) {
		return "", fmt.Errorf("无法可靠识别文本编码，请转换为 UTF-8 后上传")
	}
	return cleanText(string(decoded)), nil
}

func parsePDF(content []byte) (blocks []SourceBlock, err error) {
	defer func() {
		if recover() != nil {
			blocks, err = nil, fmt.Errorf("PDF 损坏或包含无法解析的结构")
		}
	}()
	reader, err := pdf.NewReader(bytes.NewReader(content), int64(len(content)))
	if err != nil {
		return nil, fmt.Errorf("PDF 解析失败（请检查文件是否损坏或加密）: %w", err)
	}
	pages := reader.NumPage()
	if pages > 2000 {
		return nil, fmt.Errorf("PDF 页数不能超过 2000")
	}
	total := 0
	for index := 1; index <= pages; index++ {
		text, err := reader.Page(index).GetPlainText(nil)
		if err != nil {
			return nil, fmt.Errorf("PDF 第 %d 页解析失败: %w", index, err)
		}
		text = strings.TrimSpace(cleanText(text))
		total += len(text)
		if total > maxExtractedBytes {
			return nil, fmt.Errorf("PDF 提取文本不能超过 32 MiB")
		}
		if text != "" {
			heading := fmt.Sprintf("第 %d 页", index)
			blocks = append(blocks, SourceBlock{Content: text, Heading: heading, HeadingPath: []string{heading}})
		}
	}
	if len(blocks) == 0 {
		return nil, fmt.Errorf("PDF 没有可提取文本；扫描件请先 OCR，缺少字体映射的文件请重新导出")
	}
	return blocks, nil
}

// Inspect Office ZIP sizes before a format parser expands its members.
func officeArchive(content []byte) (*zip.Reader, error) {
	reader, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
	if err != nil {
		return nil, fmt.Errorf("Office 文件损坏或不是有效的 ZIP 容器: %w", err)
	}
	if len(reader.File) > 5000 {
		return nil, fmt.Errorf("Office 容器内部文件数量过多")
	}
	var size uint64
	for _, file := range reader.File {
		if file.UncompressedSize64 > maxExtractedBytes {
			return nil, fmt.Errorf("Office 容器内部单个文件不能超过 32 MiB")
		}
		size += file.UncompressedSize64
		if size > 128<<20 {
			return nil, fmt.Errorf("Office 容器展开大小不能超过 128 MiB")
		}
	}
	return reader, nil
}

func readOfficePart(file *zip.File) ([]byte, error) {
	stream, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer stream.Close()
	content, err := io.ReadAll(io.LimitReader(stream, maxExtractedBytes+1))
	if err != nil {
		return nil, err
	}
	if len(content) > maxExtractedBytes {
		return nil, fmt.Errorf("Office XML 超过 32 MiB")
	}
	return content, nil
}

func parseXLSX(content []byte) ([]SourceBlock, error) {
	if _, err := officeArchive(content); err != nil {
		return nil, err
	}
	book, err := excelize.OpenReader(bytes.NewReader(content), excelize.Options{
		UnzipSizeLimit: 128 << 20, UnzipXMLSizeLimit: maxExtractedBytes,
	})
	if err != nil {
		return nil, fmt.Errorf("Excel 解析失败: %w", err)
	}
	defer book.Close()
	var blocks []SourceBlock
	total := 0
	for _, name := range book.GetSheetList() {
		rows, err := book.Rows(name)
		if err != nil {
			return nil, err
		}
		line := 0
		for rows.Next() {
			line++
			cells, err := rows.Columns()
			if err != nil {
				rows.Close()
				return nil, err
			}
			value := cleanText(strings.Join(cells, "\t"))
			total += len(value)
			if total > maxExtractedBytes {
				rows.Close()
				return nil, fmt.Errorf("Excel 提取文本不能超过 32 MiB")
			}
			if strings.TrimSpace(value) != "" {
				blocks = append(blocks, SourceBlock{Content: value, Heading: name, HeadingPath: []string{name}, LineFrom: line, LineTo: line})
			}
		}
		rowErr := rows.Error()
		rows.Close()
		if rowErr != nil {
			return nil, rowErr
		}
	}
	return blocks, nil
}

func parsePPTX(content []byte) ([]SourceBlock, error) {
	reader, err := officeArchive(content)
	if err != nil {
		return nil, err
	}
	type slide struct {
		number int
		file   *zip.File
	}
	var slides []slide
	for _, file := range reader.File {
		if !strings.HasPrefix(file.Name, "ppt/slides/slide") || !strings.HasSuffix(file.Name, ".xml") {
			continue
		}
		number, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(file.Name, "ppt/slides/slide"), ".xml"))
		if err == nil {
			slides = append(slides, slide{number, file})
		}
	}
	sort.Slice(slides, func(i, j int) bool { return slides[i].number < slides[j].number })
	var blocks []SourceBlock
	total := 0
	for _, item := range slides {
		content, err := readOfficePart(item.file)
		if err != nil {
			return nil, err
		}
		decoder := xml.NewDecoder(bytes.NewReader(content))
		var text strings.Builder
		for {
			token, err := decoder.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, fmt.Errorf("PPTX XML 解析失败: %w", err)
			}
			switch value := token.(type) {
			case xml.StartElement:
				if value.Name.Local == "t" {
					var part string
					if err := decoder.DecodeElement(&part, &value); err != nil {
						return nil, err
					}
					text.WriteString(part)
				}
			case xml.EndElement:
				if value.Name.Local == "p" {
					text.WriteByte('\n')
				}
			}
		}
		value := strings.TrimSpace(cleanText(text.String()))
		total += len(value)
		if total > maxExtractedBytes {
			return nil, fmt.Errorf("PPTX 提取文本不能超过 32 MiB")
		}
		if value != "" {
			heading := fmt.Sprintf("幻灯片 %d", item.number)
			blocks = append(blocks, SourceBlock{Content: value, Heading: heading, HeadingPath: []string{heading}})
		}
	}
	return blocks, nil
}
