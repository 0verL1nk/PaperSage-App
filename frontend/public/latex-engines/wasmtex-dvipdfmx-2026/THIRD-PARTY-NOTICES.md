# Third-party notices: TeX Live 2026 Dvipdfmx

PaperSage uses the WasmTex TeX Live 2026 `dvipdfmx` WebAssembly build as its
XeTeX XDV-to-PDF stage. The worker controller has been adapted to resolve
exact Kpathsea lookup tuples through PaperSage's same-origin resource gateway
and to load the XeTeX-produced fonts from the worker filesystem.

- Engine release: `2026-9147f5703951082b`
- Dvipdfmx source revision: `7f074299d01a2f8b2c0107c8ff27d12adcda6b06`
- TeX Live source commit: `fb6158926661cb7a7246b3a94a0cb170a9624d5a`
- WebAssembly engine license: GNU GPL version 2 or later. The full text is in
  [`LICENSE-GPL-2.0.txt`](./LICENSE-GPL-2.0.txt).
- Worker/runtime source, build receipts, checksums, notices, and the complete
  corresponding-source archive are listed in the pinned upstream release's
  [`manifest.json`](https://corca-ai.github.io/wasmtex/wasmtex/2026/manifest.json).
- Corresponding source archive SHA-256:
  `abe486385f5a5a606203216420d067c8e57f5d6edc3398c623af98efef14d318`
- The MIT-licensed WasmTex worker source is preserved in the release source
  archive; its engine binary retains GPL terms and is not relicensed by the
  SDK's MIT license.

The release records the exact Emscripten build and linked-component inventory.
The PaperSage worker changes are present in this repository alongside the
unmodified upstream `dvipdfmx` WebAssembly module. This notice accompanies the
distributed engine assets and does not replace the corresponding-source archive.
