package report

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"unicode/utf8"

	"yunkun/console/db"
)

const maxInlineRequestBody = 16 * 1024

var curlCommand = regexp.MustCompile(`(?im)(?:^\s*(?:(?:[-*+>]|\d+[.)])\s*)?(?:\$\s*|PS [^>\r\n]*>\s*)?curl(?:\.exe)?\s|` + "`curl(?:\\.exe)?\\s)")

func ContainsCurlCommand(text string) bool {
	return curlCommand.MatchString(text)
}

type RequestPacket struct {
	Text   string
	Reason string
}

type requestBodyStore interface {
	OpenBody(db.TrafficEvidenceSnapshot, string) (*os.File, int64, error)
}

// ReadRequestPacket only presents captured bytes; it never reconstructs a request from a URL or curl.
func ReadRequestPacket(store requestBodyStore, snapshot db.TrafficEvidenceSnapshot) (RequestPacket, error) {
	head := strings.TrimRight(snapshot.ReqHead, "\r\n")
	lines := strings.Split(strings.ReplaceAll(head, "\r\n", "\n"), "\n")
	if len(lines) == 0 || !strings.Contains(lines[0], " HTTP/") {
		return RequestPacket{Reason: "保存的请求头不完整，无法提供可直接重放的 HTTP 请求包"}, nil
	}
	host := false
	for _, line := range lines[1:] {
		if len(line) >= 5 && strings.EqualFold(line[:5], "Host:") {
			host = true
		}
	}
	if !host {
		return RequestPacket{Reason: "保存的请求头缺少 Host，无法提供可直接重放的 HTTP 请求包"}, nil
	}
	if snapshot.ReqLen > maxInlineRequestBody {
		return RequestPacket{Reason: fmt.Sprintf("请求正文 %d 字节，超过报告内嵌上限；请从证据 ZIP 下载完整 request.http", snapshot.ReqLen)}, nil
	}
	f, size, err := store.OpenBody(snapshot, "request")
	if err != nil {
		return RequestPacket{}, err
	}
	defer f.Close()
	body, err := io.ReadAll(io.LimitReader(f, maxInlineRequestBody+1))
	if err != nil {
		return RequestPacket{}, err
	}
	if len(body) > maxInlineRequestBody {
		return RequestPacket{Reason: "请求正文超过报告内嵌上限；请从证据 ZIP 下载完整 request.http"}, nil
	}
	if int64(len(body)) != snapshot.ReqLen || int64(len(body)) != size {
		return RequestPacket{Reason: "保存的请求正文长度与证据记录不符，无法提供完整 HTTP 请求包"}, nil
	}
	if !utf8.Valid(body) || bytes.IndexByte(body, 0) >= 0 {
		return RequestPacket{Reason: "请求含二进制正文；请从证据 ZIP 下载完整 request.http"}, nil
	}
	return RequestPacket{Text: head + "\n\n" + string(body)}, nil
}

func requestPacketMarkdown(packet RequestPacket) string {
	if packet.Reason != "" {
		return packet.Reason + "。\n\n"
	}
	if packet.Text == "" {
		return "未保存 HTTP 请求包；以下命令或文字证据不能替代原始请求。\n\n"
	}
	fence := "```"
	for strings.Contains(packet.Text, fence) {
		fence += "`"
	}
	return fence + "http\n" + packet.Text + "\n" + fence + "\n\n"
}
