#!/usr/bin/env python3
"""Export one updmap-generated resource with inputs captured from its log."""

from __future__ import annotations

import argparse
import hashlib
import json
import re
import shutil
from pathlib import Path


def sha256_file(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as source:
        for block in iter(lambda: source.read(1024 * 1024), b""):
            digest.update(block)
    return digest.hexdigest()


def confined_file(root: Path, value: str, label: str) -> tuple[Path, str]:
    candidate = Path(value.strip())
    resolved = candidate.resolve(strict=True)
    try:
        relative = resolved.relative_to(root)
    except ValueError as error:
        raise ValueError(f"{label} is outside TeX Live texmf-dist: {candidate}") from error
    if not resolved.is_file():
        raise ValueError(f"{label} is not a regular file: {candidate}")
    return resolved, relative.as_posix()


def parse_log(log_text: str, map_type: str) -> tuple[str, str, list[str]]:
    version_match = re.search(r"^updmap version (.+)$", log_text, re.MULTILINE)
    if not version_match:
        raise ValueError("updmap log has no version identity")

    config_match = re.search(
        r"^Using the following config files:\s*\r?\n((?:[ \t]+[^\r\n]+\r?\n)+)",
        log_text,
        re.MULTILINE,
    )
    if not config_match:
        raise ValueError("updmap log has no configuration input list")
    configs = [line.strip() for line in config_match.group(1).splitlines() if line.strip()]

    marker = f"Scanning for {map_type} entries"
    start = log_text.find(marker)
    if start < 0:
        raise ValueError(f"updmap log has no {map_type} input section")
    body = log_text[start + len(marker):]
    count_match = re.search(r"^\s*\[\s*(\d+) files?\s*\]\s*$", body, re.MULTILINE)
    if not count_match:
        raise ValueError(f"updmap log has no complete {map_type} input count")
    section = body[:count_match.start()]
    input_paths = [line.strip() for line in section.splitlines() if line.strip()]
    if len(input_paths) != int(count_match.group(1)):
        raise ValueError(
            f"updmap {map_type} input count mismatch: log={count_match.group(1)}, parsed={len(input_paths)}"
        )
    return version_match.group(1).strip(), configs[0], configs + input_paths


def export_resource(
    *,
    log_path: Path,
    artifact_path: Path,
    artifact_output: Path,
    texmf_dist: Path,
    updater_script: Path,
    lookup_kind: str,
    lookup_format: str,
    lookup_name: str,
    map_type: str,
) -> Path:
    dist_root = texmf_dist.resolve(strict=True)
    if not dist_root.is_dir():
        raise ValueError("texmf-dist root must be a directory")
    version, _, input_paths = parse_log(log_path.read_text(encoding="utf-8", errors="strict"), map_type)
    normalized: dict[str, Path] = {}
    for raw_path in input_paths:
        path, relative = confined_file(dist_root, raw_path, "updmap input")
        normalized[relative] = path
    if not normalized:
        raise ValueError("updmap log recorded no inputs")

    artifact = artifact_path.resolve(strict=True)
    if not artifact.is_file() or artifact.stat().st_size == 0:
        raise ValueError("updmap artifact must be a non-empty regular file")
    output = artifact_output.resolve()
    output.parent.mkdir(parents=True, exist_ok=True)
    shutil.copyfile(artifact, output)

    manifest_path = output.parent / "derived-resources.json"
    manifest = {
        "generator": {
            "name": "updmap-sys",
            "version": version,
            "executable_sha256": sha256_file(updater_script.resolve(strict=True)),
        },
        "resources": [
            {
                "lookup": {"kind": lookup_kind, "format": lookup_format, "name": lookup_name},
                "artifact": output.name,
                "inputs": sorted(normalized),
            }
        ],
    }
    manifest_path.write_text(json.dumps(manifest, indent=2) + "\n", encoding="utf-8")
    return manifest_path


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--log", type=Path, required=True)
    parser.add_argument("--artifact", type=Path, required=True)
    parser.add_argument("--artifact-output", type=Path, required=True)
    parser.add_argument("--texmf-dist", type=Path, required=True)
    parser.add_argument("--updmap-script", type=Path, required=True)
    parser.add_argument("--lookup-kind", choices=("pdftex", "xetex", "dvipdfmx"), required=True)
    parser.add_argument("--lookup-format", required=True)
    parser.add_argument("--lookup-name", required=True)
    parser.add_argument("--map-type", choices=("Map", "MixedMap", "KanjiMap"), required=True)
    args = parser.parse_args()
    if not re.fullmatch(r"[A-Za-z0-9._+-]{1,96}", args.lookup_format):
        parser.error("lookup format is invalid")
    if not re.fullmatch(r"[^/\\]{1,255}", args.lookup_name) or args.lookup_name in {".", ".."}:
        parser.error("lookup name is invalid")
    try:
        manifest = export_resource(
            log_path=args.log,
            artifact_path=args.artifact,
            artifact_output=args.artifact_output,
            texmf_dist=args.texmf_dist,
            updater_script=args.updmap_script,
            lookup_kind=args.lookup_kind,
            lookup_format=args.lookup_format,
            lookup_name=args.lookup_name,
            map_type=args.map_type,
        )
    except (OSError, ValueError) as error:
        parser.error(str(error))
    print(f"wrote {manifest}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
