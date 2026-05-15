from __future__ import annotations

import json
import os
import re
import shutil
import subprocess
import time
import tomllib
from pathlib import Path
from typing import Any

from .catalog import dumps_catalog
from .constants import CATALOG_FILENAME, PROFILE_NAME, PROVIDER_NAME
from .paths import AppPaths, default_paths


ROOT_KEYS = ("profile", "model", "model_provider", "model_catalog_json")


def _quote(value: str) -> str:
    return json.dumps(value)


def _backup_file(path: Path, backup_dir: Path) -> None:
    if not path.exists():
        return
    backup_dir.mkdir(parents=True, exist_ok=True)
    stamp = int(time.time())
    target = backup_dir / f"{path.name}.{stamp}"
    shutil.copy2(path, target)
    backups = sorted(backup_dir.glob(f"{path.name}.*"), key=lambda item: item.stat().st_mtime, reverse=True)
    for stale in backups[5:]:
        stale.unlink(missing_ok=True)


def _atomic_write(path: Path, text: str) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    tmp = path.with_suffix(path.suffix + ".tmp")
    tmp.write_text(text, encoding="utf-8")
    os.replace(tmp, path)


def _root_value(parsed: dict[str, Any], key: str) -> tuple[bool, str | None]:
    if key in parsed:
        value = parsed[key]
        return True, str(value) if value is not None else None
    return False, None


def _section_range(text: str, header: str) -> tuple[int, int] | None:
    lines = text.splitlines(keepends=True)
    start = None
    for index, line in enumerate(lines):
        if line.strip() == header:
            start = index
            break
    if start is None:
        return None
    end = len(lines)
    for index in range(start + 1, len(lines)):
        stripped = lines[index].strip()
        if stripped.startswith("[") and stripped.endswith("]"):
            end = index
            break
    return start, end


def _remove_section(text: str, header: str) -> str:
    lines = text.splitlines(keepends=True)
    section = _section_range(text, header)
    if section is None:
        return text
    start, end = section
    del lines[start:end]
    return "".join(lines)


def _upsert_section(text: str, header: str, body: str) -> str:
    section_text = f"{header}\n{body.rstrip()}\n"
    lines = text.splitlines(keepends=True)
    section = _section_range(text, header)
    if section is not None:
        start, end = section
        lines[start:end] = [line + "\n" for line in section_text.rstrip("\n").split("\n")]
        return "".join(lines)
    if text and not text.endswith("\n"):
        text += "\n"
    if text and not text.endswith("\n\n"):
        text += "\n"
    return text + section_text


def _set_root_values(text: str, values: dict[str, str]) -> str:
    lines = text.splitlines(keepends=True)
    table_index = next(
        (index for index, line in enumerate(lines) if line.lstrip().startswith("[") and line.strip().endswith("]")),
        len(lines),
    )
    root = lines[:table_index]
    rest = lines[table_index:]
    seen: set[str] = set()
    pattern = re.compile(r"^(\s*)([A-Za-z0-9_-]+)(\s*=)")
    for index, line in enumerate(root):
        match = pattern.match(line)
        if match and match.group(2) in values:
            key = match.group(2)
            root[index] = f"{key} = {_quote(values[key])}\n"
            seen.add(key)
    insertion = [f"{key} = {_quote(value)}\n" for key, value in values.items() if key not in seen]
    root.extend(insertion)
    return "".join(root + rest)


def _restore_root_values(text: str, saved: dict[str, dict[str, Any]]) -> str:
    lines = text.splitlines(keepends=True)
    table_index = next(
        (index for index, line in enumerate(lines) if line.lstrip().startswith("[") and line.strip().endswith("]")),
        len(lines),
    )
    root = lines[:table_index]
    rest = lines[table_index:]
    pattern = re.compile(r"^(\s*)([A-Za-z0-9_-]+)(\s*=)")
    existing: dict[str, int] = {}
    for index, line in enumerate(root):
        match = pattern.match(line)
        if match:
            existing[match.group(2)] = index
    for key in ROOT_KEYS:
        state = saved.get(key, {"present": False, "value": None})
        if state.get("present"):
            line = f"{key} = {_quote(str(state.get('value') or ''))}\n"
            if key in existing:
                root[existing[key]] = line
            else:
                root.append(line)
        elif key in existing:
            root[existing[key]] = ""
    return "".join(root + rest)


def _read_config(path: Path) -> tuple[str, dict[str, Any]]:
    text = path.read_text(encoding="utf-8") if path.exists() else ""
    parsed = tomllib.loads(text) if text.strip() else {}
    return text, parsed


def _save_restore_state(paths: AppPaths, parsed: dict[str, Any]) -> None:
    if paths.restore_file.exists() and parsed.get("profile") == PROFILE_NAME:
        return
    paths.restore_file.parent.mkdir(parents=True, exist_ok=True)
    state = {
        "root": {
            key: {"present": present, "value": value}
            for key in ROOT_KEYS
            for present, value in [_root_value(parsed, key)]
        }
    }
    _atomic_write(paths.restore_file, json.dumps(state, indent=2, sort_keys=True) + "\n")


def configure_codex_app(
    *,
    model: str,
    models: list[dict[str, Any]],
    base_url: str,
    paths: AppPaths | None = None,
) -> None:
    paths = paths or default_paths()
    text, parsed = _read_config(paths.codex_config)
    _save_restore_state(paths, parsed)
    _backup_file(paths.codex_config, paths.backup_dir)
    paths.codex_dir.mkdir(parents=True, exist_ok=True)
    _atomic_write(paths.model_catalog, dumps_catalog(models, model))

    model_catalog = str(paths.model_catalog)
    normalized_base = base_url.rstrip("/") + "/v1/"
    text = _set_root_values(
        text,
        {
            "profile": PROFILE_NAME,
            "model": model,
            "model_provider": PROVIDER_NAME,
            "model_catalog_json": model_catalog,
        },
    )
    text = _upsert_section(
        text,
        f"[profiles.{PROFILE_NAME}]",
        "\n".join(
            [
                f"openai_base_url = {_quote(normalized_base)}",
                f"model = {_quote(model)}",
                f"model_provider = {_quote(PROVIDER_NAME)}",
                f"model_catalog_json = {_quote(model_catalog)}",
            ]
        ),
    )
    text = _upsert_section(
        text,
        f"[model_providers.{PROVIDER_NAME}]",
        "\n".join(
            [
                f"name = {_quote('GitHub Copilot')}",
                f"base_url = {_quote(normalized_base)}",
                'wire_api = "responses"',
            ]
        ),
    )
    tomllib.loads(text)
    _atomic_write(paths.codex_config, text)


def restore_codex_app(paths: AppPaths | None = None) -> bool:
    paths = paths or default_paths()
    if not paths.codex_config.exists() and not paths.restore_file.exists():
        return False
    text, parsed = _read_config(paths.codex_config)
    if paths.restore_file.exists():
        state = json.loads(paths.restore_file.read_text(encoding="utf-8"))
        text = _restore_root_values(text, state.get("root", {}))
    elif parsed.get("profile") == PROFILE_NAME:
        text = _restore_root_values(text, {})
    text = _remove_section(text, f"[profiles.{PROFILE_NAME}]")
    text = _remove_section(text, f"[model_providers.{PROVIDER_NAME}]")
    tomllib.loads(text) if text.strip() else {}
    _backup_file(paths.codex_config, paths.backup_dir)
    _atomic_write(paths.codex_config, text)
    if paths.model_catalog.name == CATALOG_FILENAME:
        paths.model_catalog.unlink(missing_ok=True)
    paths.restore_file.unlink(missing_ok=True)
    return True


def launch_codex_app() -> None:
    system = os.uname().sysname if hasattr(os, "uname") else ""
    if system == "Darwin":
        subprocess.Popen(["open", "-a", "Codex"])
        return
    if system.startswith("MINGW") or system.startswith("MSYS") or os.name == "nt":
        subprocess.Popen(["cmd", "/c", "start", "", "Codex"])
        return
    raise RuntimeError("Codex App automatic launch is currently supported on macOS and Windows only.")
