#!/bin/bash
cd "$(dirname "$0")"
bash scripts/build.sh
result=$?
echo ""
read -r -p "Нажмите Enter, чтобы закрыть окно…"
exit "$result"
