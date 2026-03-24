#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
OUT_DIR="${2:-$ROOT_DIR/dist}"
VERSION="${1:-}"
STAGING_ROOT=""

if [[ -z "$VERSION" ]]; then
  printf 'usage: %s <version> [output-dir]\n' "$(basename "$0")" >&2
  exit 1
fi

if [[ "$OUT_DIR" != /* ]]; then
  OUT_DIR="$ROOT_DIR/$OUT_DIR"
fi

case "$OUT_DIR" in
  ..|../*|*/..|*/../*)
    printf 'error: output directory must not contain parent directory traversal (%s)\n' "$OUT_DIR" >&2
    exit 1
    ;;
esac

mkdir -p "$(dirname "$OUT_DIR")"
OUT_DIR="$(cd "$(dirname "$OUT_DIR")" && pwd -P)/$(basename "$OUT_DIR")"

cleanup() {
  if [[ -n "$STAGING_ROOT" && -d "$STAGING_ROOT" ]]; then
    rm -rf "$STAGING_ROOT"
  fi
}
trap cleanup EXIT

STAGING_ROOT="$(mktemp -d "$ROOT_DIR/.release-build.XXXXXX")"

case "$OUT_DIR" in
  ""|"/"|"$ROOT_DIR")
    printf 'error: refusing to remove unsafe output directory %q\n' "$OUT_DIR" >&2
    exit 1
    ;;
  "$ROOT_DIR"/*)
    ;;
  *)
    printf 'error: output directory must stay under %s (got %s)\n' "$ROOT_DIR" "$OUT_DIR" >&2
    exit 1
    ;;
esac

rm -rf "$OUT_DIR"
mkdir -p "$OUT_DIR"

build_target() {
  local goos="$1"
  local goarch="$2"
  local archive_ext="tar.gz"
  local binary_name="gemini-cli"
  local archive_name="gemini-cli-go_${VERSION}_${goos}_${goarch}"
  local stage_dir="$STAGING_ROOT/$archive_name"
  local archive_path

  if [[ "$goos" == "windows" ]]; then
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

if command -v sha256sum >/dev/null 2>&1; then
  (
    cd "$OUT_DIR"
    sha256sum gemini-cli-go_* > SHA256SUMS
  )
elif command -v shasum >/dev/null 2>&1; then
  (
    cd "$OUT_DIR"
    shasum -a 256 gemini-cli-go_* > SHA256SUMS
  )
else
  printf 'error: neither sha256sum nor shasum is available\n' >&2
  exit 1
fi
