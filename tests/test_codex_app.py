from __future__ import annotations

import tempfile
import tomllib
import unittest
from pathlib import Path

from ghc_launch_codex.codex_app import configure_codex_app, restore_codex_app
from ghc_launch_codex.constants import PROFILE_NAME, PROVIDER_NAME
from ghc_launch_codex.paths import AppPaths


def temp_paths(root: Path) -> AppPaths:
    codex_dir = root / ".codex"
    state_dir = root / ".config" / "ghc-launch-codex"
    return AppPaths(
        codex_dir=codex_dir,
        codex_config=codex_dir / "config.toml",
        model_catalog=codex_dir / "githubcopilot-launch-models.json",
        state_dir=state_dir,
        auth_file=state_dir / "auth.json",
        restore_file=state_dir / "restore.json",
        backup_dir=state_dir / "backup",
    )


class CodexAppConfigTests(unittest.TestCase):
    def test_configure_and_restore_preserves_previous_root_values(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            paths = temp_paths(Path(tmp))
            paths.codex_dir.mkdir(parents=True)
            paths.codex_config.write_text(
                'profile = "old"\nmodel = "old-model"\n\n[profiles.old]\nmodel = "old-model"\n',
                encoding="utf-8",
            )
            models = [{"id": "gpt-5.3-codex", "name": "GPT 5.3 Codex"}]
            configure_codex_app(
                model="gpt-5.3-codex",
                models=models,
                base_url="http://127.0.0.1:11435",
                paths=paths,
            )
            parsed = tomllib.loads(paths.codex_config.read_text(encoding="utf-8"))
            self.assertEqual(parsed["profile"], PROFILE_NAME)
            self.assertEqual(parsed["model_provider"], PROVIDER_NAME)
            self.assertEqual(
                parsed["profiles"][PROFILE_NAME]["openai_base_url"],
                "http://127.0.0.1:11435/v1/",
            )
            self.assertEqual(
                parsed["model_providers"][PROVIDER_NAME]["wire_api"],
                "responses",
            )
            self.assertTrue(paths.model_catalog.exists())

            self.assertTrue(restore_codex_app(paths))
            restored = tomllib.loads(paths.codex_config.read_text(encoding="utf-8"))
            self.assertEqual(restored["profile"], "old")
            self.assertEqual(restored["model"], "old-model")
            self.assertNotIn(PROFILE_NAME, restored.get("profiles", {}))
            self.assertFalse(paths.model_catalog.exists())


if __name__ == "__main__":
    unittest.main()
