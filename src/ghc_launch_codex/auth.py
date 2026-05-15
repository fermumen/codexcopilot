from __future__ import annotations

import json
import os
import stat
import time
from dataclasses import dataclass
from pathlib import Path
from typing import Any

from .constants import DEFAULT_COPILOT_CLIENT_ID
from .http_json import HttpError, request_json
from .paths import AppPaths, default_paths


def _normalize_domain(url: str) -> str:
    return url.replace("https://", "").replace("http://", "").rstrip("/")


def _github_oauth_urls(enterprise_url: str | None) -> tuple[str, str]:
    domain = _normalize_domain(enterprise_url) if enterprise_url else "github.com"
    return (
        f"https://{domain}/login/device/code",
        f"https://{domain}/login/oauth/access_token",
    )


@dataclass(frozen=True)
class CopilotAuth:
    access_token: str
    enterprise_url: str | None = None


def _write_private_json(path: Path, payload: dict[str, Any]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    tmp = path.with_suffix(path.suffix + ".tmp")
    fd = os.open(tmp, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
    try:
        with os.fdopen(fd, "w", encoding="utf-8") as f:
            json.dump(payload, f, indent=2, sort_keys=True)
            f.write("\n")
    except Exception:
        try:
            tmp.unlink()
        finally:
            raise
    os.replace(tmp, path)
    try:
        path.chmod(stat.S_IRUSR | stat.S_IWUSR)
    except OSError:
        pass


def load_auth(paths: AppPaths | None = None) -> CopilotAuth | None:
    paths = paths or default_paths()
    if not paths.auth_file.exists():
        return None
    data = json.loads(paths.auth_file.read_text(encoding="utf-8"))
    token = data.get("access_token") or data.get("token")
    if not token:
        return None
    return CopilotAuth(
        access_token=token,
        enterprise_url=data.get("enterprise_url"),
    )


def save_auth(auth: CopilotAuth, paths: AppPaths | None = None) -> None:
    paths = paths or default_paths()
    _write_private_json(
        paths.auth_file,
        {
            "access_token": auth.access_token,
            "enterprise_url": auth.enterprise_url,
            "token_type": "oauth",
        },
    )


def logout(paths: AppPaths | None = None) -> bool:
    paths = paths or default_paths()
    if paths.auth_file.exists():
        paths.auth_file.unlink()
        return True
    return False


def login(
    *,
    paths: AppPaths | None = None,
    client_id: str = DEFAULT_COPILOT_CLIENT_ID,
    enterprise_url: str | None = None,
) -> CopilotAuth:
    paths = paths or default_paths()
    device_code_url, access_token_url = _github_oauth_urls(enterprise_url)
    device = request_json(
        "POST",
        device_code_url,
        body={"client_id": client_id, "scope": "read:user"},
    )
    verification_uri = device["verification_uri"]
    user_code = device["user_code"]
    print(f"Open {verification_uri} and enter code: {user_code}")
    print("Waiting for GitHub authorization...")

    interval = int(device.get("interval", 5))
    deadline = time.monotonic() + int(device.get("expires_in", 900))
    while time.monotonic() < deadline:
        time.sleep(interval)
        try:
            token = request_json(
                "POST",
                access_token_url,
                body={
                    "client_id": client_id,
                    "device_code": device["device_code"],
                    "grant_type": "urn:ietf:params:oauth:grant-type:device_code",
                },
            )
        except HttpError as exc:
            raise RuntimeError(str(exc)) from exc
        if token.get("access_token"):
            auth = CopilotAuth(
                access_token=token["access_token"],
                enterprise_url=enterprise_url,
            )
            save_auth(auth, paths)
            return auth
        error = token.get("error")
        if error == "authorization_pending":
            continue
        if error == "slow_down":
            interval += 5
            continue
        raise RuntimeError(token.get("error_description") or f"GitHub login failed: {token}")
    raise TimeoutError("GitHub device authorization expired.")
