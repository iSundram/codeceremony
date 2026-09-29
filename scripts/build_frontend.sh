#!/usr/bin/env bash
# Build the frontend and stage it for embedding in the Go binary.
#
# The portal ships as one binary with no Node runtime, so this runs the Vite
# build and copies dist/ into the package the Go embed directive points at. The
# order matters: tokens before components, because the token layer has to be in
# the document before the layers that consume its custom properties.
#
# With no built frontend present the binary still serves the JSON API and the
# server-rendered pages, so `go test ./...` and a backend-only checkout work
# without a Node toolchain. That is deliberate: the API must not depend on a
# frontend build to be testable.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WEB="$REPO_ROOT/web"
STAGE="$REPO_ROOT/backend/internal/httpapi/webassets/spa"

if [[ "${1:-}" == "--check" ]]; then
  if [[ ! -f "$STAGE/index.html" ]]; then
    echo "error: no frontend build is staged at $STAGE" >&2
    echo "hint: run scripts/build_frontend.sh" >&2
    exit 1
  fi
  echo "a frontend build is staged at $STAGE"
  exit 0
fi

if ! command -v npm >/dev/null 2>&1; then
  echo "error: npm is not on PATH; the frontend build needs Node 20 or newer" >&2
  exit 1
fi

echo "==> installing dependencies"
cd "$WEB"
if [[ -f package-lock.json ]]; then
  npm ci --no-audit --no-fund
else
  npm install --no-audit --no-fund
fi

echo "==> typechecking"
npx tsc -b

echo "==> running the component tests"
npx vitest run

echo "==> verifying the icon inventory is current"
cd "$REPO_ROOT"
if [[ ! -d package ]]; then
  npm pack lucide-static@1.48.0 --silent >/dev/null 2>&1 || true
  tar xzf lucide-static-*.tgz 2>/dev/null || true
fi
if [[ -d package ]]; then
  python3 scripts/gen_icons.py --check || {
    rm -rf package lucide-static-*.tgz
    echo "error: the icon inventory is stale; run scripts/gen_icons.py" >&2
    exit 1
  }
  rm -rf package lucide-static-*.tgz
fi

echo "==> building"
cd "$WEB"
npx vite build

echo "==> staging for the Go embed"
rm -rf "$STAGE"
mkdir -p "$STAGE"
cp -r "$WEB/dist/." "$STAGE/"

echo "==> done"
echo "staged $(find "$STAGE" -type f | wc -l) files into $STAGE"
echo "build the binary with: cd backend && go build ./cmd/codeceremony"
