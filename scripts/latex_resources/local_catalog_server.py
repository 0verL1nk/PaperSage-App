#!/usr/bin/env python3
"""Serve a previously generated local resource catalog for browser smoke tests.

This deliberately uses the old local object mirror. It exercises the actual
SwiftLaTeX worker and browser cache without writing package objects to R2.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import mimetypes
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from urllib.parse import unquote, urlsplit


def make_handler(catalog_path: Path, object_root: Path, texmf_root: Path | None, profile: str):
    document = json.loads(catalog_path.read_text(encoding="utf-8"))
    if document.get("profile") != profile:
        raise ValueError("catalog profile does not match --profile")
    revision = document["revision"]
    entries = {
        (item["lookup"]["kind"], item["lookup"]["format"], item["lookup"]["name"]): item["sha256"]
        for item in document["entries"]
    }
    objects = {item["sha256"]: item for item in document["objects"]}
    derived = {
        (item["lookup"]["kind"], item["lookup"]["format"], item["lookup"]["name"]): item
        for item in document.get("derived_resources", [])
    }

    class Handler(BaseHTTPRequestHandler):
        def do_OPTIONS(self):
            self.send_response(204)
            self.send_header("Access-Control-Allow-Origin", "*")
            self.send_header("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
            self.send_header("Access-Control-Allow-Headers", "Range")
            self.end_headers()

        def do_HEAD(self):
            self.serve_resource(send_body=False)

        def do_GET(self):
            self.serve_resource(send_body=True)

        def serve_resource(self, send_body: bool):
            parts = [unquote(part) for part in urlsplit(self.path).path.split("/") if part]
            if len(parts) != 6 or parts[:2] != ["texlive", profile] or parts[2] != "resolve":
                self.send_error(404)
                return
            _, _, _, kind, fmt, name = parts
            digest = entries.get((kind, fmt, name))
            resource = objects.get(digest) if digest else None
            generated = derived.get((kind, fmt, name)) if resource is None else None
            if generated is not None:
                digest = generated["sha256"]
            if resource is None:
                if generated is None:
                    body = b"resource is not present in the local catalog"
                    self.send_response(404)
                    self.send_header("Content-Type", "text/plain; charset=utf-8")
                    self.send_header("Content-Length", str(len(body)))
                    self.send_header("Access-Control-Allow-Origin", "*")
                    self.end_headers()
                    if send_body:
                        self.wfile.write(body)
                    return
            if generated is not None:
                path = object_root / "derived" / digest
                if not path.is_file():
                    path = catalog_path.parent / "derived" / digest
                expected_size = generated["size"]
                content_type = generated["media_type"]
            else:
                path = object_root / digest
                if not path.is_file() and texmf_root is not None:
                    provenance = resource["provenance"][0]
                    source_path = provenance["source_path"]
                    if not source_path.startswith("texmf-dist/"):
                        self.send_error(502, "catalog source is outside the local TeX tree")
                        return
                    relative = source_path.removeprefix("texmf-dist/")
                    path = (texmf_root / relative).resolve()
                    try:
                        path.relative_to(texmf_root.resolve())
                    except ValueError:
                        self.send_error(502, "catalog source escaped the local TeX tree")
                        return
                expected_size = resource["size"]
                content_type = resource["media_type"] or mimetypes.guess_type(name)[0] or "application/octet-stream"
            try:
                data = path.read_bytes()
            except OSError:
                self.send_error(502, "catalog object is not present in the local mirror")
                return
            if hashlib.sha256(data).hexdigest() != digest:
                self.send_error(502, "local resource bytes do not match the catalog checksum")
                return
            size = len(data)
            if size != expected_size:
                self.send_error(502, "local resource size does not match the catalog")
                return
            self.send_response(200)
            self.send_header("Content-Type", content_type)
            self.send_header("Content-Length", str(size))
            self.send_header("Cache-Control", "public, max-age=31536000, immutable")
            self.send_header("ETag", f'"{digest}"')
            self.send_header("fileid" if kind != "font" else "fontid", digest)
            self.send_header("X-PaperSage-Resource-Revision", revision)
            if generated is not None:
                self.send_header("X-PaperSage-Generated-Resource", "true")
            self.send_header("Access-Control-Allow-Origin", "*")
            self.send_header("Access-Control-Expose-Headers", "fileid, fontid, X-PaperSage-Resource-Revision")
            self.end_headers()
            if send_body:
                self.wfile.write(data)

        def log_message(self, fmt: str, *args: object) -> None:
            print(fmt % args)

    return Handler


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--catalog", type=Path, required=True)
    parser.add_argument("--objects", type=Path, required=True)
    parser.add_argument("--texmf-dist", type=Path)
    parser.add_argument("--profile", required=True)
    parser.add_argument("--host", default="127.0.0.1")
    parser.add_argument("--port", type=int, default=8765)
    args = parser.parse_args()
    server = ThreadingHTTPServer((args.host, args.port), make_handler(args.catalog, args.objects, args.texmf_dist, args.profile))
    print(f"serving {args.profile} resource catalog at http://{args.host}:{args.port}", flush=True)
    server.serve_forever()


if __name__ == "__main__":
    main()
