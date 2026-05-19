#!/bin/sh
set -eu

repo="${CODEXCOPILOT_REPO:-fermumen/codexcopilot}"
version="${CODEXCOPILOT_VERSION:-latest}"
install_dir="${CODEXCOPILOT_INSTALL_DIR:-$HOME/.local/bin}"

need_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "error: required command not found: $1" >&2
    exit 1
  fi
}

detect_os() {
  case "$(uname -s)" in
    Linux) echo "linux" ;;
    Darwin) echo "darwin" ;;
    *)
      echo "error: unsupported OS: $(uname -s)" >&2
      exit 1
      ;;
  esac
}

detect_arch() {
  case "$(uname -m)" in
    x86_64 | amd64) echo "amd64" ;;
    arm64 | aarch64) echo "arm64" ;;
    *)
      echo "error: unsupported architecture: $(uname -m)" >&2
      exit 1
      ;;
  esac
}

download() {
  url="$1"
  out="$2"
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$url" -o "$out"
  elif command -v wget >/dev/null 2>&1; then
    wget -q "$url" -O "$out"
  else
    echo "error: curl or wget is required" >&2
    exit 1
  fi
}

need_cmd uname
need_cmd mktemp
need_cmd tar
need_cmd install

os="$(detect_os)"
arch="$(detect_arch)"
asset="codexcopilot_${os}_${arch}.tar.gz"

if [ "$version" = "latest" ]; then
  url="https://github.com/$repo/releases/latest/download/$asset"
else
  url="https://github.com/$repo/releases/download/$version/$asset"
fi

tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT INT TERM

archive="$tmpdir/$asset"
download "$url" "$archive"
tar -xzf "$archive" -C "$tmpdir"

mkdir -p "$install_dir"
install -m 0755 "$tmpdir/codexcopilot" "$install_dir/codexcopilot"

echo "codexcopilot installed to $install_dir/codexcopilot"

case ":$PATH:" in
  *":$install_dir:"*) ;;
  *)
    echo "warning: $install_dir is not on PATH" >&2
    echo "add it to your shell profile, or move codexcopilot to a directory on PATH" >&2
    ;;
esac

