#!/usr/bin/env bash
set -euo pipefail
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "$ROOT_DIR/scripts/load_env.sh"
cd "$ROOT_DIR/backend"
mkdir -p "$DATA_DIR"
echo "ACA backend: http://localhost:${PORT}"
go run ./cmd/server
