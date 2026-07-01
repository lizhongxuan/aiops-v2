#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VERSION="${AIOPS_NODE_VERSION:-${HOST_AGENT_VERSION:-v0.1.0}}"
GOOS_TARGET="${AIOPS_NODE_GOOS:-linux}"
GOARCH_TARGET="${AIOPS_NODE_GOARCH:-amd64}"
PLATFORM_DIR="${GOOS_TARGET}-${GOARCH_TARGET}"
ARTIFACT_DIR="$ROOT_DIR/artifacts/host-agent/$VERSION/$PLATFORM_DIR"
ARTIFACT_PATH="$ARTIFACT_DIR/host-agent"
MANIFEST_PATH="$ROOT_DIR/artifacts/host-agent/manifest.json"

sha256_file() {
  local file="$1"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$file" | awk '{print $1}'
    return
  fi
  shasum -a 256 "$file" | awk '{print $1}'
}

mkdir -p "$ARTIFACT_DIR" "$(dirname "$MANIFEST_PATH")"

(
  cd "$ROOT_DIR"
  CGO_ENABLED=0 GOOS="$GOOS_TARGET" GOARCH="$GOARCH_TARGET" \
    go build -trimpath -ldflags="-s -w" -o "$ARTIFACT_PATH" ./cmd/host-agent
)

chmod 755 "$ARTIFACT_PATH"

SHA256="$(sha256_file "$ARTIFACT_PATH")"
BUILT_AT="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"
GIT_COMMIT="$(cd "$ROOT_DIR" && git rev-parse --short HEAD 2>/dev/null || true)"

cat >"$MANIFEST_PATH" <<EOF
{
  "version": "$VERSION",
  "builtAt": "$BUILT_AT",
  "gitCommit": "$GIT_COMMIT",
  "artifacts": [
    {
      "platform": "linux/ubuntu",
      "os": "$GOOS_TARGET",
      "arch": "$GOARCH_TARGET",
      "path": "artifacts/host-agent/$VERSION/$PLATFORM_DIR/host-agent",
      "sha256": "$SHA256"
    }
  ]
}
EOF

printf '[aiops-v2] built Node artifact: %s\n' "$ARTIFACT_PATH"
printf '[aiops-v2] sha256: %s\n' "$SHA256"
