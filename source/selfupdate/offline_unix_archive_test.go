package selfupdate

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func TestOfflineCompleteUnixArchivePermissions(t *testing.T) {
	archive := filepath.Join(t.TempDir(), "unix.zip")
	file, err := os.Create(archive)
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(file)
	for _, name := range []string{"bin/yunkun", "bin/yunkun-offline", "start.sh", "stop.sh", "check.sh",
		"runtime/bin/pg_ctl", "runtime/bin/postgres", "runtime/bin/initdb", "runtime/share/postgresql/postgres.bki",
		"README.txt", "PACKAGE.json", "FILES.sha256", "POSTGRESQL_LICENSE.txt", "THIRD_PARTY_NOTICES.txt", "skills/custom/SKILL.md"} {
		header := &zip.FileHeader{Name: "pkg/" + name, Method: zip.Deflate}
		mode := os.FileMode(0644)
		if name == "bin/yunkun" || name == "bin/yunkun-offline" {
			mode = 0755
		}
		header.SetMode(mode)
		entry, err := w.CreateHeader(header)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte("fixture")); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	file.Close()
	dst := filepath.Join(t.TempDir(), "candidate")
	if err := extractOffline(archive, dst); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(dst, "bin", "yunkun"))
	if err != nil || info.Size() != 7 {
		t.Fatal("unix executable missing")
	}
	// Windows cannot represent executable mode bits. Native Unix runs enforce it.
	if os.PathSeparator == '/' && info.Mode().Perm() != 0755 {
		t.Fatal("executable permissions lost")
	}
}
