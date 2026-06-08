# codexcopilot

`codexcopilot` is a small Go launcher that configures Codex App to use GitHub Copilot models through a local OpenAI-compatible proxy.

The command is intentionally shaped like Ollama's Codex App integration:

```bash
codexcopilot launch codex-app
```

It writes a temporary Codex App provider profile, starts a local proxy, and forwards Codex App's `/v1/responses` traffic to GitHub Copilot using your own Copilot login.

This is a local compatibility tool, not a GitHub, OpenAI, or Ollama product.

## Quick Start

Install the latest release:

```bash
curl -fsSL https://raw.githubusercontent.com/fermumen/codexcopilot/master/install.sh | sh
```

By default this installs to `~/.local/bin`. To choose another location:

```bash
curl -fsSL https://raw.githubusercontent.com/fermumen/codexcopilot/master/install.sh | CODEXCOPILOT_INSTALL_DIR=/usr/local/bin sh
```

Build the binary:

```bash
go build -o bin/codexcopilot ./cmd/codexcopilot
```

Log in with GitHub's device flow:

```bash
./bin/codexcopilot auth login
```

For GitHub Enterprise:

```bash
./bin/codexcopilot auth login --enterprise-url https://github.example.com
```

Launch Codex App against GitHub Copilot:

```bash
./bin/codexcopilot launch codex-app
```

Equivalent target spelling:

```bash
./bin/codexcopilot launch codex app
```

Useful options:

```bash
./bin/codexcopilot launch codex-app --model gpt-5.1-codex
./bin/codexcopilot launch codex-app --port 11435
```

The repo also includes a convenience wrapper:

```bash
./codexcopilot launch codex-app
```

The wrapper runs `bin/codexcopilot` when present, otherwise it falls back to `go run ./cmd/codexcopilot` if Go is installed.

## Why This Exists

Codex App can use custom model providers from `~/.codex/config.toml`, but GitHub Copilot is not a plain OpenAI-compatible endpoint. Copilot requests need GitHub OAuth auth plus Copilot-specific headers such as `Openai-Intent` and `X-Initiator`.

OpenCode solves this in-process with a first-party GitHub Copilot provider. Codex App does not currently expose that provider hook to this tool, so the practical design is a local proxy:

```text
Codex App
  -> http://127.0.0.1:11435/v1/responses
  -> codexcopilot proxy
  -> https://api.githubcopilot.com/responses
```

For GitHub Enterprise, the Copilot API base becomes:

```text
https://copilot-api.<enterprise-domain>
```

## Design Inputs

The implementation mirrors two existing systems:

- Ollama's `ollama launch codex-app` flow:
  - write `~/.codex/config.toml`
  - write a model catalog JSON file
  - save restore state before changing Codex root config
  - launch or relaunch Codex App where supported

- OpenCode's official GitHub Copilot provider:
  - use GitHub's device-code OAuth flow
  - use the public Copilot OAuth client id by default
  - fetch models from Copilot's `/models` endpoint
  - add Copilot headers on every model request
  - infer `X-Initiator` as `user` or `agent`

## Architecture

The code is standard-library Go and is split by responsibility:

```text
cmd/codexcopilot/main.go   CLI command routing
internal/auth               GitHub device login and token storage
internal/copilot            Copilot API URLs, headers, model discovery/filtering
internal/catalog            Codex App model catalog generation
internal/codex              Codex config writes, restore state, app launch
internal/proxy              Local HTTP proxy and request header adaptation
internal/paths              Platform-specific config paths
```

The binary has these command groups:

```bash
codexcopilot auth login
codexcopilot auth status
codexcopilot auth logout
codexcopilot models
codexcopilot provider patch
codexcopilot provider restore
codexcopilot responses-server
codexcopilot install-server-service
codexcopilot codex
codexcopilot launch codex-app
```

## Implementation Choices

The launcher is written in Go because the deployable artifact should be a single CLI binary. The job is mostly filesystem edits, HTTP forwarding, and signal handling, which fits Go's standard library well.

The project intentionally has no third-party Go dependencies. That keeps builds simple:

```bash
go build -o bin/codexcopilot ./cmd/codexcopilot
```

The tradeoff is that Codex App config editing is purpose-built rather than handled by a full TOML library. The code only edits root model/provider keys, this tool's owned provider section, and this generated Codex profile-v2 file:

```text
~/.codex/codexcopilot-codex-app.config.toml
[model_providers.codexcopilot-codex-app]
```

Current Codex no longer accepts legacy root `profile = "..."` selection in `~/.codex/config.toml`. Profile selection is done with `codex --profile codexcopilot-codex-app`, which layers `~/.codex/codexcopilot-codex-app.config.toml` over the base config. Unrelated user config is preserved by line-oriented replacement, and previous root values are recorded before the launcher takes ownership.

## Launch Flow

`codexcopilot launch codex-app` does the following:

1. Loads a saved GitHub OAuth token, or starts device login if no token exists.
2. Calls GitHub Copilot `/models`.
3. Filters the returned models to OpenAI models that support the Responses API.
4. Chooses a default model, preferring Codex-oriented GPT-5 model ids when present.
5. Writes a Codex model catalog to `~/.codex/codexcopilot-models.json`.
6. Saves previous Codex root config values for restore.
7. Updates `~/.codex/config.toml` to select this provider.
8. Starts a local HTTP proxy on `127.0.0.1:11435`.
9. Attempts to launch Codex App on macOS or Windows.

The proxy stays in the foreground. When the launcher exits, it restores the previous Codex provider settings and removes the generated profile file.

## Standalone Responses Server

To run only the local OpenAI-compatible Responses proxy without writing Codex App config or launching Codex App:

```bash
./bin/codexcopilot responses-server
```

This mode is useful when another OpenAI-compatible client already has its provider config, or when you want to manage Codex App config separately. It still uses the saved GitHub Copilot login and listens on:

```text
http://127.0.0.1:11435/v1/
```

Change the bind address with:

```bash
./bin/codexcopilot responses-server --host 0.0.0.0 --port 11435
```

## Headless Codex Wrapper

For a temporary headless Codex CLI session, let codexcopilot own the whole lifecycle:

```bash
codexcopilot auth login
codexcopilot codex
```

This command starts the proxy, writes Codex provider settings, runs:

```bash
codex --profile codexcopilot-codex-app
```

and restores the previous Codex config when Codex exits.

Pass codexcopilot options before `--`, and Codex CLI args after it:

```bash
codexcopilot codex --model gpt-5.4 -- -C /work "inspect this repo"
```

## Systemd User Service

On Linux systems with systemd, install the Responses proxy as a persistent user service:

```bash
codexcopilot auth login
codexcopilot install-server-service
```

By default the service runs:

```bash
codexcopilot responses-server --host 127.0.0.1 --port 11435
```

For a server, WSL instance, or VM that should accept connections from another machine:

```bash
codexcopilot install-server-service --host 0.0.0.0 --port 11435
```

The command requires a saved Copilot login, writes `~/.config/systemd/user/codexcopilot.service`, runs `systemctl --user daemon-reload`, and enables the service with `systemctl --user enable --now codexcopilot.service`.

This is a user service. On systems where user services should start before login, enable linger separately with your system administrator's preferred policy.

## Provider Patch

To patch local Codex provider settings without starting the server or launching Codex App:

```bash
./bin/codexcopilot provider patch
```

By default this points Codex at the default local Responses server:

```text
http://127.0.0.1:11435/v1/
```

For a remote server:

```bash
./bin/codexcopilot provider patch --base-url http://SERVER:11435/v1/
```

`provider patch` fetches models from the configured proxy's `/models` endpoint, writes the local Codex model catalog, and makes the provider active. This command does not need GitHub Copilot auth on the patching machine.

Restore previous Codex provider settings:

```bash
./bin/codexcopilot provider restore
```

## Codex Config Written

The launcher writes root defaults in `~/.codex/config.toml` without using legacy root `profile` selection:

```toml
model = "<selected model>"
model_provider = "codexcopilot-codex-app"
model_catalog_json = "/home/you/.codex/codexcopilot-models.json"

[model_providers.codexcopilot-codex-app]
name = "GitHub Copilot"
base_url = "http://127.0.0.1:11435/v1/"
wire_api = "responses"
```

It also writes a Codex profile-v2 file at `~/.codex/codexcopilot-codex-app.config.toml` so current Codex CLI can be launched with `--profile codexcopilot-codex-app`:

```toml
model = "<selected model>"
model_provider = "codexcopilot-codex-app"
model_catalog_json = "/home/you/.codex/codexcopilot-models.json"

[model_providers.codexcopilot-codex-app]
name = "GitHub Copilot"
base_url = "http://127.0.0.1:11435/v1/"
wire_api = "responses"
```

`wire_api = "responses"` is important because Codex should send Responses API requests, not Chat Completions requests, for the selected models.

## Files Touched

The tool writes:

```text
~/.codex/config.toml
~/.codex/codexcopilot-codex-app.config.toml
~/.codex/codexcopilot-models.json
<config-home>/codexcopilot/auth.json
<config-home>/codexcopilot/codex-app-restore.json
<config-home>/codexcopilot/backup/
```

`<config-home>` is:

- Linux: `$XDG_CONFIG_HOME` or `~/.config`
- macOS: `~/Library/Application Support`
- Windows: `%APPDATA%`

Auth tokens are written with user-only permissions where the platform supports it.

## Restore Behavior

Before changing Codex config, the launcher stores the previous root values for:

```text
profile
model
model_provider
model_catalog_json
```

The `profile` key is tracked only so codexcopilot can remove older legacy config it wrote before Codex switched to profile-v2 files. New patches do not write root `profile`.

Restore command:

```bash
./bin/codexcopilot provider restore
```

Restore puts those root values back, removes this tool's owned provider section and generated profile-v2 file, deletes the generated model catalog, and removes restore state.

## Proxy Behavior

The proxy accepts Codex App requests at:

```text
http://127.0.0.1:11435/v1/
```

Path mapping:

```text
/v1/models      -> /models
/v1/responses   -> /responses
/v1/...         -> /...
```

For upstream Copilot requests it adds:

```text
Authorization: Bearer <saved GitHub OAuth token>
User-Agent: codexcopilot/0.3.0
Editor-Version: codexcopilot/0.3.0
Editor-Plugin-Version: codexcopilot/0.3.0
Copilot-Integration-Id: vscode-chat
Openai-Intent: conversation-edits
X-Initiator: user|agent
```

It also adds `Copilot-Vision-Request: true` when the request body contains image input.

Incoming `Authorization` and `X-Initiator` headers are stripped so the proxy owns auth and initiator attribution.

## Initiator Inference

GitHub Copilot distinguishes user-initiated turns from agent continuations. OpenCode can do this from internal session state. This proxy can only infer from the request body Codex App sends.

Current behavior:

- Last Responses `input[]` item with `role: "user"` -> `X-Initiator: user`
- Tool, function-call-output, assistant, or other continuation item -> `X-Initiator: agent`
- Malformed or unknown payload -> `X-Initiator: user`

That is the closest approximation available at the wire-proxy layer.

## Model Selection

For `launch codex-app`, the launcher fetches all Copilot models from:

```text
GET https://api.githubcopilot.com/models
```

For `provider patch`, the launcher fetches models from the configured proxy base URL:

```text
GET http://127.0.0.1:11435/v1/models
```

Then it keeps models that:

- are enabled by policy
- are visible in the model picker
- appear to be OpenAI models
- support `/responses` or `/v1/responses`

If no model is specified, it prefers:

```text
gpt-5.4-codex
gpt-5.4
gpt-5.3-codex
gpt-5.2-codex
gpt-5.1-codex
gpt-5-codex
gpt-5.1
gpt-5
```

Then it falls back to the first usable Copilot model returned by the API.

## Reasoning Effort

Reasoning controls are driven by the generated Codex model catalog. For each model, the launcher writes:

```json
"default_reasoning_level": "medium",
"supported_reasoning_levels": ["low", "medium", "high", "xhigh"]
```

The exact values come from Copilot model metadata when available:

```text
capabilities.supports.reasoning_effort
```

If Copilot only reports generic reasoning support, the launcher falls back to `low`, `medium`, and `high`. For `gpt-5.4*` model ids, it includes `xhigh` as well so Codex App can expose the extra effort level.

## Build and Test

```bash
go test ./...
go build -o bin/codexcopilot ./cmd/codexcopilot
```

The project intentionally avoids external Go dependencies so it can compile as a single static-ish binary with the standard Go toolchain.

## Current Constraints

- Automatic Codex App launch is implemented for macOS and Windows only.
- Linux and remote-server users should use `codexcopilot codex` for temporary headless CLI sessions, or `responses-server` plus `provider patch` for persistent/manual setups.
- The proxy must remain running while Codex App uses the provider.
- This does not implement a native Codex App provider. It adapts Codex App's provider wire protocol to GitHub Copilot.
- You need an active GitHub Copilot subscription, and model availability depends on your GitHub account and organization settings.

## Remote Server Workflow

On the Linux, WSL, or remote server that owns the GitHub Copilot login, the simplest temporary headless workflow is:

```bash
./bin/codexcopilot auth login
./bin/codexcopilot codex
```

For a persistent server that another machine will use:

```bash
./bin/codexcopilot auth login
./bin/codexcopilot responses-server --host 0.0.0.0 --port 11435
```

For a persistent user service on the server:

```bash
codexcopilot auth login
codexcopilot install-server-service --host 0.0.0.0 --port 11435
```

On the machine running Codex App:

```bash
./bin/codexcopilot provider patch --base-url http://SERVER:11435/v1/
```
