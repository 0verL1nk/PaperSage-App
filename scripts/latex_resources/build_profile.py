#!/usr/bin/env python3
"""Assemble a v2 TeX Live profile from one immutable tlnet snapshot.

The profile is the single source of truth for the resource service: the tlpdb
and every archive checksum come from the same snapshot, so the `pinned 漂移 502`
cannot happen by construction. Kpathsea resolution is delegated to the snapshot's
own `kpsewhich`, exactly like generate_catalog.py.

    python scripts/latex_resources/build_profile.py \
      --snapshot-url https://texlive.info/tlnet-archive/2026/11/22/tlnet \
      --profile tl2026-tlnet-20261122 \
      --output build/profile \
      --tlpdb tlnet/tlpkg/texlive.tlpdb.xz \
      --kpsewhich /usr/local/texlive/2026/bin/x86_64-linux/kpsewhich \
      --fc-scan /usr/bin/fc-scan \
      --texmf-dist /usr/local/texlive/2026/texmf-dist \
      --ambiguous build/ambiguous.txt \
      --derived-manifest build/derived.json \
      --derived-root build/derived

`--ambiguous` comes from `latexprofile list-ambiguous`; each name is resolved
with kpsewhich here so the runtime never guesses between same-named files.
"""

from __future__ import annotations

import argparse
import datetime
import gzip
import hashlib
import json
import mimetypes
import re
import shutil
import subprocess
import sys
from pathlib import Path

from generate_catalog import ENGINES, parse_formats, scan_font_aliases, sha256_file

LOOKUP_NAME = re.compile(r"[^/\\]{1,255}")
GENERATOR = re.compile(r"[A-Za-z0-9][A-Za-z0-9._+-]{0,127}")


def resolve(kpsewhich: Path, kind: str, fmt: dict[str, object], name: str) -> str | None:
    program, engine = ENGINES[kind]
    result = subprocess.run(
        [str(kpsewhich), f"--format={fmt['name']}", f"--progname={program}", f"--engine={engine}", name],
        text=True, capture_output=True,
    )
    if result.returncode != 0:
        return None
    resolved = [line.strip() for line in result.stdout.splitlines() if line.strip()]
    return resolved[0] if resolved else None


def package_path(texmf_dist: Path, absolute: str) -> str | None:
    """Return the texmf-dist relative source path, or None for generated files."""
    try:
        relative = Path(absolute).resolve().relative_to(texmf_dist.resolve())
    except (ValueError, OSError):
        return None
    if not relative.parts or relative.parts[0] == "tlpkg":
        return None
    return "texmf-dist/" + relative.as_posix()


def ambiguous_lookups(kpsewhich: Path, texmf_dist: Path, formats: list[dict], names: list[str]) -> list[dict]:
    by_suffix: dict[str, list[dict]] = {}
    for fmt in formats:
        for suffix in fmt["suffixes"]:
            by_suffix.setdefault(suffix.lower(), []).append(fmt)
    entries: list[dict] = []
    unresolved: list[str] = []
    for name in names:
        if not LOOKUP_NAME.fullmatch(name):
            raise ValueError(f"invalid ambiguous lookup name: {name}")
        candidates = by_suffix.get(Path(name).suffix.lower(), [])
        resolved_any = False
        for fmt in candidates:
            for kind in ENGINES:
                absolute = resolve(kpsewhich, kind, fmt, name)
                source = package_path(texmf_dist, absolute) if absolute else None
                if source is None:
                    continue
                entries.append({"k": kind, "f": str(fmt["id"]), "n": name, "p": source})
                resolved_any = True
        if not resolved_any:
            unresolved.append(name)
    if unresolved:
        raise ValueError(f"kpsewhich could not resolve ambiguous names: {', '.join(sorted(unresolved)[:10])}")
    return entries


def font_lookups(texmf_dist: Path, fc_scan: Path) -> list[dict]:
    entries = []
    for (variant, name), source in sorted(scan_font_aliases(texmf_dist, str(fc_scan)).items()):
        entries.append({
            "k": "font", "f": variant, "n": name,
            "p": "texmf-dist/" + source.relative_to(texmf_dist).as_posix(),
        })
    return entries


def derived_lookups(output: Path, manifest_path: Path, root: Path) -> tuple[list[dict], list[dict]]:
    """Copy generated files into the profile and bind them to their audit record."""
    document = json.loads(manifest_path.read_text(encoding="utf-8"))
    if not isinstance(document, dict) or set(document) != {"generator", "resources"}:
        raise ValueError("derived manifest must contain generator and resources only")
    generator = document["generator"]
    if (
        not isinstance(generator, dict)
        or set(generator) != {"name", "version", "executable_sha256"}
        or not GENERATOR.fullmatch(str(generator.get("name", "")))
        or not str(generator.get("version", "")).strip()
        or not re.fullmatch(r"[a-f0-9]{64}", str(generator.get("executable_sha256", "")))
    ):
        raise ValueError("derived generator identity is invalid")
    entries, audit = [], []
    derived_dir = output / "derived"
    derived_dir.mkdir(parents=True, exist_ok=True)
    for record in document["resources"]:
        lookup = record.get("lookup") or {}
        kind, fmt, name = lookup.get("kind"), lookup.get("format"), lookup.get("name")
        if kind not in {*ENGINES, "font"} or not isinstance(fmt, str) or not LOOKUP_NAME.fullmatch(str(name)):
            raise ValueError("derived lookup is invalid")
        artifact = (root / str(record.get("artifact", ""))).resolve()
        try:
            artifact.relative_to(root.resolve())
        except ValueError as error:
            raise ValueError("derived artifact escapes its root") from error
        if not artifact.is_file():
            raise ValueError(f"derived artifact is missing: {record.get('artifact')}")
        digest, size = sha256_file(artifact)
        if size == 0:
            raise ValueError(f"derived artifact is empty: {record.get('artifact')}")
        target = derived_dir / digest
        if not target.exists():
            shutil.copyfile(artifact, target)
        media_type = mimetypes.guess_type(artifact.name)[0] or "application/octet-stream"
        entries.append({"k": kind, "f": fmt, "n": name,
                        "d": {"sha256": digest, "size": size, "media_type": media_type}})
        audit.append({"name": name, "sha256": digest, "size": size,
                      "artifact": str(record.get("artifact")),
                      "inputs": list(record.get("inputs") or []),
                      "generator": generator})
    return entries, audit


def write_json(path: Path, document: object) -> None:
    path.write_text(json.dumps(document, ensure_ascii=False, indent=2, sort_keys=True) + "\n", encoding="utf-8")


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--snapshot-url", required=True)
    parser.add_argument("--profile", required=True)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--tlpdb", type=Path, required=True)
    parser.add_argument("--kpsewhich", type=Path, required=True)
    parser.add_argument("--fc-scan", type=Path, required=True)
    parser.add_argument("--texmf-dist", type=Path, required=True)
    parser.add_argument("--ambiguous", type=Path)
    parser.add_argument("--license-overrides", type=Path)
    parser.add_argument("--derived-manifest", type=Path)
    parser.add_argument("--derived-root", type=Path)
    args = parser.parse_args()
    if not re.fullmatch(r"[A-Za-z0-9._-]{1,96}", args.profile):
        raise ValueError("invalid profile identifier")
    if not args.tlpdb.name.endswith(".xz"):
        raise ValueError("the snapshot tlpdb must be the compressed .xz artifact")
    if (args.derived_manifest is None) != (args.derived_root is None):
        raise ValueError("--derived-manifest and --derived-root must be used together")

    args.output.mkdir(parents=True, exist_ok=True)
    tlpdb_target = args.output / "tlpdb.xz"
    shutil.copyfile(args.tlpdb, tlpdb_target)
    tlpdb_bytes = tlpdb_target.read_bytes()
    tlpdb_sha512 = hashlib.sha512(tlpdb_bytes).hexdigest()

    formats = parse_formats(args.kpsewhich)
    format_suffixes = [
        {"kind": kind, "format": str(fmt["id"]), "suffixes": fmt["suffixes"]}
        for kind in ENGINES for fmt in formats if fmt["suffixes"]
    ]

    lookups: list[dict] = []
    names = []
    if args.ambiguous:
        names = [line.strip() for line in args.ambiguous.read_text(encoding="utf-8").splitlines() if line.strip()]
    lookups += ambiguous_lookups(args.kpsewhich, args.texmf_dist, formats, names)
    lookups += font_lookups(args.texmf_dist, args.fc_scan)
    derived_audit: list[dict] = []
    if args.derived_manifest:
        entries, derived_audit = derived_lookups(args.output, args.derived_manifest, args.derived_root)
        lookups += entries

    lookups.sort(key=lambda entry: (entry["k"], entry["f"], entry["n"]))
    seen: set[tuple[str, str, str]] = set()
    for entry in lookups:
        key = (entry["k"], entry["f"], entry["n"])
        if key in seen:
            raise ValueError(f"duplicate lookup {key}")
        seen.add(key)

    encoded = json.dumps(lookups, ensure_ascii=False, separators=(",", ":")).encode("utf-8")
    compressed = gzip.compress(encoded, compresslevel=9, mtime=0)
    (args.output / "lookups.json.gz").write_bytes(compressed)
    lookups_sha256 = hashlib.sha256(compressed).hexdigest()

    license_overrides = {}
    if args.license_overrides:
        license_overrides = json.loads(args.license_overrides.read_text(encoding="utf-8"))

    write_json(args.output / "profile.json", {
        "schema_version": 1,
        "profile": args.profile,
        "repository": args.snapshot_url.rstrip("/"),
        "tlpdb": {"path": "tlpdb.xz", "sha512": tlpdb_sha512, "size": len(tlpdb_bytes)},
        "lookups": {"sha256": lookups_sha256, "entries": len(lookups)},
        "format_suffixes": format_suffixes,
        "license_overrides": license_overrides,
    })
    write_json(args.output / "profile-manifest.json", {
        "profile": args.profile,
        "snapshot": args.snapshot_url.rstrip("/"),
        "built_at": datetime.datetime.now(datetime.UTC).replace(microsecond=0).isoformat(),
        "tlpdb": {"path": "tlpdb.xz", "sha512": tlpdb_sha512, "size": len(tlpdb_bytes)},
        "lookups": {"sha256": lookups_sha256, "entries": len(lookups), "ambiguous": len(names)},
        "derived": derived_audit,
    })
    print(
        f"profile={args.profile} lookups={len(lookups)} ambiguous={len(names)} "
        f"derived={len(derived_audit)} tlpdb_bytes={len(tlpdb_bytes)} lookups_bytes={len(compressed)}",
        flush=True,
    )
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except (OSError, subprocess.CalledProcessError, ValueError) as error:
        print(f"profile build failed: {error}", file=sys.stderr)
        raise SystemExit(1)
