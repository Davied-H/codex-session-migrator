#!/usr/bin/env sh
set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$ROOT_DIR"

failures=0

section() {
  printf '\n==> %s\n' "$1"
}

fail() {
  failures=$((failures + 1))
  printf 'ERROR: %s\n' "$*" >&2
}

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    fail "$1 is required"
    return 1
  fi
  return 0
}

run_go_tests() {
  section "Go tests"
  go test ./...
}

scan_sensitive_patterns() {
  section "Sensitive information scan"

  tmp_file=$(mktemp)
  trap 'rm -f "$tmp_file"' EXIT INT TERM

  secret_pattern='BEGIN (RSA |DSA |EC |OPENSSH |)PRIVATE KEY|github_pat_[A-Za-z0-9_]+|gh[opsu]_[A-Za-z0-9_]{20,}|AKIA[0-9A-Z]{16}|xox[baprs]-[A-Za-z0-9-]{20,}|sk-[A-Za-z0-9]{20,}'
  if git grep -nIE "$secret_pattern" -- . >"$tmp_file" 2>/dev/null; then
    fail "possible private key or token found"
    cat "$tmp_file" >&2
  fi

  printf 'sensitive scan complete\n'
}

check_release_artifact_names() {
  section "Release artifact naming self-check"

  workflow=".github/workflows/release.yml"
  installer="scripts/install-release.sh"

  for file in "$workflow" "$installer"; do
    if ! [ -f "$file" ]; then
      fail "$file is missing"
    fi
  done

  while read -r goos goarch ext; do
    [ -n "$goos" ] || continue

    if ! grep -Fq "goos: $goos" "$workflow"; then
      fail "release workflow is missing GOOS $goos"
    fi
    if ! grep -Fq "goarch: $goarch" "$workflow"; then
      fail "release workflow is missing GOARCH $goarch"
    fi

    doc_name="codex-session-migrator_<version>_${goos}_${goarch}.${ext}"
    if ! grep -Fq "$doc_name" README.md; then
      fail "README is missing release asset $doc_name"
    fi
  done <<'EOF'
darwin amd64 tar.gz
darwin arm64 tar.gz
linux amd64 tar.gz
linux arm64 tar.gz
windows amd64 zip
EOF

  if ! grep -Fq 'build/codex-session-migrator_${VERSION}_${GOOS}_${GOARCH}' "$workflow"; then
    fail "release workflow no longer builds versioned package directories"
  fi
  if ! grep -Fq 'codex-session-migrator_${VERSION}_${GOOS}_${GOARCH}.tar.gz' "$workflow"; then
    fail "release workflow tar.gz archive pattern changed"
  fi
  if ! grep -Fq 'codex-session-migrator_${VERSION}_${GOOS}_${GOARCH}.zip' "$workflow"; then
    fail "release workflow zip archive pattern changed"
  fi
  if ! grep -Fq 'codex-session-migrator_${VERSION}_${goos}_${goarch}.tar.gz' "$installer"; then
    fail "install-release.sh does not resolve versioned tar.gz asset names"
  fi
  if ! grep -Fq 'checksums.txt' "$workflow" || ! grep -Fq 'checksums.txt' "$installer"; then
    fail "release checksum generation or installer verification is missing"
  fi

  printf 'release naming rules are aligned\n'
}

if require_cmd go && require_cmd git && require_cmd grep; then
  run_go_tests
  scan_sensitive_patterns
  check_release_artifact_names
fi

if [ "$failures" -ne 0 ]; then
  printf '\nPreflight failed with %s issue(s).\n' "$failures" >&2
  exit 1
fi

printf '\nPreflight passed.\n'
