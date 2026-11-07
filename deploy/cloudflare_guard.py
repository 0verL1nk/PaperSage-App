#!/usr/bin/env python3
"""Manage only the PaperSage ingress table; never start or stop Caddy.

Install as a root-owned, non-writable-by-others file. Caddy must Requires= and
After= the guard unit. Before removal, stop Caddy and the guard unit; otherwise
RemainAfterExit would let Caddy restart without restoring the removed table.
There is deliberately no cache: offline boot fails rather than using stale IPs.
"""

import argparse
import ipaddress
import subprocess
import sys
from urllib.error import HTTPError, URLError
from urllib.request import HTTPRedirectHandler, ProxyHandler, Request, build_opener

IPV4_URL = "https://www.cloudflare.com/ips-v4"
IPV6_URL = "https://www.cloudflare.com/ips-v6"
# A finite per-request timeout bounds boot delay when Cloudflare is unreachable.
TIMEOUT_SECONDS = 15
# Observed official lists were 230 and 104 bytes; 1 MiB per list provides
# thousands of times that headroom, not a promise of future size. Root may
# adjust --max-list-bytes in the systemd unit; oversized lists are rejected.
MAX_LIST_BYTES = 1024 * 1024
NFT = "/usr/sbin/nft"
TABLE = "inet papersage_cloudflare"

# Reject overlaps, not just endpoints: a large prefix may contain private IPs.
FORBIDDEN = {
    4: tuple(map(ipaddress.ip_network, (
        "0.0.0.0/8", "10.0.0.0/8", "100.64.0.0/10", "127.0.0.0/8",
        "169.254.0.0/16", "172.16.0.0/12", "192.0.0.0/24", "192.0.2.0/24",
        "192.168.0.0/16", "198.18.0.0/15", "198.51.100.0/24",
        "203.0.113.0/24", "224.0.0.0/4", "240.0.0.0/4",
    ))),
    6: tuple(map(ipaddress.ip_network, (
        "::/128", "::1/128", "::ffff:0:0/96", "100::/64", "2001:db8::/32",
        "fc00::/7", "fe80::/10", "ff00::/8",
    ))),
}


class NoRedirect(HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        raise HTTPError(newurl, code, "redirects are not permitted", headers, fp)


def make_opener():
    # Never allow proxy environment settings to redirect a privileged fetch.
    return build_opener(ProxyHandler({}), NoRedirect())


def parse_networks(text, family):
    networks = []
    for line in text.splitlines():
        line = line.strip()
        if not line:
            continue
        network = ipaddress.ip_network(line, strict=True)
        if (network.version != family or network.prefixlen == 0 or
                not network.is_global or
                any(network.overlaps(blocked) for blocked in FORBIDDEN[family])):
            raise ValueError(f"unacceptable IPv{family} network: {line}")
        networks.append(str(network))
    if not networks:
        raise ValueError(f"empty IPv{family} list")
    return networks


def fetch_lists(opener=None, max_list_bytes=MAX_LIST_BYTES):
    if max_list_bytes <= 0:
        raise ValueError("list byte limit must be positive")
    opener = opener or make_opener()
    lists = []
    for url, family in ((IPV4_URL, 4), (IPV6_URL, 6)):
        request = Request(url, headers={"User-Agent": "PaperSage-Firewall-Preflight/1.0"})
        with opener.open(request, timeout=TIMEOUT_SECONDS) as response:
            if response.status != 200 or response.geturl() != url:
                raise ValueError(f"unexpected response for {url}")
            data = response.read(max_list_bytes + 1)
            if len(data) > max_list_bytes:
                raise ValueError(f"IPv{family} list exceeds byte limit {max_list_bytes}")
            content_length = response.getheader("Content-Length")
            if content_length is not None and (not content_length.isascii() or
                                               not content_length.isdecimal() or
                                               int(content_length) != len(data)):
                raise ValueError(f"IPv{family} response has invalid content length")
            lists.append(parse_networks(data.decode("ascii"), family))
    return tuple(lists)


def render_rules(ipv4, ipv6):
    # Inputs are revalidated even when the caller bypasses fetch_lists.
    v4 = ", ".join(parse_networks("\n".join(ipv4), 4))
    v6 = ", ".join(parse_networks("\n".join(ipv6), 6))
    return f'''add table {TABLE}
delete table {TABLE}
add table {TABLE}
add set {TABLE} cloudflare_v4 {{ type ipv4_addr; flags interval; elements = {{ {v4} }}; }}
add set {TABLE} cloudflare_v6 {{ type ipv6_addr; flags interval; elements = {{ {v6} }}; }}
add chain {TABLE} input {{ type filter hook input priority filter; policy accept; }}
add rule {TABLE} input iifname "lo" accept
add rule {TABLE} input tcp dport {{ 80, 443 }} ip saddr @cloudflare_v4 accept
add rule {TABLE} input udp dport {{ 80, 443 }} ip saddr @cloudflare_v4 accept
add rule {TABLE} input tcp dport {{ 80, 443 }} ip6 saddr @cloudflare_v6 accept
add rule {TABLE} input udp dport {{ 80, 443 }} ip6 saddr @cloudflare_v6 accept
add rule {TABLE} input tcp dport {{ 80, 443 }} drop
add rule {TABLE} input udp dport {{ 80, 443 }} drop
'''


def check(max_list_bytes=MAX_LIST_BYTES):
    rules = render_rules(*fetch_lists(max_list_bytes=max_list_bytes))
    subprocess.run([NFT, "-c", "-f", "-"], input=rules, text=True, check=True)


def refresh(max_list_bytes=MAX_LIST_BYTES):
    rules = render_rules(*fetch_lists(max_list_bytes=max_list_bytes))
    # A single nft -f transaction replaces just this table or changes nothing.
    subprocess.run([NFT, "-f", "-"], input=rules, text=True, check=True)


def remove():
    subprocess.run([NFT, "-f", "-"], input=f"delete table {TABLE}\n", text=True, check=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest="command", required=True)
    for name, help_text in (
        ("check", "fetch verified lists and run nft syntax check without applying"),
        ("refresh", "fetch verified lists and atomically update the dedicated table"),
    ):
        command = commands.add_parser(name, help=help_text)
        command.add_argument("--max-list-bytes", type=int, default=MAX_LIST_BYTES,
                             help="maximum bytes per official list (root-controlled override)")
    removal = commands.add_parser("remove", help="delete only the dedicated table after Caddy is stopped")
    removal.add_argument("--caddy-stopped", action="store_true", required=True,
                         help="acknowledge that Caddy has already been stopped")
    removal.add_argument("--guard-stopped", action="store_true", required=True,
                         help="acknowledge that the guard unit has already been stopped")
    args = parser.parse_args()
    try:
        if args.command == "check":
            check(max_list_bytes=args.max_list_bytes)
        elif args.command == "refresh":
            refresh(max_list_bytes=args.max_list_bytes)
        else:
            remove()
    except (ValueError, OSError, UnicodeError, HTTPError, URLError, subprocess.CalledProcessError) as exc:
        print(f"Cloudflare guard failed: {exc}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
