import subprocess
import sys
import threading
import unittest
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path


SCRIPT = Path(__file__).with_name("check_profile_resources.py")


class Handler(BaseHTTPRequestHandler):
    def do_GET(self) -> None:
        name = self.path.rsplit("/", 1)[-1]
        if name == "missing.sty":
            self.send_error(502)
            return
        if name == "truncated.sty":
            self.send_response(200)
            self.send_header("Content-Length", "100")
            self.end_headers()
            self.wfile.write(b"short")
            self.wfile.flush()
            self.close_connection = True
            return
        body = b"resource bytes"
        self.send_response(200)
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, *args: object) -> None:
        pass


class CheckProfileResourcesTests(unittest.TestCase):
    def setUp(self) -> None:
        self.server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        self.thread = threading.Thread(target=self.server.serve_forever, daemon=True)
        self.thread.start()
        self.base_url = f"http://127.0.0.1:{self.server.server_address[1]}"

    def tearDown(self) -> None:
        self.server.shutdown()
        self.server.server_close()

    def run_probe(self, *names: str) -> subprocess.CompletedProcess[str]:
        return subprocess.run(
            [sys.executable, str(SCRIPT), "--base-url", self.base_url, "--profile", "p", "--timeout", "5", *names],
            capture_output=True, text=True, check=False,
        )

    def test_all_resources_available(self) -> None:
        result = self.run_probe("a.sty", "b.sty")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("ok   a.sty 200 14 bytes", result.stdout)

    def test_reports_each_failure_and_exits_nonzero(self) -> None:
        result = self.run_probe("a.sty", "missing.sty")
        self.assertEqual(result.returncode, 1)
        self.assertIn("FAIL missing.sty HTTP 502", result.stdout)

    def test_truncated_body_is_reported_as_such(self) -> None:
        result = self.run_probe("truncated.sty")
        self.assertEqual(result.returncode, 1)
        self.assertIn("FAIL truncated.sty truncated body", result.stdout)


if __name__ == "__main__":
    unittest.main()
