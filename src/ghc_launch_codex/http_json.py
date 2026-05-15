from __future__ import annotations

import json
import urllib.error
import urllib.request
from typing import Any


class HttpError(RuntimeError):
    def __init__(self, status: int, url: str, body: bytes):
        text = body.decode("utf-8", "replace")
        super().__init__(f"HTTP {status} from {url}: {text[:1000]}")
        self.status = status
        self.url = url
        self.body = body


def request_json(
    method: str,
    url: str,
    *,
    headers: dict[str, str] | None = None,
    body: Any | None = None,
    timeout: int = 30,
) -> Any:
    data = None
    request_headers = dict(headers or {})
    if body is not None:
        data = json.dumps(body).encode("utf-8")
        request_headers.setdefault("Content-Type", "application/json")
    request_headers.setdefault("Accept", "application/json")
    req = urllib.request.Request(url, data=data, headers=request_headers, method=method)
    try:
        with urllib.request.urlopen(req, timeout=timeout) as res:
            payload = res.read()
    except urllib.error.HTTPError as exc:
        raise HttpError(exc.code, url, exc.read()) from exc
    if not payload:
        return None
    return json.loads(payload)

