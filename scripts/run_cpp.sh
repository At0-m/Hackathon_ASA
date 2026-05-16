#!/usr/bin/env bash
set -euo pipefail
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "$ROOT_DIR/scripts/load_env.sh"
cd "$ROOT_DIR/services/cpp-analyzer"
if [ ! -x build/ai_analyzer ]; then
  cmake -S . -B build -DCMAKE_BUILD_TYPE=Release
  cmake --build build --parallel
fi
echo "ACA C++ analyzer: http://localhost:${ANALYZER_PORT}"
./build/ai_analyzer
