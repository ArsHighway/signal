#!/usr/bin/env bash
set -euo pipefail
export PATH="/opt/homebrew/bin:/usr/local/bin:/usr/local/go/bin:$PATH"
cd "$(dirname "${BASH_SOURCE[0]}")/.."
if ! command -v go >/dev/null || ! command -v npm >/dev/null; then
  echo "Для сборки нужны Go 1.23+ и Node.js 22+ с npm."
  exit 1
fi
if [[ ! -d node_modules ]]; then npm ci; fi
npm run build
mkdir -p bin
go build -trimpath -o bin/signalcare ./cmd/server
echo "Готово: интерфейс и сервер собраны."
