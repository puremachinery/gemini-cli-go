#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT_DIR="${2:-$ROOT_DIR/dist}"
VERSION="${1:-}"
STAGING_ROOT=""

if [[ -z "$VERSION" ]]; then
  printf 'usage: %s <version> [output-dir]\n' "$(basename "$0")" >&2
  exit 1
fi

cleanup() {
  if [[ -n "$STAGING_ROOT" && -d "$STAGING_ROOT" ]]; then
    rm -rf "$STAGING_ROOT"
  fi
}
trap cleanup EXIT

STAGING_ROOT="$(mktemp -d "$ROOT_DIR/.release-build.XXXXXX")"

rm -rf "$OUT_DIR"
mkdir -p "$OUT_DIR"

build_target() {
  local goos="$1"
  local goarch="$2"
  local ext=""
  local archive_ext="tar.gz"
  local binary_name="gemini-cli"
  local archive_name="gemini-cli-go_${VERSION}_${goos}_${goarch}"
  local stage_dir="$STAGING_ROOT/$archive_name"
  local archive_path

  if [[ "$goos" == "windows" ]]; then
    ext=".exe"
    archive_ext="zip"
    binary_name="gemini-cli.exe"
  fi

  mkdir -p "$stage_dir"
  archive_path="$OUT_DIR/${archive_name}.${archive_ext}"

  env \
    CGO_ENABLED=0 \
    GOOS="$goos" \
    GOARCH="$goarch" \
    go build \
      -trimpath \
      -ldflags "-s -w -X github.com/puremachinery/gemini-cli-go/internal/version.Version=$VERSION" \
      -o "$stage_dir/$binary_name" \
      ./cmd/gemini-cli

  cp "$ROOT_DIR/LICENSE" "$stage_dir/LICENSE"

  if [[ "$archive_ext" == "zip" ]]; then
    (
      cd "$stage_dir"
      zip -q "$archive_path" "$binary_name" LICENSE
    )
  else
    (
      cd "$stage_dir"
      tar -czf "$archive_path" "$binary_name" LICENSE
    )
  fi
}

cd "$ROOT_DIR"

build_target darwin amd64
build_target darwin arm64
build_target linux amd64
build_target linux arm64
build_target windows amd64

: >"$OUT_DIR/SHA256SUMS"
for asset in "$OUT_DIR"/gemini-cli-go_*; do
  checksum="$(shasum -a 256 "$asset" | awk '{print $1}')"
  printf '%s  %s\n' "$checksum" "$(basename "$asset")" >>"$OUT_DIR/SHA256SUMS"
done
