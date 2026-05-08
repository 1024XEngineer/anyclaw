#!/usr/bin/env python3
"""Serve static files with an index.html fallback for React Router."""

from __future__ import annotations

import argparse
from http.server import SimpleHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from urllib.error import HTTPError, URLError
from urllib.parse import urljoin
from urllib.request import Request, urlopen


REGISTRY_PROXY_TARGET = ""


class SpaRequestHandler(SimpleHTTPRequestHandler):
    def end_headers(self) -> None:
        if self.path == "/" or self.path == "/index.html" or not Path(self.path).suffix:
            self.send_header("Cache-Control", "no-store")
        elif self.path.startswith("/assets/"):
            self.send_header("Cache-Control", "public, max-age=31536000, immutable")
        super().end_headers()

    def do_POST(self) -> None:  # noqa: N802
        self.proxy_or_reject()

    def do_PUT(self) -> None:  # noqa: N802
        self.proxy_or_reject()

    def do_PATCH(self) -> None:  # noqa: N802
        self.proxy_or_reject()

    def do_DELETE(self) -> None:  # noqa: N802
        self.proxy_or_reject()

    def do_OPTIONS(self) -> None:  # noqa: N802
        self.proxy_or_reject()

    def send_head(self):  # type: ignore[override]
        if self.path == "/v1" or self.path.startswith("/v1/") or self.path.startswith("/v1?"):
            self.proxy_registry_request()
            return None

        requested_path = Path(self.translate_path(self.path))
        if requested_path.exists():
            return super().send_head()

        if self.path.startswith("/assets/"):
            return super().send_head()

        self.path = "/index.html"
        return super().send_head()

    def proxy_or_reject(self) -> None:
        if self.path == "/v1" or self.path.startswith("/v1/") or self.path.startswith("/v1?"):
            self.proxy_registry_request()
            return
        self.send_error(405, "Method not allowed")

    def proxy_registry_request(self) -> None:
        if not REGISTRY_PROXY_TARGET:
            self.send_error(502, "Registry proxy target is not configured")
            return

        target = urljoin(REGISTRY_PROXY_TARGET.rstrip("/") + "/", self.path.lstrip("/"))
        headers = {
            key: value
            for key, value in self.headers.items()
            if key.lower() not in {"host", "connection", "content-length", "accept-encoding"}
        }
        headers["X-Forwarded-Host"] = self.headers.get("Host", "")
        headers["X-Forwarded-Proto"] = "https" if self.headers.get("X-Forwarded-Proto") == "https" else "http"
        body = None
        if self.command in {"POST", "PUT", "PATCH"}:
            length = int(self.headers.get("Content-Length", "0") or "0")
            body = self.rfile.read(length) if length > 0 else b""

        request = Request(target, data=body, headers=headers, method=self.command)
        try:
            with urlopen(request, timeout=30) as response:
                body_bytes = response.read()
                self.send_response(response.status)
                for key, value in response.headers.items():
                    if key.lower() in {"connection", "transfer-encoding", "content-encoding", "content-length"}:
                        continue
                    self.send_header(key, value)
                self.send_header("Content-Length", str(len(body_bytes)))
                self.send_header("Connection", "close")
                self.end_headers()
                self.wfile.write(body_bytes)
                self.close_connection = True
        except HTTPError as err:
            body_bytes = err.read()
            self.send_response(err.code)
            for key, value in err.headers.items():
                if key.lower() in {"connection", "transfer-encoding", "content-encoding", "content-length"}:
                    continue
                self.send_header(key, value)
            self.send_header("Content-Length", str(len(body_bytes)))
            self.send_header("Connection", "close")
            self.end_headers()
            self.wfile.write(body_bytes)
            self.close_connection = True
        except URLError as err:
            self.send_error(502, f"Registry proxy failed: {err.reason}")


def main() -> None:
    global REGISTRY_PROXY_TARGET

    parser = argparse.ArgumentParser(description="Serve a static SPA directory.")
    parser.add_argument("--host", default="0.0.0.0")
    parser.add_argument("--port", type=int, default=8080)
    parser.add_argument("--directory", default=".")
    parser.add_argument("--registry-target", default="")
    args = parser.parse_args()
    REGISTRY_PROXY_TARGET = args.registry_target

    def handler(*handler_args, **handler_kwargs):
        return SpaRequestHandler(*handler_args, directory=args.directory, **handler_kwargs)

    server = ThreadingHTTPServer((args.host, args.port), handler)
    server.serve_forever()


if __name__ == "__main__":
    main()
