#!/usr/bin/env python3
"""Assemble the Windows ICO from the full-detail and small-size app icons.

Run after rendering appicon.svg / appicon-small.svg / tray.svg to PNG
(see README). Committed outputs: build/appicon.png, build/windows/icon.ico,
build/brand/tray-16.png, build/brand/tray-32.png.
"""

import io
import struct
from pathlib import Path

from PIL import Image

BASE = Path(__file__).resolve().parent
BUILD = BASE.parent
FULL = Image.open(BASE / "appicon.png").convert("RGBA")
SMALL = Image.open(BASE / "appicon-small.svg".replace(".svg", ".png")).convert("RGBA")


def png_bytes(image: Image.Image, size: int) -> bytes:
    buf = io.BytesIO()
    image.resize((size, size), Image.LANCZOS).save(buf, format="PNG")
    return buf.getvalue()


def write_ico(path: Path) -> None:
    sizes = [16, 20, 24, 32, 40, 48, 64, 128, 256]
    entries = []
    blob = b""
    offset = 6 + 16 * len(sizes)
    for size in sizes:
        data = png_bytes(SMALL if size <= 32 else FULL, size)
        edge = 0 if size >= 256 else size
        entries.append(
            struct.pack("<BBBBHHII", edge, edge, 0, 0, 1, 32, len(data), offset)
        )
        blob += data
        offset += len(data)
    path.write_bytes(struct.pack("<HHH", 0, 1, len(sizes)) + b"".join(entries) + blob)


write_ico(BUILD / "windows" / "icon.ico")
(BUILD / "appicon.png").write_bytes((BASE / "appicon.png").read_bytes())

tray = Image.open(BASE / "tray.png").convert("RGBA")
for size in (16, 32):
    tray.resize((size, size), Image.LANCZOS).save(BASE / f"tray-{size}.png")

print("icons written")
