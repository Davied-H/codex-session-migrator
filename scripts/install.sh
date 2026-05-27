#!/usr/bin/env bash
set -euo pipefail

APP_NAME="codex-session-migrator"
SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd -- "${SCRIPT_DIR}/.." && pwd)"
PREFIX="${PREFIX:-"${HOME}/.local"}"
BINDIR="${BINDIR:-"${PREFIX}/bin"}"
TARGET="${BINDIR}/${APP_NAME}"

if ! command -v go >/dev/null 2>&1; then
  echo "error: go is required but was not found in PATH" >&2
  exit 1
fi

mkdir -p "${BINDIR}"

cd "${PROJECT_ROOT}"
echo "building ${APP_NAME}..."
go build -trimpath -o "${TARGET}" ./cmd/codex-session-migrator
chmod 0755 "${TARGET}"

echo "installed: ${TARGET}"

case ":${PATH}:" in
  *":${BINDIR}:"*) ;;
  *)
    echo
    echo "note: ${BINDIR} is not in PATH."
    echo "add this to your shell profile if needed:"
    echo "  export PATH=\"${BINDIR}:\$PATH\""
    ;;
esac

echo
echo "run:"
echo "  ${APP_NAME}"
