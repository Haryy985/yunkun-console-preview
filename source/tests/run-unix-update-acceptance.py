"""Build synthetic versions and exercise native offline updates, without deployment.

Requires Go/Python and a built embedded UI on a TEST machine, not on a client.
The supplied clean Unix ZIP must match its published SHA256SUMS entry. Results
and private synthetic fixtures stay in a new, explicitly requested directory.
"""

import argparse
import hashlib
import json
import os
from pathlib import Path, PurePosixPath
import platform
import re
import shutil
import stat
import subprocess
import sys
import zipfile


def file_hash(path):
    with Path(path).open("rb") as source:
        return hashlib.file_digest(source, "sha256").hexdigest()


def extract_clean_package(archive, destination):
    seen, roots, total = set(), set(), 0
    with zipfile.ZipFile(archive) as package:
        if len(package.infolist()) > 25000:
            raise ValueError("Package entry limit exceeded")
        entries = []
        for item in package.infolist():
            raw = item.filename.rstrip("/")
            parts = raw.split("/")
            if (len(parts) < 2 or "\\" in raw or ":" in raw or
                    any(p in ("", ".", "..") for p in parts)):
                # ZIP root directory entries carry no data.
                if item.is_dir() and len(parts) == 1 and parts[0] not in ("", ".", ".."):
                    continue
                raise ValueError("Unsafe package path")
            roots.add(parts[0])
            rel = PurePosixPath(*parts[1:])
            if any(p.lower().startswith(".yunkun-local") or
                   p.lower() in ("config.json", ".env", ".git") or p.lower().endswith(".key")
                   for p in rel.parts):
                raise ValueError("Private data is not allowed in acceptance inputs")
            mode = item.external_attr >> 16
            if stat.S_ISLNK(mode) or (stat.S_IFMT(mode) not in (0, stat.S_IFREG, stat.S_IFDIR)):
                raise ValueError("Links and special files are not allowed")
            if item.is_dir():
                continue
            key = str(rel).casefold()
            if key in seen:
                raise ValueError("Duplicate package path")
            seen.add(key)
            total += item.file_size
            if total > 4 << 30 or item.file_size > 1 << 30:
                raise ValueError("Expanded package limit exceeded")
            entries.append((item, rel, mode & 0o755))
        if len(roots) != 1:
            raise ValueError("Expected one package root")
        for required in ("bin/yunkun", "bin/yunkun-offline", "runtime/bin/pg_ctl",
                         "runtime/bin/postgres", "runtime/bin/initdb", "package.json"):
            if required not in seen:
                raise ValueError("Not a complete Unix offline package")
        destination.mkdir(parents=True, exist_ok=False)
        for item, rel, mode in entries:
            target = destination.joinpath(*rel.parts)
            target.parent.mkdir(parents=True, exist_ok=True)
            with package.open(item) as source, target.open("xb") as output:
                shutil.copyfileobj(source, output)
            target.chmod(mode or 0o600)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--package", type=Path, required=True)
    parser.add_argument("--sha256", required=True)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    if platform.system() not in ("Linux", "Darwin") or os.geteuid() == 0:
        raise RuntimeError("Use a native Linux/macOS test machine, as a regular user without sudo")
    if not re.fullmatch(r"[a-fA-F0-9]{64}", args.sha256) or file_hash(args.package) != args.sha256.lower():
        raise ValueError("Base package SHA-256 mismatch; no fixture was created")
    os.umask(0o077)
    output = args.output.absolute()
    if output.exists() or output.is_symlink():
        raise ValueError("Output must be a NEW directory, never a customer installation")
    output.mkdir(parents=True)
    output = output.resolve()
    source = Path(__file__).resolve().parent.parent
    report = dict(passed=False, os=platform.system(), machine=platform.machine(),
                  base_sha256=args.sha256.lower(), stages=[],
                  scope="same-source synthetic versions; not historical release migration or Gatekeeper acceptance")
    env = {k: v for k, v in os.environ.items() if not
           (k.startswith(("YUNKUN_", "ARTEX_", "PG", "DYLD_")) or
            k in ("GOOS", "GOARCH", "LD_PRELOAD", "LD_LIBRARY_PATH"))}
    env["CGO_ENABLED"] = "0"
    log_path = output / "acceptance.log"

    def run(command, timeout=900, extra=None):
        with log_path.open("ab") as log:
            try:
                subprocess.run([str(a) for a in command], cwd=source, env=env | (extra or {}),
                               stdout=log, stderr=subprocess.STDOUT, check=True, timeout=timeout)
            except (subprocess.CalledProcessError, subprocess.TimeoutExpired):
                # All fixtures are synthetic. Export only exception class and source
                # locations; raw launcher/worker logs may include generated secrets.
                lines = log_path.read_text(errors="replace").splitlines()
                clues = [line.strip()[:500] for line in lines[-100:]
                         if line.lstrip().startswith(('Traceback', 'File "', 'RuntimeError:',
                                                       'AssertionError', 'OSError:',
                                                       'PermissionError:', 'FileNotFoundError:'))]
                diagnostic = {"stage": report["stages"][-1] if report["stages"] else "setup",
                              "exceptions": clues[-12:]}
                (output / "failure-diagnostics.json").write_text(json.dumps(diagnostic, indent=2))
                raise

    try:
        template = output / "clean-template"
        extract_clean_package(args.package, template)
        goos, goarch = subprocess.check_output(
            ["go", "env", "GOOS", "GOARCH"], cwd=source, env=env, text=True).split()
        manifest = json.loads((template / "PACKAGE.json").read_text())
        if manifest["platform"] != f"{goos}-{goarch}":
            raise ValueError("Base package does not match this native OS/CPU")
        report["platform"] = manifest["platform"]
        report["source_commit"] = subprocess.check_output(
            ["git", "rev-parse", "HEAD"], cwd=source, text=True).strip()
        snapshot = {}
        for pattern in ("selfupdate/*.go", "cmd/yunkun/*.go", "cmd/yunkun-offline/*.go",
                        "scripts/offline-*", "tests/*unix*.py", "go.mod", "go.sum"):
            for path in sorted(source.glob(pattern)):
                snapshot[str(path.relative_to(source))] = file_hash(path)
        (output / "source-sha256.json").write_text(json.dumps(snapshot, indent=2))
        if not (source / "server/webui/current/login/index.html").is_file():
            raise RuntimeError("Build web/out and copy it to server/webui/current before native acceptance")
        old, candidate = output / "installed with spaces", output / "candidate with spaces"
        for root, version in ((old, "v98.0.0-offline"), (candidate, "v98.0.1-offline")):
            shutil.copytree(template, root)
            for name in ("yunkun", "yunkun-offline"):
                run(["go", "build", "-trimpath", "-tags", "embedui", "-ldflags", f"-X main.version={version}",
                     "-o", root / "bin" / name, f"./cmd/{name}"])
            for dest, script in (("start.sh", "offline-unix-start.sh"), ("stop.sh", "offline-unix-stop.sh"),
                                 ("check.sh", "offline-unix-check.sh")):
                shutil.copyfile(source / "scripts" / script, root / dest)
                (root / dest).chmod(0o700)
            if goos == "darwin":
                shutil.copyfile(source / "scripts/offline-macos-start.command", root / "Start-Yunkun.command")
                (root / "Start-Yunkun.command").chmod(0o700)
            (root / "VERSION").write_text(version + "\n")
            fixture_manifest = manifest | {"version": version, "synthetic_acceptance_fixture": True}
            (root / "PACKAGE.json").write_text(json.dumps(fixture_manifest, indent=2))
            sums = [f"{file_hash(p)}  {p.relative_to(root).as_posix()}"
                    for p in sorted(root.rglob("*")) if p.is_file() and p.name != "FILES.sha256"]
            (root / "FILES.sha256").write_text("\n".join(sums) + "\n")
        report["stages"].append("native fixture builds")
        run(["go", "test", "./selfupdate", "./cmd/yunkun-offline", "-count=1", "-v"],
            extra={"YUNKUN_NATIVE_CANDIDATE": str(candidate)})
        report["stages"].append("unit tests and real native candidate preparation")
        # First-install testing owns a separate copy and never shares a database.
        fresh = output / "fresh install with spaces"
        shutil.copytree(candidate, fresh)
        run([sys.executable, source / "tests/unix-offline-smoke.py", fresh, output / "fresh-install.json"])
        report["stages"].append("native first install and restart")
        run([sys.executable, source / "tests/offline-oneclick-unix-smoke.py",
             old, candidate, output / "upgrade-rollback.json"])
        report["stages"].append("native rollback then successful update; data and keys preserved")
        report["passed"] = True
    finally:
        (output / "acceptance.json").write_text(json.dumps(report, indent=2))
    print(f"PASS: {report['platform']}; results in {output}")


if __name__ == "__main__":
    main()
