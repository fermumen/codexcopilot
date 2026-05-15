from __future__ import annotations

import json
import sys
import urllib.error
import urllib.parse
import urllib.request
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from typing import Any

from .auth import CopilotAuth
from .copilot import copilot_api_base, copilot_headers, fetch_models


HOP_BY_HOP_HEADERS = {
    "connection",
    "content-length",
    "host",
    "keep-alive",
    "proxy-authenticate",
    "proxy-authorization",
    "te",
    "trailer",
    "transfer-encoding",
    "upgrade",
    "accept-encoding",
}


def _has_image(value: Any) -> bool:
    if isinstance(value, dict):
        kind = value.get("type")
        if kind in {"input_image", "image_url"}:
            return True
        return any(_has_image(item) for item in value.values())
    if isinstance(value, list):
        return any(_has_image(item) for item in value)
    return False


def _body_has_image(body: bytes) -> bool:
    try:
        return _has_image(json.loads(body.decode("utf-8")))
    except Exception:
        return False


def _initiator(body: bytes) -> str:
    try:
        payload = json.loads(body.decode("utf-8"))
    except Exception:
        return "agent"
    messages = payload.get("messages") or payload.get("input") or []
    if isinstance(messages, list) and messages:
        last = messages[-1]
        if isinstance(last, dict) and last.get("role") == "user":
            return "user"
    return "agent"


def _upstream_path(path: str) -> str:
    parsed = urllib.parse.urlsplit(path)
    raw_path = parsed.path
    if raw_path in {"/health", "/v1/health"}:
        return raw_path
    if raw_path == "/v1/models":
        upstream = "/models"
    elif raw_path.startswith("/v1/"):
        upstream = raw_path[3:]
    else:
        upstream = raw_path
    if parsed.query:
        upstream += "?" + parsed.query
    return upstream


class ProxyHandler(BaseHTTPRequestHandler):
    server_version = "GhcLaunchCodex/0.1.0"
    protocol_version = "HTTP/1.1"

    def log_message(self, fmt: str, *args: object) -> None:
        sys.stderr.write("%s - %s\n" % (self.address_string(), fmt % args))

    @property
    def auth(self) -> CopilotAuth:
        return self.server.auth  # type: ignore[attr-defined]

    def _cors(self) -> None:
        self.send_header("Access-Control-Allow-Origin", "*")
        self.send_header("Access-Control-Allow-Headers", "authorization,content-type,openai-beta")
        self.send_header("Access-Control-Allow-Methods", "GET,POST,OPTIONS")

    def do_OPTIONS(self) -> None:
        self.send_response(204)
        self._cors()
        self.send_header("Content-Length", "0")
        self.end_headers()

    def do_GET(self) -> None:
        path = urllib.parse.urlsplit(self.path).path
        if path in {"/health", "/v1/health"}:
            self._json(200, {"ok": True})
            return
        if path in {"/models", "/v1/models"}:
            models = fetch_models(self.auth)
            self._json(200, {"object": "list", "data": models})
            return
        self._json(404, {"error": {"message": f"Unknown path {path}"}})

    def do_POST(self) -> None:
        length = int(self.headers.get("Content-Length", "0"))
        body = self.rfile.read(length) if length else b""
        upstream = urllib.parse.urljoin(copilot_api_base(self.auth), _upstream_path(self.path))
        headers = copilot_headers(
            self.auth,
            initiator=_initiator(body),
            vision=_body_has_image(body) if body else False,
        )
        for key, value in self.headers.items():
            if key.lower() not in HOP_BY_HOP_HEADERS and key.lower() != "authorization":
                headers[key] = value
        req = urllib.request.Request(upstream, data=body, headers=headers, method="POST")
        try:
            with urllib.request.urlopen(req, timeout=600) as res:
                self.send_response(res.status)
                self._cors()
                self.send_header("Connection", "close")
                for key, value in res.headers.items():
                    if key.lower() not in HOP_BY_HOP_HEADERS:
                        self.send_header(key, value)
                self.end_headers()
                while True:
                    chunk = res.read(65536)
                    if not chunk:
                        break
                    self.wfile.write(chunk)
                    self.wfile.flush()
        except urllib.error.HTTPError as exc:
            payload = exc.read()
            self.send_response(exc.code)
            self._cors()
            self.send_header("Content-Type", exc.headers.get("Content-Type", "application/json"))
            self.send_header("Content-Length", str(len(payload)))
            self.send_header("Connection", "close")
            self.end_headers()
            self.wfile.write(payload)

    def _json(self, status: int, payload: dict[str, Any]) -> None:
        body = json.dumps(payload).encode("utf-8")
        self.send_response(status)
        self._cors()
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)


class CopilotProxyServer(ThreadingHTTPServer):
    def __init__(self, address: tuple[str, int], auth: CopilotAuth):
        super().__init__(address, ProxyHandler)
        self.auth = auth


def make_server(host: str, port: int, auth: CopilotAuth) -> CopilotProxyServer:
    return CopilotProxyServer((host, port), auth)
