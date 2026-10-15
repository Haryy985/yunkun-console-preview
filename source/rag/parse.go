package rag

import (
	"bytes"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

// SourceBlock is a logical section extracted from a supported source document.
// Line numbers are best-effort for formats that do not have a line-oriented
// representation (for example JSON and HTML).
type SourceBlock struct {
	Content     string
	Heading     string
	HeadingPath []string
	LineFrom    int
	LineTo      int
}

type Chunk struct {
	Ordinal     int
	Heading     string
	HeadingPath []string
	Content     string
	ContentHash string
	LineFrom    int
	LineTo      int
}

func ParseDocument(name, mimeType string, content []byte) ([]SourceBlock, error) {
	mime := strings.ToLower(strings.TrimSpace(mimeType))
	if strings.HasPrefix(mime, ExtractedTextMIME) {
		return parseMarkdown(string(content)), nil
	}
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".md", ".markdown":
		text, err := decodeText(content)
		if err != nil {
			return nil, fmt.Errorf("decode markdown: %w", err)
		}
		return parseMarkdown(text), nil
	case ".txt", ".text", ".xml", ".yaml", ".yml", ".log", ".sql", ".ini", ".conf",
		".properties", ".rst", ".go", ".py", ".js", ".ts", ".tsx", ".jsx", ".sh", "":
		text, err := decodeText(content)
		if err != nil {
			return nil, fmt.Errorf("decode text: %w", err)
		}
		return parsePlainText(text), nil
	case ".json":
		text, err := decodeText(content)
		if err != nil {
			return nil, fmt.Errorf("decode json: %w", err)
		}
		var value any
		if err := json.Unmarshal([]byte(text), &value); err != nil {
			return nil, fmt.Errorf("parse json: %w", err)
		}
		pretty, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("format json: %w", err)
		}
		return parsePlainText(string(pretty)), nil
	case ".csv":
		text, err := decodeText(content)
		if err != nil {
			return nil, fmt.Errorf("decode csv: %w", err)
		}
		return parseCSV([]byte(text))
	case ".html", ".htm":
		text, err := decodeText(content)
		if err != nil {
			return nil, fmt.Errorf("decode html: %w", err)
		}
		return parseHTML([]byte(text))
	case ".docx":
		return parseDOCX(content)
	case ".pdf":
		return parsePDF(content)
	case ".xlsx":
		return parseXLSX(content)
	case ".pptx":
		return parsePPTX(content)
	default:
		if strings.Contains(strings.ToLower(mimeType), "markdown") {
			text, err := decodeText(content)
			if err != nil {
				return nil, fmt.Errorf("decode markdown: %w", err)
			}
			return parseMarkdown(text), nil
		}
		if strings.Contains(strings.ToLower(mimeType), "wordprocessingml") {
			return parseDOCX(content)
		}
		if strings.Contains(strings.ToLower(mimeType), "pdf") {
			return parsePDF(content)
		}
		if strings.Contains(strings.ToLower(mimeType), "spreadsheetml") {
			return parseXLSX(content)
		}
		if strings.Contains(strings.ToLower(mimeType), "presentationml") {
			return parsePPTX(content)
		}
		if strings.HasPrefix(strings.ToLower(mimeType), "text/") {
			text, err := decodeText(content)
			if err != nil {
				return nil, fmt.Errorf("decode text: %w", err)
			}
			return parsePlainText(text), nil
		}
		return nil, fmt.Errorf("unsupported document format: %s", name)
	}
}

func parseDOCX(content []byte) ([]SourceBlock, error) {
	reader, err := officeArchive(content)
	if err != nil {
		return nil, fmt.Errorf("parse docx zip: %w", err)
	}
	var document []byte
	for _, file := range reader.File {
		if file.Name != "word/document.xml" {
			continue
		}
		document, err = readOfficePart(file)
		if err != nil {
			return nil, fmt.Errorf("read docx document.xml: %w", err)
		}
		break
	}
	if len(document) == 0 {
		return nil, fmt.Errorf("docx document.xml not found")
	}

	type paragraph struct {
		text  string
		style string
	}
	paragraphs := make([]paragraph, 0)
	decoder := xml.NewDecoder(bytes.NewReader(document))
	var current *paragraph
	var inText bool
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("parse docx document.xml: %w", err)
		}
		switch value := token.(type) {
		case xml.StartElement:
			switch value.Name.Local {
			case "p":
				if current != nil {
					paragraphs = append(paragraphs, *current)
				}
				current = &paragraph{}
			case "pStyle":
				if current == nil {
					continue
				}
				for _, attribute := range value.Attr {
					if attribute.Name.Local == "val" {
						current.style = attribute.Value
						break
					}
				}
			case "t":
				inText = current != nil
			case "tab":
				if current != nil {
					current.text += "\t"
				}
			case "br", "cr":
				if current != nil && current.text != "" && !strings.HasSuffix(current.text, "\n") {
					current.text += "\n"
				}
			}
		case xml.CharData:
			if current != nil && inText {
				current.text += string(value)
			}
		case xml.EndElement:
			switch value.Name.Local {
			case "t":
				inText = false
			case "p":
				if current != nil {
					paragraphs = append(paragraphs, *current)
					current = nil
				}
			}
		}
	}
	if current != nil {
		paragraphs = append(paragraphs, *current)
	}

	blocks := make([]SourceBlock, 0, len(paragraphs))
	var headingPath []string
	hasTitle := false
	for index, item := range paragraphs {
		text := strings.TrimSpace(item.text)
		if text == "" {
			continue
		}
		if level := docxHeadingLevel(item.style); level > 0 {
			if strings.EqualFold(strings.TrimSpace(item.style), "title") {
				hasTitle = true
			} else if hasTitle {
				level++
			}
			for len(headingPath) >= level {
				headingPath = headingPath[:len(headingPath)-1]
			}
			headingPath = append(headingPath, text)
			blocks = append(blocks, SourceBlock{
				Content:     text,
				Heading:     text,
				HeadingPath: append([]string(nil), headingPath...),
				LineFrom:    index + 1,
				LineTo:      index + 1,
			})
			continue
		}
		blocks = append(blocks, SourceBlock{
			Content:     text,
			Heading:     lastHeading(headingPath),
			HeadingPath: append([]string(nil), headingPath...),
			LineFrom:    index + 1,
			LineTo:      index + 1,
		})
	}
	return blocks, nil
}

func docxHeadingLevel(style string) int {
	style = strings.TrimSpace(strings.ToLower(style))
	if style == "title" {
		return 1
	}
	if !strings.HasPrefix(style, "heading") {
		return 0
	}
	value, err := strconv.Atoi(strings.TrimPrefix(style, "heading"))
	if err != nil || value < 1 || value > 6 {
		return 0
	}
	return value
}

func lastHeading(path []string) string {
	if len(path) == 0 {
		return ""
	}
	return path[len(path)-1]
}

func ChunkBlocks(blocks []SourceBlock, chunkSize, overlap int) []Chunk {
	if chunkSize <= 0 {
		chunkSize = 900
	}
	if overlap < 0 || overlap >= chunkSize {
		overlap = chunkSize / 8
	}
	result := make([]Chunk, 0, len(blocks))
	for _, block := range blocks {
		text := strings.TrimSpace(block.Content)
		if text == "" {
			continue
		}
		runes := []rune(text)
		for start, ordinal := 0, len(result); start < len(runes); ordinal++ {
			end := start + chunkSize
			if end > len(runes) {
				end = len(runes)
			}
			part := strings.TrimSpace(string(runes[start:end]))
			if part != "" {
				sum := sha256.Sum256([]byte(part))
				result = append(result, Chunk{
					Ordinal:     ordinal,
					Heading:     block.Heading,
					HeadingPath: append([]string(nil), block.HeadingPath...),
					Content:     part,
					ContentHash: hex.EncodeToString(sum[:]),
					LineFrom:    block.LineFrom,
					LineTo:      block.LineTo,
				})
			}
			if end == len(runes) {
				break
			}
			next := end - overlap
			if next <= start {
				next = end
			}
			start = next
		}
	}
	return result
}

func parseMarkdown(text string) []SourceBlock {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	var blocks []SourceBlock
	var current []string
	currentStart := 1
	currentHeading := ""
	var headingPath []string
	flush := func(end int) {
		value := strings.TrimSpace(strings.Join(current, "\n"))
		if value != "" {
			blocks = append(blocks, SourceBlock{
				Content:     value,
				Heading:     currentHeading,
				HeadingPath: append([]string(nil), headingPath...),
				LineFrom:    currentStart,
				LineTo:      end,
			})
		}
		current = nil
	}
	for index, line := range lines {
		lineNo := index + 1
		trimmed := strings.TrimSpace(line)
		level, title := markdownHeading(trimmed)
		if level > 0 {
			flush(lineNo - 1)
			for len(headingPath) >= level {
				headingPath = headingPath[:len(headingPath)-1]
			}
			headingPath = append(headingPath, title)
			currentHeading = title
			currentStart = lineNo
			current = []string{title}
			continue
		}
		if trimmed == "" {
			flush(lineNo - 1)
			currentStart = lineNo + 1
			continue
		}
		if len(current) == 0 {
			currentStart = lineNo
		}
		current = append(current, line)
	}
	flush(len(lines))
	return blocks
}

func markdownHeading(line string) (int, string) {
	level := 0
	for level < len(line) && line[level] == '#' {
		level++
	}
	if level == 0 || level > 6 || (len(line) > level && line[level] != ' ') {
		return 0, ""
	}
	title := strings.TrimSpace(line[level:])
	if title == "" {
		return 0, ""
	}
	return level, title
}

func parsePlainText(text string) []SourceBlock {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	var blocks []SourceBlock
	var current []string
	start := 1
	flush := func(end int) {
		value := strings.TrimSpace(strings.Join(current, "\n"))
		if value != "" {
			blocks = append(blocks, SourceBlock{Content: value, LineFrom: start, LineTo: end})
		}
		current = nil
	}
	for index, line := range lines {
		lineNo := index + 1
		if strings.TrimSpace(line) == "" {
			flush(lineNo - 1)
			start = lineNo + 1
			continue
		}
		if len(current) == 0 {
			start = lineNo
		}
		current = append(current, line)
	}
	flush(len(lines))
	return blocks
}

func parseCSV(content []byte) ([]SourceBlock, error) {
	reader := csv.NewReader(strings.NewReader(string(content)))
	reader.FieldsPerRecord = -1
	rows, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("parse csv: %w", err)
	}
	if len(rows) == 0 {
		return nil, nil
	}
	headers := rows[0]
	blocks := make([]SourceBlock, 0, len(rows)-1)
	for index, row := range rows[1:] {
		var fields []string
		for column, value := range row {
			key := fmt.Sprintf("列%d", column+1)
			if column < len(headers) && strings.TrimSpace(headers[column]) != "" {
				key = strings.TrimSpace(headers[column])
			}
			fields = append(fields, key+": "+strings.TrimSpace(value))
		}
		if len(fields) == 0 {
			continue
		}
		blocks = append(blocks, SourceBlock{
			Content:  strings.Join(fields, "\n"),
			LineFrom: index + 2,
			LineTo:   index + 2,
		})
	}
	return blocks, nil
}

func parseHTML(content []byte) ([]SourceBlock, error) {
	root, err := html.Parse(strings.NewReader(string(content)))
	if err != nil {
		return nil, fmt.Errorf("parse html: %w", err)
	}
	var parts []string
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode && (node.Data == "script" || node.Data == "style" || node.Data == "noscript") {
			return
		}
		if node.Type == html.TextNode {
			value := strings.TrimSpace(node.Data)
			if value != "" {
				parts = append(parts, value)
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)
	text := strings.Join(parts, "\n")
	return parsePlainText(text), nil
}
