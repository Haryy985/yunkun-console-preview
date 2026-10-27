"""Input validation for native acceptance, runnable on the development host."""

import importlib.util
from pathlib import Path
import stat
import tempfile
import unittest
import zipfile

spec = importlib.util.spec_from_file_location(
    "acceptance", Path(__file__).with_name("run-unix-update-acceptance.py"))
acceptance = importlib.util.module_from_spec(spec)
spec.loader.exec_module(acceptance)


class AcceptanceInputTests(unittest.TestCase):
    required = ("bin/yunkun", "bin/yunkun-offline", "runtime/bin/pg_ctl",
                "runtime/bin/postgres", "runtime/bin/initdb", "PACKAGE.json")

    def fixture(self, root, extra=(), omit=False):
        archive = root / "input.zip"
        with zipfile.ZipFile(archive, "w") as package:
            if not omit:
                for name in self.required:
                    item = zipfile.ZipInfo("package/" + name)
                    item.external_attr = (stat.S_IFREG | 0o755) << 16
                    package.writestr(item, b"fixture")
            for name, mode in extra:
                item = zipfile.ZipInfo(name)
                item.external_attr = mode << 16
                package.writestr(item, b"fixture")
        return archive

    def test_clean_package(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            acceptance.extract_clean_package(self.fixture(root), root / "out")
            self.assertEqual((root / "out/bin/yunkun").read_bytes(), b"fixture")
            self.assertFalse((root / "out/.yunkun-local").exists())

    def test_rejects_before_writing(self):
        for name in ("package/../outside", "/absolute/outside", "package/bin/jwt.key",
                     "package/.yunkun-local/config.json", "package/runtime/config.json",
                     "other/file", "package/BIN/YUNKUN"):
            with self.subTest(name=name), tempfile.TemporaryDirectory() as temp:
                root = Path(temp)
                archive = self.fixture(root, [(name, stat.S_IFREG | 0o600)])
                with self.assertRaises(ValueError):
                    acceptance.extract_clean_package(archive, root / "out")
                self.assertFalse((root / "out").exists())

    def test_links_and_special_files(self):
        for mode in (stat.S_IFLNK, stat.S_IFIFO, stat.S_IFCHR):
            with self.subTest(mode=mode), tempfile.TemporaryDirectory() as temp:
                root = Path(temp)
                with self.assertRaises(ValueError):
                    acceptance.extract_clean_package(
                        self.fixture(root, [("package/runtime/link", mode | 0o700)]), root / "out")
                self.assertFalse((root / "out").exists())

    def test_missing_runtime(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            with self.assertRaises(ValueError):
                acceptance.extract_clean_package(self.fixture(root, omit=True), root / "out")
            self.assertFalse((root / "out").exists())

    def test_existing_destination_preserved(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            out = root / "out"
            out.mkdir()
            marker = out / "preserve"
            marker.write_text("unchanged")
            with self.assertRaises(FileExistsError):
                acceptance.extract_clean_package(self.fixture(root), out)
            self.assertEqual(marker.read_text(), "unchanged")
            self.assertEqual(list(out.iterdir()), [marker])


if __name__ == "__main__":
    unittest.main()
