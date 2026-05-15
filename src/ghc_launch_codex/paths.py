from __future__ import annotations

import os
import platform
from dataclasses import dataclass
from pathlib import Path

from .constants import APP_NAME, CATALOG_FILENAME


@dataclass(frozen=True)
class AppPaths:
    codex_dir: Path
    codex_config: Path
    model_catalog: Path
    state_dir: Path
    auth_file: Path
    restore_file: Path
    backup_dir: Path


def config_home() -> Path:
    system = platform.system()
    if system == "Windows":
        root = os.environ.get("APPDATA")
        return Path(root) if root else Path.home() / "AppData" / "Roaming"
    if system == "Darwin":
        return Path.home() / "Library" / "Application Support"
    return Path(os.environ.get("XDG_CONFIG_HOME", Path.home() / ".config"))


def default_paths() -> AppPaths:
    codex_dir = Path.home() / ".codex"
    state_dir = config_home() / APP_NAME
    return AppPaths(
        codex_dir=codex_dir,
        codex_config=codex_dir / "config.toml",
        model_catalog=codex_dir / CATALOG_FILENAME,
        state_dir=state_dir,
        auth_file=state_dir / "auth.json",
        restore_file=state_dir / "codex-app-restore.json",
        backup_dir=state_dir / "backup",
    )

