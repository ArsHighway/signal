#!/usr/bin/env bash
set -euo pipefail
export PATH="/opt/homebrew/bin:/opt/homebrew/opt/postgresql@14/bin:/opt/homebrew/opt/postgresql@16/bin:/usr/local/bin:/usr/local/go/bin:$PATH"
PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
export PROJECT_ROOT
cd "$PROJECT_ROOT"
# Ignore an unrelated DATABASE_URL inherited from another project.
export DATABASE_URL="${SIGNAL_DATABASE_URL:-}"
if [[ -f .env ]]; then set -a; source .env; set +a; fi
source scripts/database.sh
trap stop_project_database EXIT
prepare_database
if [[ ! -x bin/signalcare || ! -f dist/index.html ]]; then bash scripts/build.sh; fi
echo ""
echo "Сигнал · учебный прототип"
echo "Кабинет:    http://127.0.0.1:8087"
echo "Диспетчер:  http://127.0.0.1:8087/dispatcher"
echo "Для остановки нажмите Ctrl+C. Данные сохранятся в .local/postgres."
echo ""
./bin/signalcare
