# AGENTS.md

## Repo Shape
- Go 1.22 module `github.com/fermumen/codexcopilot`; intentionally stdlib-only with no `go.sum`, so do not add dependencies casually.
- CLI entrypoint is `cmd/codexcopilot/main.go`; `./codexcopilot` is a wrapper that runs `bin/codexcopilot` if present, otherwise `go run ./cmd/codexcopilot`.
- Package boundaries: `internal/auth` GitHub device OAuth and token storage, `internal/copilot` API/model/header logic, `internal/catalog` Codex model catalog JSON, `internal/codex` Codex config/restore/app launch, `internal/proxy` local Copilot API proxy, `internal/paths` platform config paths.

## Commands
- Full verification: `go test ./...`
- Focused test example: `go test ./internal/codex -run TestConfigureWritesCodexCopilotProviderNames`
- Build binary: `go build -o bin/codexcopilot ./cmd/codexcopilot`
- Release workflow tests first, then cross-builds tagged `v*` releases with `CGO_ENABLED=0` for linux/darwin/windows assets.

## Runtime Gotchas
- Commands that configure Codex write real user files: `~/.codex/config.toml`, `~/.codex/codexcopilot-codex-app.config.toml`, `~/.codex/codexcopilot-models.json`, plus state under `<config-home>/codexcopilot/`. The Codex dir honors `CODEX_HOME` (used as the directory itself, like Codex CLI), falling back to `~/.codex`. In tests or manual experiments, set temp `HOME`, `CODEX_HOME`, and `XDG_CONFIG_HOME`.
- Current Codex uses profile-v2 files plus `codex --profile codexcopilot-codex-app`; do not reintroduce legacy root `profile = "..."` settings.
- Provider config must keep `wire_api = "responses"` and a base URL normalized to end in `/v1/`; the default local proxy is `http://127.0.0.1:11435/v1/`.
- The proxy maps `/v1/models` to Copilot `/models` and `/v1/responses` to `/responses`, but preserves `/v1/messages` and `/v1/messages/count_tokens` for Copilot's native Anthropic Messages shim. It owns Copilot auth/initiator headers and strips incoming `Authorization`, `X-Api-Key`, and `X-Initiator`.
- `launch codex-app` auto-launches only on macOS/Windows; Linux/remote workflows should use `codexcopilot codex`, `responses-server`, or `install-server-service`.
- OAuth uses the public client id in `internal/auth` unless `--client-id` or `GHC_COPILOT_CLIENT_ID` is provided; Enterprise auth uses `--enterprise-url` and Copilot API base `https://copilot-api.<domain>`.

## Implementation Constraints
- Model selection intentionally keeps only policy-enabled, picker-visible OpenAI models with Responses API support, then prefers the `internal/copilot.DefaultModelHints` order.
- Reasoning levels come from Copilot metadata when present; fallback adds `xhigh` only for `gpt-5.4*` ids.
- `internal/codex` edits TOML with line-oriented helpers, not a TOML parser; preserve unrelated user config and restore-state behavior when changing it.
- Vanilla mode (opt-in with `--vanilla`) disables Codex extras: `[features]` `apps`/`plugins`/`workspace_dependencies`, root `web_search = "disabled"`, and `[skills.bundled] enabled = false`; `image_generation = false` is always written. `multi_agent` and `goals` stay enabled. All managed keys are captured in restore state.
