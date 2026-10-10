import base64
import gzip
import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path


SCRIPT = Path(__file__).with_name("build_package_index.py")


def tlpdb_package(name: str, filename: str, license_name: str, source_path: str | None = None) -> str:
    return "\n".join([
        f"name {name}",
        f"catalogue {name}",
        f"catalogue-license {license_name}",
        "revision 12345",
        "containersize 42",
        "containerchecksum " + ("a" * 128),
        "runfiles size=1",
        f" {source_path or f'texmf-dist/tex/latex/{name}/{filename}'}",
        "",
    ]) + "\n"


class BuildPackageIndexTests(unittest.TestCase):
    def setUp(self) -> None:
        self.temp = tempfile.TemporaryDirectory()
        self.root = Path(self.temp.name)
        self.catalog = self.root / "base-catalog.json"
        self.catalog.write_text(json.dumps({
            "schema_version": 3,
            "profile": "base",
            "revision": "base",
            "entries": [{"lookup": {"kind": "xetex", "format": "26", "name": "known.sty"}, "sha256": "a" * 64}],
            "objects": [{"sha256": "a" * 64, "size": 1, "media_type": "text/plain", "provenance": [{}]}],
        }), encoding="utf-8")
        self.tlpdb = self.root / "texlive.tlpdb"
        self.output = self.root / "output"

    def tearDown(self) -> None:
        self.temp.cleanup()

    def run_builder(self, *extra_args: str) -> subprocess.CompletedProcess[str]:
        return subprocess.run(
            [
                sys.executable, str(SCRIPT),
                "--base-catalog", str(self.catalog),
                "--tlpdb", str(self.tlpdb),
                "--profile", "full-tlpdb-test",
                "--output", str(self.output),
                *extra_args,
            ],
            text=True,
            capture_output=True,
            check=False,
        )

    def test_indexes_package_archive_provenance_and_preserves_filename_collisions(self) -> None:
        self.tlpdb.write_text(
            tlpdb_package("package-a", "shared.sty", "lppl1.3c")
            + tlpdb_package("package-b", "shared.sty", "gpl3"),
            encoding="utf-8",
        )
        result = self.run_builder()
        self.assertEqual(result.returncode, 0, result.stderr)
        document = json.loads((self.output / "catalog.json").read_text(encoding="utf-8"))
        index_bytes = gzip.decompress(base64.b64decode(document["package_index_gzip"]))
        index = json.loads(index_bytes)
        self.assertEqual(len(index["shared.sty"]), 2)
        self.assertEqual({record["n"] for record in index["shared.sty"]}, {"package-a", "package-b"})
        self.assertEqual({record["l"] for record in index["shared.sty"]}, {"lppl1.3c", "gpl3"})
        self.assertEqual({record["u"] for record in index["shared.sty"]}, {
            "https://ctan.org/pkg/package-a?lang=en",
            "https://ctan.org/pkg/package-b?lang=en",
        })
        self.assertEqual(document["package_index_entries"], sum(map(len, index.values())))

    def test_excludes_package_manager_tree_but_keeps_tex_distribution_docs(self) -> None:
        self.tlpdb.write_text(
            tlpdb_package("ghostscript-assets", "ucrr8a.pfb", "gpl3", "tlpkg/tlgs/Resource/Font/ucrr8a.pfb")
            + tlpdb_package("mwe", "example-image-a.pdf", "lppl1.3c", "texmf-dist/doc/latex/mwe/example-image-a.pdf"),
            encoding="utf-8",
        )
        result = self.run_builder()
        self.assertEqual(result.returncode, 0, result.stderr)
        document = json.loads((self.output / "catalog.json").read_text(encoding="utf-8"))
        index = json.loads(gzip.decompress(base64.b64decode(document["package_index_gzip"])))
        self.assertNotIn("ucrr8a.pfb", index)
        self.assertEqual(index["example-image-a.pdf"][0]["n"], "mwe")

    def test_excludes_unlicensed_packages_and_writes_a_report(self) -> None:
        self.tlpdb.write_text(tlpdb_package("unknown-license", "unknown.sty", ""), encoding="utf-8")
        result = self.run_builder()
        self.assertEqual(result.returncode, 0, result.stderr)
        document = json.loads((self.output / "catalog.json").read_text(encoding="utf-8"))
        index = json.loads(gzip.decompress(base64.b64decode(document["package_index_gzip"])))
        self.assertNotIn("unknown.sty", index)
        report = json.loads((self.output / "unlicensed-package-report.json").read_text(encoding="utf-8"))
        self.assertEqual(report["excluded_packages"], [{
            "package": "unknown-license",
            "files": ["tex/latex/unknown-license/unknown.sty"],
        }])

    def test_strict_mode_refuses_unlicensed_packages(self) -> None:
        self.tlpdb.write_text(tlpdb_package("unknown-license", "unknown.sty", ""), encoding="utf-8")
        result = self.run_builder("--strict-licenses")
        self.assertEqual(result.returncode, 1)
        self.assertIn("unknown-license", result.stderr)

    def test_accepts_explicit_license_override_with_https_source(self) -> None:
        self.tlpdb.write_text(tlpdb_package("unknown-license", "unknown.sty", ""), encoding="utf-8")
        overrides = self.root / "license-overrides.json"
        overrides.write_text(json.dumps({
            "unknown-license": {"license": "LPPL-1.3c", "source": "https://ctan.org/pkg/unknown-license"},
        }), encoding="utf-8")
        result = self.run_builder("--license-overrides", str(overrides))
        self.assertEqual(result.returncode, 0, result.stderr)
        document = json.loads((self.output / "catalog.json").read_text(encoding="utf-8"))
        index = json.loads(gzip.decompress(base64.b64decode(document["package_index_gzip"])))
        self.assertEqual(index["unknown.sty"][0]["l"], "LPPL-1.3c")
        self.assertEqual(index["unknown.sty"][0]["u"], "https://ctan.org/pkg/unknown-license")


if __name__ == "__main__":
    unittest.main()
