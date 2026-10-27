package selfupdate

import (
	"archive/zip"
	"os"
	"path/filepath"
	"runtime"
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
		"Start-Yunkun.command", "README.txt", "PACKAGE.json", "FILES.sha256", "POSTGRESQL_LICENSE.txt", "THIRD_PARTY_NOTICES.txt", "skills/custom/SKILL.md"} {
		header := &zip.FileHeader{Name: "pkg/" + name, Method: zip.Deflate}
		mode := os.FileMode(0644)
		if name == "bin/yunkun" || name == "bin/yunkun-offline" || name == "Start-Yunkun.command" {
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
	info, err = os.Stat(filepath.Join(dst, "Start-Yunkun.command"))
	if err != nil || (os.PathSeparator == '/' && info.Mode().Perm() != 0755) {
		t.Fatal("macOS launcher or executable permissions lost")
	}
}

func TestOfflineBinaryPlatformCheckedBeforeExecution(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if err := checkOfflineBinaryPlatform(self, runtime.GOOS, runtime.GOARCH); err != nil {
		t.Fatal(err)
	}
	if err := checkOfflineBinaryPlatform(self, "wrong-os", runtime.GOARCH); err == nil {
		t.Fatal("wrong OS accepted")
	}
	if err := checkOfflineBinaryPlatform(self, runtime.GOOS, "wrong-arch"); err == nil {
		t.Fatal("wrong CPU accepted")
	}
	fake := filepath.Join(t.TempDir(), "fake")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\necho v1.1.3-offline\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := checkOfflineBinaryPlatform(fake, runtime.GOOS, runtime.GOARCH); err == nil {
		t.Fatal("version-only script accepted as released Go binary")
	}
}

func TestOfflineSupportedPlatformsAndAssets(t *testing.T) {
	for _, tc := range []struct {
		os, arch  string
		supported bool
	}{
		{"windows", "amd64", true}, {"linux", "amd64", true}, {"linux", "arm64", true},
		{"darwin", "amd64", true}, {"darwin", "arm64", true},
		{"windows", "arm64", false}, {"linux", "386", false}, {"freebsd", "amd64", false},
	} {
		t.Run(tc.os+"/"+tc.arch, func(t *testing.T) {
			if supportedOfflinePlatform(tc.os, tc.arch) != tc.supported {
				t.Fatal("platform support mismatch")
			}
			if !tc.supported {
				return
			}
			name := AssetName("v1.1.3-offline", tc.os, tc.arch)
			release := &Release{TagName: "v1.1.3-offline", Assets: []Asset{{Name: name}}}
			if asset, found := FindOfflineAsset(release, tc.os, tc.arch); !found || asset.Name != name {
				t.Fatal("platform package not selected")
			}
		})
	}
	for _, name := range []string{"pkg/Start-Yunkun.command/other", "pkg/Other.command", "pkg/Start-Yunkun.command.key"} {
		if _, err := offlineEntry(name); err == nil {
			t.Fatalf("unapproved entry accepted: %s", name)
		}
	}
}
