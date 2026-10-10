#!/usr/bin/env python3
"""Attach a complete, compressed TeX Live filename index to a catalog profile.

The index contains package archive provenance only. Resource bytes remain
on-demand: the Go gateway downloads and verifies one TeX Live package archive
when a unique filename is first requested.
"""

from __future__ import annotations

import argparse
import base64
import gzip
import hashlib
import json
import re
import sys
from pathlib import Path
from urllib.parse import urlsplit

from generate_catalog import parse_tlpdb

MAX_PACKAGE_INDEX_JSON_BYTES = 128 * 1024 * 1024


def load_license_overrides(path: Path | None) -> dict[str, dict[str, str]]:
    if path is None:
        return {}
    document = json.loads(path.read_text(encoding="utf-8"))
    if not isinstance(document, dict):
        raise ValueError("license overrides must be a JSON object")
    overrides: dict[str, dict[str, str]] = {}
    for package, override in document.items():
        source = override.get("source") if isinstance(override, dict) else None
        parsed_source = urlsplit(source) if isinstance(source, str) else None
        if (
            not isinstance(package, str)
            or not package
            or not isinstance(override, dict)
            or not isinstance(override.get("license"), str)
            or not override["license"].strip()
            or parsed_source is None
            or parsed_source.scheme != "https"
            or not parsed_source.netloc
        ):
            raise ValueError(f"invalid license override for {package!r}")
        overrides[package] = {
            "license": override["license"].strip(),
            "source": override["source"],
        }
    return overrides


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--base-catalog", type=Path, required=True)
    parser.add_argument("--tlpdb", type=Path, required=True)
    parser.add_argument("--license-overrides", type=Path)
    parser.add_argument(
        "--strict-licenses",
        action="store_true",
        help="fail instead of excluding packages with no declared or sourced license",
    )
    parser.add_argument("--profile", required=True)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    if not re.fullmatch(r"[A-Za-z0-9._-]{1,96}", args.profile):
        parser.error("invalid profile identifier")

    catalog = json.loads(args.base_catalog.read_text(encoding="utf-8"))
    if catalog.get("schema_version") != 3 or not catalog.get("entries") or not catalog.get("objects"):
        raise ValueError("base catalog schema or contents are invalid")
    package_files = parse_tlpdb(args.tlpdb)
    license_overrides = load_license_overrides(args.license_overrides)
    index: dict[str, list[dict[str, object]]] = {}
    unlicensed_packages: dict[str, set[str]] = {}
    for source_path, metadata in sorted(package_files.items()):
        # TeX Live also ships package-manager and Ghostscript support files in
        # tlpkg. They are not in Kpathsea's search tree and must not create
        # false basename collisions with TeX runtime files.
        if source_path.startswith("tlpkg/"):
            continue
        name = source_path.rsplit("/", 1)[-1]
        package_name = metadata["name"]
        license_name = metadata["license"].strip()
        override = license_overrides.get(package_name)
        if license_name:
            license_source = (
                f"https://ctan.org/pkg/{metadata['catalogue']}?lang=en"
                if re.fullmatch(r"[A-Za-z0-9._+-]{1,128}", metadata["catalogue"])
                else "https://www.tug.org/texlive/copying.html"
            )
        elif override:
            license_name = override["license"]
            license_source = override["source"]
        else:
            unlicensed_packages.setdefault(package_name, set()).add(source_path)
            continue
        record = {
            "p": f"texmf-dist/{source_path}",
            "n": package_name,
            "r": metadata["revision"],
            "s": int(metadata["container_size"]),
            "h": metadata["container_checksum"],
            "l": license_name,
            "u": license_source,
        }
        for key in sorted({name, name.lower()}):
            index.setdefault(key, []).append(record)

    if unlicensed_packages and args.strict_licenses:
        packages = ", ".join(sorted(unlicensed_packages))
        raise ValueError(
            "package index refuses packages without license metadata or an explicit sourced override: "
            f"{packages}"
        )

    encoded_json = json.dumps(index, ensure_ascii=False, separators=(",", ":")).encode("utf-8")
    if len(encoded_json) > MAX_PACKAGE_INDEX_JSON_BYTES:
        raise ValueError(
            "uncompressed package filename index exceeds the server limit "
            f"({len(encoded_json)} > {MAX_PACKAGE_INDEX_JSON_BYTES} bytes)"
        )
    compressed = gzip.compress(encoded_json, compresslevel=9, mtime=0)
    catalog["profile"] = args.profile
    catalog["revision"] = args.profile
    catalog["package_index_gzip"] = base64.b64encode(compressed).decode("ascii")
    catalog["package_index_sha256"] = hashlib.sha256(compressed).hexdigest()
    catalog["package_index_entries"] = sum(len(paths) for paths in index.values())

    args.output.mkdir(parents=True, exist_ok=True)
    output_path = args.output / "catalog.json"
    output_path.write_text(json.dumps(catalog, ensure_ascii=False, separators=(",", ":")) + "\n", encoding="utf-8")
    unlicensed_report = args.output / "unlicensed-package-report.json"
    if unlicensed_packages:
        unlicensed_report.write_text(
            json.dumps({
                "reason": "TeX Live package archive has no catalogue-license; no sourced override was configured",
                "excluded_packages": [
                    {"package": package, "files": sorted(files)}
                    for package, files in sorted(unlicensed_packages.items())
                ],
            }, ensure_ascii=False, indent=2) + "\n",
            encoding="utf-8",
        )
    else:
        unlicensed_report.unlink(missing_ok=True)
    print(
        f"profile={args.profile} package_paths={len(package_files)} index_keys={len(index)} "
        f"index_entries={catalog['package_index_entries']} inflated_bytes={len(encoded_json)} "
        f"compressed_bytes={len(compressed)} "
        f"catalog_bytes={output_path.stat().st_size} "
        f"excluded_unlicensed_packages={len(unlicensed_packages)}",
        flush=True,
    )
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except (OSError, ValueError) as error:
        print(f"package index generation failed: {error}", file=sys.stderr)
        raise SystemExit(1)
