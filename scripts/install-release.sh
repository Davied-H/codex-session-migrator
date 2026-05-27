#!/usr/bin/env sh
set -eu

REPO="${CSM_REPO:-Davied-H/codex-session-migrator}"
VERSION="${CSM_VERSION:-latest}"
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"
BIN_NAME="codex-session-migrator"

usage() {
  cat <<EOF
Install codex-session-migrator from GitHub Releases.

Usage:
  sh scripts/install-release.sh
  INSTALL_DIR=/usr/local/bin sh scripts/install-release.sh
  CSM_VERSION=v0.0.1 sh scripts/install-release.sh

Environment:
  INSTALL_DIR  Directory to install codex-session-migrator into. Defaults to $HOME/.local/bin.
  CSM_VERSION  Release tag to install. Defaults to latest.
  CSM_REPO     GitHub repository. Defaults to Davied-H/codex-session-migrator.
EOF
}

case "${1:-}" in
  -h|--help)
    usage
    exit 0
    ;;
  "")
    ;;
  *)
    echo "Unknown argument: $1" >&2
    usage >&2
    exit 2
    ;;
esac

need() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "$1 is required but was not found on PATH." >&2
    exit 1
  fi
}

os=$(uname -s | tr '[:upper:]' '[:lower:]')
arch=$(uname -m)

case "$os" in
  darwin) goos="darwin" ;;
  linux) goos="linux" ;;
  *)
    echo "Unsupported OS: $os" >&2
    exit 1
    ;;
esac

case "$arch" in
  x86_64|amd64) goarch="amd64" ;;
  arm64|aarch64) goarch="arm64" ;;
  *)
    echo "Unsupported architecture: $arch" >&2
    exit 1
    ;;
esac

need tar
auth_token="${GH_TOKEN:-${GITHUB_TOKEN:-}}"
if command -v curl >/dev/null 2>&1; then
  if [ -n "$auth_token" ]; then
    fetch() { curl -fsSL -H "Authorization: Bearer $auth_token" "$1" -o "$2"; }
  else
    fetch() { curl -fsSL "$1" -o "$2"; }
  fi
elif command -v wget >/dev/null 2>&1; then
  if [ -n "$auth_token" ]; then
    fetch() { wget --header="Authorization: Bearer $auth_token" -qO "$2" "$1"; }
  else
    fetch() { wget -qO "$2" "$1"; }
  fi
else
  echo "curl or wget is required." >&2
  exit 1
fi

tmp_dir=$(mktemp -d)
trap 'rm -rf "$tmp_dir"' EXIT INT TERM

if [ "$VERSION" = "latest" ]; then
  api_url="https://api.github.com/repos/$REPO/releases/latest"
  need sed
  latest_json="$tmp_dir/latest.json"
  fetch "$api_url" "$latest_json"
  tag=$(sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$latest_json" | head -n 1)
  if [ -z "$tag" ]; then
    echo "Could not resolve latest release for $REPO." >&2
    exit 1
  fi
  VERSION="$tag"
fi

archive="codex-session-migrator_${VERSION}_${goos}_${goarch}.tar.gz"
base_url="https://github.com/$REPO/releases/download/$VERSION"
archive_url="$base_url/$archive"
checksum_url="$base_url/checksums.txt"

echo "Downloading $archive..."
fetch "$archive_url" "$tmp_dir/$archive"

if fetch "$checksum_url" "$tmp_dir/checksums.txt"; then
  if command -v sha256sum >/dev/null 2>&1; then
    (cd "$tmp_dir" && grep "  $archive\$" checksums.txt | sha256sum -c -)
  elif command -v shasum >/dev/null 2>&1; then
    (cd "$tmp_dir" && grep "  $archive\$" checksums.txt | shasum -a 256 -c -)
  else
    echo "Warning: sha256sum/shasum not found; skipping checksum verification." >&2
  fi
fi

mkdir -p "$INSTALL_DIR"
tar -C "$tmp_dir" -xzf "$tmp_dir/$archive"
package_dir="$tmp_dir/codex-session-migrator_${VERSION}_${goos}_${goarch}"
install -m 0755 "$package_dir/$BIN_NAME" "$INSTALL_DIR/$BIN_NAME"

echo "Installed codex-session-migrator $VERSION to $INSTALL_DIR/$BIN_NAME"
case ":$PATH:" in
  *":$INSTALL_DIR:"*) echo "Run it with: codex-session-migrator" ;;
  *)
    echo "Add this directory to PATH:"
    echo "  export PATH=\"$INSTALL_DIR:\$PATH\""
    ;;
esac
