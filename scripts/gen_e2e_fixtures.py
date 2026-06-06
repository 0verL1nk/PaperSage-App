"""Generate minimal valid PDF, DOCX, and TEX fixtures for E2E tests.

Outputs to frontend/tests/e2e/fixtures/. All files are < 5KB and
self-contained — no external libraries required to produce them.

Regenerate via:
    python3 scripts/gen_e2e_fixtures.py
"""
from __future__ import annotations

import struct
import zipfile
from pathlib import Path

OUT = Path("/home/ling/GraduateProject/frontend/tests/e2e/fixtures")
OUT.mkdir(parents=True, exist_ok=True)


# ── sample.pdf (minimal valid 1-page PDF, ~700 bytes) ─────────────
def make_pdf() -> bytes:
    """Return a minimal valid PDF containing the text 'Hello E2E'."""
    objects: list[bytes] = []

    # 1: Catalog
    objects.append(b"1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n")
    # 2: Pages
    objects.append(
        b"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n"
    )
    # 3: Page
    objects.append(
        b"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] "
        b"/Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>\nendobj\n"
    )
    # 4: Content stream
    content = b"BT /F1 24 Tf 100 700 Td (Hello E2E) Tj ET"
    objects.append(
        b"4 0 obj\n<< /Length "
        + str(len(content)).encode()
        + b" >>\nstream\n"
        + content
        + b"\nendstream\nendobj\n"
    )
    # 5: Font
    objects.append(
        b"5 0 obj\n<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>\nendobj\n"
    )

    out = bytearray(b"%PDF-1.4\n%\xe2\xe3\xcf\xd3\n")
    offsets = [0]
    for obj in objects:
        offsets.append(len(out))
        out += obj
    xref_offset = len(out)
    out += b"xref\n0 6\n0000000000 65535 f \n"
    for off in offsets[1:]:
        out += f"{off:010d} 00000 n \n".encode()
    out += b"trailer\n<< /Size 6 /Root 1 0 R >>\nstartxref\n"
    out += str(xref_offset).encode() + b"\n%%EOF\n"
    return bytes(out)


# ── sample.docx (minimal valid Word document, ~2.5KB) ─────────────
def make_docx() -> bytes:
    """Return a minimal valid .docx (Word 2007+) with one paragraph."""
    # A docx is a zip with a specific structure. The minimum required
    # parts are [Content_Types].xml, _rels/.rels, word/document.xml.
    content_types = (
        b'<?xml version="1.0" encoding="UTF-8" standalone="yes"?>\n'
        b'<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">'
        b'<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>'
        b'<Default Extension="xml" ContentType="application/xml"/>'
        b'<Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>'
        b'</Types>'
    )
    rels = (
        b'<?xml version="1.0" encoding="UTF-8" standalone="yes"?>\n'
        b'<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">'
        b'<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>'
        b'</Relationships>'
    )
    document = (
        b'<?xml version="1.0" encoding="UTF-8" standalone="yes"?>\n'
        b'<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">'
        b'<w:body><w:p><w:r><w:t>Hello E2E document.</w:t></w:r></w:p></w:body>'
        b'</w:document>'
    )

    import io

    buf = io.BytesIO()
    with zipfile.ZipFile(buf, "w", zipfile.ZIP_DEFLATED) as z:
        z.writestr("[Content_Types].xml", content_types)
        z.writestr("_rels/.rels", rels)
        z.writestr("word/document.xml", document)
    return buf.getvalue()


# ── main.tex ──────────────────────────────────────────────────────
TEX_CONTENT = (
    "% Sample LaTeX for E2E upload tests\n"
    "\\documentclass{article}\n"
    "\\title{E2E Sample}\n"
    "\\author{Playwright}\n"
    "\\begin{document}\n"
    "\\maketitle\n"
    "Hello from the E2E upload fixture.\n"
    "\\end{document}\n"
).encode("utf-8")


def main() -> None:
    (OUT / "sample.pdf").write_bytes(make_pdf())
    (OUT / "sample.docx").write_bytes(make_docx())
    (OUT / "main.tex").write_bytes(TEX_CONTENT)
    for p in sorted(OUT.iterdir()):
        print(f"  {p.name:14s} {p.stat().st_size:6d} bytes")


if __name__ == "__main__":
    main()
