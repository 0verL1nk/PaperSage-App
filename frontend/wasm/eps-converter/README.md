# EPS to PDF WebAssembly

This small `wasm-bindgen` wrapper uses the pinned Stet 0.8.4 Rust library with
only its PDF output feature. It converts EPS bytes to vector PDF bytes in the
browser worker; it does not run TeX and makes no network requests.

## Build

Requirements: Rust 1.95.0, the `wasm32-unknown-unknown` target, and
`wasm-bindgen-cli` 0.2.129.

From this directory, run:

```powershell
cargo build --locked --release --target wasm32-unknown-unknown
wasm-bindgen --target web --out-dir ../../public/latex-engines/stet `
  target/wasm32-unknown-unknown/release/papersage_eps_pdf_wasm.wasm
```

The generated `.js` and `_bg.wasm` files are shipped as the browser runtime.
The PDF writer feature is selected without Stet's raster renderer or PDF
reader. The wrapper supplies the browser's UTC clock through standard PDF
metadata because `SystemTime::now()` is unavailable on `wasm32-unknown-unknown`;
it also forwards Rust panics to the worker so they appear in compile errors.
Keep the Cargo lockfile, notices, and generated runtime in sync.

Stet 0.8.4 is dual licensed under Apache-2.0 or MIT. The wrapper is MIT OR
Apache-2.0. Consult the generated Cargo dependency tree and the upstream crate
licenses before changing the pinned version.
