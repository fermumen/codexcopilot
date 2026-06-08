---
name: codexcopilot-upgrade
description: Review and upgrade codexcopilot against fast-moving OpenCode GitHub Copilot auth/provider behavior and Codex CLI/App config, catalog, tools, and Responses API changes. Use when asked to check upstream drift, sync assumptions, bump releases, or plan/perform codexcopilot upgrades.
allowed-tools: Read, Grep, Glob, Bash, apply_patch
---

# Codexcopilot Upgrade Workflow

`codexcopilot` depends on behavior that changes quickly in two upstream projects:

- OpenCode's GitHub Copilot provider and auth flow.
- Codex CLI/App provider config, model catalog, feature flags, and Responses request shape.

Use this workflow to check those upstreams, record the exact reviewed commits, and translate changes into small `codexcopilot` updates.

## Current Baseline

Record these values when starting an upgrade pass. Update this section only after completing the review and merging any required changes.

| Source | Baseline |
| --- | --- |
| `codexcopilot` | `697d9d129d907261b6baee9e28d5eff9289f1024` |
| Codex CLI installed locally | `codex-cli 0.137.0` |
| OpenCode upstream HEAD checked | `b34d9242d1786ccfc5208f94bcbab5c2a1efb29d` |
| Codex upstream HEAD checked | `26d932983398147e4443bd655ce24a6ce6833a1c` |

## Upgrade Goals

- Keep GitHub Copilot auth compatible with GitHub and Enterprise accounts.
- Keep proxy request adaptation compatible with Copilot's current Responses API expectations.
- Keep Codex config generation compatible with current Codex CLI/App provider and model catalog formats.
- Avoid copying large upstream implementations; port only the smallest behavior changes we need.
- Preserve user config and restore-state behavior.

## Temporary Clone Layout

Use `/tmp/opencode` for all external clones so the repo stays clean.

```bash
mkdir -p /tmp/opencode/codexcopilot-upgrade
git clone --depth 1 https://github.com/sst/opencode.git /tmp/opencode/codexcopilot-upgrade/opencode
git clone --depth 1 https://github.com/openai/codex.git /tmp/opencode/codexcopilot-upgrade/codex
```

Record reviewed SHAs:

```bash
git -C /tmp/opencode/codexcopilot-upgrade/opencode rev-parse HEAD
git -C /tmp/opencode/codexcopilot-upgrade/codex rev-parse HEAD
codex --version
git rev-parse HEAD
```

## OpenCode Review Checklist

Focus on the GitHub Copilot provider, not unrelated OpenCode agent behavior.

Search targets:

```bash
grep -R "github copilot\|copilot\|device/code\|Openai-Intent\|X-Initiator\|Copilot-Integration-Id" /tmp/opencode/codexcopilot-upgrade/opencode
```

Compare against local code:

- `internal/auth/auth.go`
- `internal/copilot/copilot.go`
- `internal/proxy/proxy.go`

Questions to answer:

- Did the OAuth client ID or device auth scope change?
- Did OpenCode add a real token refresh flow or change token storage metadata?
- Did Copilot API base URLs change for public GitHub or Enterprise?
- Did required Copilot headers change?
- Did `Openai-Intent`, `X-Initiator`, or `Copilot-Integration-Id` behavior change?
- Did OpenCode start stripping or transforming new unsupported tools?
- Did image, vision, or attachment handling change?
- Did model filtering or picker visibility logic change?

Expected local outputs:

- Small patches in `internal/auth`, `internal/copilot`, or `internal/proxy`.
- Tests for any new header, auth, model, or request-body behavior.
- Documentation update if the auth app/client ID story changes.

## Codex Review Checklist

Focus on config format, model catalog schema, feature flags, and Responses request shape.

Search targets:

```bash
grep -R "model_catalog_json\|model_provider\|wire_api\|profile\|image_generation\|image_tool\|codex_apps\|features" /tmp/opencode/codexcopilot-upgrade/codex
```

Compare against local code:

- `internal/codex/codex.go`
- `internal/catalog/catalog.go`
- `internal/proxy/proxy.go`

Questions to answer:

- Did Codex profile-v2 file naming or layering change?
- Did `wire_api = "responses"` or provider TOML shape change?
- Did `model_catalog_json` schema add required fields?
- Did feature flags change names or defaults, especially native tools such as `image_generation`?
- Did Codex App/CLI start sending new top-level `tools` values that Copilot rejects?
- Did `/v1/responses` request payloads change in a way that affects initiator inference?
- Did `/mcp` or built-in `codex_apps` behavior become part of model calls by default?
- Did Codex model IDs, reasoning effort names, summaries, verbosity, or context-window fields change?

Expected local outputs:

- Keep TOML edits line-oriented unless a parser becomes necessary.
- Preserve unrelated user config and restore-state behavior.
- Add tests before changing config restoration or model catalog output.
- Add proxy strip/adaptation tests for every newly unsupported upstream tool.

## Model Selection Review

Current policy:

- Filter to policy-enabled, picker-visible OpenAI models that support Responses API.
- Default to the highest non-mini `gpt-*` version available.
- Prefer `-codex` only when generic and Codex variants have the same version.
- Honor explicit `--model` exactly if Copilot returns it.

On each upgrade pass:

```bash
codexcopilot models
codexcopilot responses-server --port 11436
```

Confirm startup logs choose the intended default model, then stop the temporary server.

## Smoke Tests

Run unit tests:

```bash
go test ./...
```

If Go is not installed locally, use Docker:

```bash
docker run --rm --mount type=bind,source="$PWD",target=/work -w /work golang:1.22 /usr/local/go/bin/go test ./...
```

Run proxy compatibility smoke against a temporary port:

```bash
codexcopilot responses-server --port 11436
curl -fsS -X POST http://127.0.0.1:11436/v1/responses \
  -H 'content-type: application/json' \
  --data '{"model":"gpt-5.5","input":"Reply with exactly: smoke-ok","tools":[{"type":"image_tool"}]}'
```

Expected result:

- Request succeeds.
- Response text includes `smoke-ok`.
- Response `tools` does not include unsupported image generation tools.

Run Codex CLI smoke against the managed provider:

```bash
codex --dangerously-bypass-approvals-and-sandbox -C /tmp exec "Reply with exactly: cli-smoke-ok"
```

Check session metadata when needed:

```bash
grep -R "\"model_provider\":\"codexcopilot-codex-app\"" ~/.codex/sessions
```

## Release Procedure

After changes are tested:

```bash
git status --short
git diff
git log --oneline -10
git add <intended files>
git commit -m "Concise change summary"
git push origin master
git tag vX.Y.Z
git push origin vX.Y.Z
```

Wait for the release asset:

```bash
curl -fsI https://github.com/fermumen/codexcopilot/releases/download/vX.Y.Z/codexcopilot_linux_amd64.tar.gz
```

Install latest release:

```bash
curl -fsSL https://raw.githubusercontent.com/fermumen/codexcopilot/master/install.sh | sh
```

Restart service:

```bash
systemctl --user restart codexcopilot.service
systemctl --user status codexcopilot.service --no-pager
journalctl --user -u codexcopilot.service -n 30 --no-pager
```

## Service Ownership

Preferred long-running setup on Linux is user systemd:

```bash
codexcopilot install-server-service
```

Avoid running a simultaneous tmux `codexcopilot responses-server` on the same port. The temporary wrapper restores provider config on exit and can conflict with the long-lived server's provider patch.

## Known Watch Items

- Dedicated OAuth app: current default client ID displays the upstream app owner in GitHub auth. A `codexcopilot` GitHub OAuth app would improve trust and admin/audit clarity.
- Native Codex tools: Codex may add tools that Copilot rejects. Keep proxy stripping narrow and test each rejected `tools[].type` explicitly.
- Built-in Codex Apps MCP: `features.apps = false` suppresses `codex_apps` locally, but remote clients may still send app/tool state.
- Model catalog schema: Codex may require new catalog fields without warning.
- Reasoning levels: keep fallback behavior conservative and prefer Copilot metadata when present.
