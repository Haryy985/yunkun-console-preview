"""Exercise the installer's embedded extractor without root or network access."""

import argparse
import hashlib
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
import zipfile


SOURCE = Path(__file__).resolve().parents[1] / "scripts" / "install-vps.sh"
START = "python3.11 - \"$ARCHIVE\" \"$INSTALL\" \"$PLATFORM\" <<'PY'\n"
REQUIRED = (
    "bin/yunkun", "bin/yunkun-offline", "runtime/bin/postgres",
    "runtime/bin/initdb", "runtime/bin/pg_ctl", "start.sh", "check.sh", "stop.sh",
)
VERSION = "v1.1.3-offline"
ROOT = "yunkun-" + VERSION + "/"


def extractor():
    text = SOURCE.read_text(encoding="utf-8").replace("\r\n", "\n")
    return text.split(START, 1)[1].split("\nPY\n", 1)[0]


def make_archive(path, extra=None, corrupt_hash=False):
    files = {name: b"test" for name in REQUIRED}
    files["VERSION"] = (VERSION + "\n").encode()
    files["PACKAGE.json"] = json.dumps({
        "platform": "linux-amd64", "version": VERSION, "data_included": False
    }).encode()
    if extra:
        files.update(extra)
    files["FILES.sha256"] = "".join(
        hashlib.sha256(content).hexdigest() + "  " + name + "\n"
        for name, content in sorted(files.items())
    ).encode()
    if corrupt_hash:
        files["bin/yunkun"] = b"changed"
    with zipfile.ZipFile(path, "w") as archive:
        for name, content in files.items():
            archive.writestr(ROOT + name, content)


def extract(path, target, platform="linux-amd64"):
    return subprocess.run(
        [sys.executable, "-c", extractor(), str(path), str(target), platform],
        capture_output=True, text=True, check=False,
    )


class InstallerExtractionTest(unittest.TestCase):
    def test_valid_package(self):
        with tempfile.TemporaryDirectory() as temp:
            path = Path(temp) / "package.zip"
            make_archive(path)
            target = Path(temp) / "install"
            result = extract(path, target)
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual((target / "VERSION").read_text().strip(), VERSION)

    def test_traversal_rejected_before_installation(self):
        with tempfile.TemporaryDirectory() as temp:
            path = Path(temp) / "package.zip"
            make_archive(path, {"../outside": b"bad"})
            target = Path(temp) / "install"
            self.assertNotEqual(extract(path, target).returncode, 0)
            self.assertFalse(target.exists())

    def test_private_data_rejected_before_installation(self):
        with tempfile.TemporaryDirectory() as temp:
            path = Path(temp) / "package.zip"
            make_archive(path, {".yunkun-local/installation.json": b"secret"})
            target = Path(temp) / "install"
            self.assertNotEqual(extract(path, target).returncode, 0)
            self.assertFalse(target.exists())

    def test_inner_hash_mismatch(self):
        with tempfile.TemporaryDirectory() as temp:
            path = Path(temp) / "package.zip"
            make_archive(path, corrupt_hash=True)
            self.assertNotEqual(extract(path, Path(temp) / "install").returncode, 0)

    def test_duplicate_entry_rejected(self):
        with tempfile.TemporaryDirectory() as temp:
            path = Path(temp) / "package.zip"
            make_archive(path)
            with zipfile.ZipFile(path, "a") as archive:
                archive.writestr(ROOT + "VERSION", b"bad")
            self.assertNotEqual(extract(path, Path(temp) / "install").returncode, 0)


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--package", type=Path)
    parser.add_argument("--platform", default="linux-amd64")
    args, unittest_args = parser.parse_known_args()
    result = unittest.main(argv=[sys.argv[0], *unittest_args], exit=False)
    if not result.result.wasSuccessful() or args.package is None:
        sys.exit(not result.result.wasSuccessful())
    with tempfile.TemporaryDirectory() as temp:
        target = Path(temp) / "install"
        extracted = extract(args.package, target, args.platform)
        if extracted.returncode:
            sys.exit(extracted.stderr)
        print("Real archive extracted and verified:", args.package.name)
