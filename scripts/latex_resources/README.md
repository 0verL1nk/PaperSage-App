# TeX Live resource catalog generation

`generate_catalog.py` builds package-backed lookups from a pinned TeX Live
installation and its `texlive.tlpdb`. A small number of runtime configuration
files are created by TeX Live tools rather than shipped in a package archive.
Those files can be represented as **derived resources** without pretending
they came from a package tarball.

## Derived resource manifest

The pinned build step must run its real generator (for example, the matching
TeX Live `updmap-sys`) and write a JSON manifest next to the generated files:

```json
{
  "generator": {
    "name": "updmap-sys",
    "version": "TeX Live 2026 r80534",
    "executable_sha256": "<64 lowercase hex characters>"
  },
  "resources": [
    {
      "lookup": { "kind": "dvipdfmx", "format": "11", "name": "<runtime filename>" },
      "artifact": "<path relative to the generated tree>",
      "inputs": [
        "<path relative to texmf-dist>",
        "<another path relative to texmf-dist>"
      ]
    }
  ]
}
```

The manifest is generic: it does not reserve a filename or a specific TeX
package. Each listed lookup is exact. The catalog builder reads and hashes the
artifact and every declared input, resolves each input against the pinned
TLPDB, and records package revision, archive checksum, license, and license
source. It rejects missing files, paths outside either configured tree,
unlicensed/untracked inputs, duplicate lookups, and conflicting package-backed
lookups. The generator executable hash and identity/version are also retained.

Run catalog generation with both options:

```text
--derived-manifest <manifest.json> --derived-root <generated-output-root>
```

The output contains `derived/<sha256>` blobs and `derived_resources` catalog
records. The resource publisher validates each blob against its content key,
uploads only these derived runtime artifacts under the profile's immutable
`derived/` prefix, verifies them, then publishes the catalog. Package/font
resource files remain on-demand from their verified TeX Live package archives.

The builder verifies that declared inputs exist in the pinned installation,
match their TLPDB provenance, and have the recorded byte hashes. It cannot
prove that a generator read every declared file; the profile build recipe must
capture the generator's complete input set. A newly generated profile must
therefore be built with the exact pinned TeX Live toolchain and then validated
through the publisher and browser resolver before it is considered qualified.

The publisher stores `catalog.json` as deterministic gzip bytes at the same
profile key and verifies the decoded contents after upload. The Go resource
reader accepts both gzip catalogs and existing plain-JSON catalogs. This keeps
profile storage and cold-start reads smaller without adding per-file R2 objects.

For `updmap-sys`, capture its own run log rather than guessing which map files
were active. `export_updmap_resource.py` parses the log's configuration list
and the selected `Map`, `MixedMap`, or `KanjiMap` input section, rejects missing
or inconsistent entries, copies the generated artifact, and writes a manifest
with the exact script hash and requested runtime lookup. For example:

```text
python scripts/latex_resources/export_updmap_resource.py \
  --log <TEXMFSYSVAR>/web2c/updmap.log \
  --artifact <TEXMFSYSVAR>/fonts/map/dvipdfmx/updmap/kanjix.map \
  --artifact-output <derived-root>/kanjix.map \
  --texmf-dist <TEXMFDIST> \
  --updmap-script <TEXMFDIST>/scripts/texlive/updmap.pl \
  --lookup-kind dvipdfmx --lookup-format 11 --lookup-name kanjix.map \
  --map-type KanjiMap
```

On Windows, run `updmap-sys.exe` with TeX Live's `bin/windows` directory as
the current directory. That keeps the bundled `kpsewhich` discoverable by the
TeX Live launcher while it constructs the child process path.

## Complete TeX Live package filename index

The exact Kpathsea catalog is generated from the pinned local TeX Live tree,
but the on-demand archive gateway can serve other packages without installing
all of TeX Live or uploading package contents to R2. Build its filename index
from the complete `tlpkg/texlive.tlpdb` belonging to the same immutable TeX
Live repository snapshot:

```text
python scripts/latex_resources/build_package_index.py \
  --base-catalog <generated-catalog.json> \
  --tlpdb <complete-repository-tlpdb> \
  --license-overrides scripts/latex_resources/license-overrides.json \
  --profile <immutable-profile-id> \
  --output <profile-output-dir>
```

This index stores filenames and package archive provenance only; the Go
gateway still fetches the single selected archive on a cache miss, validates
its TeX Live archive checksum, extracts the requested member, and validates
the member hash. A filename that maps to multiple packages is rejected as
ambiguous. Packages without a TLPDB license or an explicit HTTPS-sourced
override are omitted and recorded in `unlicensed-package-report.json`; their
files are never exposed through the on-demand index. Use `--strict-licenses`
to reject the whole index instead. The builder never substitutes a placeholder
license label. Keep the full TLPDB bytes/hash and repository snapshot in the
profile build record so package revisions, archive checksums, and license
data stay tied to the same source state.
