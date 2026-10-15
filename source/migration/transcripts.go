package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var oldWorkerMarker = regexp.MustCompile(`<!-- ARTEX_WORKER_CHAT:([A-Za-z0-9][A-Za-z0-9_.:-]*) -->`)

func transformRecord(line []byte) ([]byte, bool, error) {
	if len(bytes.TrimSpace(line)) == 0 {
		return line, false, nil
	}
	var record map[string]json.RawMessage
	if err := json.Unmarshal(line, &record); err != nil || record == nil {
		return nil, false, errors.New("invalid transcript JSON")
	}
	var typ string
	if err := json.Unmarshal(record["type"], &typ); err != nil {
		return nil, false, errors.New("missing transcript record type")
	}
	if typ != "message" {
		return line, false, nil
	}
	var message map[string]json.RawMessage
	if err := json.Unmarshal(record["message"], &message); err != nil || message == nil {
		return nil, false, errors.New("invalid transcript message")
	}
	var role string
	if err := json.Unmarshal(message["role"], &role); err != nil {
		return nil, false, errors.New("missing message role")
	}
	if role != "user" {
		return line, false, nil
	}
	var blocks []map[string]json.RawMessage
	if err := json.Unmarshal(message["content"], &blocks); err != nil {
		return nil, false, errors.New("invalid message content")
	}
	changed := false
	for _, block := range blocks {
		var blockType string
		_ = json.Unmarshal(block["type"], &blockType)
		if blockType != "text" {
			continue
		}
		var text string
		if err := json.Unmarshal(block["text"], &text); err != nil {
			return nil, false, errors.New("invalid text block")
		}
		next := oldWorkerMarker.ReplaceAllString(text, "<!-- YUNKUN_WORKER_CHAT:${1} -->")
		if next != text {
			block["text"], _ = json.Marshal(next)
			changed = true
		}
	}
	if !changed {
		return line, false, nil
	}
	message["content"], _ = json.Marshal(blocks)
	record["message"], _ = json.Marshal(message)
	next, err := json.Marshal(record)
	if err != nil {
		return nil, false, err
	}
	if bytes.HasSuffix(line, []byte("\r\n")) {
		next = append(next, '\r', '\n')
	} else if bytes.HasSuffix(line, []byte("\n")) {
		next = append(next, '\n')
	}
	return next, true, nil
}

func splitLines(data []byte, atEOF bool) (int, []byte, error) {
	if i := bytes.IndexByte(data, '\n'); i >= 0 {
		return i + 1, data[:i+1], nil
	}
	if atEOF && len(data) != 0 {
		return len(data), data, nil
	}
	return 0, nil, nil
}

func convertTranscript(r io.Reader, w io.Writer) (int, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 64*1024*1024)
	scanner.Split(splitLines)
	count := 0
	for scanner.Scan() {
		next, changed, err := transformRecord(scanner.Bytes())
		if err != nil {
			return 0, err
		}
		if _, err := w.Write(next); err != nil {
			return 0, err
		}
		if changed {
			count++
		}
	}
	return count, scanner.Err()
}

type transcriptChange struct {
	path string
	hash [32]byte
	mode fs.FileMode
}

func inspectTranscript(path string) (int, [32]byte, error) {
	var sum [32]byte
	f, err := os.Open(path)
	if err != nil {
		return 0, sum, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := convertTranscript(io.TeeReader(f, h), io.Discard)
	copy(sum[:], h.Sum(nil))
	return n, sum, err
}

func runTranscripts(root, backup string, apply bool) error {
	if root == "" {
		return errors.New("-root is required")
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("root must be an existing non-symlink directory")
	}
	var plan []transcriptChange
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("symlink in transcript tree; refusing migration")
		}
		if entry.IsDir() || strings.ToLower(filepath.Ext(path)) != ".jsonl" {
			return nil
		}
		n, sum, err := inspectTranscript(path)
		if err != nil {
			return fmt.Errorf("invalid transcript %s: %w", filepath.Base(path), err)
		}
		if n > 0 {
			info, err := entry.Info()
			if err != nil || !info.Mode().IsRegular() {
				return errors.New("transcript is not a regular file")
			}
			plan = append(plan, transcriptChange{path, sum, info.Mode().Perm()})
		}
		return nil
	})
	if err != nil {
		return err
	}
	fmt.Printf("transcript files requiring migration: %d\n", len(plan))
	if !apply || len(plan) == 0 {
		return nil
	}
	if backup == "" {
		return errors.New("-backup requires a new private directory")
	}
	backup, err = filepath.Abs(backup)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(root, backup)
	if err != nil || rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))) {
		return errors.New("backup must be outside transcript tree, on the same volume")
	}
	if err := os.Mkdir(backup, 0700); err != nil {
		return errors.New("cannot create new backup directory")
	}
	// Validate and back up the entire batch before replacing its first file.
	for _, item := range plan {
		content, err := os.ReadFile(item.path)
		if err != nil || sha256.Sum256(content) != item.hash {
			return errors.New("transcript changed during migration; no files replaced")
		}
		rel, err := filepath.Rel(root, item.path)
		if err != nil {
			return err
		}
		target := filepath.Join(backup, rel)
		if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			return err
		}
		if err := writeExclusive(target, content); err != nil {
			return errors.New("transcript backup failed; no files replaced")
		}
	}
	for _, item := range plan {
		if err := replaceTranscript(item); err != nil {
			return fmt.Errorf("batch may be partially applied; keep backup and rerun check: %w", err)
		}
	}
	fmt.Println("transcript files migrated; all originals retained in private backup")
	return nil
}

func replaceTranscript(item transcriptChange) error {
	input, err := os.Open(item.path)
	if err != nil {
		return err
	}
	defer input.Close()
	temp, err := os.CreateTemp(filepath.Dir(item.path), ".yunkun-migrate-*")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	h := sha256.New()
	_, err = convertTranscript(io.TeeReader(input, h), temp)
	var sum [32]byte
	copy(sum[:], h.Sum(nil))
	if err == nil && sum != item.hash {
		err = errors.New("transcript changed before replacement")
	}
	if err == nil {
		err = temp.Chmod(item.mode)
	}
	if err == nil {
		err = temp.Sync()
	}
	closeErr := temp.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err := input.Close(); err != nil {
		return err
	}
	return os.Rename(temp.Name(), item.path)
}
