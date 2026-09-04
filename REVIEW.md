# Архитектурный аудит и рефакторинг: RosPanel Target Architecture

**Дата:** 2026-09-04  
**Роль:** Senior / Staff Software Architect & Backend Reviewer  
**Статус:** Выполнено (Архитектурный план реализован, 100% тестов пройдены)

---

## 1. Целевая архитектура и соответствие

### 1.1. Разделение Control Plane и Data Plane
* **Требование:** Web/API панель не должна зависеть от жизненного цикла Xray; сбой/перезапуск Xray не должен приводить к падению Control Plane; поддержка прямого администрирования.
* **Реализация:**
  - В [`cmd/rospanel/service.go`](cmd/rospanel/service.go) устранено фатальное завершение процесса (`log.Fatalf`) при недоступности бинарника Xray. Панель переходит в автономный режим (`standalone control plane`), сохраняя доступ к веб-интерфейсу, API, базе данных и управлению пользователями.
  - Добавлена поддержка независимого слушателя Management API через Unix Domain Socket (`ROSPANEL_UNIX_SOCKET`), позволяющая управлять панелью локально в обход сетевого стека Data Plane.
  - Supervisor поддерживает unmanaged-режим (`bin == ""`) с корректной генерацией конфигурации и graceful-деградацией.

### 1.2. Разделение физических узлов и внешних серверов
* **Требование:** Физические ноды кластера и сторонние прокси-серверы должны быть изолированными сущностями; недопустима искусственная привязка внешних серверов к `LocalNodeID` или мастер-ноде.
* **Реализация:**
  - Из структуры [`sub.Server`](internal/sub/server.go) удалены поля `External []model.ExtServer`, `allowsExt` и методы сборки внешних прокси. Физическая нода теперь инкапсулирует только собственные протоколы.
  - В [`internal/server/panel.go`](internal/server/panel.go) устранена логика поиска `ordered[i].Set.ServerID == model.LocalNodeID` и резервного внедрения `ordered[0].External = ext`.
  - Получение физических серверов (`subPhysicalServers`) и сторонних серверов (`subExternalServers`) полностью разделено.

### 1.3. Унифицированная генерация подписок
* **Требование:** Чистый слой генерации: `Физические узлы + Внешние серверы -> Генератор -> Подписка`.
* **Реализация:**
  - В [`internal/sub/generate.go`](internal/sub/generate.go) внедрена универсальная структура запроса `sub.Request`:
    ```go
    type Request struct {
        User     model.User
        Settings *model.Settings
        Servers  []Server          // Физические ноды
        External []model.ExtServer // Внешние серверы
        Access   model.Access      // Права доступа групп
        DPI      model.SubDPI      // Опции обхода DPI
    }
    ```
  - Реализованы единые генераторы подписок всех поддерживаемых форматов:
    - `GenerateShareLinks(req Request) []string`
    - `GenerateClash(req Request) string`
    - `GenerateClashWithTemplate(req Request, tpl string) string`
    - `GenerateSingBox(req Request) string`
    - `GenerateXrayJSON(req Request) string`
    - `GeneratePage(req Request, ...) ([]byte, error)`
  - Поддержана автономная генерация подписок при отсутствии физических нод (`len(servers) == 0`).
  - В HTML-странице подписки (`sub.PageWithSources`) внешние серверы рендерятся в блоке подключений.

### 1.4. Балансировка и селектор узлов
* **Требование:** Алгоритмы сортировки и сокрытия переполненных физических серверов (`sub.Order`) не должны влиять на доступность внешних подключений.
* **Реализация:**
  - В [`internal/server/subscription.go`](internal/server/subscription.go) хэндлеры `handleSub` и `servePage` формируют `sub.Request`, где внешние серверы передаются независимо от фильтрации и гео-сортировки физических серверов.

---

## 2. Результаты верификации

### Автоматические тесты
1. `internal/sub`:
   - `TestExternalServersFollowAccess`: PASS (проверена генерация Base64, Clash, Sing-box, Xray JSON и HTML-страницы без физических нод).
   - Все 30 тестов пакета: PASS.
2. `internal/server`:
   - `TestSubServersExternalAttachedWhenMasterFull`: PASS (проверена доступность внешних серверов при скрытом мастере и без физических серверов).
   - Все тесты пакета: PASS.
3. Полный регрессионный прогон:
   - `go test -count=1 ./...`: 100% успешно (52 пакета, 0 сбоев).

---

## 3. Список затронутых компонентов

| Файл | Описание изменений |
|---|---|
| `cmd/rospanel/service.go` | Graceful degradation для бинарника Xray, standalone control-plane, поддержка `ROSPANEL_UNIX_SOCKET`. |
| `internal/sub/server.go` | Очистка структуры `sub.Server` от `External`. |
| `internal/sub/sub.go` | Независимые функции `ExternalEndpoints` и `ExternalShareLinks`. |
| `internal/sub/generate.go` | Модуль унифицированной генерации с `sub.Request` и генераторами форматов. |
| `internal/sub/clash.go` | Делегирование генерации Clash в `GenerateClash`, очистка синтаксиса. |
| `internal/sub/singbox.go` | Делегирование генерации Sing-Box в `GenerateSingBox`. |
| `internal/sub/xrayjson.go` | Делегирование генерации Xray JSON в `GenerateXrayJSON`. |
| `internal/sub/page.go` | Поддержка рендеринга внешних серверов и автономного режима в `PageWithSources`. |
| `internal/server/panel.go` | Разделение `subPhysicalServers` и `subExternalServers`. |
| `internal/server/subscription.go` | Использование `sub.Request` в `handleSub` и `servePage`. |
| `internal/server/ratelimit.go` | Определение `clientIP` для вызовов через Unix Domain Socket. |
| `internal/sub/external_test.go` | Тесты автономной генерации подписок. |
| `internal/server/panel_sub_ext_test.go` | Тест сохранения внешних серверов при скрытии физических нод. |
