# RosPanel Agent Guide & Workspace Invariants

Проект: **Shu1t3/rospanel-shu1t3** — форк панели управления прокси-серверами AppsGanin/rospanel.  
Архитектура: Распределённая система Master-Node.  
Бэкенд: Go 1.27 + CGO-free SQLite (WAL) + интеграция с Xray-core / Sing-box / WireGuard / AmneziaWG.  
Фронтенд: React 19 SPA + Tailwind CSS v4 + Biome + Playwright (`web/`). Встраивается в Go-бинарник через `web/embed.go`.

---

## 1. Release Workflow (Обязательное правило)

Before pushing changes to a branch that triggers a RosPanel release, merging a release PR, or publishing a release by hand, apply the [rospanel-release-lab skill](.agents/skills/rospanel-release-lab/SKILL.md). Complete its Linux checks for the candidate before the first action that can trigger publication. Reuse the evidence for a release PR that only updates version metadata, after checking its diff and base commit. Report any missing coverage and stop publication until the checks pass or the user explicitly accepts the identified gap.

---

## 2. Ключевые инварианты проекта

1. **Защита форка (Fork Safeguards)**:
   - Неприкосновенны уникальные подсистемы форка:
     * **Мульти-хоп маршрутизация**: сквозные VLESS цепочки (вход → транзит → выход), редактор `web/src/MultiHopDialog.tsx`, выделенный диапазон портов `28443–28506`, обратные маршруты (`manager_multihop_reverse.go`, `reverse_guard.go`), возвращение на входную ноду.
     * **Лимиты устройств**: отслеживание сессий (`ActiveDeviceCountForUser`), ротация токенов подписки, принудительное завершение соединений.
     * **REALITY & Decoy**: расширенный пул сайтов-доноров РФ и twin names, совмещение decoy и REALITY на порту 443 по SNI, исключение HTTP/3 рекламирования.
     * **Безопасность и хранилище**: brute-guard на уровне `nftables`, идемпотентная обработка дубликатов колонок в миграциях SQLite, сравнение токенов в константном времени.
     * **Инфраструктура**: переменная окружения `ROSPANEL_REPO` для независимого репозитория обновлений.
   - Подробнее: [.agents/rules/fork-safeguards.md](.agents/rules/fork-safeguards.md). При слияниях с upstream руководствоваться навыком [rospanel-sync-upstream](.agents/skills/rospanel-sync-upstream/SKILL.md).

2. **Связка фронтенда и бэкенда**:
   - Фронтенд компилируется в `web/dist` и встраивается в Go-бинарник через `web/embed.go` (`//go:embed all:dist`).
   - При любых изменениях в директории `web/` обязательно выполнить:
     * `npm --prefix web run typecheck`
     * `npm --prefix web run lint`
     * `npm --prefix web run build` (обязательно до компиляции Go или прогона Go-тестов, зависящих от встроенного фронтенда).

3. **Синхронизация мультиязычности (i18n)**:
   - Поддерживаются строго 5 языков: Русский (`ru`), Английский (`en`), Фарси (`fa`), Арабский (`ar`), Китайский (`zh`).
   - Любое изменение текста UI требует обновления **всех 5 файлов** в `web/src/i18n/`.
   - Добавление ошибок/сообщений бэкенда требует обновления **всех 5 файлов** в `internal/i18n/` и запуска кодогенерации `go generate ./internal/i18n/...`.
   - Тесты `callsites_test.go` и `TestCatalogsMatch` проверяют синхронность в CI. Подробнее: [.agents/rules/i18n-synchronization.md](.agents/rules/i18n-synchronization.md).

4. **База данных и SQLite миграции**:
   - База работает в режиме WAL на `modernc.org/sqlite` (single-writer).
   - Запрещено удерживать транзакции во время сетевых запросов или долгих операций ввода-вывода (во избежание lock starvation).
   - Миграции `ALTER TABLE` должны быть строго идемпотентными. Номера миграций назначаются с запасом, чтобы не конфликтовать с апстримом. Подробнее: [.agents/rules/sqlite-and-migrations.md](.agents/rules/sqlite-and-migrations.md).

5. **Генерация конфигураций Xray**:
   - Конфигурации Xray генерируются детерминированно в `internal/xray/`.
   - Выключенные протоколы и фичи (например, REALITY) не должны порождать фантомные или невалидные секции в `config.json`.
   - Для проверки интеграции с Xray используется `RP_XRAY_BIN`. Подробнее: [.agents/rules/xray-and-networking.md](.agents/rules/xray-and-networking.md).

---

## 3. Навигация по правилам и навыкам

- **Точечные правки без лишнего шума**: [.agents/rules/surgical-editing.md](.agents/rules/surgical-editing.md)
- **Дисциплина терминала и проверочные команды**: [.agents/rules/terminal-discipline.md](.agents/rules/terminal-discipline.md)
- **Коммиты и Release-please**: [.agents/rules/changelog-and-commits.md](.agents/rules/changelog-and-commits.md)
- **Идиоматичный Go в контексте RosPanel**: [.agents/rules/go-idiomatic.md](.agents/rules/go-idiomatic.md)
- **Горутины, контексты и блокировки**: [.agents/rules/go-concurrency.md](.agents/rules/go-concurrency.md)
- **Оптимизация токенов и чтение контекста**: [.agents/rules/token-optimization.md](.agents/rules/token-optimization.md)
- **Синхронизация с upstream**: навык [rospanel-sync-upstream](.agents/skills/rospanel-sync-upstream/SKILL.md)
- **Ревью кода перед коммитом**: навык [rospanel-code-review](.agents/skills/rospanel-code-review/SKILL.md)
- **Разработка UI**: навык [rospanel-frontend](.agents/skills/rospanel-frontend/SKILL.md)
- **Отладка Xray**: навык [rospanel-xray-lab](.agents/skills/rospanel-xray-lab/SKILL.md)
