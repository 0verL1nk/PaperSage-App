import gzip
import hashlib
import json
import lzma
import os
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path


SCRIPT = Path(__file__).with_name("build_profile.py")

KPSEWHICH = """#!{python}
import os, sys
args = sys.argv[1:]
if args == ["--help-formats"]:
    print("  tex: .tex [variables: TEXINPUTS]")
    print("  sty: .sty [variables: TEXINPUTS]")
    raise SystemExit(0)
name = args[-1]
root = os.environ["FAKE_TEXMF_DIST"]
if name == "shared.sty":
    print(os.path.join(root, "tex/latex/shared/shared.sty"))
    raise SystemExit(0)
raise SystemExit(1)
"""

FC_SCAN = """#!{python}
import sys
print("Fam-Regular|Fam Regular|Fam|Regular")
"""


class BuildProfileTests(unittest.TestCase):
    def setUp(self) -> None:
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.texmf = self.root / "texmf-dist"
        (self.texmf / "tex/latex/shared").mkdir(parents=True)
        (self.texmf / "tex/latex/shared/shared.sty").write_text("% shared\n")
        (self.texmf / "fonts/opentype/public/fam").mkdir(parents=True)
        (self.texmf / "fonts/opentype/public/fam/Fam.otf").write_bytes(b"OTTO")
        self.kpsewhich = self.script("kpsewhich", KPSEWHICH)
        self.fc_scan = self.script("fc-scan", FC_SCAN)
        self.tlpdb = self.root / "texlive.tlpdb.xz"
        self.tlpdb.write_bytes(lzma.compress(b"name shared\n\n"))

    def script(self, name: str, body: str) -> Path:
        path = self.root / name
        path.write_text(body.replace("{python}", sys.executable))
        path.chmod(0o755)
        return path

    def run_build(self, *extra: str) -> subprocess.CompletedProcess[str]:
        environment = dict(os.environ, FAKE_TEXMF_DIST=str(self.texmf))
        return subprocess.run(
            [
                sys.executable, str(SCRIPT),
                "--snapshot-url", "https://example.test/tlnet/",
                "--profile", "tl2026-test",
                "--output", str(self.root / "out"),
                "--tlpdb", str(self.tlpdb),
                "--kpsewhich", str(self.kpsewhich),
                "--fc-scan", str(self.fc_scan),
                "--texmf-dist", str(self.texmf),
                *extra,
            ],
            capture_output=True, text=True, check=False, env=environment,
        )

    def test_builds_profile_with_overrides_fonts_and_derived(self) -> None:
        ambiguous = self.root / "ambiguous.txt"
        ambiguous.write_text("shared.sty\n")
        derived_root = self.root / "derived-src"
        derived_root.mkdir()
        (derived_root / "hyphen.cfg").write_text("% generated\n")
        manifest = self.root / "derived.json"
        manifest.write_text(json.dumps({
            "generator": {"name": "tlmgr", "version": "TeX Live 2026", "executable_sha256": "a" * 64},
            "resources": [{
                "lookup": {"kind": "xetex", "format": "26", "name": "hyphen.cfg"},
                "artifact": "hyphen.cfg",
                "inputs": ["texmf-dist/tex/generic/config/language.dat"],
            }],
        }))

        result = self.run_build(
            "--ambiguous", str(ambiguous),
            "--derived-manifest", str(manifest),
            "--derived-root", str(derived_root),
        )
        self.assertEqual(result.returncode, 0, result.stderr)
        output = self.root / "out"
        profile = json.loads((output / "profile.json").read_text())
        self.assertEqual(profile["schema_version"], 1)
        self.assertEqual(profile["profile"], "tl2026-test")
        self.assertEqual(profile["repository"], "https://example.test/tlnet")
        tlpdb = (output / "tlpdb.xz").read_bytes()
        self.assertEqual(profile["tlpdb"]["sha512"], hashlib.sha512(tlpdb).hexdigest())
        self.assertEqual(profile["tlpdb"]["size"], len(tlpdb))
        self.assertTrue(profile["format_suffixes"])

        compressed = (output / "lookups.json.gz").read_bytes()
        self.assertEqual(profile["lookups"]["sha256"], hashlib.sha256(compressed).hexdigest())
        lookups = json.loads(gzip.decompress(compressed))
        self.assertEqual(profile["lookups"]["entries"], len(lookups))
        shared = [entry for entry in lookups if entry["n"] == "shared.sty"]
        self.assertEqual(len(shared), 3, "one override per engine kind")
        self.assertEqual(shared[0]["p"], "texmf-dist/tex/latex/shared/shared.sty")
        fonts = [entry for entry in lookups if entry["k"] == "font"]
        self.assertEqual({entry["n"] for entry in fonts}, {"fam", "fam-regular", "fam regular"})
        derived = [entry for entry in lookups if entry["n"] == "hyphen.cfg"]
        self.assertEqual(len(derived), 1)
        artifact = output / "derived" / derived[0]["d"]["sha256"]
        self.assertTrue(artifact.is_file())

        manifest_document = json.loads((output / "profile-manifest.json").read_text())
        self.assertEqual(manifest_document["derived"][0]["generator"]["name"], "tlmgr")
        self.assertIn("built_at", manifest_document)

    def test_unresolved_ambiguous_names_are_skipped(self) -> None:
        # Names no TeX format can reach (e.g. LICENSE.md) get no override; the
        # offline verify still fails if a required resource ends up here.
        ambiguous = self.root / "ambiguous.txt"
        ambiguous.write_text("missing.sty\n")
        result = self.run_build("--ambiguous", str(ambiguous))
        self.assertEqual(result.returncode, 0, result.stderr)
        lookups = json.loads(gzip.decompress((self.root / "out" / "lookups.json.gz").read_bytes()))
        self.assertFalse([entry for entry in lookups if entry["n"] == "missing.sty"])
        manifest = json.loads((self.root / "out" / "profile-manifest.json").read_text())
        self.assertEqual(manifest["lookups"]["unresolved"], 1)

    def test_derived_manifest_requires_generator_identity(self) -> None:
        derived_root = self.root / "derived-src"
        derived_root.mkdir()
        (derived_root / "hyphen.cfg").write_text("% generated\n")
        manifest = self.root / "derived.json"
        manifest.write_text(json.dumps({
            "generator": {"name": "tlmgr", "version": "TeX Live 2026", "executable_sha256": "not-a-hash"},
            "resources": [{
                "lookup": {"kind": "xetex", "format": "26", "name": "hyphen.cfg"},
                "artifact": "hyphen.cfg",
                "inputs": ["texmf-dist/tex/generic/config/language.dat"],
            }],
        }))
        result = self.run_build("--derived-manifest", str(manifest), "--derived-root", str(derived_root))
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("generator identity is invalid", result.stderr)


if __name__ == "__main__":
    unittest.main()
