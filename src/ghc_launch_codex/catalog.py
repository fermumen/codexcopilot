from __future__ import annotations

import json
from typing import Any

from .copilot import (
    model_context_window,
    model_supports_reasoning,
    model_supports_tools,
    model_supports_vision,
)


BASE_INSTRUCTIONS = """You are Codex, a coding agent. Follow the user's instructions, inspect local files before editing, and keep changes narrowly scoped."""


def codex_model_catalog(models: list[dict[str, Any]], selected: str) -> dict[str, Any]:
    entries = []
    for index, model in enumerate(models):
        model_id = str(model["id"])
        efforts = model_supports_reasoning(model)
        default_effort = "medium" if "medium" in efforts else (efforts[0] if efforts else None)
        context_window = model_context_window(model)
        entries.append(
            {
                "slug": model_id,
                "display_name": model.get("name") or model_id,
                "description": "GitHub Copilot model",
                "max_tokens": None,
                "context_window": context_window,
                "max_context_window": context_window,
                "auto_compact_token_limit": None,
                "effective_context_window_percent": 95,
                "default_reasoning_level": default_effort,
                "supported_reasoning_levels": efforts,
                "supports_reasoning_summaries": bool(efforts),
                "default_reasoning_summary": "auto" if efforts else None,
                "support_verbosity": False,
                "supports_verbosity": False,
                "default_verbosity": None,
                "supported_in_api": True,
                "supports_parallel_tool_calls": model_supports_tools(model),
                "supports_image_detail_original": False,
                "supports_search_tool": False,
                "input_modalities": ["text", "image"] if model_supports_vision(model) else ["text"],
                "output_modalities": ["text"],
                "shell_type": "default",
                "visibility": "list",
                "priority": 1000 - index,
                "base_instructions": BASE_INSTRUCTIONS,
                "model_messages": None,
                "apply_patch_tool_type": None,
                "web_search_tool_type": None,
                "truncation_policy": {"mode": "bytes", "limit": 10000},
                "experimental_supported_tools": [],
                "additional_speed_tiers": [],
                "availability_nux": None,
                "upgrade": None,
            }
        )
    entries.sort(key=lambda item: (item["slug"] != selected, -int(item["priority"])))
    return {"models": entries}


def dumps_catalog(models: list[dict[str, Any]], selected: str) -> str:
    return json.dumps(codex_model_catalog(models, selected), indent=2, sort_keys=True) + "\n"
