import importlib.util
import subprocess
import sys
import unittest
from pathlib import Path
from unittest import mock
from urllib.error import HTTPError, URLError
from urllib.request import Request


SCRIPT = Path(__file__).with_name("cloudflare_guard.py")
SPEC = importlib.util.spec_from_file_location("cloudflare_guard", SCRIPT)
guard = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(guard)


class Response:
    def __init__(self, url, data, status=200, final_url=None, content_length=None):
        self.url, self.data, self.status = final_url or url, data, status
        self.content_length = content_length
        self.read_sizes = []

    def __enter__(self):
        return self

    def __exit__(self, *args):
        pass

    def geturl(self):
        return self.url

    def getheader(self, name):
        return self.content_length if name == "Content-Length" else None

    def read(self, size=-1):
        self.read_sizes.append(size)
        return self.data[:size] if size >= 0 else self.data


class Opener:
    def __init__(self, responses):
        self.responses = iter(responses)
        self.requests = []
        self.returned = []

    def open(self, request, timeout):
        self.requests.append((request, timeout))
        value = next(self.responses)
        if isinstance(value, Exception):
            raise value
        url = request.full_url if hasattr(request, "full_url") else request
        response = Response(url, *value)
        self.returned.append(response)
        return response


class CloudflareGuardTests(unittest.TestCase):
    def test_command_has_help(self):
        result = subprocess.run([sys.executable, "-I", str(SCRIPT), "--help"], capture_output=True, text=True)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("refresh", result.stdout)
        self.assertIn("remove", result.stdout)

    def test_valid_public_lists_and_family(self):
        self.assertEqual(guard.parse_networks("1.1.1.0/24\n8.8.8.0/24\n", 4), ["1.1.1.0/24", "8.8.8.0/24"])
        self.assertEqual(guard.parse_networks("2606:4700::/32\n", 6), ["2606:4700::/32"])

    def test_reject_bad_lists(self):
        for value, family in [
            ("", 4), ("  \n", 6), ("0.0.0.0/0", 4), ("::/0", 6),
            ("10.0.0.0/8", 4), ("127.0.0.0/8", 4), ("169.254.0.0/16", 4),
            ("8.0.0.0/5", 4), ("::1/128", 6), ("fc00::/7", 6),
            ("fe80::/10", 6), ("2606:4700::/32", 4), ("1.1.1.0/24; flush ruleset", 4),
            ("1.1.1.1/24", 4), ("1.1.1.0/24\nBAD", 4),
        ]:
            with self.subTest(value=value), self.assertRaises(ValueError):
                guard.parse_networks(value, family)

    def test_fetch_checks_both_official_urls(self):
        opener = Opener([(b"1.1.1.0/24\n",), (b"2606:4700::/32\n",)])
        self.assertEqual(guard.fetch_lists(opener), (["1.1.1.0/24"], ["2606:4700::/32"]))
        self.assertTrue(all(isinstance(request, Request) for request, _ in opener.requests))
        self.assertEqual([request.full_url for request, _ in opener.requests], [
            "https://www.cloudflare.com/ips-v4", "https://www.cloudflare.com/ips-v6",
        ])
        for request, timeout in opener.requests:
            self.assertEqual(request.get_header("User-agent"), "PaperSage-Firewall-Preflight/1.0")
            self.assertGreater(timeout, 0)

    def test_list_byte_limit_accepts_exact_boundary_and_reads_at_most_one_extra(self):
        ipv4, ipv6 = b"1.1.1.0/24", b"2606:4700::/32"
        limit = max(len(ipv4), len(ipv6))
        opener = Opener([(ipv4.ljust(limit, b"\n"), 200, None, str(limit)),
                         (ipv6.ljust(limit, b"\n"), 200, None, str(limit))])
        self.assertEqual(guard.fetch_lists(opener, max_list_bytes=limit), (["1.1.1.0/24"], ["2606:4700::/32"]))
        self.assertEqual([response.read_sizes for response in opener.returned], [[limit + 1], [limit + 1]])

    def test_oversize_either_list_rejects_entire_refresh_without_nft(self):
        ipv4, ipv6 = b"1.1.1.0/24", b"2606:4700::/32"
        limit = max(len(ipv4), len(ipv6))
        v4, v6 = ipv4.ljust(limit, b"\n"), ipv6.ljust(limit, b"\n")
        for responses in ([(v4 + b"\n",)], [(v4,), (v6 + b"\n",)]):
            opener = Opener(responses)
            with self.subTest(responses=responses), mock.patch.object(guard, "make_opener", return_value=opener), mock.patch.object(guard.subprocess, "run") as run:
                with self.assertRaisesRegex(ValueError, "limit"):
                    guard.refresh(max_list_bytes=limit)
                run.assert_not_called()
                self.assertTrue(all(response.read_sizes == [limit + 1] for response in opener.returned))

    def test_truncated_content_length_never_applies_partial_list(self):
        opener = Opener([(b"1.1.1.0/24\n", 200, None, "30"), (b"2606:4700::/32\n",)])
        with mock.patch.object(guard, "make_opener", return_value=opener), mock.patch.object(guard.subprocess, "run") as run:
            with self.assertRaisesRegex(ValueError, "length"):
                guard.refresh()
            run.assert_not_called()

    def test_invalid_size_override_rejected_before_any_request(self):
        for value in (0, -1):
            opener = Opener([])
            with self.subTest(value=value), self.assertRaises(ValueError):
                guard.fetch_lists(opener, max_list_bytes=value)
            self.assertEqual(opener.requests, [])

    def test_http_error_redirect_failure_or_empty_never_apply(self):
        failures = [
            [(b"1.1.1.0/24", 500)],
            [HTTPError("https://www.cloudflare.com/ips-v4", 403, "Forbidden", {}, None)],
            [(b"1.1.1.0/24", 200, "https://evil.invalid/ips-v4")],
            [URLError("offline")],
            [(b"1.1.1.0/24",), (b"",)],
            [(b"1.1.1.0/24",), (b"fc00::/7",)],
            [(b"\xff",)],
        ]
        for responses in failures:
            with self.subTest(responses=responses), mock.patch.object(guard, "make_opener", return_value=Opener(responses)), mock.patch.object(guard.subprocess, "run") as run:
                with self.assertRaises((ValueError, OSError, UnicodeError)):
                    guard.refresh()
                run.assert_not_called()

    def test_redirects_refused(self):
        handler = guard.NoRedirect()
        with self.assertRaises(HTTPError):
            handler.redirect_request(None, None, 302, "Found", {}, "https://evil.invalid")

    def test_rules_are_one_atomic_nft_batch_targeting_only_web_ports(self):
        rules = guard.render_rules(["1.1.1.0/24"], ["2606:4700::/32"])
        self.assertIn("add table inet papersage_cloudflare", rules)
        self.assertIn("delete table inet papersage_cloudflare", rules)
        self.assertNotIn("flush ruleset", rules.lower())
        self.assertIn('iifname "lo" accept', rules)
        self.assertIn("policy accept", rules)
        self.assertIn("ip saddr @cloudflare_v4 accept", rules)
        self.assertIn("ip6 saddr @cloudflare_v6 accept", rules)
        self.assertIn("tcp dport { 80, 443 } drop", rules)
        self.assertIn("udp dport { 80, 443 } drop", rules)
        self.assertNotIn("22", rules)
        for line in rules.splitlines():
            if "saddr @" in line:
                self.assertIn("dport { 80, 443 }", line)
        with mock.patch.object(guard, "fetch_lists", return_value=(["1.1.1.0/24"], ["2606:4700::/32"])), mock.patch.object(guard.subprocess, "run") as run:
            guard.refresh()
        self.assertEqual(run.call_count, 1)
        args, kwargs = run.call_args
        self.assertEqual(args[0], ["/usr/sbin/nft", "-f", "-"])
        self.assertEqual(kwargs["input"], rules)
        self.assertTrue(kwargs["check"])

    def test_cli_passes_size_override_to_check_and_refresh(self):
        for command in ("check", "refresh"):
            with self.subTest(command=command), mock.patch.object(sys, "argv", [str(SCRIPT), command, "--max-list-bytes", "2048"]), mock.patch.object(guard, command) as action:
                self.assertEqual(guard.main(), 0)
                action.assert_called_once_with(max_list_bytes=2048)

    def test_check_validates_without_applying(self):
        with mock.patch.object(guard, "fetch_lists", return_value=(["1.1.1.0/24"], ["2606:4700::/32"])), mock.patch.object(guard.subprocess, "run") as run:
            guard.check()
        args, kwargs = run.call_args
        self.assertEqual(args[0], ["/usr/sbin/nft", "-c", "-f", "-"])
        self.assertEqual(kwargs["input"], guard.render_rules(["1.1.1.0/24"], ["2606:4700::/32"]))
        self.assertTrue(kwargs["check"])

    def test_fetch_failure_returns_failure_status_without_changing_rules(self):
        with mock.patch.object(sys, "argv", [str(SCRIPT), "refresh"]), mock.patch.object(guard, "make_opener", return_value=Opener([URLError("offline")])), mock.patch.object(guard.subprocess, "run") as run, mock.patch("sys.stderr"):
            self.assertEqual(guard.main(), 1)
            run.assert_not_called()

    def test_nft_failure_propagates_instead_of_report_success(self):
        with mock.patch.object(guard, "fetch_lists", return_value=(["1.1.1.0/24"], ["2606:4700::/32"])), mock.patch.object(guard.subprocess, "run", side_effect=subprocess.CalledProcessError(1, "nft")):
            with self.assertRaises(subprocess.CalledProcessError):
                guard.refresh()

    def test_remove_only_target_table_requires_explicit_acknowledgement(self):
        result = subprocess.run([sys.executable, "-I", str(SCRIPT), "remove"], capture_output=True, text=True)
        self.assertNotEqual(result.returncode, 0)
        with mock.patch.object(sys, "argv", [str(SCRIPT), "remove", "--caddy-stopped"]), mock.patch.object(guard, "remove") as remove, mock.patch("sys.stderr"):
            with self.assertRaises(SystemExit):
                guard.main()
            remove.assert_not_called()
        with mock.patch.object(guard.subprocess, "run") as run:
            guard.remove()
        args, kwargs = run.call_args
        self.assertEqual(args[0], ["/usr/sbin/nft", "-f", "-"])
        self.assertEqual(kwargs["input"], "delete table inet papersage_cloudflare\n")
        self.assertTrue(kwargs["check"])

    def test_service_does_not_delete_or_flush_rules_on_stop(self):
        service = Path(__file__).with_name("papersage-cloudflare-guard.service").read_text()
        self.assertIn("Type=oneshot", service)
        self.assertIn("RemainAfterExit=yes", service)
        self.assertIn("TimeoutStartSec=60", service)
        self.assertIn("Before=caddy.service", service)
        self.assertIn(" refresh", service)
        self.assertNotIn("ExecStop=", service)
        self.assertNotIn("nftables.service", service)


if __name__ == "__main__":
    unittest.main()
