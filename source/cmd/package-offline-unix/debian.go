package main

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

type debSource struct {
	file, hash, member, destination string
}

// Debian bullseye Packages metadata pins these unmodified GCC runtime files.
var gccSources = map[string][]debSource{
	"amd64": {
		{"libstdc++6_10.2.1-6_amd64.deb", "5c155c58935870bf3b4bfe769116841c0d286a74f59eccfd5645693ac23f06b1", "./usr/lib/x86_64-linux-gnu/libstdc++.so.6.0.28", "runtime/lib/libstdc++.so.6"},
		{"libgcc-s1_10.2.1-6_amd64.deb", "e478f2709d8474165bb664de42e16950c391f30eaa55bc9b3573281d83a29daf", "./lib/x86_64-linux-gnu/libgcc_s.so.1", "runtime/lib/libgcc_s.so.1"},
	},
	"arm64": {
		{"libstdc++6_10.2.1-6_arm64.deb", "7869aa540cc46e9f3d4267d5bde2af0e5b429a820c1d6f1a4cfccfe788c31890", "./usr/lib/aarch64-linux-gnu/libstdc++.so.6.0.28", "runtime/lib/libstdc++.so.6"},
		{"libgcc-s1_10.2.1-6_arm64.deb", "e2fcdb378d3c1ad1bcb64d4fb6b37aab44011152beca12a4944f435a2582df1f", "./lib/aarch64-linux-gnu/libgcc_s.so.1", "runtime/lib/libgcc_s.so.1"},
	},
}

var gccCopyright = debSource{
	"gcc-10-base_10.2.1-6_amd64.deb", "be65535e94f95fbf04b104e8ab36790476f063374430f7dfc6c516cbe2d2cd1e",
	"./usr/share/doc/gcc-10-base/copyright", "runtime/licenses/GCC-COPYRIGHT.txt",
}

func addGCC(entries map[string]entry, cache, arch string) ([]map[string]string, error) {
	specs, ok := gccSources[arch]
	if !ok || cache == "" {
		return nil, errors.New("Linux requires -system-runtime-cache with the pinned Debian GCC runtime packages")
	}
	specs = append(append([]debSource{}, specs...), gccCopyright)
	var provenance []map[string]string
	for _, spec := range specs {
		b, err := regularFile(filepath.Join(cache, spec.file))
		if err != nil {
			return nil, err
		}
		if fmt.Sprintf("%x", sha256.Sum256(b)) != spec.hash {
			return nil, fmt.Errorf("Debian runtime checksum mismatch: %s", spec.file)
		}
		tarball, err := debMember(b, "data.tar.xz")
		if err != nil {
			return nil, err
		}
		cmd := exec.Command("tar", "-xOf", "-", spec.member)
		cmd.Stdin = bytes.NewReader(tarball)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		data, err := cmd.Output()
		if err != nil {
			return nil, fmt.Errorf("extract pinned Debian member: %w: %s", err, stderr.String())
		}
		if len(data) == 0 || len(data) > 8<<20 {
			return nil, errors.New("invalid Debian runtime member size")
		}
		mode := fs.FileMode(0644)
		if strings.HasPrefix(spec.destination, "runtime/lib/") {
			if err := validateBinary(data, "linux", arch); err != nil {
				return nil, err
			}
			mode = 0755
		}
		entries[spec.destination] = entry{data, mode}
		provenance = append(provenance, map[string]string{
			"url":    "https://deb.debian.org/debian/pool/main/g/gcc-10/" + spec.file,
			"sha256": spec.hash, "member": spec.member, "destination": spec.destination,
		})
	}
	license, err := regularFile(filepath.Join(cache, "GPL-3.0.txt"))
	if err != nil {
		return nil, err
	}
	if fmt.Sprintf("%x", sha256.Sum256(license)) != "3972dc9744f6499f0f9b2dbf76696f2ae7ad8af9b23dde66d6af86c9dfb36986" {
		return nil, errors.New("GPLv3 license checksum mismatch")
	}
	entries["runtime/licenses/GPL-3.0.txt"] = entry{license, 0644}
	notice := entries["THIRD_PARTY_NOTICES.txt"]
	notice.data = append(notice.data, []byte(
		"\nAdditional unmodified Debian GCC 10.2.1-6 libstdc++ and libgcc runtime libraries:\n"+
			"Copyright Free Software Foundation, Inc. GPLv3 or later with GCC Runtime Library Exception 3.1.\n"+
			"Full copyright and exception: runtime/licenses/GCC-COPYRIGHT.txt; GPL: runtime/licenses/GPL-3.0.txt.\n"+
			"Corresponding source accompanies this release in third-party-gcc-10.2.1-6-source.zip.\n"+
			"Original source directory: https://deb.debian.org/debian/pool/main/g/gcc-10/\n")...)
	entries["THIRD_PARTY_NOTICES.txt"] = notice
	return provenance, nil
}

// debMember reads the fixed-width ar container, without filesystem extraction.
func debMember(b []byte, wanted string) ([]byte, error) {
	if !bytes.HasPrefix(b, []byte("!<arch>\n")) {
		return nil, errors.New("invalid Debian ar signature")
	}
	for offset := 8; offset < len(b); {
		if len(b)-offset < 60 {
			return nil, errors.New("truncated Debian ar header")
		}
		h := b[offset : offset+60]
		if string(h[58:]) != "`\n" {
			return nil, errors.New("invalid Debian ar header")
		}
		size, err := strconv.ParseInt(strings.TrimSpace(string(h[48:58])), 10, 64)
		offset += 60
		if err != nil || size < 0 || size > 128<<20 || size > int64(len(b)-offset) {
			return nil, errors.New("invalid Debian ar member size")
		}
		name := strings.TrimSuffix(strings.TrimSpace(string(h[:16])), "/")
		if name == wanted {
			return b[offset : offset+int(size)], nil
		}
		offset += int(size) + int(size%2)
	}
	return nil, errors.New("missing Debian data.tar.xz")
}
