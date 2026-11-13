# Browser LaTeX engine assets

## SwiftLaTeX engines

The pdfTeX and XeTeX workers and WebAssembly modules are built from the private
PaperSage SwiftLaTeX mirror. The mirror is synchronized with upstream SwiftLaTeX
and contains PaperSage changes for current TeX Live engine sources.

- Source repository: https://github.com/0verL1nk/papersage-swiftlatex (private)
- TeX Live source commit: `ef78b612ce99951b9f133e389d2ea84f80d7f2cc`
- pdfTeX: 1.40.29; XeTeX: 0.999998
- Build toolchain: Emscripten 3.1.64 in Ubuntu 22.04
- License: AGPL-3.0; the existing SwiftLaTeX-derived browser engine is retained under this license by project decision. See [`LICENSE`](./LICENSE) and the upstream source link above.
- Worker notices: [`LICENSE`](./LICENSE)

The previous SwiftLaTeX Dvipdfmx binary is no longer used. XeTeX XDV output is
passed to the separately licensed TeX Live 2026 Dvipdfmx build below.

## XeTeX XDV finalizer

The current finalizer is WasmTex's TeX Live 2026 Dvipdfmx build. PaperSage
adapts the worker resolver and font transfer; the upstream WASM module is
unchanged. Exact revisions, hashes, source archive checksum, and GPL terms are
in [`wasmtex-dvipdfmx-2026/THIRD-PARTY-NOTICES.md`](./wasmtex-dvipdfmx-2026/THIRD-PARTY-NOTICES.md)
and [`BUILD-RECEIPT.xetex.json`](./wasmtex-dvipdfmx-2026/BUILD-RECEIPT.xetex.json).

## Browser EPS conversion

EPS files are converted in a dedicated browser worker by the Stet EPS→PDF
WebAssembly build. The pinned binary and its third-party license texts are in
[`stet/`](./stet/). The worker reads the EPS DSC bounding box, sets the PDF page
size, and offsets the drawing to the page origin before conversion.

## Browser acceptance status

The current v7 resource profile passed browser compilation for six
representative archives: IEEEtran, Springer Nature, Nature preprint, Elsevier,
ACM, and the supplied HIT thesis. The PDFs were opened with the product PDF.js;
the HIT output also contained the expected Chinese title text. The tested HIT
archive produced an 18-page PDF with the browser XeTeX + Dvipdfmx pipeline, and
the EPS corpus passed the browser converter checks. See the
[`acceptance matrix`](../../../openspec/changes/browser-latex-wasm-compiler/acceptance-matrix.md)
for fixture hashes and limits.

This is reproducible fixture acceptance, not a visual/typesetting review, a
publisher-submission guarantee, or proof that every template and package is
supported. The catalog is pinned to its TeX Live profile and resolves resources
on demand. The LuaTeX engine remains unavailable in PaperSage until its own
engine, formats, and resource profile pass browser acceptance.
