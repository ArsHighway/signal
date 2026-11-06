#!/usr/bin/env bash
# Sourced by start.sh. This cluster belongs only to this project.
prepare_database() {
  if [[ -n "${DATABASE_URL:-}" ]]; then
    echo "Используем PostgreSQL из DATABASE_URL."
    return
  fi
  local data_dir="$PROJECT_ROOT/.local/postgres"
  local port="${LOCAL_PG_PORT:-55439}"
  if ! command -v initdb >/dev/null || ! command -v pg_ctl >/dev/null; then
    echo "Нужен PostgreSQL 14+ (initdb и pg_ctl) или запущенная база с DATABASE_URL."
    echo "Альтернатива: docker compose up --build"
    return 1
  fi
  mkdir -p "$PROJECT_ROOT/.local"
  if [[ ! -f "$data_dir/PG_VERSION" ]]; then
    printf '%s\n' 'signal_local_only' > "$PROJECT_ROOT/.local/pg-password"
    chmod 600 "$PROJECT_ROOT/.local/pg-password"
    echo "Создаём отдельную учебную базу проекта…"
    initdb -D "$data_dir" -U signal --pwfile="$PROJECT_ROOT/.local/pg-password" --auth-local=trust --auth-host=scram-sha-256 --encoding=UTF8 --locale=C > "$PROJECT_ROOT/.local/initdb.log"
  fi
  if ! pg_ctl -D "$data_dir" status >/dev/null 2>&1; then
    if pg_isready -h 127.0.0.1 -p "$port" >/dev/null 2>&1; then
      echo "Порт $port занят другой базой. Укажите LOCAL_PG_PORT в .env."
      return 1
    fi
    pg_ctl -D "$data_dir" -l "$PROJECT_ROOT/.local/postgres.log" -o "-h 127.0.0.1 -p $port -k ''" -w start
    PROJECT_PG_STARTED=1
  fi
  export PGPASSWORD=signal_local_only
  if ! psql -h 127.0.0.1 -p "$port" -U signal -d postgres -tAc "SELECT 1 FROM pg_database WHERE datname='signalcare'" | grep -q '^1$'; then
    createdb -h 127.0.0.1 -p "$port" -U signal signalcare
  fi
  unset PGPASSWORD
  export DATABASE_URL="postgres://signal:signal_local_only@127.0.0.1:$port/signalcare?sslmode=disable"
}

stop_project_database() {
  if [[ "${PROJECT_PG_STARTED:-0}" == "1" ]]; then
    pg_ctl -D "$PROJECT_ROOT/.local/postgres" -m fast -w stop >/dev/null 2>&1 || true
  fi
}
