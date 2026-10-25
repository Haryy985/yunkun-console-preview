package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestDebianMemberBounds(t *testing.T) {
	header := fmt.Sprintf("%-16s%-12s%-6s%-6s%-8s%-10d`\n", "data.tar.xz/", "0", "0", "0", "100644", 3)
	valid := []byte("!<arch>\n" + header + "abc\n")
	member, err := debMember(valid, "data.tar.xz")
	if err != nil || string(member) != "abc" {
		t.Fatalf("valid Debian member: %q %v", member, err)
	}
	for _, b := range [][]byte{[]byte("bad"), valid[:20], valid[:len(valid)-2]} {
		if _, err := debMember(b, "data.tar.xz"); err == nil {
			t.Fatal("accepted malformed Debian archive")
		}
	}
}

func TestSafeNameRejectsPrivateDataAndTraversal(t *testing.T) {
	for _, name := range []string{
		"../config.json", "/runtime/bin/postgres", "runtime/../../secrets",
		"skills\\..\\config.json", "skills/C:/secret", "skills/.env",
		"skills/node_modules/x", "skills/.git/config", ".yunkun-local-archive/data",
		"skills/installation.json", "runtime/postgres-data/base", "skills/debug.log",
		"runtime/../lib/x", "skills/backup.bak",
	} {
		if err := safeName(name); err == nil {
			t.Errorf("accepted %q", name)
		}
	}
	for _, name := range []string{"skills/workflow/SKILL.md", "runtime/bin/postgres", "NOTICE.md", "config.example.json"} {
		if err := safeName(name); err != nil {
			t.Errorf("rejected %q: %v", name, err)
		}
	}
}

func TestRuntimeChecksumBeforeParsing(t *testing.T) {
	archive := filepath.Join(t.TempDir(), "runtime.tar.gz")
	if err := os.WriteFile(archive, []byte("untrusted archive"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := readRuntime(archive, sources["linux-amd64"]); err == nil || err.Error() != "cached runtime archive checksum mismatch" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDependencyBoundary(t *testing.T) {
	for _, test := range []struct {
		os, lib string
		allow   bool
	}{
		{"linux", "libc.so.6", true}, {"linux", "libcrypto.so.1.1", false},
		{"linux", "libxml2.so.2", false}, {"darwin", "/usr/lib/libSystem.B.dylib", true},
		{"darwin", "/Library/edb/lib/libpython.dylib", false},
		{"darwin", "@rpath/libpq.5.dylib", false},
	} {
		if isSystemLibrary(test.os, test.lib) != test.allow {
			t.Errorf("%s %s", test.os, test.lib)
		}
	}
}
