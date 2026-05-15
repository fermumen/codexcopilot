from __future__ import annotations

import argparse
import json
import signal
import sys
import threading
import time

from .auth import load_auth, login, logout
from .codex_app import configure_codex_app, launch_codex_app, restore_codex_app
from .constants import DEFAULT_COPILOT_CLIENT_ID, DEFAULT_HOST, DEFAULT_PORT
from .copilot import choose_model, codex_app_models, fetch_models
from .paths import default_paths
from .proxy import make_server


def _target(parts: list[str]) -> str:
    return "-".join(part.lower() for part in parts)


def _ensure_auth(args: argparse.Namespace):
    auth = load_auth()
    if auth:
        if args.enterprise_url and auth.enterprise_url != args.enterprise_url:
            return login(client_id=args.client_id, enterprise_url=args.enterprise_url)
        return auth
    return login(client_id=args.client_id, enterprise_url=args.enterprise_url)


def cmd_auth(args: argparse.Namespace) -> int:
    if args.auth_command == "login":
        login(client_id=args.client_id, enterprise_url=args.enterprise_url)
        print("GitHub Copilot login saved.")
        return 0
    if args.auth_command == "logout":
        removed = logout()
        print("Removed saved GitHub Copilot login." if removed else "No saved login found.")
        return 0
    auth = load_auth()
    if not auth:
        print("No saved GitHub Copilot login.")
        return 1
    scope = auth.enterprise_url or "github.com"
    print(f"Saved GitHub Copilot login found for {scope}.")
    return 0


def cmd_models(args: argparse.Namespace) -> int:
    auth = _ensure_auth(args)
    models = fetch_models(auth)
    if args.json:
        print(json.dumps(models, indent=2, sort_keys=True))
        return 0
    for model in models:
        name = model.get("name") or model["id"]
        picker = "" if model.get("model_picker_enabled", True) else " (hidden)"
        print(f"{model['id']}\t{name}{picker}")
    return 0


def cmd_serve(args: argparse.Namespace) -> int:
    auth = _ensure_auth(args)
    server = make_server(args.host, args.port, auth)
    print(f"GitHub Copilot proxy listening on http://{args.host}:{args.port}/v1/")
    try:
        server.serve_forever()
    except KeyboardInterrupt:
        pass
    finally:
        server.server_close()
    return 0


def _serve_in_background(host: str, port: int, auth):
    server = make_server(host, port, auth)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    return server, thread


def cmd_launch(args: argparse.Namespace) -> int:
    target = _target(args.target)
    if target != "codex-app":
        raise SystemExit(f"Unknown launch target {target!r}. Supported target: codex-app")
    paths = default_paths()
    if args.restore:
        restored = restore_codex_app(paths)
        print("Restored Codex App config." if restored else "No Codex App config restore state found.")
        if not args.no_launch and not args.config_only:
            try:
                launch_codex_app()
                print("Requested Codex App launch.")
            except RuntimeError as exc:
                print(str(exc), file=sys.stderr)
                return 1
        if args.no_launch:
            return 0
        return 0

    auth = _ensure_auth(args)
    remote_models = fetch_models(auth)
    models = codex_app_models(remote_models)
    if not models:
        raise SystemExit("GitHub Copilot returned no OpenAI Responses API models usable by Codex App.")
    selected = choose_model(models, args.model)
    base_url = f"http://{args.host}:{args.port}"
    configure_codex_app(model=selected, models=models, base_url=base_url, paths=paths)
    print(f"Configured Codex App profile {selected!r} at {paths.codex_config}.")

    if args.config_only:
        return 0

    server, thread = _serve_in_background(args.host, args.port, auth)
    print(f"GitHub Copilot proxy listening on {base_url}/v1/")

    if not args.no_launch:
        try:
            launch_codex_app()
            print("Requested Codex App launch.")
        except RuntimeError as exc:
            print(str(exc), file=sys.stderr)

    stop = threading.Event()

    def handle_signal(_signum, _frame):
        stop.set()

    old_int = signal.signal(signal.SIGINT, handle_signal)
    old_term = signal.signal(signal.SIGTERM, handle_signal)
    try:
        while not stop.is_set() and thread.is_alive():
            time.sleep(0.25)
    finally:
        signal.signal(signal.SIGINT, old_int)
        signal.signal(signal.SIGTERM, old_term)
        server.shutdown()
        server.server_close()
    return 0


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(prog="githubcopilot")
    parser.set_defaults(func=None)

    sub = parser.add_subparsers(dest="command")

    auth = sub.add_parser("auth")
    auth_sub = auth.add_subparsers(dest="auth_command", required=True)
    auth_login = auth_sub.add_parser("login")
    auth_login.add_argument("--client-id", default=DEFAULT_COPILOT_CLIENT_ID)
    auth_login.add_argument("--enterprise-url")
    auth_login.set_defaults(func=cmd_auth)
    auth_status = auth_sub.add_parser("status")
    auth_status.set_defaults(func=cmd_auth)
    auth_logout = auth_sub.add_parser("logout")
    auth_logout.set_defaults(func=cmd_auth)

    models = sub.add_parser("models")
    models.add_argument("--json", action="store_true")
    models.add_argument("--client-id", default=DEFAULT_COPILOT_CLIENT_ID)
    models.add_argument("--enterprise-url")
    models.set_defaults(func=cmd_models)

    serve = sub.add_parser("serve")
    serve.add_argument("--host", default=DEFAULT_HOST)
    serve.add_argument("--port", type=int, default=DEFAULT_PORT)
    serve.add_argument("--client-id", default=DEFAULT_COPILOT_CLIENT_ID)
    serve.add_argument("--enterprise-url")
    serve.set_defaults(func=cmd_serve)

    launch = sub.add_parser("launch")
    launch.add_argument("target", nargs="+")
    launch.add_argument("--model")
    launch.add_argument("--host", default=DEFAULT_HOST)
    launch.add_argument("--port", type=int, default=DEFAULT_PORT)
    launch.add_argument("--config-only", action="store_true")
    launch.add_argument("--no-launch", action="store_true")
    launch.add_argument("--restore", action="store_true")
    launch.add_argument("--client-id", default=DEFAULT_COPILOT_CLIENT_ID)
    launch.add_argument("--enterprise-url")
    launch.set_defaults(func=cmd_launch)

    return parser


def main(argv: list[str] | None = None) -> int:
    parser = build_parser()
    args = parser.parse_args(argv)
    if args.func is None:
        parser.print_help()
        return 2
    return int(args.func(args))


if __name__ == "__main__":
    raise SystemExit(main())
