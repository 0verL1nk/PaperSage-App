import tempfile
import unittest
from pathlib import Path

import export_updmap_resource as exporter


class ParseUpdmapLogTests(unittest.TestCase):
    def test_parses_exact_config_and_selected_map_inputs(self) -> None:
        log = """updmap version r78104 (2026-02-24)
Using the following config files:
  /tl/texmf-dist/web2c/updmap.cfg

Scanning for KanjiMap entries
/tl/texmf-dist/fonts/map/dvipdfmx/ptex-fontmaps/ptex.map

    [ 1 file]

Scanning for Map entries
"""
        version, config, inputs = exporter.parse_log(log, "KanjiMap")
        self.assertEqual(version, "r78104 (2026-02-24)")
        self.assertEqual(config, "/tl/texmf-dist/web2c/updmap.cfg")
        self.assertEqual(
            inputs,
            [
                "/tl/texmf-dist/web2c/updmap.cfg",
                "/tl/texmf-dist/fonts/map/dvipdfmx/ptex-fontmaps/ptex.map",
            ],
        )

    def test_zero_map_files_still_tracks_configuration(self) -> None:
        log = """updmap version r78104
Using the following config files:
  /tl/texmf-dist/web2c/updmap.cfg

Scanning for KanjiMap entries

    [ 0 files]
"""
        _, _, inputs = exporter.parse_log(log, "KanjiMap")
        self.assertEqual(inputs, ["/tl/texmf-dist/web2c/updmap.cfg"])

    def test_rejects_incomplete_or_untracked_log_sections(self) -> None:
        incomplete = """updmap version r1
Using the following config files:
  /tl/texmf-dist/web2c/updmap.cfg
"""
        with self.assertRaisesRegex(ValueError, "no KanjiMap input section"):
            exporter.parse_log(incomplete, "KanjiMap")
        with self.assertRaisesRegex(ValueError, "count mismatch"):
            exporter.parse_log(
                """updmap version r1
Using the following config files:
  /tl/texmf-dist/web2c/updmap.cfg
Scanning for KanjiMap entries
/tl/texmf-dist/map.map
[ 2 files]
""",
                "KanjiMap",
            )

    def test_export_copies_artifact_and_hashes_provenanced_inputs(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            texmf = root / "texmf-dist"
            (texmf / "web2c").mkdir(parents=True)
            config = texmf / "web2c" / "updmap.cfg"
            config.write_text("KanjiMap sample.map\n", encoding="utf-8")
            log = root / "updmap.log"
            log.write_text(
                """updmap version r1
Using the following config files:
  """ + str(config) + """

Scanning for KanjiMap entries

    [ 0 files]
""",
                encoding="utf-8",
            )
            artifact = root / "kanjix.map"
            artifact.write_text("% generated\n", encoding="utf-8")
            script = root / "updmap.pl"
            script.write_text("# pinned generator\n", encoding="utf-8")
            artifact_output = root / "derived" / "kanjix.map"
            manifest_path = exporter.export_resource(
                log_path=log,
                artifact_path=artifact,
                artifact_output=artifact_output,
                texmf_dist=texmf,
                updater_script=script,
                lookup_kind="dvipdfmx",
                lookup_format="11",
                lookup_name="kanjix.map",
                map_type="KanjiMap",
            )
            self.assertEqual(artifact_output.read_bytes(), artifact.read_bytes())
            document = __import__("json").loads(manifest_path.read_text(encoding="utf-8"))
            self.assertEqual(document["generator"]["name"], "updmap-sys")
            self.assertEqual(document["resources"][0]["inputs"], ["web2c/updmap.cfg"])

    def test_rejects_inputs_outside_the_texmf_tree(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            inside = root / "texmf-dist"
            inside.mkdir()
            outside = root / "outside.map"
            outside.write_text("map", encoding="utf-8")
            with self.assertRaisesRegex(ValueError, "outside TeX Live"):
                exporter.confined_file(inside.resolve(), str(outside), "updmap input")


if __name__ == "__main__":
    unittest.main()
