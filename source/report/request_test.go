package report

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"yunkun/console/db"
	"yunkun/console/evidence"
)

func TestRequestPacketDoesNotInventOrTruncate(t *testing.T) {
	dir := t.TempDir()
	store := evidence.New(nil, nil, dir)
	writeBody := func(body []byte) db.TrafficEvidenceSnapshot {
		t.Helper()
		hash := sha256.Sum256(body)
		id := hex.EncodeToString(hash[:])
		path := filepath.Join(dir, "blobs", id[:2], id+".bin")
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, body, 0600); err != nil {
			t.Fatal(err)
		}
		return db.TrafficEvidenceSnapshot{ReqHead: "POST / HTTP/1.1\r\nHost: example.local", ReqHash: id, ReqLen: int64(len(body))}
	}
	for _, tc := range []struct {
		body   []byte
		reason string
	}{
		{[]byte("small"), ""},
		{[]byte{0, 255}, "二进制"},
		{[]byte(strings.Repeat("x", maxInlineRequestBody+1)), "超过报告内嵌上限"},
	} {
		packet, err := ReadRequestPacket(store, writeBody(tc.body))
		if err != nil {
			t.Fatal(err)
		}
		if tc.reason == "" && !strings.Contains(packet.Text, "\n\nsmall") {
			t.Fatalf("missing body: %+v", packet)
		}
		if tc.reason != "" && (packet.Text != "" || !strings.Contains(packet.Reason, tc.reason)) {
			t.Fatalf("incorrectly presented full packet: %+v", packet)
		}
	}
	snapshot := writeBody([]byte("proof"))
	snapshot.ReqHead = "POST / HTTP/1.1"
	packet, err := ReadRequestPacket(store, snapshot)
	if err != nil || packet.Text != "" || !strings.Contains(packet.Reason, "Host") {
		t.Fatalf("invented host: %+v %v", packet, err)
	}
	if !strings.Contains(requestPacketMarkdown(RequestPacket{}), "未保存 HTTP 请求包") {
		t.Fatal("missing evidence was not labeled")
	}
}
