// Command package-offline-unix assembles data-free Unix release ZIPs.
package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"debug/buildinfo"
	"debug/elf"
	"debug/macho"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

type runtimeSource struct{ label, hash string }

var sources = map[string]runtimeSource{
	"linux-amd64":  {"linux-amd64", "c351a41602006e4f560e0391dc29be7c3aa84636757566e05f399f1696c30168"},
	"linux-arm64":  {"linux-arm64", "ab709bd01ff797ecc8f01875647c722f6b1300edf734b337f8f56fdff385c01b"},
	"darwin-amd64": {"macos-intel", "15f2fae4c8e1dd68761e467735f6d649412db8389012940abf5274bbadf8cf15"},
	"darwin-arm64": {"macos-apple-silicon", "435d205d86ca4fe228a25ef784b11adf7f0820939baa077e06113cebbd71c389"},
}

type entry struct {
	data []byte
	mode fs.FileMode
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	source := flag.String("source", ".", "current reviewed source directory")
	cache := flag.String("runtime-cache", "", "directory containing pinned v1.0.3 runtime archives")
	systemCache := flag.String("system-runtime-cache", "", "directory containing pinned Debian GCC runtime packages and licenses")
	platform := flag.String("platform", "", "linux-amd64, linux-arm64, darwin-amd64 or darwin-arm64")
	binaries := flag.String("binaries", "", "directory containing current yunkun and yunkun-offline binaries")
	output := flag.String("output", "", "new release output directory")
	flag.Parse()
	spec, ok := sources[*platform]
	if !ok || *cache == "" || *binaries == "" || *output == "" {
		return errors.New("provide -platform, -runtime-cache, -binaries and -output")
	}
	versionBytes, err := os.ReadFile(filepath.Join(*source, "VERSION"))
	if err != nil {
		return err
	}
	version := strings.TrimSpace(string(versionBytes))
	if !regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+-offline$`).MatchString(version) {
		return errors.New("invalid offline version")
	}
	runtimeArchive := filepath.Join(*cache, "yunkun-v1.0.3-offline-"+spec.label+".tar.gz")
	entries, err := readRuntime(runtimeArchive, spec)
	if err != nil {
		return err
	}
	apache, err := regularFile(filepath.Join(*systemCache, "APACHE-2.0.txt"))
	if err != nil {
		return err
	}
	if fmt.Sprintf("%x", sha256.Sum256(apache)) != "cfc7749b96f63bd31c3c42b5c471bf756814053e847c10f3eb003417bc523d30" {
		return errors.New("Apache 2.0 license checksum mismatch")
	}
	entries["runtime/licenses/APACHE-2.0.txt"] = entry{apache, 0644}
	goos, goarch, _ := strings.Cut(*platform, "-")
	var additionalRuntime []map[string]string
	if goos == "linux" {
		additionalRuntime, err = addGCC(entries, *systemCache, goarch)
		if err != nil {
			return err
		}
	}
	for _, name := range []string{"yunkun", "yunkun-offline"} {
		b, err := regularFile(filepath.Join(*binaries, name))
		if err != nil {
			return err
		}
		if err := validateBinary(b, goos, goarch); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		info, err := buildinfo.Read(bytes.NewReader(b))
		if err != nil {
			return fmt.Errorf("%s build metadata: %w", name, err)
		}
		if !bytes.Contains(b, []byte(version)) || info.Main.Path != "yunkun/console" {
			return fmt.Errorf("%s lacks expected project/version marker %s; native -version verification is also required", name, version)
		}
		entries["bin/"+name] = entry{b, 0755}
	}
	for dest, src := range map[string]string{
		"start.sh": "scripts/offline-unix-start.sh", "stop.sh": "scripts/offline-unix-stop.sh",
		"check.sh": "scripts/offline-unix-check.sh", "LICENSE": "LICENSE", "NOTICE.md": "NOTICE.md",
	} {
		b, err := regularFile(filepath.Join(*source, filepath.FromSlash(src)))
		if err != nil {
			return err
		}
		mode := fs.FileMode(0644)
		if strings.HasSuffix(dest, ".sh") {
			mode = 0755
			b = bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n"))
		}
		entries[dest] = entry{b, mode}
	}
	if goos == "darwin" {
		b, err := regularFile(filepath.Join(*source, "scripts", "offline-macos-start.command"))
		if err != nil {
			return err
		}
		entries["Start-Yunkun.command"] = entry{bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n")), 0755}
	}
	skillRoot := filepath.Join(*source, "skills")
	if err := filepath.WalkDir(skillRoot, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(skillRoot, p)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		name := "skills/" + filepath.ToSlash(rel)
		if err := safeName(name); err != nil {
			return err
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("linked skill rejected: %s", rel)
		}
		if d.IsDir() {
			return nil
		}
		b, err := regularFile(p)
		if err != nil {
			return err
		}
		mode := fs.FileMode(0644)
		if strings.HasSuffix(name, ".sh") || bytes.HasPrefix(b, []byte("#!")) {
			mode = 0755
		}
		entries[name] = entry{b, mode}
		return nil
	}); err != nil {
		return err
	}
	deps, err := checkCoreDependencies(entries, goos, goarch)
	if err != nil {
		return err
	}
	entries["VERSION"] = entry{[]byte(version + "\n"), 0644}
	readme, err := regularFile(filepath.Join(*source, "docs", "OFFLINE_UNIX.txt"))
	if err != nil {
		return err
	}
	entries["README.txt"] = entry{append([]byte("Yunkun "+version+" "+*platform+"\n\n"), readme...), 0644}
	manifest := map[string]any{
		"version": version, "platform": *platform, "postgresql": "18.4",
		"runtime_source_sha256": spec.hash, "core_dependencies": deps,
		"data_included": false, "build_toolchains_required": false,
		"native_acceptance":  "not implied by archive assembly; see external acceptance results",
		"additional_runtime": additionalRuntime,
	}
	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	entries["PACKAGE.json"] = entry{append(manifestBytes, '\n'), 0644}
	names := make([]string, 0, len(entries))
	for name := range entries {
		names = append(names, name)
	}
	sort.Strings(names)
	var checksums strings.Builder
	for _, name := range names {
		fmt.Fprintf(&checksums, "%x  %s\n", sha256.Sum256(entries[name].data), name)
	}
	entries["FILES.sha256"] = entry{[]byte(checksums.String()), 0644}
	names = append(names, "FILES.sha256")
	sort.Strings(names)
	if err := os.MkdirAll(*output, 0755); err != nil {
		return err
	}
	archive := filepath.Join(*output, "yunkun-"+strings.TrimPrefix(version, "v")+"-"+*platform+".zip")
	f, err := os.OpenFile(archive, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return err
	}
	done := false
	defer func() {
		f.Close()
		if !done {
			os.Remove(archive)
		}
	}()
	z := zip.NewWriter(f)
	for _, name := range names {
		if err := safeName(name); err != nil {
			return err
		}
		e := entries[name]
		h := &zip.FileHeader{Name: "yunkun-" + version + "/" + name, Method: zip.Deflate}
		h.SetMode(e.mode)
		h.SetModTime(time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC))
		w, err := z.CreateHeader(h)
		if err != nil {
			return err
		}
		if _, err := w.Write(e.data); err != nil {
			return err
		}
	}
	if err := z.Close(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	sum, err := hashFile(archive)
	if err != nil {
		return err
	}
	if err := os.WriteFile(archive+".sha256", []byte(sum+"  "+filepath.Base(archive)+"\n"), 0644); err != nil {
		return err
	}
	done = true
	fmt.Printf("%s\nSHA256 %s\n%d reviewed entries; no installation directory read\n", archive, sum, len(names))
	return nil
}

func safeName(name string) error {
	if name == "" || strings.Contains(name, "\\") || strings.HasPrefix(name, "/") ||
		path.Clean(name) != name || name == ".." || strings.HasPrefix(name, "../") || strings.Contains(name, ":") {
		return fmt.Errorf("unsafe archive path: %s", name)
	}
	for _, part := range strings.Split(name, "/") {
		switch strings.ToLower(part) {
		case ".yunkun-local", ".git", ".tools", "node_modules", "__pycache__", ".env",
			"config.json", "installation.json", "postgres-data", "postmaster.pid":
			return fmt.Errorf("private/generated entry rejected: %s", name)
		}
		if strings.HasPrefix(part, ".yunkun-local") || strings.HasSuffix(part, ".log") ||
			strings.HasSuffix(part, ".dump") || strings.HasSuffix(part, ".bak") {
			return fmt.Errorf("private/generated entry rejected: %s", name)
		}
	}
	return nil
}

func regularFile(p string) ([]byte, error) {
	st, err := os.Lstat(p)
	if err != nil {
		return nil, err
	}
	if !st.Mode().IsRegular() || st.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("not a regular release file: %s", p)
	}
	return os.ReadFile(p)
}

func hashFile(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func readRuntime(archive string, spec runtimeSource) (map[string]entry, error) {
	hash, err := hashFile(archive)
	if err != nil {
		return nil, err
	}
	if hash != spec.hash {
		return nil, errors.New("cached runtime archive checksum mismatch")
	}
	f, err := os.Open(archive)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	defer gz.Close()
	t := tar.NewReader(gz)
	root := "yunkun-v1.0.3-offline-" + spec.label + "/"
	out := make(map[string]entry)
	var total int64
	for {
		h, err := t.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		if !strings.HasPrefix(h.Name, root) {
			return nil, errors.New("unexpected runtime archive root")
		}
		name := strings.TrimPrefix(h.Name, root)
		if !strings.HasPrefix(name, "runtime/") && name != "POSTGRESQL_LICENSE.txt" {
			continue
		}
		if h.Typeflag == tar.TypeDir {
			continue
		}
		if err := safeName(name); err != nil {
			return nil, err
		}
		// These pinned archives contain ordinary files only; reject new link behavior.
		if h.Typeflag != tar.TypeReg || h.Size < 0 || h.Size > 256<<20 || total+h.Size > 1<<30 {
			return nil, fmt.Errorf("unexpected runtime entry: %s", name)
		}
		if _, exists := out[name]; exists {
			return nil, fmt.Errorf("duplicate runtime entry: %s", name)
		}
		total += h.Size
		b, err := io.ReadAll(t)
		if err != nil {
			return nil, err
		}
		mode := fs.FileMode(0644)
		if h.Mode&0111 != 0 {
			mode = 0755
		}
		out[name] = entry{b, mode}
	}
	if _, ok := out["runtime/share/postgresql/postgres.bki"]; !ok {
		return nil, errors.New("runtime is missing PostgreSQL initialization data")
	}
	out["THIRD_PARTY_NOTICES.txt"] = entry{[]byte(
		"PostgreSQL 18.4 is distributed under PostgreSQL License (POSTGRESQL_LICENSE.txt).\n" +
			"Runtime packaging: Zonky Embedded Postgres Binaries, Apache License 2.0.\n" +
			"https://github.com/zonkyio/embedded-postgres-binaries\n" +
			"Upstream notices inside runtime/ are retained. Project attribution: NOTICE.md and LICENSE.\n"), 0644}
	return out, nil
}

func validateBinary(b []byte, goos, arch string) error {
	if goos == "linux" {
		f, err := elf.NewFile(bytes.NewReader(b))
		if err != nil {
			return err
		}
		defer f.Close()
		expected := elf.EM_X86_64
		if arch == "arm64" {
			expected = elf.EM_AARCH64
		}
		if f.Machine != expected {
			return errors.New("wrong ELF architecture")
		}
		return nil
	}
	f, err := macho.NewFile(bytes.NewReader(b))
	if err != nil {
		return err
	}
	defer f.Close()
	expected := macho.CpuAmd64
	if arch == "arm64" {
		expected = macho.CpuArm64
	}
	if f.Cpu != expected {
		return errors.New("wrong Mach-O architecture")
	}
	return nil
}

func checkCoreDependencies(entries map[string]entry, goos, arch string) ([]string, error) {
	queue := []string{"runtime/bin/postgres", "runtime/bin/initdb", "runtime/bin/pg_ctl"}
	visited := map[string]bool{}
	system := map[string]bool{}
	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]
		if visited[name] {
			continue
		}
		visited[name] = true
		e, ok := entries[name]
		if !ok || strings.HasPrefix(name, "runtime/bin/") && e.mode&0111 == 0 {
			return nil, fmt.Errorf("missing executable runtime dependency: %s", name)
		}
		var imports []string
		if goos == "linux" {
			f, err := elf.NewFile(bytes.NewReader(e.data))
			if err != nil {
				return nil, err
			}
			imports, err = f.ImportedLibraries()
			f.Close()
			if err != nil {
				return nil, err
			}
		} else {
			fat, err := macho.NewFatFile(bytes.NewReader(e.data))
			var f *macho.File
			if err == nil {
				for _, a := range fat.Arches {
					if arch == "amd64" && a.Cpu == macho.CpuAmd64 || arch == "arm64" && a.Cpu == macho.CpuArm64 {
						f = a.File
					}
				}
				defer fat.Close()
			} else {
				f, err = macho.NewFile(bytes.NewReader(e.data))
				if err == nil {
					defer f.Close()
				}
			}
			if f == nil {
				return nil, fmt.Errorf("runtime Mach-O lacks requested CPU: %s", name)
			}
			imports, err = f.ImportedLibraries()
			if err != nil {
				return nil, err
			}
		}
		for _, lib := range imports {
			base := path.Base(lib)
			bundled := "runtime/lib/" + base
			if _, ok := entries[bundled]; ok {
				queue = append(queue, bundled)
				continue
			}
			if isSystemLibrary(goos, lib) {
				system[lib] = true
				continue
			}
			return nil, fmt.Errorf("unbundled dependency %s required by %s", lib, name)
		}
	}
	deps := make([]string, 0, len(system))
	for name := range system {
		deps = append(deps, name)
	}
	sort.Strings(deps)
	return deps, nil
}

func isSystemLibrary(goos, lib string) bool {
	if goos == "darwin" {
		return strings.HasPrefix(lib, "/usr/lib/") || strings.HasPrefix(lib, "/System/Library/")
	}
	switch lib {
	case "libc.so.6", "libm.so.6", "libpthread.so.0", "librt.so.1", "libdl.so.2", "libresolv.so.2",
		"libutil.so.1", "ld-linux-x86-64.so.2", "ld-linux-aarch64.so.1":
		return true
	}
	return false
}
