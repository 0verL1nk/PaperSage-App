# Browser LaTeX acceptance matrix

Date: 2026-10-09. Execution: Chromium/Playwright, browser-hosted SwiftLaTeX
WASM, and the feature branch Vercel Preview API for TeX resources. Resource
profile: `tl2026-swift-ef78b612-r80534-v7`.

All six archives compiled in the browser. The product PDF.js opened each PDF
and traversed its page tree. The HIT output additionally contained the expected
Chinese title text.

| Fixture | Root file | Engine | PDF pages | PDF bytes | Archive SHA-256 |
|---|---|---:|---:|---:|---|
| IEEEtran | `bare_jrnl.tex` | pdfLaTeX | 1 | 55,298 | `e0cd4f5afbd42c8076092280e72b3e0a5111efe501d35de9f715cfb8da313cb4` |
| Springer Nature | `sn-article.tex` | pdfLaTeX | 11 | 376,408 | `812e76dcaa9c28dc1bff1fb6065d51729b67d4ea140552a05088317414a3ecae` |
| Nature preprint | `nature-template.tex` | pdfLaTeX | 4 | 89,524 | `f45b628959b9ae7470a938612b04a34c50d2c5985a6dc153153f270a1cdead92` |
| Elsevier elsarticle | `elsarticle-template-num.tex` | pdfLaTeX | 4 | 130,094 | `0a8fa8bdf615871b894edec338acedc82136a0ea296ca1c997e2004700ee4611` |
| ACM acmart | `sigconf.tex` | pdfLaTeX | 6 | 1,065,820 | `f443348f3ce980119633a6760ff49e9531da3c7b28a41a331e68c50b1b05684b` |
| User HIT thesis | `examples/hitbook/chinese/thesis.tex` | XeLaTeX | 18 | 383,562 | `bdff05d69cbf68df7620610cc18b36ed596915236405f19d573209e1a470ac2e` |

The Elsevier and ACM fixtures include class files generated from the official
CTAN source packages. Browser EPS processing was separately exercised on all
six EPS files in the supplied HIT archive, including PDF page bounds and
browser-cache reuse. The evidence verifies these fixed inputs and this resource
profile. It is not a visual/typesetting review, a publisher-submission
guarantee, or proof that arbitrary templates and packages are supported.

## Requalification with reproducible SwiftLaTeX artifacts (2026-10-10)

Executed Chromium/Playwright against the PaperSage frontend worktree and its
Vercel feature API resource gateway using the fixed SwiftLaTeX build receipt
`frontend/public/latex-engines/swiftlatex-build-receipt.json`. Build source:
`0verL1nk/papersage-swiftlatex@4f6851dbc016a924659394f334d49dd89735b238`;
Ubuntu 22.04; Emscripten 3.1.64. The build receipt pins all four pdfTeX/XeTeX
JS/WASM hashes, and the manifest validator checks each one against the shipped
bytes. The six original ZIP digests above were verified before compilation.

| Fixture | PDF.js pages | PDF bytes | Result |
|---|---:|---:|---|
| IEEEtran | 1 | 55,298 | Pass |
| Springer Nature | 11 | 376,405 | Pass |
| Nature preprint | 4 | 89,524 | Pass |
| Elsevier elsarticle | 4 | 130,094 | Pass |
| ACM acmart | 6 | 1,065,820 | Pass |
| User HIT thesis | 18 | 383,563 | Pass; expected `哈尔滨工业大学` text found |

All outputs were opened and traversed by the product's PDF.js version. The HIT
cover was also rendered by that PDF.js build; Chinese title, author, and
university text were visibly present. PDFs are saved under
`E:\LING\Downloads\PaperSage-LaTeX-Acceptance\pdfs-reproducible-38051086676`.
The matrix took 12.6 minutes; the HIT compile took 4.8 minutes. The observed
Chromium process working set peaked near 1.4 GiB. These measurements describe
this machine and fixture, not a minimum-browser-memory guarantee. Local
MiKTeX Poppler lacks Adobe-GB1 CMap data and emits CMap/font lookup errors for
the HIT PDF; PDF.js rendering confirms those local Poppler diagnostics are not
missing glyphs in the browser-generated cover.

This is structural/text and one-page visual acceptance for the listed inputs.
It does not certify all-page visual layout, arbitrary TeX packages, or venue
submission compliance.
