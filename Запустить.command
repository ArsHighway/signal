#!/bin/bash
cd "$(dirname "$0")"
bash scripts/start.sh
result=$?
if [[ $result -ne 0 ]]; then
  echo ""
  echo "Запуск завершился с ошибкой. Инструкция находится в README.md."
  read -r -p "Нажмите Enter, чтобы закрыть окно…"
fi
exit "$result"
