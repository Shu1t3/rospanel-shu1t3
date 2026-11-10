# Terminal & Command Discipline

## 1. Общие принципы
- **Минимизация вывода**: всегда добавляйте тихие флаги или фильтруйте вывод потенциально многословных команд (через `head -n 30`, `tail -n 30` или `grep`).
- **Таргетированное тестирование**: запускайте тесты только для затронутого пакета или конкретного сценария, а не всего проекта целиком.

---

## 2. Стандартные проверочные команды RosPanel

### Фронтенд (`web/`):
- Проверка типов TypeScript:
  ```bash
  npm --prefix web run typecheck
  ```
- Линтинг Biome:
  ```bash
  npm --prefix web run lint
  ```
- Сборка дистрибутива (обязательна перед компиляцией Go, если менялся код в `web/`):
  ```bash
  npm --prefix web run build
  ```

### Бэкенд (Go):
- Точечный запуск конкретного теста:
  ```bash
  go test -v ./internal/<package>/... -run TestFunctionName
  ```
- Тестирование пакета с детектором гонок:
  ```bash
  go test -race ./internal/<package>/...
  ```
- Интеграционные тесты Xray с локальным бинарником:
  ```bash
  RP_XRAY_BIN=/usr/local/bin/xray go test -v ./internal/xray/...
  ```
- Проверка каталогов i18n и вызовов:
  ```bash
  go test -v ./internal/i18n/...
  ```
- Проверка известных уязвимостей зависимостей:
  ```bash
  go run golang.org/x/vuln/cmd/govulncheck@latest ./...
  ```
