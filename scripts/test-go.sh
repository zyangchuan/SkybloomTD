#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

modules=(
  "services/document-content/src/api"
  "services/game-service/src"
)

for module in "${modules[@]}"; do
  echo "==> Testing ${module}"
  (
    cd "${ROOT_DIR}/${module}"
    GOCACHE="${PWD}/.gocache" go test ./...
    GOCACHE="${PWD}/.gocache" go clean -cache >/dev/null 2>&1 || true
    rm -rf "${PWD}/.gocache"
  )
done

echo "All Go unit and integration tests passed."
