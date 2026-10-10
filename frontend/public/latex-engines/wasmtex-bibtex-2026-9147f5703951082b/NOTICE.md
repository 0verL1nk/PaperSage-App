# Browser BibTeX engine notices

This directory contains the TeX Live 2026 BibTeX WebAssembly engine and its
worker glue from WasmTex release `engine-2026-9147f5703951082b`.

- WasmTex and BibTeX build source revision: `48988d296e42e7501dd5928b89b056c38d07b45d`
- TeX Live source commit: `fb6158926661cb7a7246b3a94a0cb170a9624d5a`
- Build receipt: [`BUILD-RECEIPT.bibtex.json`](./BUILD-RECEIPT.bibtex.json)
- Distribution terms: BibTeX/Web2C notices, LGPL-2.1-or-later Kpathsea, and
  linked-component notices. This is not covered by WasmTex's SDK MIT license.
- License texts: [`LICENSE-BibTeX.txt`](./LICENSE-BibTeX.txt) and
  [`LICENSE-LGPL-2.1.txt`](./LICENSE-LGPL-2.1.txt)
- Full third-party notices: [WasmTex THIRD_PARTY_NOTICES.md](https://github.com/corca-ai/wasmtex/blob/48988d296e42e7501dd5928b89b056c38d07b45d/THIRD_PARTY_NOTICES.md)
- Corresponding source archive:
  [wasmtex-2026-9147f5703951082b-source.tar.xz](https://github.com/corca-ai/wasmtex/releases/download/engine-2026-9147f5703951082b/wasmtex-2026-9147f5703951082b-source.tar.xz)
  (SHA-256 `abe486385f5a5a606203216420d067c8e57f5d6edc3398c623af98efef14d318`)

The WasmTe worker controller and generated Emscripten JavaScript/WASM are
distributed to browsers, so the corresponding-source and notice requirements
apply. The worker adapter is pinned to this engine release and uses the
profile's exact `pdftex` Kpathsea lookup tuples for formats 6 (`.bib`) and 7
(`.bst`).
