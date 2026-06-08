# codexcopilot Memory & Notes

## Project Location
- `/home/ubuntu/developer/codexcopilot`
- Module: `github.com/fermumen/codexcopilot`
- Go 1.22, zero external dependencies

## Origin Session
- **Session ID**: `019e2d71-d84a-73b1-b2bc-9d9396722516`
- **Date**: 2026-05-15, 21:01:36–21:18:09 UTC
- **CLI**: Codex CLI v0.130.0, source: vscode
- **Originator**: `codex_chatgpt_ios_remote` (delegated from ChatGPT iOS app to Codex CLI)
- **Model**: GPT-5.5 (OpenAI), reasoning effort: xhigh
- **Initial CWD**: `/home/ubuntu/developer`
- **Original goal**: Research Ollama's `ollama launch codex-app` + OpenCode's first-party GitHub Copilot provider, then create a Python prototype `ghc-launch-codex` that does the same thing but routes through GitHub Copilot auth/models
- **Python prototype created at**: `/home/ubuntu/developer/ghc-launch-codex`
- **Python → Go port**: The Python code was later ported to this Go project by the user.
- **Git commit (Python prototype)**: `da81f2c Initial GitHub Copilot Codex launcher`

## Full Session Narrative

### 1. User Request (21:01:42 UTC)
The user saw Ollama 0.24 announcement (`ollama launch codex-app`) and asked the agent to:
- Clone Ollama repo to investigate how `launch codex-app` works
- Investigate OpenCode's first-party GitHub Copilot provider
- Build a similar tool called `githubcopilot launch codex-app` using the user's GitHub Copilot login
- Put the work in `./ghc-launch-codex` and init a git repo

### 2. Initial Research Phase (21:01:54–21:02:14)

**Web searches performed:**
- `Ollama "ollama launch codex-app"`
- `Ollama 0.24 codex-app`
- `opencode GitHub Copilot plugin`
- Fetched: `https://docs.ollama.com/integrations/codex-app`
- Fetched: Ollama source files (PR #16120, `cmd/launch/`)
- Fetched: OpenCode GitHub Copilot provider source on GitHub

**Research on Ollama's codex-app integration (PR #16120):**

From `cmd/cmd.go`:
- CLI command registered at `cmd/cmd.go:2495` via `launch.LaunchCmd(goAgainCmd, codexAppCmd)`.

From `cmd/launch/registry.go:90`:
- `codex-app` registered with aliases: `codex-desktop`, `codex-gui`.
- Registry maps platform names → launch configs (e.g. "darwin", "windows", "linux" with `launch_type` and `guid`).

From `cmd/launch/codex_app.go`:
- Key function: `NewCodexAppCommand()` returns a Cobra command with `RunE`.
- Flow:
  1. Resolve Codex App path via `plist`, `where`, or `findCodexApp()`.
  2. Load existing `~/.codex/config.toml`, save previous values to `~/.ollama/launch/codex-app-restore.json`.
  3. Write new config: `model`, `model_provider = "ollama-launch-codex-app"`, `model_catalog_json`.
  4. Provider section: `[model_providers.ollama-launch-codex-app]` with `name = "Ollama"`, `base_url = "<ollama-url>/v1/"`.
  5. Write model catalog via `writeCodexAppModelCatalog()`.
  6. Launch Codex App via `exec.Command("open", "-a", "Codex")` on macOS, `exec.Command("cmd", "/c", "start", ...)` on Windows.
- Restore file keys: `profile`, `model_provider`, `model_catalog_json`.
- Set `model` and `profile` in root config.

OpenCode's GitHub Copilot provider (from anomalyco/opencode source):

Provider location: `packages/opencode/src/plugin/github-copilot/copilot.ts`

From reading the GitHub file directly:
- Provider id: `github-copilot`
- Default API base: `https://api.githubcopilot.com`
- **Auth**: Full GitHub device code OAuth flow:
  - Device code URL: `https://github.com/login/device/code`
  - Token URL: `https://github.com/login/oauth/access_token`
  - Client ID: `Ov23li8tweQw6odWQebz` (public Copilot OAuth app)
  - Scopes: `read:user`, `copilot`
- Token stored in: `Global.Path.data/auth.json` with mode `0600`
- Token format: `{"access": "...", "refresh": "..."}` (same value for both; no real refresh flow)
- For Enterprise: device code and token URLs use the enterprise domain instead of `github.com`

**Copilot API requests:**
- `GET /models` to discover available models
- Model capabilities include: `capabilities.supports.responses`, `capabilities.supports.chat_completions`, `capabilities.supports.reasoning_effort`
- Filtering: `model_picker_enabled`, `policy.state != "disabled"`, `capabilities.supports.responses` or `capabilities.supports.tool_use`
- Headers added: `Authorization: Bearer <token>`, `User-Agent: github/opencode`, `Openai-Intent`, `Copilot-Integration-Id`, `X-Initiator` (inferred as `user`/`agent`)

### 3. Subagent Spawns for Deep Research (21:02:30–21:07:00)

Two subagents were created:

**Subagent 1 – Ollama research:**
- Cloned: `https://github.com/ollama/ollama` (shallow, depth 1)
- Analyzed `cmd/launch/` directory
- Found `codex_app.go` containing the full implementation
- Extracted: config writing pattern, restore file format, model catalog generation, app launch code
- Key detail: Uses `model_providers.ollama-launch-codex-app` in TOML, sets `profile = "codex-app"` in root
- Finds Codex.app path via `mdfind 'kMDItemContentType == "com.apple.application-bundle" && kMDItemFSName == "Codex.app"'` (macOS Spotlight)

**Subagent 2 – OpenCode research:**
- Cloned: `https://github.com/anomalyco/opencode` (shallow, depth 1)
- Navigated to `packages/opencode/src/plugin/github-copilot/`
- Found `copilot.ts` – the core implementation
- Auth flow: device code grant, HTTP polling, token storage in plugin data directory (0600 permissions)
- Enterprise support: replace `github.com` with customer domain
- Split `apiBaseUrl` into separate properties: `chatBaseUrl` and `modelAwareBaseUrl`
- Key insight: Copilot API base URL is versioned: `https://api.githubcopilot.com` for models, `https://api.githubcopilot.com/responses` for the Responses API
- Notes: No token refresh, same value stored as both `access` and `refresh`

### 4. Building the Python Prototype (21:07:00–21:17:00)

The agent created the `ghc-launch-codex` Python project at `/home/ubuntu/developer/ghc-launch-codex`.

**Files created:**

| File | Purpose |
|------|---------|
| `README.md` | Project overview and usage |
| `pyproject.toml` | Python project config (setuptools) |
| `githubcopilot` | Executable shell script entry point |
| `src/ghc_launch_codex/__init__.py` | Package init |
| `src/ghc_launch_codex/auth.py` | GitHub device OAuth flow, token persistence |
| `src/ghc_launch_codex/catalog.py` | Codex App model catalog JSON generation |
| `src/ghc_launch_codex/cli.py` | CLI argument parsing and command routing |
| `src/ghc_launch_codex/codex_app.py` | Codex config patching, restore, app launch |
| `src/ghc_launch_codex/constants.py` | Shared constants (paths, defaults) |
| `src/ghc_launch_codex/copilot.py` | Copilot API model fetch, filtering |
| `src/ghc_launch_codex/http_json.py` | HTTP helper for JSON API calls |
| `src/ghc_launch_codex/paths.py` | Platform-specific config directory resolution |
| `src/ghc_launch_codex/proxy.py` | Local HTTP proxy, path remapping, header injection |
| `tests/test_codex_app.py` | Unit tests for config/restore |
| `tests/test_copilot.py` | Unit tests for model filtering |

**Final file list (after cleanup):**
```
./.gitignore
./README.md
./githubcopilot
./pyproject.toml
./src/ghc_launch_codex/__init__.py
./src/ghc_launch_codex/auth.py
./src/ghc_launch_codex/catalog.py
./src/ghc_launch_codex/cli.py
./src/ghc_launch_codex/codex_app.py
./src/ghc_launch_codex/constants.py
./src/ghc_launch_codex/copilot.py
./src/ghc_launch_codex/http_json.py
./src/ghc_launch_codex/paths.py
./src/ghc_launch_codex/proxy.py
./tests/test_codex_app.py
./tests/test_copilot.py
```

### 5. Verification (21:17:26 UTC)

- `./githubcopilot auth status` → correctly reported no saved GitHub Copilot login
- `PYTHONPATH=src python3 -m unittest discover -s tests -v` → ran tests
- `PYTHONPATH=src python3 -m compileall src tests` → compiled cleanly
- `./githubcopilot --help` → displayed usage

### 6. Session Summary (21:18:09 UTC)

The agent summarized with commit `da81f2c`, listing CLI commands:
- `./githubcopilot auth login`
- `./githubcopilot models`
- `./githubcopilot serve`
- `./githubcopilot launch codex-app`
- `./githubcopilot launch codex app`

**Token usage**: ~2M total tokens (1.8M input, 29K output), ~17.5 minutes wall time.

## Key Research Findings Mined from Session

### Ollama Launch Pattern (codex_app.go)
- Writes `~/.codex/config.toml` with `model_provider = "ollama-launch-codex-app"`
- Writes model catalog to `~/.codex/ollama-launch-models.json`
- Saves restore state to `~/.ollama/launch/codex-app-restore.json`
- Restore puts back: `model`, `model_provider`, `model_catalog_json`, `profile`
- Provider section name: `ollama-launch-codex-app`, base_url points to Ollama's local endpoint
- App launch: `open -a Codex` (macOS), `cmd /c start codex:` (Windows)
- Finds Codex.app via macOS Spotlight (`mdfind`)

### OpenCode Copilot Provider Pattern (copilot.ts)
- OAuth device flow with client id `Ov23li8tweQw6odWQebz`
- Token file at `{dataDir}/auth.json` (0600)
- No real token refresh mechanism
- API base: `https://api.githubcopilot.com`
- Enterprise: domain replacement from provided enterprise URL
- Models endpoint: `GET /models`
- Model filter: `capabilities.supports.responses === true`, `policy.state !== "disabled"`, `model_picker_enabled === true`
- Key headers: `Authorization`, `User-Agent: github/opencode`, `Openai-Intent`, `Copilot-Integration-Id: vscode-chat`, `X-Initiator`
- `X-Initiator` inferred from session state in OpenCode; wire-proxy must infer from request body

## Key Design Decisions (from the follow-up Go implementation)

1. **Go over Python**: Zero-dependency single binary; stdlib `net/http`, `os/exec`, etc. suffice.
2. **Proxy pattern over native provider**: Codex App does not expose the plugin hook; a local HTTP proxy is the practical integration point.
3. **Responses API**: `wire_api = "responses"` is critical — Codex App uses the Responses API, not Chat Completions.
4. **Profile-v2 over legacy profile**: Modern Codex CLI uses profile-v2 files (`~/.codex/<name>.config.toml`) with `--profile <name>`, not legacy root `profile = "..."`.
5. **OAuth client id**: `Ov23li8tweQw6odWQebz` from OpenCode; overridable via `GHC_COPILOT_CLIENT_ID` env var.
6. **Initiator inference at wire level**: OpenCode has internal session state; proxy infers from the last `input[]` item's `role`.
7. **Reasoning effort in catalog**: Copilot's `capabilities.supports.reasoning_effort` drives `supported_reasoning_levels`; `gpt-5.4*` models get `xhigh`.
8. **Systemd service**: `install-server-service` provides persistent proxy via `~/.config/systemd/user/codexcopilot.service`.
9. **Codex wrapper**: `codexcopilot codex` runs proxy + `codex --profile codexcopilot-codex-app` in one shot, restores on exit.

## Known Issues / Technical Debt
- No real token refresh flow (token from OAuth is used as-is)
- Codex App auto-launch only on macOS (`open -a Codex`) and Windows
- Linux users must use `codex` or `responses-server` subcommands
- Initiator inference is a heuristic (last input role) rather than session-aware
- Config editing is line-oriented rather than using a TOML library; could break on unusual formatting

## Source Code Map (Go project)

| File | Approx Lines | Purpose |
|------|-------------|---------|
| `cmd/codexcopilot/main.go` | 557 | CLI commands, orchestration |
| `internal/auth/auth.go` | 200 | OAuth device flow, token persistence |
| `internal/copilot/copilot.go` | 314 | Copilot API, model filtering, headers |
| `internal/catalog/catalog.go` | 108 | Codex model catalog JSON builder |
| `internal/codex/codex.go` | 370 | Config TOML editing, restore, app launch |
| `internal/proxy/proxy.go` | 276 | HTTP proxy, path mapping, initiator inference |
| `internal/paths/paths.go` | 60 | Platform-specific path resolution |

## Useful Commands

```bash
# Build
go build -o bin/codexcopilot ./cmd/codexcopilot

# Test
go test ./...

# Login
go run ./cmd/codexcopilot auth login

# Enterprise
go run ./cmd/codexcopilot auth login --enterprise-url https://github.example.com

# Launch (requires auth)
go run ./cmd/codexcopilot launch codex-app

# Headless Codex wrapper
go run ./cmd/codexcopilot codex

# Managed proxy server
go run ./cmd/codexcopilot responses-server --host 0.0.0.0 --port 11435

# Systemd install (Linux)
go run ./cmd/codexcopilot install-server-service

# Provider patch (remote)
go run ./cmd/codexcopilot provider patch --base-url http://SERVER:11435/v1/
```

## References
- Ollama PR #16120 — codex-app launch integration
- Ollama source: `cmd/launch/codex_app.go`, `cmd/launch/registry.go`
- OpenCode source: `packages/opencode/src/plugin/github-copilot/copilot.ts`
- Codex App API: Responses API, profile-v2 config, `--profile` flag
- GitHub Copilot API: `api.githubcopilot.com`, device OAuth flow, Copilot headers
