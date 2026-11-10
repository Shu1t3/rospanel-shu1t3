---
name: rospanel-sync-upstream
description: Step-by-step procedure to safely synchronize the fork with upstream AppsGanin/rospanel while preserving all fork safeguards (Multi-Hop, Russian REALITY pools, device limits, migration idempotency, custom update repo).
allowed-tools: Bash(bash:*)
---

# Синхронизация форка с Upstream (AppsGanin/rospanel)

Используйте этот навык при подтягивании новых релизов, фич или исправлений безопасности из официального репозитория `AppsGanin/rospanel` в форк `Shu1t3/rospanel-shu1t3`.

---

## 1. Подготовка и анализ диапазона коммитов

1. Убедитесь в чистоте рабочей директории:
   ```bash
   git status
   ```
2. Подтяните актуальное состояние апстрима:
   ```bash
   git fetch upstream main --tags
   ```
3. Определите диапазон входящих изменений апстрима относительно базы форка:
   ```bash
   git log --oneline HEAD..upstream/main
   ```
4. Изучите затронутые файлы и оцените пересечения с модификациями форка:
   ```bash
   git diff --stat HEAD...upstream/main
   ```

---

## 2. Анализ зон повышенного риска

Перед слиянием обратите особое внимание на следующие критические области:

### 2.1. Миграции SQLite (`internal/store/migrations/`)
- Проверьте, добавил ли апстрим новые SQL-файлы.
- Если номера миграций апстрима совпадают с номерами миграций форка:
  * Миграции форка перенумеровываются с запасом вперед (например, с `0066` в `0140+`).
  * Проверьте, что все новые миграции используют `CREATE TABLE IF NOT EXISTS` и идемпотентны.

### 2.2. Защищённые компоненты форка
Сверьтесь с правилом [.agents/rules/fork-safeguards.md](../../rules/fork-safeguards.md):
- **Мульти-хоп маршруты**: проверьте, не затронуты ли `manager_multihop*.go`, `multihop.go`, `MultiHopDialog.tsx`.
- **REALITY и доноры**: не перезаписан ли расширенный список доноров РФ и логика SNI decoy на порту 443.
- **Лимиты устройств**: сохранена ли логика `ActiveDeviceCountForUser`.
- **Репозиторий обновлений**: сохранена ли поддержка переменной `ROSPANEL_REPO`.

### 2.3. Интернационализация (i18n)
- Если апстрим добавил новые ключи локализации в `internal/i18n/` или `web/src/i18n/`, убедитесь, что они добавлены во **все 5 языков** (`ru`, `en`, `ar`, `fa`, `zh`).

---

## 3. Выполнение слияния и разрешение конфликтов

1. Запустите слияние:
   ```bash
   git merge upstream/main
   ```
2. При возникновении конфликтов разрешайте их с безусловным сохранением функциональности форка:
   - В логике inbounds/routing объединяйте возможности апстрима с цепочками Multi-Hop форка.
   - В настройках соединений сохраняйте расширенные опции доноров REALITY.
3. Проверьте статус:
   ```bash
   git diff --check
   ```

---

## 4. Контур валидации после слияния

Выполните полный цикл проверок:

```bash
# 1. Валидация фронтенда
npm --prefix web run typecheck
npm --prefix web run lint
npm --prefix web run build

# 2. Проверка соответствия каталогов i18n
go test -v ./internal/i18n/...

# 3. Проверка миграций хранилища
go test -v ./internal/store/...

# 4. Проверка ядра и сетевого уровня с детектором гонок
go test -race ./internal/core/... ./internal/xray/...
```

---

## 5. Оформление коммита

Формируйте коммит слияния строго по стандарту Conventional Commits:

**Тема:**
```
feat: sync upstream vX.Y.Z preserving fork safeguards
```
или
```
chore: sync upstream vX.Y.Z preserving fork safeguards
```

**Тело коммита:**
```markdown
- Upstream changes: synced commits up to upstream/main (tag vX.Y.Z).
- Fork safeguards:
  - Preserved multi-hop route manager and subscription profiles.
  - Maintained extended Russian donor pool for REALITY and port 443 SNI decoy sharing.
  - Retained device tracking limits and safe idempotent SQLite migrations.
- Verification: passed web typecheck/build, go vet, and race-detector test suite.
```

Перед выпуском релиза выполните проверку в соответствии с [rospanel-release-lab](../rospanel-release-lab/SKILL.md).

