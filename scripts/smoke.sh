#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BUILD_DIR="$(mktemp -d)"
CLI_BIN="$BUILD_DIR/gemini-cli"

cleanup() {
  rm -rf "$BUILD_DIR"
}
trap cleanup EXIT

say() {
  printf '\n==> %s\n' "$*"
}

cd "$ROOT_DIR"

say "Preflight"
command -v go >/dev/null

go version
git status --short || true

say "Build"
go build -o "$CLI_BIN" ./cmd/gemini-cli

say "Basic CLI"
"$CLI_BIN" --version
"$CLI_BIN" --help >/dev/null

if [ -n "${GEMINI_API_KEY:-}" ]; then
  say "Headless prompt (GEMINI_API_KEY)"
  "$CLI_BIN" --prompt "ping" >/dev/null
else
  say "GEMINI_API_KEY not set; skipping headless prompt"
fi

say "Interactive smoke"
cat <<'CHECKLIST'
Manual steps:
  1) Run: <temp>/gemini-cli (printed below)
  2) Complete OAuth login
  3) Try: /help, /model, /memory show, /clear, /quit
  4) Restart and try: /resume (if enabled)
CHECKLIST
printf 'Binary: %s\n' "$CLI_BIN"
read -r -p "Press Enter when done..." _

say "Done"
