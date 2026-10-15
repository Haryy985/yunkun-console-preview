package main

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func fixtureCredential(t *testing.T, key []byte, aad string) string {
	t.Helper()
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	nonce := bytes.Repeat([]byte{7}, aead.NonceSize())
	return base64.StdEncoding.EncodeToString(aead.Seal(nonce, nonce, []byte(`{"api_key":"fixture","future_field":17}`), []byte(aad)))
}

func TestCredentialMigration(t *testing.T) {
	key := bytes.Repeat([]byte{2}, 32)
	old := fixtureCredential(t, key, oldAAD)
	next, changed, err := convertCredential(key, old)
	if err != nil || !changed || next == old {
		t.Fatalf("conversion failed: %v", err)
	}
	block, _ := aes.NewCipher(key)
	aead, _ := cipher.NewGCM(block)
	raw, _ := base64.StdEncoding.DecodeString(next)
	plain, err := aead.Open(nil, raw[:aead.NonceSize()], raw[aead.NonceSize():], []byte(newAAD))
	if err != nil || string(plain) != `{"api_key":"fixture","future_field":17}` {
		t.Fatal("payload changed or new format is unreadable")
	}
	for _, current := range []string{next, ""} {
		got, changed, err := convertCredential(key, current)
		if err != nil || changed || got != current {
			t.Fatal("migration is not idempotent")
		}
	}
	if _, _, err := convertCredential(bytes.Repeat([]byte{3}, 32), old); err == nil {
		t.Fatal("wrong key accepted")
	}
	raw[len(raw)-1] ^= 1
	for _, invalid := range []string{"bad", base64.StdEncoding.EncodeToString(raw)} {
		if _, _, err := convertCredential(key, invalid); err == nil {
			t.Fatal("invalid credentials accepted")
		}
	}
}

func TestDatabasePlanAndRollback(t *testing.T) {
	ctx := context.Background()
	conn, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetMaxOpenConns(1)
	for _, table := range []string{"proxy_pools", "proxy_discovery_sources", "rag_documents"} {
		column := "credentials"
		if table == "rag_documents" {
			column = "mime_type"
		}
		if _, err := conn.Exec("CREATE TABLE " + table + "(id INTEGER PRIMARY KEY, " + column + " TEXT)"); err != nil {
			t.Fatal(err)
		}
	}
	key := bytes.Repeat([]byte{4}, 32)
	old := fixtureCredential(t, key, oldAAD)
	current := fixtureCredential(t, key, newAAD)
	for _, table := range []string{"proxy_pools", "proxy_discovery_sources"} {
		if _, err := conn.Exec("INSERT INTO "+table+" VALUES (1,?), (2,?), (3,'')", old, current); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := conn.Exec("INSERT INTO rag_documents VALUES (1,?), (2,?), (3,'text/plain')", oldMIME+"; charset=utf-8", newMIME); err != nil {
		t.Fatal(err)
	}
	tx, _ := conn.BeginTx(ctx, nil)
	plan, err := databasePlan(ctx, tx, key)
	if err != nil || len(plan) != 3 {
		t.Fatalf("plan len=%d err=%v", len(plan), err)
	}
	if err := applyDatabasePlan(ctx, tx, plan); err != nil {
		t.Fatal(err)
	}
	// Force a conditional update failure after successful updates and roll back all.
	if err := applyDatabasePlan(ctx, tx, plan); err == nil {
		t.Fatal("stale plan accepted")
	}
	tx.Rollback()
	var got string
	conn.QueryRow("SELECT credentials FROM proxy_pools WHERE id=1").Scan(&got)
	if got != old {
		t.Fatal("rollback did not preserve old value")
	}
	tx, _ = conn.BeginTx(ctx, nil)
	plan, err = databasePlan(ctx, tx, key)
	if err != nil {
		t.Fatal(err)
	}
	if err := applyDatabasePlan(ctx, tx, plan); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	tx, _ = conn.BeginTx(ctx, nil)
	plan, err = databasePlan(ctx, tx, key)
	tx.Rollback()
	if err != nil || len(plan) != 0 {
		t.Fatalf("repeated migration: %v", err)
	}
	if _, err := conn.Exec("UPDATE proxy_discovery_sources SET credentials='corrupt' WHERE id=1"); err != nil {
		t.Fatal(err)
	}
	tx, _ = conn.BeginTx(ctx, nil)
	_, err = databasePlan(ctx, tx, key)
	tx.Rollback()
	if err == nil {
		t.Fatal("corrupt row did not abort planning")
	}
}

func TestTranscriptScopeAndBackup(t *testing.T) {
	old := "<!-- ARTEX_WORKER_CHAT:request-17 -->"
	lines := []string{
		`{"uuid":"u1","session_id":"s1","usage":{"prompt":17},"type":"message","future":42,"message":{"role":"user","content":[{"type":"text","text":"` + old + `"},{"type":"tool_result","content":[{"type":"text","text":"` + old + `"}]}]}}`,
		`{"uuid":"u2","type":"message","message":{"role":"assistant","content":[{"type":"text","text":"` + old + `"}]}}`,
		`{"uuid":"u3","type":"boundary","future":99}`,
	}
	source := strings.Join(lines, "\r\n") + "\r\n"
	dir := t.TempDir()
	root := filepath.Join(dir, "transcripts")
	os.Mkdir(root, 0700)
	path := filepath.Join(root, "session.jsonl")
	os.WriteFile(path, []byte(source), 0600)
	if err := runTranscripts(root, "", false); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	if string(raw) != source {
		t.Fatal("dry run changed transcript")
	}
	backup := filepath.Join(dir, "backup")
	if err := runTranscripts(root, backup, true); err != nil {
		t.Fatal(err)
	}
	raw, _ = os.ReadFile(path)
	actual := strings.Split(string(raw), "\r\n")
	if actual[1] != lines[1] || actual[2] != lines[2] {
		t.Fatal("unrelated records changed")
	}
	var record map[string]json.RawMessage
	if err := json.Unmarshal([]byte(actual[0]), &record); err != nil {
		t.Fatal(err)
	}
	if string(record["uuid"]) != `"u1"` || string(record["future"]) != "42" || !strings.Contains(actual[0], "YUNKUN_WORKER_CHAT:request-17") || !strings.Contains(actual[0], "ARTEX_WORKER_CHAT:request-17") {
		t.Fatal("unknown fields lost or nested tool result changed")
	}
	raw, _ = os.ReadFile(filepath.Join(backup, "session.jsonl"))
	if string(raw) != source {
		t.Fatal("backup mismatch")
	}
	if err := runTranscripts(root, "", false); err != nil {
		t.Fatal(err)
	}
	var result bytes.Buffer
	count, err := convertTranscript(strings.NewReader(strings.Join(actual, "\r\n")), &result)
	if err != nil || count != 0 {
		t.Fatal("transcript migration not idempotent")
	}
}

func TestTranscriptPrevalidationAndExclusiveBackup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "invalid.jsonl")
	os.WriteFile(path, []byte("not json\n"), 0600)
	backup := filepath.Join(t.TempDir(), "backup")
	if err := runTranscripts(dir, backup, true); err == nil {
		t.Fatal("invalid JSON accepted")
	}
	if _, err := os.Stat(backup); !os.IsNotExist(err) {
		t.Fatal("backup created before validation")
	}
	file := filepath.Join(t.TempDir(), "backup.json")
	if err := writeExclusive(file, []byte("original")); err != nil {
		t.Fatal(err)
	}
	if err := writeExclusive(file, []byte("overwrite")); err == nil {
		t.Fatal("existing backup overwritten")
	}
}
