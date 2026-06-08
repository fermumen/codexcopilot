# codexcopilot Technical Documentation

## Overview

`codexcopilot` is a Go-based CLI tool that enables Codex App to use GitHub Copilot models through a local OpenAI-compatible proxy. It mirrors Ollama's `ollama launch codex-app` integration pattern.

## Architecture

```
┌─────────────┐     ┌──────────────────┐     ┌──────────────────────┐
│  Codex App  │────▶│  Local Proxy     │────▶│  GitHub Copilot API  │
│             │     │  127.0.0.1:11435 │     │  api.githubcopilot.com│
└─────────────┘     └──────────────────┘     └──────────────────────┘
                           │
                    ┌──────┴──────┐
                    │  Codex Config│
                    │  ~/.codex/   │
                    └─────────────┘
```

### Components

| Package | Responsibility |
|---------|----------------|
| `cmd/codexcopilot` | CLI command routing, orchestration |
| `internal/auth` | GitHub device OAuth flow, token storage |
| `internal/copilot` | Copilot API URLs, headers, model discovery/filtering |
| `internal/catalog` | Codex App model catalog generation |
| `internal/codex` | Codex config writes, restore state, app launch |
| `internal/proxy` | Local HTTP proxy, request header adaptation |
| `internal/paths` | Platform-specific config paths |

## Command Structure

```
codexcopilot auth login|logout|status
codexcopilot models
codexcopilot provider patch|restore
codexcopilot responses-server
codexcopilot install-server-service
codexcopilot codex
codexcopilot launch codex-app
```

## Launch Flow (`codexcopilot launch codex-app`)

1. **Auth**: Load saved GitHub OAuth token or start device login
2. **Models**: Fetch models from `GET https://api.githubcopilot.com/models`
3. **Filter**: Keep only OpenAI models supporting Responses API
4. **Select**: Default to `gpt-5.4-codex` > `gpt-5.4` > first usable model
5. **Catalog**: Write `~/.codex/codexcopilot-models.json`
6. **Backup**: Save previous Codex root config values
7. **Configure**: Update `~/.codex/config.toml` with provider settings
8. **Proxy**: Start HTTP server on `127.0.0.1:11435`
9. **Launch**: Attempt to open Codex App (macOS/Windows)

## Proxy Behavior

### Path Mapping
```
/v1/models    → /models
/v1/responses → /responses
/v1/...       → /...
```

### Added Headers
```
Authorization: Bearer <token>
User-Agent: codexcopilot/0.4.1
Editor-Version: codexcopilot/0.4.1
Editor-Plugin-Version: codexcopilot/0.4.1
Copilot-Integration-Id: vscode-chat
Openai-Intent: conversation-edits
X-Initiator: user|agent
Copilot-Vision-Request: true (when image input detected)
```

### Initiator Inference
- Last `input[]` item with `role: "user"` → `X-Initiator: user`
- Tool/function-call-output/assistant continuation → `X-Initiator: agent`
- Unknown/malformed → `X-Initiator: user`

## Files Touched

```
~/.codex/config.toml
~/.codex/codexcopilot-codex-app.config.toml
~/.codex/codexcopilot-models.json
<config-home>/codexcopilot/auth.json
<config-home>/codexcopilot/codex-app-restore.json
<config-home>/codexcopilot/backup/
```

## Codex Config Written

### `~/.codex/config.toml` (root)
```toml
model = "<selected>"
model_provider = "codexcopilot-codex-app"
model_catalog_json = "/home/you/.codex/codexcopilot-models.json"

[model_providers.codexcopilot-codex-app]
name = "GitHub Copilot"
base_url = "http://127.0.0.1:11435/v1/"
wire_api = "responses"
```

### `~/.codex/codexcopilot-codex-app.config.toml` (profile-v2)
```toml
model = "<selected>"
model_provider = "codexcopilot-codex-app"
model_catalog_json = "/home/you/.codex/codexcopilot-models.json"

[model_providers.codexcopilot-codex-app]
name = "GitHub Copilot"
base_url = "http://127.0.0.1:11435/v1/"
wire_api = "responses"
```

## Model Selection

Filters Copilot models for:
- `model_picker_enabled: true`
- `policy.state != "disabled"`
- OpenAI model family (gpt-, o1, o3, o4 prefixes)
- Supports `/responses` or `/v1/responses` endpoint

Preference order:
1. `gpt-5.4-codex`
2. `gpt-5.4`
3. `gpt-5.3-codex`
4. `gpt-5.2-codex`
5. `gpt-5.1-codex`
6. `gpt-5-codex`
7. `gpt-5.1`
8. `gpt-5`
9. First usable model

## Enterprise Support

For GitHub Enterprise:
- Auth: `codexcopilot auth login --enterprise-url https://github.example.com`
- Copilot API base: `https://copilot-api.<domain>`
- OAuth domain derived from enterprise URL

## Build & Test

```bash
go test ./...
go build -o bin/codexcopilot ./cmd/codexcopilot
```

No external Go dependencies - single static binary with standard library.

## Constraints

- Auto Codex App launch: macOS/Windows only
- Linux/remote: use `codexcopilot codex` or `responses-server`
- Proxy must stay running during Codex App use
- Requires active GitHub Copilot subscription
- Model availability depends on account/org settings

## Reasoning Effort

For each model, the catalog JSON includes:
```json
"default_reasoning_level": "medium",
"supported_reasoning_levels": [
  {"effort": "low", "description": "Fast responses with lighter reasoning"},
  {"effort": "medium", "description": "Balances speed and reasoning depth"},
  {"effort": "high", "description": "Greater reasoning depth for complex problems"},
  {"effort": "xhigh", "description": "Extra high reasoning depth (gpt-5.4* only)"}
],
"supports_reasoning_summaries": false,
"default_reasoning_summary": "none"
```

Values come from Copilot model capabilities when available.
