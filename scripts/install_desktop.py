"""Copy the reviewed project to a new Desktop folder without replacing files."""
from pathlib import Path
import shutil
import sys

source = Path(__file__).resolve().parents[1]
destination = Path(sys.argv[1]).expanduser().resolve()
if destination.exists():
    raise SystemExit(f"Папка уже существует, файлы не изменены: {destination}")

shutil.copytree(
    source,
    destination,
    ignore=shutil.ignore_patterns("node_modules", ".local", ".git", ".env", "__pycache__", "*.log"),
)
print(f"Проект сохранён: {destination}")
print("Исходный код, готовый сервер, интерфейс, инструкция и файл запуска скопированы.")
