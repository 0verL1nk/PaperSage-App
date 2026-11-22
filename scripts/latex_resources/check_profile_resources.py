#!/usr/bin/env python3
"""Probe a published profile's on-demand resources after a release.

A profile pins each package archive's size and checksum, but TeX Live tlnet
keeps moving: once a pinned archive is replaced upstream, every file that still
has to be fetched on demand answers 502. Run this after publishing a profile
and fail the release when a probe does not answer 200.

    python scripts/latex_resources/check_profile_resources.py \
      --base-url https://papersage.example.com \
      --profile tl2026-swift-ef78b612-r80534-v7 \
      kvoptions.sty geometry.sty ifxetex.sty ccaption.sty layouts.sty \
      gbt7714.sty hyphen.cfg
"""

from __future__ import annotations

import argparse
import http.client
import sys
import urllib.error
import urllib.request


# Cloudflare answers 403 to the default Python-urllib agent, which would make
# every probe look like a missing resource.
USER_AGENT = "PaperSage-resource-check/1.0"


def probe(url: str, timeout: float) -> tuple[bool, str]:
    request = urllib.request.Request(url, method="GET", headers={"Accept": "*/*", "User-Agent": USER_AGENT})
    try:
        with urllib.request.urlopen(request, timeout=timeout) as response:
            body = response.read()
            if response.status != 200:
                return False, f"HTTP {response.status}"
            return True, f"200 {len(body)} bytes"
    except urllib.error.HTTPError as error:
        return False, f"HTTP {error.code}"
    except http.client.IncompleteRead as error:
        # A Content-Length announced before the body was known truncates the
        # response exactly this way; edge proxies turn it into their own 502.
        return False, f"truncated body ({len(error.partial)} bytes before close)"
    except (urllib.error.URLError, OSError) as error:
        return False, f"request failed: {error}"


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--base-url", required=True)
    parser.add_argument("--profile", required=True)
    parser.add_argument("--kind", default="xetex")
    parser.add_argument("--format", default="26")
    parser.add_argument("--timeout", type=float, default=30.0)
    parser.add_argument("names", nargs="+", help="resource filenames to probe")
    args = parser.parse_args()

    base = args.base_url.rstrip("/")
    failures = 0
    for name in args.names:
        url = f"{base}/texlive/{args.profile}/resolve/{args.kind}/{args.format}/{name}"
        ok, detail = probe(url, args.timeout)
        print(f"{'ok  ' if ok else 'FAIL'} {name} {detail}", flush=True)
        failures += 0 if ok else 1
    return 1 if failures else 0


if __name__ == "__main__":
    sys.exit(main())
