"""Browser-driven smoke test for the fttp reverse proxy.

Boots a tiny Python backend, generates a self-signed cert via the project's
certgen tool, starts the proxy as a subprocess, then drives headless Chrome
to fetch a page through the proxy. Exercises the full HTTP/2-over-TLS path
the way a real browser does, which the in-process Go integration test
cannot.

Run:
    pip install -r tests/requirements.txt
    pytest tests/browser_test.py -v

Requirements:
    - Chrome or Chromium installed (Selenium Manager auto-resolves the driver).
    - `go` on PATH.
"""

from __future__ import annotations

import contextlib
import http.server
import json
import os
import socket
import socketserver
import subprocess
import threading
import time
from pathlib import Path
from typing import Iterator

import pytest
from selenium import webdriver
from selenium.webdriver.chrome.options import Options
from selenium.webdriver.common.by import By
from selenium.webdriver.support import expected_conditions as EC
from selenium.webdriver.support.ui import WebDriverWait

REPO_ROOT = Path(__file__).resolve().parent.parent


def _free_port() -> int:
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as s:
        s.bind(("127.0.0.1", 0))
        return s.getsockname()[1]


def _wait_for_port(port: int, timeout: float = 10.0) -> None:
    deadline = time.time() + timeout
    while time.time() < deadline:
        with contextlib.suppress(OSError):
            with socket.create_connection(("127.0.0.1", port), timeout=0.2):
                return
        time.sleep(0.1)
    raise RuntimeError(f"port {port} never opened")


class _BackendHandler(http.server.BaseHTTPRequestHandler):
    """Returns a tiny HTML page for /api/v1 and JSON for everything else."""

    def log_message(self, *_args, **_kwargs):  # silence noisy default logger
        return

    def _serve(self):
        if self.path.startswith("/api/v1"):
            body = (
                b"<!doctype html><html><body>"
                b"<h1 id='hello'>Hello from backend</h1>"
                b"<p id='path'>" + self.path.encode() + b"</p>"
                b"</body></html>"
            )
            self.send_response(200)
            self.send_header("Content-Type", "text/html; charset=utf-8")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)
            return

        payload = json.dumps({"method": self.command, "path": self.path}).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(payload)))
        self.end_headers()
        self.wfile.write(payload)

    do_GET = _serve
    do_POST = _serve


@pytest.fixture(scope="module")
def stack() -> Iterator[dict]:
    """Spin backend + proxy with a self-signed cert for the duration of the module."""
    workdir = Path(os.environ.get("FTTP_TEST_WORKDIR", "/tmp/fttp-browser-test"))
    workdir.mkdir(parents=True, exist_ok=True)

    # 1. Generate a fresh self-signed cert.
    cert_name = "browser-test"
    cert_path = workdir / f"{cert_name}-cert.pem"
    key_path = workdir / f"{cert_name}-key.pem"
    subprocess.run(
        [
            "go", "run", "./tools/certgen",
            "-org", "fttp-test",
            "-cn", "localhost",
            "-on", "browser",
            "-ip", "127.0.0.1",
            "-name", str(workdir / cert_name),
        ],
        cwd=REPO_ROOT,
        check=True,
    )

    # 2. Backend on a free port, in a thread.
    backend_port = _free_port()
    httpd = socketserver.ThreadingTCPServer(("127.0.0.1", backend_port), _BackendHandler)
    backend_thread = threading.Thread(target=httpd.serve_forever, daemon=True)
    backend_thread.start()

    # 3. Config + proxy on a free port.
    proxy_port = _free_port()
    config_path = workdir / "config.yaml"
    log_path = workdir / "proxy.log"
    config_path.write_text(
        f"""server:
  port: {proxy_port}
  routes:
    - path: "/api/v1"
      host: "http://127.0.0.1:{backend_port}"
      target_path: "/api/v1"
add_header:
  X-Test: ["browser-value"]
caching:
  enabled: false
  ttl: 3600
blacklist: []
logger:
  level: "warn"
  file: {str(log_path)!r}
"""
    )

    proxy_proc = subprocess.Popen(
        [
            "go", "run", ".",
            "-cert", str(cert_path),
            "-key", str(key_path),
            "-config", str(config_path),
        ],
        cwd=REPO_ROOT,
    )
    try:
        _wait_for_port(proxy_port)
        yield {
            "proxy_port": proxy_port,
            "backend_port": backend_port,
        }
    finally:
        proxy_proc.terminate()
        with contextlib.suppress(subprocess.TimeoutExpired):
            proxy_proc.wait(timeout=5)
        if proxy_proc.poll() is None:
            proxy_proc.kill()
        httpd.shutdown()
        httpd.server_close()


@pytest.fixture(scope="module")
def driver() -> Iterator[webdriver.Chrome]:
    opts = Options()
    opts.add_argument("--headless=new")
    opts.add_argument("--no-sandbox")
    opts.add_argument("--disable-dev-shm-usage")
    # Self-signed cert: tell Chrome to accept it for our test origin.
    opts.add_argument("--ignore-certificate-errors")
    opts.set_capability("acceptInsecureCerts", True)

    drv = webdriver.Chrome(options=opts)
    try:
        yield drv
    finally:
        drv.quit()


def test_browser_loads_proxied_page(stack, driver):
    url = f"https://127.0.0.1:{stack['proxy_port']}/api/v1/landing"
    driver.get(url)

    WebDriverWait(driver, 5).until(
        EC.presence_of_element_located((By.ID, "hello"))
    )

    assert driver.find_element(By.ID, "hello").text == "Hello from backend"
    assert driver.find_element(By.ID, "path").text == "/api/v1/landing"


def test_browser_follows_subpath_with_query(stack, driver):
    url = f"https://127.0.0.1:{stack['proxy_port']}/api/v1/users?id=42"
    driver.get(url)

    WebDriverWait(driver, 5).until(
        EC.presence_of_element_located((By.ID, "path"))
    )

    # Backend echoes the path it received from the proxy. Query string is on
    # the URL but not rendered in the body, so we just verify the path tail
    # was preserved end-to-end.
    assert driver.find_element(By.ID, "path").text == "/api/v1/users"
