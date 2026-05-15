# ghc-launch-codex

`githubcopilot launch codex-app` configures Codex App to use a local OpenAI-compatible proxy backed by your GitHub Copilot account.

It mirrors the shape of Ollama's `ollama launch codex-app` integration:

- writes a Codex App profile into `~/.codex/config.toml`
- writes a model catalog into `~/.codex/githubcopilot-launch-models.json`
- keeps restore state under this tool's config directory
- exposes a local `/v1/responses` endpoint for Codex App
- uses GitHub Copilot's device login flow and model API

## Build

```bash
go build -o bin/githubcopilot ./cmd/githubcopilot
```

The repo includes a convenience wrapper:

```bash
./githubcopilot --help
```

The wrapper runs `bin/githubcopilot` when present, otherwise it falls back to `go run ./cmd/githubcopilot` if Go is installed.

## Log in

```bash
./githubcopilot auth login
```

The login uses the GitHub device flow. The OAuth client id defaults to the one used by OpenCode's official GitHub Copilot provider and can be overridden with `GHC_COPILOT_CLIENT_ID`.

For GitHub Enterprise, pass:

```bash
./githubcopilot auth login --enterprise-url https://github.example.com
```

## Launch Codex App

```bash
./githubcopilot launch codex-app
```

Equivalent:

```bash
./githubcopilot launch codex app
```

Useful options:

```bash
./githubcopilot launch codex-app --model gpt-5.1-codex
./githubcopilot launch codex-app --port 11435
./githubcopilot launch codex-app --config-only
./githubcopilot launch codex-app --restore
```

The normal launch command starts the proxy in the foreground. Leave that process running while Codex App is open.

## Restore Codex config

```bash
./githubcopilot launch codex-app --restore --no-launch
```

## Notes

This is a local compatibility tool, not a GitHub or OpenAI product. You are responsible for using it within your GitHub Copilot plan and terms.
