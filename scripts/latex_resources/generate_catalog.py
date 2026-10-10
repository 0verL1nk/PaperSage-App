#!/usr/bin/env python3
"""Build an immutable SwiftLaTeX resource catalog from a pinned TeX Live tree.

The catalog uses the pinned Kpathsea binary itself to resolve every lookup, so
engine-specific path order and Kpathsea format suffixes are not reimplemented.
Run this on the same platform as the supplied TeX Live installation.
"""

from __future__ import annotations

import argparse
import concurrent.futures
import hashlib
import json
import mimetypes
import os
import re
import shutil
import sqlite3
import subprocess
import sys
from collections.abc import Iterable, Iterator
from pathlib import Path
from urllib.parse import urlparse


ENGINES = {
    "xetex": ("xetex", "xetex"),
    "pdftex": ("pdftex", "pdftex"),
    "dvipdfmx": ("dvipdfmx", "xetex"),
}
MAX_RESOURCE_BYTES = 128 * 1024 * 1024
MAX_KPSE_BATCH_NAMES = 128
# Stay well below CreateProcess' command-line limit on Windows (32,767 UTF-16
# code units), while keeping ordinary batches large enough to amortize startup.
MAX_KPSE_BATCH_ARG_CHARS = 24_000


def run_text(args: list[str]) -> str:
    result = subprocess.run(args, check=True, text=True, capture_output=True)
    return result.stdout


def parse_enum(header: Path) -> list[str]:
    source = header.read_text(encoding="utf-8")
    match = re.search(
        r"typedef\s+enum\s*\{(?P<body>.*?)\}\s*kpse_file_format_type\s*;",
        source,
        re.DOTALL,
    )
    if not match:
        raise ValueError(f"could not find kpse_file_format_type in {header}")
    values = re.findall(r"\bkpse_([a-z0-9_]+)_format\b", match.group("body"))
    if not values or values[-1] != "last":
        raise ValueError("Kpathsea format enum is missing kpse_last_format")
    return values[:-1]


def parse_formats(kpsewhich: Path, enum_names: list[str] | None = None) -> list[dict[str, object]]:
    output = run_text([str(kpsewhich), "--help-formats"])
    formats: list[dict[str, object]] = []
    for line in output.splitlines():
        match = re.match(r"^(.+?):\s*(.*?)\s+\[variables:", line)
        if not match:
            continue
        name = match.group(1).strip()
        suffixes = []
        for token in match.group(2).split():
            suffix = (token if token.startswith(".") else f".{token}").lower()
            if suffix not in suffixes:
                suffixes.append(suffix)
        formats.append({"name": name, "suffixes": suffixes})
    if enum_names is not None and len(formats) != len(enum_names):
        raise ValueError(
            "Kpathsea format enum/help mismatch: "
            f"{len(enum_names)} enum values vs {len(formats)} help formats"
        )
    for format_id, fmt in enumerate(formats):
        fmt["id"] = str(format_id)
        if enum_names is not None:
            fmt["enum"] = enum_names[format_id]
    return formats


def parse_tlpdb(tlpdb: Path) -> dict[str, dict[str, str]]:
    packages: dict[str, dict[str, str]] = {}
    current: dict[str, object] | None = None
    files: list[str] = []
    in_runfiles = False

    def finish() -> None:
        if current is None:
            return
        package = str(current.get("name", ""))
        if not package:
            return
        license_name = str(current.get("catalogue-license", "")).strip()
        revision = str(current.get("revision", "")).strip()
        container_size = str(current.get("containersize", "")).strip()
        container_checksum = str(current.get("containerchecksum", "")).strip()
        catalogue = str(current.get("catalogue", "")).strip()
        # Some TLPDB stanzas (notably 00texlive.installer) describe files
        # shipped outside the package archive set. They are not fetchable via
        # /archive/<name>.tar.xz and must not claim paths from an actual
        # archive-backed package. A half-populated archive record is corrupt.
        if not container_size and not container_checksum:
            return
        if not container_size or not container_checksum:
            raise ValueError(f"incomplete package archive metadata for {package}")
        if not container_size.isdecimal() or int(container_size) <= 0:
            raise ValueError(f"invalid package archive size for {package}")
        if not re.fullmatch(r"[a-f0-9]{128}", container_checksum):
            raise ValueError(f"invalid package archive checksum for {package}")
        for name in files:
            normalized = name.removeprefix("RELOC/").removeprefix("texmf-dist/")
            previous = packages.get(normalized)
            if previous and previous["name"] != package:
                raise ValueError(
                    f"ambiguous package provenance for {normalized}: "
                    f"{previous['name']} and {package}"
                )
            packages[normalized] = {
                "name": package,
                "license": license_name,
                "revision": revision,
                "container_size": container_size,
                "container_checksum": container_checksum,
                "catalogue": catalogue,
            }

    for line in tlpdb.read_text(encoding="utf-8", errors="strict").splitlines():
        if not line:
            finish()
            current = None
            files = []
            in_runfiles = False
            continue
        if line[0].isspace():
            if in_runfiles:
                files.append(line.strip())
            continue
        if current is None:
            current = {}
        if line.startswith("name "):
            current["name"] = line[5:].strip()
            in_runfiles = False
        elif line.startswith("catalogue-license "):
            current["catalogue-license"] = line[len("catalogue-license "):].strip()
            in_runfiles = False
        elif line.startswith("revision "):
            current["revision"] = line[len("revision "):].strip()
            in_runfiles = False
        elif line.startswith("containersize "):
            current["containersize"] = line[len("containersize "):].strip()
            in_runfiles = False
        elif line.startswith("containerchecksum "):
            current["containerchecksum"] = line[len("containerchecksum "):].strip()
            in_runfiles = False
        elif line.startswith("catalogue "):
            current["catalogue"] = line[len("catalogue "):].strip()
            in_runfiles = False
        elif line.startswith("runfiles "):
            in_runfiles = True
        else:
            in_runfiles = False
    finish()
    return packages


def content_type(file_path: Path) -> str:
    guessed, _ = mimetypes.guess_type(file_path.name)
    return guessed or "application/octet-stream"


def resolve_one(task: tuple[str, str, str, str, str, str]) -> tuple[str, str, str, str] | None:
    binary, kind, format_name, program, engine_name, request_name = task
    args = [
        binary,
        f"--format={format_name}",
        f"--progname={program}",
        f"--engine={engine_name}",
        request_name,
    ]
    result = subprocess.run(args, text=True, capture_output=True)
    if result.returncode != 0:
        return None
    resolved = result.stdout.strip().splitlines()
    if not resolved:
        return None
    return kind, format_name, request_name, resolved[0]


def resolve_batch(
    tasks: tuple[tuple[str, str, str, str, str, str], ...],
) -> list[tuple[tuple[str, str, str, str, str, str], tuple[str, str, str, str] | None]]:
    """Resolve compatible requests in one Kpathsea process.

    kpsewhich emits paths for hits and may omit misses while returning a
    nonzero exit code. Accept only paths whose basename identifies exactly one
    requested name; retry unresolved or ambiguous names individually so mixed
    batches preserve the original exact lookup behavior.
    """
    if not tasks:
        return []
    first = tasks[0]
    binary, kind, format_name, program, engine_name, _ = first
    compatible = all(
        task[:5] == first[:5]
        and task[5]
        and not task[5].startswith("-")
        and "\n" not in task[5]
        and "\r" not in task[5]
        for task in tasks
    )
    if len(tasks) == 1 or not compatible:
        return [(task, resolve_one(task)) for task in tasks]

    args = [
        binary,
        f"--format={format_name}",
        f"--progname={program}",
        f"--engine={engine_name}",
        *(task[5] for task in tasks),
    ]
    try:
        result = subprocess.run(args, text=True, capture_output=True)
    except (OSError, subprocess.SubprocessError):
        return [(task, resolve_one(task)) for task in tasks]

    lines = [line.strip() for line in result.stdout.splitlines() if line.strip()]
    if result.returncode == 0 and len(lines) == len(tasks):
        return [
            (task, (kind, format_name, task[5], path))
            for task, path in zip(tasks, lines, strict=True)
        ]

    by_basename: dict[str, list[tuple[str, str, str, str, str, str]]] = {}
    for task in tasks:
        by_basename.setdefault(Path(task[5]).name.casefold(), []).append(task)
    resolved_by_task: dict[tuple[str, str, str, str, str, str], str] = {}
    ambiguous: set[tuple[str, str, str, str, str, str]] = set()
    for path in lines:
        matches = by_basename.get(Path(path).name.casefold(), [])
        if len(matches) != 1:
            ambiguous.update(matches)
            continue
        task = matches[0]
        previous = resolved_by_task.get(task)
        if previous is not None and previous != path:
            ambiguous.add(task)
        else:
            resolved_by_task[task] = path

    results = []
    for task in tasks:
        path = resolved_by_task.get(task)
        if path is not None and task not in ambiguous:
            resolved = (kind, format_name, task[5], path)
        else:
            resolved = resolve_one(task)
        results.append((task, resolved))
    return results


def task_batches(
    tasks: Iterable[tuple[str, str, str, str, str, str]],
    max_names: int = MAX_KPSE_BATCH_NAMES,
    max_arg_chars: int = MAX_KPSE_BATCH_ARG_CHARS,
) -> Iterator[tuple[tuple[str, str, str, str, str, str], ...]]:
    """Yield bounded batches from an already compatible, ordered task stream."""
    batch: list[tuple[str, str, str, str, str, str]] = []
    arg_chars = 0
    for task in tasks:
        task_header = task[:5]
        item_chars = len(task[5].encode("utf-16-le")) // 2 + 1
        if batch and (
            task_header != batch[0][:5]
            or len(batch) >= max_names
            or arg_chars + item_chars > max_arg_chars
        ):
            yield tuple(batch)
            batch = []
            arg_chars = 0
        batch.append(task)
        arg_chars += item_chars
    if batch:
        yield tuple(batch)


def candidate_names(
    root: Path,
    formats: list[dict[str, object]],
    installed_runfiles: set[str] | None = None,
) -> dict[str, set[str]]:
    result: dict[str, set[str]] = {str(fmt["name"]): set() for fmt in formats}
    by_suffix: dict[str, list[str]] = {}
    suffixless = [str(fmt["name"]) for fmt in formats if not fmt["suffixes"]]
    for fmt in formats:
        for suffix in fmt["suffixes"]:
            by_suffix.setdefault(str(suffix).lower(), []).append(str(fmt["name"]))

    allowed_names = None if installed_runfiles is None else {
        name.casefold() for name in installed_runfiles
    }
    for directory, _, files in os.walk(root):
        relative_directory = Path(directory).relative_to(root)
        is_tex_tree = bool(relative_directory.parts) and relative_directory.parts[0] == "tex"
        for filename in files:
            relative_name = (relative_directory / filename).as_posix().casefold()
            if allowed_names is not None and relative_name not in allowed_names:
                continue
            if is_tex_tree and "tex" in result:
                # Kpathsea's TeX format accepts arbitrary explicit suffixes.
                # The engine requests .ini and .ltx files through this format,
                # although --help-formats does not list those suffixes.
                result["tex"].add(filename)
            lower_name = filename.lower()
            has_known_suffix = False
            for suffix, format_names in by_suffix.items():
                if not lower_name.endswith(suffix):
                    continue
                has_known_suffix = True
                for format_name in format_names:
                    result[format_name].add(filename)
                    result[format_name].add(filename[:-len(suffix)])
            if not Path(filename).suffix and not has_known_suffix:
                for format_name in suffixless:
                    result[format_name].add(filename)
    return result


def sha256_file(file_path: Path) -> tuple[str, int]:
    digest = hashlib.sha256()
    size = 0
    with file_path.open("rb") as handle:
        for block in iter(lambda: handle.read(1024 * 1024), b""):
            size += len(block)
            if size > MAX_RESOURCE_BYTES:
                raise ValueError(f"resource exceeds size limit: {file_path}")
            digest.update(block)
    return digest.hexdigest(), size


def scan_font_aliases(root: Path, scanner: str) -> dict[tuple[str, str], Path]:
    """Index TeX OpenType/TrueType files by filename and PostScript name."""
    ranked_aliases: dict[tuple[str, str], tuple[int, Path]] = {}
    font_root = root / "fonts"
    for relative_dir, variant, suffixes in (
        ("opentype", "OT", {".otf"}),
        ("truetype", "TT", {".ttf", ".ttc", ".dfont"}),
    ):
        directory = font_root / relative_dir
        if not directory.is_dir():
            continue
        for source in sorted(directory.rglob("*")):
            if not source.is_file() or source.suffix.lower() not in suffixes:
                continue
            result = subprocess.run(
                [scanner, "--format", "%{postscriptname}|%{fullname}|%{family}|%{style}\\n", str(source)],
                check=True,
                text=True,
                capture_output=True,
            )
            for record in result.stdout.splitlines():
                fields = record.split("|")
                if len(fields) != 4:
                    continue
                postscript, full_names, family_names, styles = fields
                style_names = [value.strip().lower() for value in styles.split(",") if value.strip()]
                regular_style = any(value in {"regular", "book", "plain", "normal", "roman"} for value in style_names)
                aliases = [(source.stem, 4)]
                aliases.extend((value, 5) for value in postscript.split(","))
                aliases.extend((value, 3) for value in full_names.split(","))
                aliases.extend((value, 2 if regular_style else 1) for value in family_names.split(","))
                for raw_name, priority in aliases:
                    name = raw_name.strip().lower()
                    if not name or name == "-":
                        continue
                    key = (variant, name)
                    prior = ranked_aliases.get(key)
                    if prior is None or priority > prior[0] or (priority == prior[0] and source.as_posix() < prior[1].as_posix()):
                        ranked_aliases[key] = (priority, source)
    return {key: value[1] for key, value in ranked_aliases.items()}


def font_lookup_entries(
    aliases: dict[tuple[str, str], Path],
    formats: list[dict[str, object]],
) -> dict[tuple[str, str, str], Path]:
    """Map Fontconfig aliases and engine file lookups to their exact font file.

    SwiftLaTeX's fontconfig adapter requests resources as ``font/<variant>/<name>``
    (for example ``font/OT/fandolsong-regular``). Those requests are distinct
    from Kpathsea's numeric-format lookups and must be present as first-class
    catalog entries; the package filename index cannot resolve PostScript or
    family aliases.
    """
    font_entries: dict[tuple[str, str, str], Path] = {}
    font_format_ids: dict[str, str] = {}
    for format_name, suffix in (("opentype", ".otf"), ("truetype", ".ttf")):
        matching = [fmt for fmt in formats if suffix in fmt["suffixes"]]
        if matching:
            font_format_ids[format_name] = str(matching[0]["id"])

    for (variant, name), source in aliases.items():
        lookup = ("font", variant, name)
        previous = font_entries.get(lookup)
        if previous is not None and previous != source:
            raise ValueError(f"conflicting font resource lookup for {lookup}")
        font_entries[lookup] = source

        format_name = "opentype" if source.suffix.lower() == ".otf" else "truetype"
        format_id = font_format_ids.get(format_name)
        if format_id is None:
            raise ValueError(f"Kpathsea does not expose the {format_name} font format")
        # XeTeX and Dvipdfmx can request literal font names through Kpathsea.
        for kind in ("xetex", "dvipdfmx"):
            lookup = (kind, format_id, name)
            previous = font_entries.get(lookup)
            if previous is not None and previous != source:
                raise ValueError(f"conflicting font file lookup for {lookup}")
            font_entries[lookup] = source
    return font_entries


def relative_file(root: Path, value: object, label: str) -> tuple[str, Path]:
    if not isinstance(value, str) or not value or "\\" in value:
        raise ValueError(f"{label} must be a relative POSIX path")
    relative = Path(value)
    if relative.is_absolute() or any(part in {"", ".", ".."} for part in relative.parts):
        raise ValueError(f"{label} must stay inside its configured root")
    resolved = (root / relative).resolve(strict=True)
    try:
        resolved.relative_to(root)
    except ValueError as error:
        raise ValueError(f"{label} escapes its configured root") from error
    if not resolved.is_file():
        raise ValueError(f"{label} is not a regular file")
    return relative.as_posix(), resolved


def build_derived_resources(
    manifest_path: Path,
    derived_root: Path,
    texmf_root: Path,
    packages: dict[str, dict[str, str]],
    license_overrides: dict[str, dict[str, str]],
    fallback_license_label: str | None,
    fallback_license_source: str | None,
) -> list[dict[str, object]]:
    """Validate generated runtime files and bind them to their inputs.

    The manifest is emitted by the pinned TeX Live build step after its
    generator runs. Its paths are only hints: files are read from the
    configured roots, and all input hashes and archive provenance are
    recomputed from the pinned installation/TLPDB here.
    """
    document = json.loads(manifest_path.read_text(encoding="utf-8"))
    if not isinstance(document, dict) or set(document) != {"generator", "resources"}:
        raise ValueError("derived resource manifest must contain generator and resources only")
    generator = document["generator"]
    if (
        not isinstance(generator, dict)
        or set(generator) != {"name", "version", "executable_sha256"}
        or not isinstance(generator.get("name"), str)
        or not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9._+-]{0,127}", generator["name"])
        or not isinstance(generator.get("version"), str)
        or not generator["version"].strip()
        or len(generator["version"]) > 256
        or not isinstance(generator.get("executable_sha256"), str)
        or not re.fullmatch(r"[a-f0-9]{64}", generator["executable_sha256"])
    ):
        raise ValueError("derived resource generator identity is invalid")
    resources = document["resources"]
    if not isinstance(resources, list) or not resources:
        raise ValueError("derived resource manifest must contain at least one resource")

    output: list[dict[str, object]] = []
    seen_lookups: set[tuple[str, str, str]] = set()
    for record in resources:
        if not isinstance(record, dict) or set(record) != {"lookup", "artifact", "inputs"}:
            raise ValueError("derived resource record has invalid fields")
        lookup = record["lookup"]
        if (
            not isinstance(lookup, dict)
            or set(lookup) != {"kind", "format", "name"}
            or not isinstance(lookup.get("kind"), str)
            or lookup["kind"] not in {*ENGINES, "font"}
            or not isinstance(lookup.get("format"), str)
            or not re.fullmatch(r"[A-Za-z0-9._+-]{1,96}", lookup["format"])
            or not isinstance(lookup.get("name"), str)
            or not re.fullmatch(r"[^/\\]{1,255}", lookup["name"])
            or lookup["name"] in {".", ".."}
        ):
            raise ValueError("derived resource lookup is invalid")
        lookup_key = (lookup["kind"], lookup["format"], lookup["name"])
        if lookup_key in seen_lookups:
            raise ValueError(f"duplicate derived resource lookup {lookup_key}")
        seen_lookups.add(lookup_key)

        artifact_name, artifact_path = relative_file(derived_root, record["artifact"], "derived artifact")
        artifact_hash, artifact_size = sha256_file(artifact_path)
        if artifact_size == 0:
            raise ValueError(f"derived artifact is empty: {artifact_name}")

        input_paths = record["inputs"]
        if not isinstance(input_paths, list) or not input_paths:
            raise ValueError(f"derived artifact {artifact_name} must identify its input files")
        input_records = []
        seen_inputs: set[str] = set()
        for input_path in input_paths:
            source_name, source = relative_file(texmf_root, input_path, "derived input")
            if source_name in seen_inputs:
                raise ValueError(f"duplicate input {source_name} for derived artifact {artifact_name}")
            seen_inputs.add(source_name)
            normalized = source_name.removeprefix("RELOC/").removeprefix("texmf-dist/")
            provenance = packages.get(normalized)
            if provenance is None:
                raise ValueError(f"derived input is absent from TeX Live package metadata: {source_name}")
            license_name = provenance["license"]
            if not license_name:
                override = license_overrides.get(provenance["name"])
                if override:
                    license_name = override["license"]
                    license_source = override["source"]
                elif fallback_license_label:
                    license_name = fallback_license_label
                    license_source = fallback_license_source or ""
                else:
                    raise ValueError(f"derived input has no license metadata: {provenance['name']}:{source_name}")
            else:
                catalogue = provenance["catalogue"]
                license_source = (
                    f"https://ctan.org/pkg/{catalogue}?lang=en"
                    if catalogue and re.fullmatch(r"[A-Za-z0-9._+-]{1,128}", catalogue)
                    else "https://www.tug.org/texlive/copying.html"
                )
            input_hash, _ = sha256_file(source)
            input_records.append({
                "source_path": f"texmf-dist/{normalized}",
                "sha256": input_hash,
                "package": provenance["name"],
                "package_revision": provenance["revision"],
                "container_size": int(provenance["container_size"]),
                "container_checksum": provenance["container_checksum"],
                "license": license_name,
                "license_source": license_source,
            })

        output.append({
            "lookup": lookup,
            "sha256": artifact_hash,
            "size": artifact_size,
            "media_type": content_type(artifact_path),
            "object_key": f"derived/{artifact_hash}",
            "generator": generator,
            "inputs": input_records,
            "artifact_path": artifact_name,
        })
    return output


def write_catalog_output(
    output: Path,
    document: dict[str, object],
    derived_resources: list[dict[str, object]],
    derived_root: Path | None,
) -> Path:
    """Write the profile catalog and verified content-addressed derived blobs."""
    serialized_resources: list[dict[str, object]] = []
    existing_lookups = {
        (entry["lookup"]["kind"], entry["lookup"]["format"], entry["lookup"]["name"])
        for entry in document.get("entries", [])
    }
    seen_lookups = set(existing_lookups)
    for resource in derived_resources:
        lookup = resource["lookup"]
        key = (lookup["kind"], lookup["format"], lookup["name"])
        if key in seen_lookups:
            raise ValueError(f"derived resource conflicts with existing lookup {key}")
        seen_lookups.add(key)
        if derived_root is None:
            raise ValueError("derived root is required when catalog has derived resources")
        _, artifact = relative_file(derived_root, resource["artifact_path"], "derived artifact")
        digest, size = sha256_file(artifact)
        if digest != resource["sha256"] or size != resource["size"]:
            raise ValueError(f"derived artifact changed after manifest validation: {artifact}")
        artifact_directory = output / "derived"
        artifact_directory.mkdir(parents=True, exist_ok=True)
        destination = artifact_directory / digest
        if destination.exists():
            existing_hash, existing_size = sha256_file(destination)
            if existing_hash != digest or existing_size != size:
                raise ValueError(f"existing derived artifact does not match its content key: {destination}")
        else:
            shutil.copyfile(artifact, destination)
        serialized_resources.append({
            key: value for key, value in resource.items() if key != "artifact_path"
        })

    catalog_document = dict(document)
    if serialized_resources:
        catalog_document["derived_resources"] = serialized_resources
    catalog_path = output / "catalog.json"
    catalog_path.write_text(
        json.dumps(catalog_document, ensure_ascii=False, separators=(",", ":")) + "\n",
        encoding="utf-8",
    )
    return catalog_path


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--kpsewhich", type=Path, required=True)
    parser.add_argument(
        "--types-header",
        type=Path,
        help="optional Kpathsea enum header; without it, format IDs follow this binary's --help-formats order",
    )
    parser.add_argument("--texmf-dist", type=Path, required=True)
    parser.add_argument("--tlpdb", type=Path, required=True)
    parser.add_argument(
        "--derived-manifest",
        type=Path,
        help="generator manifest describing reproducible TeX runtime artifacts",
    )
    parser.add_argument(
        "--derived-root",
        type=Path,
        help="root containing generated artifacts referenced by --derived-manifest",
    )
    parser.add_argument("--profile", required=True)
    parser.add_argument("--license-overrides", type=Path)
    parser.add_argument("--fallback-license-label")
    parser.add_argument("--fallback-license-source")
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument(
        "--lookup-cache",
        type=Path,
        help="SQLite cache for exact Kpathsea queries (defaults beside --output)",
    )
    parser.add_argument("--workers", type=int, default=8)
    args = parser.parse_args()

    if not re.fullmatch(r"[A-Za-z0-9._-]{1,96}", args.profile):
        parser.error("invalid profile identifier")
    if bool(args.fallback_license_label) != bool(args.fallback_license_source):
        parser.error("fallback license label and source must be supplied together")
    if bool(args.derived_manifest) != bool(args.derived_root):
        parser.error("--derived-manifest and --derived-root must be supplied together")
    if args.fallback_license_source:
        fallback_url = urlparse(args.fallback_license_source)
        if fallback_url.scheme != "https" or not fallback_url.netloc:
            parser.error("fallback license source must be an HTTPS URL")
    license_overrides: dict[str, dict[str, str]] = {}
    if args.license_overrides:
        license_overrides = json.loads(args.license_overrides.read_text(encoding="utf-8"))
        if not isinstance(license_overrides, dict):
            parser.error("license overrides must be a JSON object")
        for package, override in license_overrides.items():
            if (
                not isinstance(package, str)
                or not isinstance(override, dict)
                or not isinstance(override.get("license"), str)
                or not override["license"].strip()
                or not isinstance(override.get("source"), str)
            ):
                parser.error(f"invalid license override for {package!r}")
            override_url = urlparse(override["source"])
            if override_url.scheme != "https" or not override_url.netloc:
                parser.error(f"license override source for {package!r} must be HTTPS")
    root = args.texmf_dist.resolve(strict=True)
    if not root.is_dir() or args.workers < 1 or args.workers > 32:
        parser.error("invalid TeX tree or worker count")
    derived_root = args.derived_root.resolve(strict=True) if args.derived_root else None
    if derived_root is not None and not derived_root.is_dir():
        parser.error("derived root must be a directory")

    enums = parse_enum(args.types_header) if args.types_header else None
    formats = parse_formats(args.kpsewhich, enums)
    format_by_name = {str(fmt["name"]): fmt for fmt in formats}
    packages = parse_tlpdb(args.tlpdb)
    candidates = candidate_names(root, formats, set(packages))
    output = args.output.resolve()
    output.mkdir(parents=True, exist_ok=True)
    cache_path = (args.lookup_cache or output.parent / f"{output.name}.lookup-cache.sqlite3").resolve()
    cache_path.parent.mkdir(parents=True, exist_ok=True)
    cache = sqlite3.connect(cache_path)
    cache.execute(
        "CREATE TABLE IF NOT EXISTS resolutions ("
        "engine TEXT NOT NULL, format TEXT NOT NULL, name TEXT NOT NULL, "
        "resolved_path TEXT, PRIMARY KEY(engine, format, name))"
    )
    cache.commit()
    def tasks():
        for kind, (program, engine_name) in ENGINES.items():
            for format_name, names in candidates.items():
                for name in sorted(names):
                    if not name or "/" in name or "\\" in name or name in {".", ".."}:
                        continue
                    yield (str(args.kpsewhich), kind, format_name, program, engine_name, name)

    total_tasks = sum(len(names) for names in candidates.values()) * len(ENGINES)
    cached_resolutions = {
        (str(engine), str(format_id), str(name)): path
        for engine, format_id, name, path in cache.execute(
            "SELECT engine, format, name, resolved_path FROM resolutions"
        )
    }
    cached_count = len(cached_resolutions)

    print(
        f"Kpathsea {len(formats)} formats; {total_tasks} exact lookups; "
        f"{len(packages)} package-file provenance records; "
        f"{cached_count} cached lookups",
        flush=True,
    )
    resolved_entries: dict[tuple[str, str, str], Path] = {}
    with concurrent.futures.ThreadPoolExecutor(max_workers=args.workers) as pool:
        pending: dict[
            concurrent.futures.Future[list[tuple[tuple[str, str, str, str, str, str], tuple[str, str, str, str] | None]]],
            tuple[tuple[str, str, str, str, str, str], ...],
        ] = {}
        task_iterator = iter(task_batches(tasks()))
        exhausted = False
        index = 0
        while pending or not exhausted:
            while len(pending) < args.workers * 4 and not exhausted:
                try:
                    batch = next(task_iterator)
                except StopIteration:
                    exhausted = True
                    continue
                uncached_batch = []
                for task in batch:
                    _, kind, format_name, _, _, request_name = task
                    cache_key = (kind, str(format_by_name[format_name]["id"]), request_name)
                    if cache_key not in cached_resolutions:
                        uncached_batch.append(task)
                        continue
                    cached_path = cached_resolutions[cache_key]
                    if cached_path:
                        source = Path(cached_path).resolve(strict=True)
                        try:
                            source.relative_to(root)
                        except ValueError as error:
                            raise ValueError(f"cached path escapes TeX tree: {source}") from error
                        lookup = (kind, str(format_by_name[format_name]["id"]), request_name)
                        resolved_entries[lookup] = source
                    index += 1
                if not uncached_batch:
                    continue
                pending[pool.submit(resolve_batch, tuple(uncached_batch))] = tuple(uncached_batch)
            if not pending:
                continue
            completed, remaining = concurrent.futures.wait(
                pending, return_when=concurrent.futures.FIRST_COMPLETED
            )
            completed_tasks = {future: pending[future] for future in completed}
            pending = {future: pending[future] for future in remaining}
            for future in completed:
                for task, result in future.result():
                    index += 1
                    _, kind, format_name, _, _, request_name = task
                    cache_key = (kind, str(format_by_name[format_name]["id"]), request_name)
                    if result is None:
                        cache.execute(
                            "INSERT OR REPLACE INTO resolutions VALUES (?, ?, ?, NULL)",
                            cache_key,
                        )
                        cached_resolutions[cache_key] = None
                        continue
                    if index % 1000 == 0:
                        print(f"resolved {index}/{total_tasks}", flush=True)
                    kind, format_name, request_name, raw_path = result
                    source = Path(raw_path).resolve(strict=True)
                    if not source.is_file():
                        continue
                    try:
                        relative = source.relative_to(root).as_posix()
                    except ValueError:
                        continue
                    fmt = format_by_name[format_name]
                    lookup = (kind, str(fmt["id"]), request_name)
                    prior = resolved_entries.get(lookup)
                    if prior is not None and prior != source:
                        raise ValueError(f"conflicting Kpathsea result for {lookup}")
                    resolved_entries[lookup] = source
                    cache.execute(
                        "INSERT OR REPLACE INTO resolutions VALUES (?, ?, ?, ?)",
                        (kind, str(fmt["id"]), request_name, str(source)),
                    )
                    cached_resolutions[cache_key] = str(source)
            cache.commit()

    cache.close()
    font_dirs = [root / "fonts" / name for name in ("opentype", "truetype")]
    has_indexable_fonts = any(
        directory.is_dir()
        and any(path.is_file() and path.suffix.lower() in {".otf", ".ttf", ".ttc", ".dfont"} for path in directory.rglob("*"))
        for directory in font_dirs
    )
    if has_indexable_fonts:
        scanner = shutil.which("fc-scan")
        if not scanner:
            raise ValueError("fc-scan is required to index TeX OpenType and TrueType font names")
        font_entries = font_lookup_entries(scan_font_aliases(root, scanner), formats)
        for lookup, source in font_entries.items():
            previous = resolved_entries.get(lookup)
            if previous is not None and previous != source:
                raise ValueError(f"conflicting font resource lookup for {lookup}")
            resolved_entries[lookup] = source

    if not resolved_entries:
        raise ValueError("Kpathsea resolved no resources")
    entries = []
    object_records: dict[str, dict[str, object]] = {}
    object_provenance_keys: dict[str, set[tuple[object, ...]]] = {}
    file_hashes: dict[Path, tuple[str, int]] = {}
    unlicensed = []
    fallback_license_entries = 0
    for (kind, format_id, name), source in sorted(resolved_entries.items()):
        relative = source.relative_to(root).as_posix()
        provenance = packages.get(relative)
        if not provenance:
            raise ValueError(f"resolved file is absent from TeX Live package metadata: {relative}")
        license_name = provenance["license"]
        if not license_name:
            override = license_overrides.get(provenance["name"])
            if override:
                license_name = override["license"]
                license_source = override["source"]
            elif args.fallback_license_label:
                license_name = args.fallback_license_label
                license_source = args.fallback_license_source
                fallback_license_entries += 1
            else:
                unlicensed.append(f"{provenance['name']}:{relative}")
                continue
        else:
            catalogue = provenance["catalogue"]
            if catalogue and re.fullmatch(r"[A-Za-z0-9._+-]{1,128}", catalogue):
                license_source = f"https://ctan.org/pkg/{catalogue}?lang=en"
            else:
                license_source = (
                    args.fallback_license_source
                    or "https://www.tug.org/texlive/copying.html"
                )
        cached_hash = file_hashes.get(source)
        if cached_hash is None:
            cached_hash = sha256_file(source)
            file_hashes[source] = cached_hash
        digest, size = cached_hash
        entries.append({
            "lookup": {"kind": kind, "format": format_id, "name": name},
            "sha256": digest,
        })
        object_record = object_records.setdefault(digest, {
            "sha256": digest,
            "size": size,
            "media_type": content_type(source),
            "provenance": [],
        })
        provenance_record = {
            "source_path": f"texmf-dist/{relative}",
            "package": provenance["name"],
            "package_revision": provenance["revision"],
            "container_size": int(provenance["container_size"] or "0"),
            "container_checksum": provenance["container_checksum"],
            "license": license_name,
            "license_source": license_source,
        }
        if (
            provenance_record["container_size"] <= 0
            or not re.fullmatch(r"[a-f0-9]{128}", str(provenance_record["container_checksum"]))
        ):
            raise ValueError(f"package archive metadata is incomplete for {provenance['name']}:{relative}")
        provenance_key = tuple(provenance_record.values())
        seen_provenance = object_provenance_keys.setdefault(digest, set())
        if provenance_key not in seen_provenance:
            seen_provenance.add(provenance_key)
            object_record["provenance"].append(provenance_record)

    derived_resources = []
    if args.derived_manifest and derived_root is not None:
        derived_resources = build_derived_resources(
            args.derived_manifest,
            derived_root,
            root,
            packages,
            license_overrides,
            args.fallback_license_label,
            args.fallback_license_source,
        )
    if unlicensed:
        unlicensed_path = output / "unlicensed-report.json"
        unlicensed_path.write_text(
            json.dumps(sorted(set(unlicensed)), ensure_ascii=False, indent=2) + "\n",
            encoding="utf-8",
        )
        preview = "\n".join(sorted(set(unlicensed))[:30])
        raise ValueError(
            f"{len(set(unlicensed))} resolved files have no catalogue-license metadata; "
            f"generation refuses to publish them. First entries:\n{preview}"
        )
    (output / "unlicensed-report.json").unlink(missing_ok=True)

    document: dict[str, object] = {
        "schema_version": 3,
        "profile": args.profile,
        "revision": args.profile,
        "objects": list(object_records.values()),
        "entries": entries,
        "format_suffixes": [
            {
                "kind": kind,
                "format": str(fmt["id"]),
                "suffixes": fmt["suffixes"],
            }
            for kind in ENGINES
            for fmt in formats
            if fmt["suffixes"]
        ],
    }
    catalog_path = write_catalog_output(output, document, derived_resources, derived_root)
    print(
        f"catalog entries={len(entries)} unique objects={len(object_records)} "
        f"catalog bytes={catalog_path.stat().st_size} "
        f"entries using TeX Live fallback license={fallback_license_entries}",
        flush=True,
    )
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except (OSError, subprocess.CalledProcessError, ValueError) as error:
        print(f"catalog generation failed: {error}", file=sys.stderr)
        raise SystemExit(1)
