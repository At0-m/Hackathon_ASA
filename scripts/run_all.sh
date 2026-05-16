#!/usr/bin/env bash
set -euo pipefail
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "$ROOT_DIR/scripts/load_env.sh"
PIDS=()
cleanup() {
  for pid in "${PIDS[@]}"; do
    kill "$pid" >/dev/null 2>&1 || true
  done
}
trap cleanup EXIT INT TERM
(
  cd "$ROOT_DIR/services/cpp-analyzer"
  if [ ! -x build/ai_analyzer ]; then
    cmake -S . -B build -DCMAKE_BUILD_TYPE=Release
    cmake --build build --parallel
  fi
  ./build/ai_analyzer
) & PIDS+=("$!")
sleep 1
(
  cd "$ROOT_DIR/backend"
  mkdir -p "$DATA_DIR"
  go run ./cmd/server
) & PIDS+=("$!")
sleep 2
(
  cd "$ROOT_DIR/frontend"
  if command -v pnpm >/dev/null 2>&1; then
    if [ ! -d node_modules ]; then pnpm install; fi
    VITE_API_BASE_URL="$VITE_API_BASE_URL" pnpm run dev -- --host 0.0.0.0
  else
    if [ ! -d node_modules ]; then npm install; fi
    VITE_API_BASE_URL="$VITE_API_BASE_URL" npm run dev -- --host 0.0.0.0
  fi
) & PIDS+=("$!")
echo ""
echo "ACA started"
echo "frontend: http://localhost:5173"
echo "backend:  http://localhost:${PORT}/health"
echo "analyzer: http://localhost:${ANALYZER_PORT}/health"
wait
