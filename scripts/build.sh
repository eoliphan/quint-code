#!/usr/bin/env bash
# Build haft CLI binary.
# Usage: ./scripts/build.sh [--install]
#
# Output:
#   bin/haft — Go binary

set -euo pipefail

PROJECT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$PROJECT_DIR"

echo "=== Building haft CLI ==="

echo "Building Go binary..."
mkdir -p bin
go build -o bin/haft ./cmd/haft
echo "  bin/haft"

if [[ "${1:-}" == "--install" ]]; then
  echo "Installing CLI..."
  mkdir -p "$HOME/.local/bin"
  cp bin/haft "$HOME/.local/bin/haft"
  chmod +x "$HOME/.local/bin/haft"
  echo "  ~/.local/bin/haft"
fi

echo ""
echo "Done. Run: ./bin/haft"
