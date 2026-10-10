# Browser LaTeX template acceptance

`template-compile.spec.ts` accepts local ZIP archives through
`BROWSER_LATEX_TEMPLATE_CASES`. It compiles each selected project in Chromium,
checks the PDF signature, opens the result with the application's PDF.js
version, walks every page in the page tree, and optionally checks extracted
text. A successful LaTeX return alone is not sufficient if PDF.js cannot parse
the resulting document.

## Case manifest format

Set `BROWSER_LATEX_TEMPLATE_CASES` to a JSON array. Each case requires:

- `name`: stable short label used in test output and the optional saved PDF name.
- `sourceUrl` (optional): official publisher or CTAN page used to obtain the
  archive. Omit this for a user-provided local archive and describe its origin
  outside the repository.
- `zipPath`: absolute path to the local template ZIP.
- `sha256`: lowercase or uppercase SHA-256 of the exact ZIP bytes (64 hex digits).
- `mainFile`: path of the root `.tex` file inside the archive, including any
  directory within the archive.
- `engine`: `pdflatex` or `xelatex`.
- `expectedText` (optional): a distinctive phrase expected in the generated
  PDF's extracted text. The runner normalizes Unicode to NFKC, collapses
  whitespace, and ignores case before checking for the phrase.

Example shape (replace the ZIP path, digest, and phrase with values from the
archive being tested):

```json
[
  {
    "name": "IEEEtran",
    "sourceUrl": "https://ctan.org/pkg/ieeetran",
    "zipPath": "E:\\LING\\Downloads\\IEEEtran-template.zip",
    "sha256": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
    "mainFile": "bare_jrnl.tex",
    "engine": "pdflatex",
    "expectedText": "A distinctive phrase present in this template's output"
  }
]
```

On PowerShell, calculate the digest from the downloaded archive itself:

```powershell
(Get-FileHash -LiteralPath 'E:\LING\Downloads\IEEEtran-template.zip' -Algorithm SHA256).Hash.ToLowerInvariant()
```

Run against a local frontend and a reachable browser resource API (the API
Preview may require the Vercel access headers supported by the runner):

```powershell
$env:RUN_BROWSER_LATEX_E2E = '1'
$env:PLAYWRIGHT_BASE_URL = 'http://localhost:5188'
$env:BROWSER_LATEX_RESOURCE_API = 'https://<feature-api-preview>'
$env:BROWSER_LATEX_TEMPLATE_CASES = Get-Content -Raw '.\template-cases.local.json'
pnpm exec playwright test --config tests/e2e/latex/playwright.config.ts tests/e2e/latex/template-compile.spec.ts
```

`template-cases.local.json` is a developer-created local manifest and should
not be committed when it contains personal download paths. The real template
ZIPs downloaded to a user's Downloads folder are test inputs and must not be
copied into this repository. Keep source page, exact archive version, ZIP
SHA-256, `mainFile`, and engine together in the local manifest or an acceptance
record so a result can be reproduced without publishing the archive.

## Official template source pages

Use the publisher or CTAN package page to obtain the template. If the page
offers multiple archives or revisions, record the exact selected download and
the ZIP digest in the case manifest; a package landing page by itself does not
identify the bytes tested.

| Template family | Official source page | Notes |
|---|---|---|
| IEEEtran | [CTAN: IEEEtran](https://ctan.org/pkg/ieeetran) | Record the selected archive and root file; `bare_jrnl.tex` is one test entry point. |
| Springer Nature | [Springer Nature LaTeX author support](https://www.springernature.com/gp/authors/campaigns/latex-author-support/see-where-our-services-will-take-you/18782940) | Record the exact official sample archive and root file. |
| Elsevier elsarticle | [CTAN: elsarticle](https://ctan.org/pkg/elsarticle) | Some CTAN downloads are source bundles; run the included installer if the `.cls` is generated from `.ins`, then record what the tested ZIP contains. |
| ACM acmart | [CTAN: acmart](https://ctan.org/pkg/acmart) | Some source bundles require the documented `.ins` installation step before a compilable fixture has `acmart.cls`. |
| Nature preprint class | [CTAN: nature](https://ctan.org/pkg/nature) | This is a CTAN preprint class, not evidence that Nature accepts that class for submissions. |
| HIT thesis archive | User-provided local ZIP | Not an official journal template source; keep the user's archive outside the repository and record its exact digest locally. |

## Result interpretation and current known evidence

PDF.js parsing proves the output has a readable PDF page tree and allows
page-count and optional text checks. It does not prove visual fidelity,
submission-rule compliance, correct citations, or that all fonts and graphics
are present. Inspect the PDF visually for acceptance-critical templates.

The existing acceptance record in
`E:\LING\Downloads\PaperSage-LaTeX-Acceptance\README.md` reports the IEEEtran
case passing on resource profile `tl2026-swift-ef78b612-r80534-v4`; Springer
Nature and the CTAN Nature preprint case stopped at missing packages; Elsevier
and ACM stopped because their generated class files were absent; and the user
HIT archive stopped at a missing TeX Gyre font. Its later v6 rerun was blocked
by Vercel Deployment Protection returning HTML instead of TeX resources, so it
is neither a v6 pass nor a template failure. Those archives are not stored in
this repository. Consult the acceptance record for exact historical details.
