import tempfile
import unittest
import hashlib
import json
from types import SimpleNamespace
from pathlib import Path
from unittest.mock import patch

from generate_catalog import (
    build_derived_resources,
    candidate_names,
    font_lookup_entries,
    main as generate_catalog_main,
    parse_formats,
    parse_tlpdb,
    relative_file,
    resolve_batch,
    task_batches,
    scan_font_aliases,
    write_catalog_output,
)


class ParseTLPDBTests(unittest.TestCase):
    def parse(self, text: str) -> dict[str, dict[str, str]]:
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "texlive.tlpdb"
            path.write_text(text, encoding="utf-8")
            return parse_tlpdb(path)

    def test_skips_non_archive_installer_stanza(self) -> None:
        packages = self.parse(
            """name 00texlive.installer
runfiles size=1
 RELOC/install-tl

name texlive-scripts
revision 123
containersize 100
containerchecksum """ + "a" * 128 + """
runfiles size=1
 RELOC/install-tl
"""
        )

        self.assertEqual(packages["install-tl"]["name"], "texlive-scripts")


    def test_rejects_ambiguous_archive_backed_path(self) -> None:
        text = """name package-one
revision 1
containersize 10
containerchecksum """ + "a" * 128 + """
runfiles size=1
 texmf-dist/tex/example.sty

name package-two
revision 2
containersize 10
containerchecksum """ + "b" * 128 + """
runfiles size=1
 texmf-dist/tex/example.sty
"""

        with self.assertRaisesRegex(ValueError, "ambiguous package provenance"):
            self.parse(text)

    def test_includes_explicit_names_with_unlisted_tex_suffixes(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            tex_dir = root / "tex" / "latex" / "tex-ini-files"
            tex_dir.mkdir(parents=True)
            (tex_dir / "pdflatex.ini").write_text("", encoding="utf-8")

            names = candidate_names(root, [{"name": "tex", "suffixes": [".tex"]}])

        self.assertIn("pdflatex.ini", names["tex"])

    def test_runtime_candidates_exclude_files_outside_package_runfiles(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            runtime = root / "tex" / "latex" / "sample" / "sample.sty"
            source = root / "source" / "latex" / "sample" / "sample.dtx"
            runtime.parent.mkdir(parents=True)
            source.parent.mkdir(parents=True)
            runtime.write_text("", encoding="utf-8")
            source.write_text("", encoding="utf-8")

            names = candidate_names(
                root,
                [{"name": "tex", "suffixes": [".tex", ".sty", ".dtx"]}],
                {"tex/latex/sample/sample.sty"},
            )

        self.assertIn("sample.sty", names["tex"])
        self.assertNotIn("sample.dtx", names["tex"])

    def test_indexes_font_filename_and_postscript_name(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            font = root / "fonts" / "opentype" / "public" / "sample" / "SamplePS.otf"
            font.parent.mkdir(parents=True)
            font.write_bytes(b"font")
            with patch(
                "generate_catalog.subprocess.run",
                return_value=SimpleNamespace(stdout="SamplePS|Sample Full Name|Sample Family|Regular\n"),
            ):
                aliases = scan_font_aliases(root, "fc-scan")

        self.assertEqual(aliases[("OT", "sampleps")], font)

    def test_emits_fontconfig_lookups_for_filename_and_postscript_aliases(self) -> None:
        font = Path("/texmf-dist/fonts/opentype/public/fandol/FandolSong-Regular.otf")
        entries = font_lookup_entries(
            {
                ("OT", "fandolsong-regular"): font,
                ("OT", "fandolsong"): font,
            },
            [
                {"name": "opentype", "id": "5", "suffixes": [".otf"]},
                {"name": "truetype", "id": "6", "suffixes": [".ttf"]},
            ],
        )

        self.assertEqual(entries[("font", "OT", "fandolsong-regular")], font)
        self.assertEqual(entries[("font", "OT", "fandolsong")], font)
        self.assertEqual(entries[("xetex", "5", "fandolsong")], font)
        self.assertEqual(entries[("dvipdfmx", "5", "fandolsong")], font)


class ParseFormatsTests(unittest.TestCase):
    def test_normalizes_suffix_case_before_deduplicating(self) -> None:
        output = "truetype fonts (truetype): ttf ttc TTF TTC dfont [variables: TEXFONTS]\n"
        with patch("generate_catalog.run_text", return_value=output):
            formats = parse_formats(Path("kpsewhich"))

        self.assertEqual(formats[0]["suffixes"], [".ttf", ".ttc", ".dfont"])


class KpathseaBatchTests(unittest.TestCase):
    def task(self, name: str) -> tuple[str, str, str, str, str, str]:
        return ("kpsewhich", "xetex", "tex", "xetex", "xetex", name)

    def test_batch_maps_one_output_line_to_each_input_in_order(self) -> None:
        tasks = (self.task("alpha.sty"), self.task("beta.sty"))
        with patch(
            "generate_catalog.subprocess.run",
            return_value=SimpleNamespace(returncode=0, stdout="/tree/alpha.sty\n/tree/beta.sty\n"),
        ) as run:
            resolved = resolve_batch(tasks)

        self.assertEqual(
            resolved,
            [
                (tasks[0], ("xetex", "tex", "alpha.sty", "/tree/alpha.sty")),
                (tasks[1], ("xetex", "tex", "beta.sty", "/tree/beta.sty")),
            ],
        )
        self.assertEqual(run.call_count, 1)
        self.assertEqual(run.call_args.args[0][-2:], ["alpha.sty", "beta.sty"])

    def test_nonzero_batch_keeps_unambiguous_hits_and_retries_only_misses(self) -> None:
        tasks = (self.task("present.sty"), self.task("missing.sty"))

        def run(args, **_kwargs):
            if len(args) > 5:
                return SimpleNamespace(returncode=1, stdout="/tree/present.sty\n")
            if args[-1] == "present.sty":
                return SimpleNamespace(returncode=0, stdout="/tree/present.sty\n")
            return SimpleNamespace(returncode=1, stdout="")

        with patch("generate_catalog.subprocess.run", side_effect=run) as mocked:
            resolved = resolve_batch(tasks)

        self.assertEqual(resolved[0][1], ("xetex", "tex", "present.sty", "/tree/present.sty"))
        self.assertIsNone(resolved[1][1])
        self.assertEqual(mocked.call_count, 2)

    def test_batch_retries_hit_when_output_basename_is_ambiguous(self) -> None:
        tasks = (self.task("alpha.sty"), self.task("ALPHA.STY"))

        def run(args, **_kwargs):
            if len(args) > 5:
                return SimpleNamespace(returncode=1, stdout="/tree/ALPHA.STY\n")
            return SimpleNamespace(returncode=0, stdout=f"/exact/{args[-1]}\n")

        with patch("generate_catalog.subprocess.run", side_effect=run) as mocked:
            resolved = resolve_batch(tasks)

        self.assertEqual([item[1][3] for item in resolved if item[1]], ["/exact/alpha.sty", "/exact/ALPHA.STY"])
        self.assertEqual(mocked.call_count, 3)

    def test_malformed_success_batch_falls_back_to_exact_queries(self) -> None:
        tasks = (self.task("alpha.sty"), self.task("beta.sty"))
        calls = 0

        def run(args, **_kwargs):
            nonlocal calls
            calls += 1
            if calls == 1:
                return SimpleNamespace(returncode=0, stdout="/tree/alpha.sty\n")
            return SimpleNamespace(returncode=0, stdout=f"/tree/{args[-1]}\n")

        with patch("generate_catalog.subprocess.run", side_effect=run):
            resolved = resolve_batch(tasks)

        self.assertEqual([item[1][3] for item in resolved if item[1]], ["/tree/alpha.sty", "/tree/beta.sty"])
        self.assertEqual(calls, 2)

    def test_batch_builder_preserves_order_and_respects_compatibility_and_limits(self) -> None:
        one = self.task("a.sty")
        tasks = [one, self.task("b.sty"), ("kpsewhich", "pdftex", "tex", "pdftex", "pdftex", "c.sty")]

        batches = list(task_batches(tasks, max_names=2, max_arg_chars=100))

        self.assertEqual([[task[-1] for task in batch] for batch in batches], [["a.sty", "b.sty"], ["c.sty"]])

    def make_derived_fixture(self, root: Path):
        texmf = root / "texmf-dist"
        generated = root / "texmf-var"
        texmf.mkdir()
        artifact = generated / "runtime" / "generated.map"
        artifact.parent.mkdir(parents=True)
        artifact.write_bytes(b"generated map bytes\n")
        config = texmf / "web2c" / "updmap.cfg"
        config.parent.mkdir(parents=True)
        config.write_text("Map sample.map\n", encoding="utf-8")
        source_map = texmf / "fonts" / "map" / "sample" / "sample.map"
        source_map.parent.mkdir(parents=True)
        source_map.write_bytes(b"font-name font-name <font.otf\n")
        package_meta = {
            "name": "texlive-maps",
            "license": "LPPL-1.3c",
            "revision": "12345",
            "container_size": "100",
            "container_checksum": "a" * 128,
            "catalogue": "texlive-maps",
        }
        packages = {
            "web2c/updmap.cfg": {**package_meta, "name": "texlive-config"},
            "fonts/map/sample/sample.map": package_meta,
        }
        manifest = root / "derived-manifest.json"
        manifest.write_text(json.dumps({
            "generator": {
                "name": "updmap-sys",
                "version": "TeX Live 2026 r80534",
                "executable_sha256": "b" * 64,
            },
            "resources": [{
                "lookup": {"kind": "dvipdfmx", "format": "11", "name": "generated.map"},
                "artifact": "runtime/generated.map",
                "inputs": ["web2c/updmap.cfg", "fonts/map/sample/sample.map"],
            }],
        }), encoding="utf-8")
        return texmf, generated, packages, manifest

    def test_builds_derived_resource_with_generator_and_input_provenance(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            texmf, generated, packages, manifest = self.make_derived_fixture(root)

            result = build_derived_resources(
                manifest,
                generated,
                texmf,
                packages,
                {},
                None,
                None,
            )
            output = root / "catalog-output"
            base_document = {
                "schema_version": 3,
                "profile": "test-profile",
                "revision": "test-profile",
                "objects": [],
                "entries": [{
                    "lookup": {"kind": "xetex", "format": "26", "name": "sample.sty"},
                    "sha256": "c" * 64,
                }],
            }
            catalog_path = write_catalog_output(output, base_document, result, generated)
            written = json.loads(catalog_path.read_text(encoding="utf-8"))
            derived_record = written["derived_resources"][0]
            self.assertEqual(derived_record["sha256"], resource_hash := hashlib.sha256(b"generated map bytes\n").hexdigest())
            self.assertNotIn("artifact_path", derived_record)
            self.assertEqual((output / "derived" / resource_hash).read_bytes(), b"generated map bytes\n")
            conflict_document = {
                **base_document,
                "entries": [{
                    "lookup": {"kind": "dvipdfmx", "format": "11", "name": "generated.map"},
                    "sha256": "c" * 64,
                }],
            }
            with self.assertRaisesRegex(ValueError, "conflicts with existing lookup"):
                write_catalog_output(root / "conflicting-output", conflict_document, result, generated)

        self.assertEqual(len(result), 1)
        resource = result[0]
        self.assertEqual(resource["lookup"], {"kind": "dvipdfmx", "format": "11", "name": "generated.map"})
        self.assertEqual(resource["sha256"], hashlib.sha256(b"generated map bytes\n").hexdigest())
        self.assertEqual(resource["object_key"], f"derived/{resource['sha256']}")
        self.assertEqual(resource["generator"]["name"], "updmap-sys")
        self.assertEqual([item["source_path"] for item in resource["inputs"]], [
            "texmf-dist/web2c/updmap.cfg",
            "texmf-dist/fonts/map/sample/sample.map",
        ])
        self.assertEqual(resource["inputs"][0]["package"], "texlive-config")
        self.assertEqual(len(resource["inputs"][0]["sha256"]), 64)

    def test_rejects_unsafe_paths_missing_licenses_and_invalid_generator_identity(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            texmf, generated, packages, manifest = self.make_derived_fixture(root)
            original = json.loads(manifest.read_text(encoding="utf-8"))

            self.assertRaisesRegex(ValueError, "stay inside", relative_file, generated, "../outside.map", "artifact")
            outside = root / "outside.map"
            outside.write_bytes(b"outside")
            symlink = generated / "escape.map"
            try:
                symlink.symlink_to(outside)
            except OSError:
                self.skipTest("filesystem does not permit creating a symlink for this test")
            self.assertRaisesRegex(ValueError, "escapes", relative_file, generated, "escape.map", "artifact")

            original["generator"]["executable_sha256"] = "not-a-hash"
            manifest.write_text(json.dumps(original), encoding="utf-8")
            with self.assertRaisesRegex(ValueError, "generator identity"):
                build_derived_resources(manifest, generated, texmf, packages, {}, None, None)

            original["generator"]["executable_sha256"] = "b" * 64
            original["resources"][0]["artifact"] = "../outside.map"
            manifest.write_text(json.dumps(original), encoding="utf-8")
            with self.assertRaisesRegex(ValueError, "stay inside"):
                build_derived_resources(manifest, generated, texmf, packages, {}, None, None)

            original["resources"][0]["artifact"] = "runtime/generated.map"
            original["resources"][0]["inputs"] = ["../outside.map"]
            manifest.write_text(json.dumps(original), encoding="utf-8")
            with self.assertRaisesRegex(ValueError, "stay inside"):
                build_derived_resources(manifest, generated, texmf, packages, {}, None, None)

            original["resources"][0]["inputs"] = ["web2c/updmap.cfg"]
            manifest.write_text(json.dumps(original), encoding="utf-8")
            unlicensed = {"web2c/updmap.cfg": {**packages["web2c/updmap.cfg"], "license": ""}}
            with self.assertRaisesRegex(ValueError, "no license metadata"):
                build_derived_resources(manifest, generated, texmf, unlicensed, {}, None, None)

    def test_cli_writes_derived_catalog_record_and_content_addressed_blob(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            texmf, generated, packages, manifest = self.make_derived_fixture(root)
            tlpdb = root / "texlive.tlpdb"
            tlpdb.write_text(
                "name texlive-config\nrevision 12345\ncatalogue-license LPPL-1.3c\ncontainersize 100\ncontainerchecksum " + "a" * 128
                + "\ncatalogue texlive-config\nrunfiles size=1\n texmf-dist/web2c/updmap.cfg\n\n"
                + "name texlive-maps\nrevision 12345\ncatalogue-license LPPL-1.3c\ncontainersize 100\ncontainerchecksum " + "a" * 128
                + "\ncatalogue texlive-maps\nrunfiles size=1\n texmf-dist/fonts/map/sample/sample.map\n\n"
                + "name sample-package\nrevision 12345\ncatalogue-license LPPL-1.3c\ncontainersize 100\ncontainerchecksum " + "a" * 128
                + "\ncatalogue sample-package\nrunfiles size=1\n texmf-dist/tex/latex/sample/sample.sty\n",
                encoding="utf-8",
            )
            sample = texmf / "tex" / "latex" / "sample" / "sample.sty"
            sample.parent.mkdir(parents=True)
            sample.write_text("\\ProvidesPackage{sample}\n", encoding="utf-8")
            output = root / "catalog-output"

            def fake_run(args, **_kwargs):
                if "--help-formats" in args:
                    return SimpleNamespace(stdout="tex: .tex [variables: TEXINPUTS]\n")
                requested = args[-1]
                if requested == "sample.sty":
                    return SimpleNamespace(returncode=0, stdout=str(sample) + "\n")
                return SimpleNamespace(returncode=1, stdout="")

            argv = [
                "generate_catalog.py",
                "--kpsewhich", "fake-kpsewhich",
                "--texmf-dist", str(texmf),
                "--tlpdb", str(tlpdb),
                "--profile", "test-profile",
                "--derived-manifest", str(manifest),
                "--derived-root", str(generated),
                "--output", str(output),
                "--workers", "1",
            ]
            with patch("generate_catalog.subprocess.run", side_effect=fake_run), patch("sys.argv", argv):
                self.assertEqual(generate_catalog_main(), 0)

            document = json.loads((output / "catalog.json").read_text(encoding="utf-8"))
            self.assertEqual(len(document["derived_resources"]), 1)
            resource = document["derived_resources"][0]
            self.assertEqual(resource["lookup"]["name"], "generated.map")
            self.assertNotIn("artifact_path", resource)
            self.assertIn("inputs", resource)
            blob = output / "derived" / resource["sha256"]
            self.assertEqual(blob.read_bytes(), b"generated map bytes\n")
            self.assertEqual(hashlib.sha256(blob.read_bytes()).hexdigest(), resource["sha256"])


if __name__ == "__main__":
    unittest.main()
