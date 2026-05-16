#!/usr/bin/env bash
set -euo pipefail
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
if [ -f "$ROOT_DIR/.env.local" ]; then
  set -a
  source "$ROOT_DIR/.env.local"
  set +a
elif [ -f "$ROOT_DIR/.env" ]; then
  set -a
  source "$ROOT_DIR/.env"
  set +a
fi
export PORT="${PORT:-8080}"
export DATA_DIR="${DATA_DIR:-./data}"
export CPP_ANALYZER_URL="${CPP_ANALYZER_URL:-http://localhost:9090}"
export FRONTEND_BASE_URL="${FRONTEND_BASE_URL:-http://localhost:5173}"
export PUBLIC_BASE_URL="${PUBLIC_BASE_URL:-http://localhost:8080}"
export MISTRAL_API_URL="${MISTRAL_API_URL:-https://api.mistral.ai/v1/chat/completions}"
export MISTRAL_MODEL="${MISTRAL_MODEL:-mistral-small-latest}"
export MISTRAL_JUDGE_MODEL="${MISTRAL_JUDGE_MODEL:-${MISTRAL_MODEL}}"
export ALICE_API_URL="${ALICE_API_URL:-https://llm.api.cloud.yandex.net/foundationModels/v1/completion}"
export ALICE_MODEL="${ALICE_MODEL:-yandexgpt-lite}"
export ANALYZER_PORT="${ANALYZER_PORT:-9090}"
export VITE_API_BASE_URL="${VITE_API_BASE_URL:-http://localhost:8080}"
