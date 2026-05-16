#!/usr/bin/env bash
set -euo pipefail
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "$ROOT_DIR/scripts/load_env.sh"
cd "$ROOT_DIR/frontend"
if command -v pnpm >/dev/null 2>&1; then
  if [ ! -d node_modules ]; then pnpm install; fi
  echo "ACA frontend: http://localhost:5173"
  VITE_API_BASE_URL="$VITE_API_BASE_URL" pnpm run dev -- --host 0.0.0.0
else
  if [ ! -d node_modules ]; then npm install; fi
  echo "ACA frontend: http://localhost:5173"
  VITE_API_BASE_URL="$VITE_API_BASE_URL" npm run dev -- --host 0.0.0.0
fi
