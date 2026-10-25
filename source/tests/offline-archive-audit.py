"""Audit final archives without extracting or reading any customer installation."""

import hashlib
import json
from pathlib import Path, PurePosixPath
import re
import struct
import sys
import zipfile


directory = Path(sys.argv[1])
source = Path(sys.argv[2])
output = Path(sys.argv[3])
root = "yunkun-v1.1.0-offline/"
private_names = {
    ".env", ".git", ".tools", ".yunkun-local", "node_modules", "__pycache__",
    "config.json", "installation.json", "process.json", "postgres-data",
    "postmaster.pid", "jwt.key", "proxy-pool.key", "pgpass", ".pgpass",
}
results = []
skill_files = {
    "skills/" + p.relative_to(source / "skills").as_posix(): hashlib.sha256(p.read_bytes()).hexdigest()
    for p in (source / "skills").rglob("*") if p.is_file()
}
for platform in ("windows-amd64", "linux-amd64", "linux-arm64", "darwin-amd64", "darwin-arm64"):
    archive = directory / f"yunkun-1.1.0-offline-{platform}.zip"
    with zipfile.ZipFile(archive) as z:
        files = {}
        modes = {}
        names = set()
        for item in z.infolist():
            name = item.filename.replace("\\", "/")
            assert name.startswith(root), name
            relative = name[len(root):].rstrip("/")
            if not relative:
                continue
            parts = PurePosixPath(relative).parts
            assert all(p not in ("..", ".") and ":" not in p for p in parts), name
            assert not (private_names & {p.lower() for p in parts}), name
            assert not any(p.startswith(".yunkun-local") for p in parts), name
            assert not relative.endswith((".log", ".dump", ".bak", ".etl")), name
            assert relative not in names, name
            names.add(relative)
            if item.is_dir():
                continue
            files[relative] = hashlib.sha256(z.read(item)).hexdigest()
            modes[relative] = item.external_attr >> 16
            assert (modes[relative] & 0o170000) != 0o120000, name
        assert {k: files[k] for k in files if k.startswith("skills/")} == skill_files, "Skills differ from current source"
        if platform.startswith("windows"):
            for relative in ("bin/yunkun.exe", "runtime/pgsql-18.4/bin/postgres.exe",
                             "runtime/vc_redist.x64.exe", "setup-and-start-offline.ps1",
                             "setup-and-start-offline.bat"):
                assert relative in files, relative
            assert files["bin/yunkun.exe"] == hashlib.sha256(
                Path(r"F:\YunkunOffline\.build\oneclick-20261022\yunkun.exe").read_bytes()
            ).hexdigest(), "Windows application differs from verified update binary"
            ps = z.read(root + "setup-and-start-offline.ps1")
            assert b"NORMA_RIPGREP_NO_INSTALL" in ps and b"PSModulePath" in ps
            assert ps == (source / "setup-and-start-offline.ps1").read_bytes()
            config = json.loads(z.read(root + "config.example.json").decode("utf-8-sig"))
            assert not config.get("database", {}).get("password"), "Example contains database password"
        else:
            manifest = json.loads(z.read(root + "PACKAGE.json"))
            assert manifest["version"] == "v1.1.0-offline"
            assert manifest["platform"] == platform and manifest["data_included"] is False
            checksums = {}
            for line in z.read(root + "FILES.sha256").decode().splitlines():
                digest, name = line.split("  ", 1)
                checksums[name] = digest
            assert checksums == {k: v for k, v in files.items() if k != "FILES.sha256"}, "Package manifest differs"
            for relative in ("bin/yunkun", "bin/yunkun-offline", "start.sh", "stop.sh", "check.sh",
                             "runtime/bin/postgres", "runtime/bin/initdb", "runtime/bin/pg_ctl"):
                assert modes[relative] & 0o111, f"Not executable: {relative}"
            if platform.startswith("linux"):
                for relative in ("runtime/lib/libstdc++.so.6", "runtime/lib/libgcc_s.so.1"):
                    b = z.read(root + relative)
                    assert b[:4] == b"\x7fELF"
                    assert struct.unpack("<H", b[18:20])[0] == (62 if platform.endswith("amd64") else 183)
                needed = set()
                for name in files:
                    if name.startswith(("runtime/bin/", "runtime/lib/")):
                        needed.update(re.findall(rb"GLIBC_([0-9]+\.[0-9]+)(?:\x00|[^.0-9])", z.read(root + name)))
                maximum = max((tuple(map(int, x.decode().split("."))) for x in needed), default=(0, 0))
                assert maximum <= (2, 27), maximum
            else:
                assert modes["Start-Yunkun.command"] & 0o111
        results.append({
            "platform": platform, "archive": archive.name, "bytes": archive.stat().st_size,
            "sha256": hashlib.sha256(archive.read_bytes()).hexdigest(),
            "files": len(files), "skills_match_current_source": True,
            "private_file_scan": "passed", "native_test_implied": False,
        })

assert len(results) == 5
companion = directory / "third-party-gcc-10.2.1-6-source.zip"
with zipfile.ZipFile(companion) as z:
    for name, expected in {
        "gcc-10_10.2.1.orig.tar.xz": "ea3c05faa381486e6b859c047dc14977418bf1ccda4567064e016493fd6fffec",
        "gcc-10_10.2.1-6.debian.tar.xz": "a95d6b9da2be83f9751850b002021281411ff1003d9feb77298b131da47820b3",
    }.items():
        assert hashlib.sha256(z.read(name)).hexdigest() == expected, name
output.parent.mkdir(parents=True, exist_ok=True)
output.write_text(json.dumps({"ok": True, "packages": results}, indent=2) + "\n")
print(json.dumps({"ok": True, "packages": results}, indent=2))
