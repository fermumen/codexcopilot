# ghc-launch-codex

`ghc-launch-codex` is a small Go launcher that configures Codex App to use GitHub Copilot models through a local OpenAI-compatible proxy.

The command is intentionally shaped like Ollama's Codex App integration:

```bash
githubcopilot launch codex-app
```

It writes a temporary Codex App provider profile, starts a local proxy, and forwards Codex App's `/v1/responses` traffic to GitHub Copilot using your own Copilot login.

This is a local compatibility tool, not a GitHub, OpenAI, or Ollama product.

## Quick Start

Build the binary:

```bash
go build -o bin/githubcopilot ./cmd/githubcopilot
```

Log in with GitHub's device flow:

```bash
./bin/githubcopilot auth login
```

For GitHub Enterprise:

```bash
./bin/githubcopilot auth login --enterprise-url https://github.example.com
```

Launch Codex App against GitHub Copilot:

```bash
./bin/githubcopilot launch codex-app
```

Equivalent target spelling:

```bash
./bin/githubcopilot launch codex app
```

Useful options:

```bash
./bin/githubcopilot launch codex-app --model gpt-5.1-codex
./bin/githubcopilot launch codex-app --port 11435
./bin/githubcopilot launch codex-app --config-only
./bin/githubcopilot launch codex-app --server-only
./bin/githubcopilot launch codex-app --restore --no-launch
```

The repo also includes a convenience wrapper:

```bash
./githubcopilot launch codex-app
```

The wrapper runs `bin/githubcopilot` when present, otherwise it falls back to `go run ./cmd/githubcopilot` if Go is installed.

## Why This Exists

Codex App can use custom model providers from `~/.codex/config.toml`, but GitHub Copilot is not a plain OpenAI-compatible endpoint. Copilot requests need GitHub OAuth auth plus Copilot-specific headers such as `Openai-Intent` and `X-Initiator`.

OpenCode solves this in-process with a first-party GitHub Copilot provider. Codex App does not currently expose that provider hook to this tool, so the practical design is a local proxy:

```text
Codex App
  -> http://127.0.0.1:11435/v1/responses
  -> ghc-launch-codex proxy
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
cmd/githubcopilot/main.go   CLI command routing
internal/auth               GitHub device login and token storage
internal/copilot            Copilot API URLs, headers, model discovery/filtering
internal/catalog            Codex App model catalog generation
internal/codex              Codex config writes, restore state, app launch
internal/proxy              Local HTTP proxy and request header adaptation
internal/paths              Platform-specific config paths
```

The binary has these command groups:

```bash
githubcopilot auth login
githubcopilot auth status
githubcopilot auth logout
githubcopilot models
githubcopilot serve
githubcopilot responses-server
githubcopilot launch codex-app
```

## Implementation Choices

The launcher is written in Go because the deployable artifact should be a single CLI binary. The job is mostly filesystem edits, HTTP forwarding, and signal handling, which fits Go's standard library well.

The project intentionally has no third-party Go dependencies. That keeps builds simple:

```bash
go build -o bin/githubcopilot ./cmd/githubcopilot
```

The tradeoff is that Codex App config editing is purpose-built rather than handled by a full TOML library. The code only edits root keys and the two owned sections:

```text
[profiles.githubcopilot-launch-codex-app]
[model_providers.githubcopilot-launch-codex-app]
```

Unrelated user config is preserved by line-oriented replacement, and previous root values are recorded before the launcher takes ownership.

## Launch Flow

`githubcopilot launch codex-app` does the following:

1. Loads a saved GitHub OAuth token, or starts device login if no token exists.
2. Calls GitHub Copilot `/models`.
3. Filters the returned models to OpenAI models that support the Responses API.
4. Chooses a default model, preferring Codex-oriented GPT-5 model ids when present.
5. Writes a Codex model catalog to `~/.codex/githubcopilot-launch-models.json`.
6. Saves previous Codex root config values for restore.
7. Updates `~/.codex/config.toml` to select this provider.
8. Starts a local HTTP proxy on `127.0.0.1:11435`.
9. Attempts to launch Codex App on macOS or Windows.

The proxy stays in the foreground. Leave it running while Codex App is using this provider.

## Standalone Responses Server

To run only the local OpenAI-compatible Responses proxy without writing Codex App config or launching Codex App:

```bash
./bin/githubcopilot responses-server
```

Equivalent:

```bash
./bin/githubcopilot serve
./bin/githubcopilot launch codex-app --server-only
```

This mode is useful when another OpenAI-compatible client already has its provider config, or when you want to manage Codex App config separately. It still uses the saved GitHub Copilot login and listens on:

```text
http://127.0.0.1:11435/v1/
```

Change the bind address with:

```bash
./bin/githubcopilot responses-server --host 0.0.0.0 --port 11435
```

## Codex Config Written

The launcher writes root config values:

```toml
profile = "githubcopilot-launch-codex-app"
model = "<selected model>"
model_provider = "githubcopilot-launch-codex-app"
model_catalog_json = "/home/you/.codex/githubcopilot-launch-models.json"
```

It also writes owned profile and provider sections:

```toml
[profiles.githubcopilot-launch-codex-app]
openai_base_url = "http://127.0.0.1:11435/v1/"
model = "<selected model>"
model_provider = "githubcopilot-launch-codex-app"
model_catalog_json = "/home/you/.codex/githubcopilot-launch-models.json"

[model_providers.githubcopilot-launch-codex-app]
name = "GitHub Copilot"
base_url = "http://127.0.0.1:11435/v1/"
wire_api = "responses"
```

`wire_api = "responses"` is important because Codex App should send Responses API requests, not Chat Completions requests, for the selected models.

## Files Touched

The tool writes:

```text
~/.codex/config.toml
~/.codex/githubcopilot-launch-models.json
<config-home>/ghc-launch-codex/auth.json
<config-home>/ghc-launch-codex/codex-app-restore.json
<config-home>/ghc-launch-codex/backup/
```

`<config-home>` is:

- Linux: `$XDG_CONFIG_HOME` or `~/.config`
- macOS: `~/Library/Application Support`
- Windows: `%APPDATA%`

Auth tokens are written with user-only permissions where the platform supports it.

## Restore Behavior

Before changing Codex App config, the launcher stores the previous root values for:

```text
profile
model
model_provider
model_catalog_json
```

Restore command:

```bash
./bin/githubcopilot launch codex-app --restore --no-launch
```

Restore puts those root values back, removes this tool's owned profile/provider sections, deletes the generated model catalog, and removes restore state.

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
User-Agent: ghc-launch-codex/0.1.0
Editor-Version: ghc-launch-codex/0.1.0
Editor-Plugin-Version: ghc-launch-codex/0.1.0
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

The launcher fetches all Copilot models from:

```text
GET https://api.githubcopilot.com/models
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
go build -o bin/githubcopilot ./cmd/githubcopilot
```

The project intentionally avoids external Go dependencies so it can compile as a single static-ish binary with the standard Go toolchain.

## Current Constraints

- Automatic Codex App launch is implemented for macOS and Windows only.
- Linux users can still use `--config-only` or `--no-launch` and open Codex App manually if available.
- The proxy must remain running while Codex App uses the provider.
- This does not implement a native Codex App provider. It adapts Codex App's provider wire protocol to GitHub Copilot.
- You need an active GitHub Copilot subscription, and model availability depends on your GitHub account and organization settings.
