---
name: rospanel-xray-lab
description: Local validation, testing, and debugging of Xray configuration generation in RosPanel. Covers VLESS, REALITY, Multi-Hop chains, port collision detection, and testing with RP_XRAY_BIN.
allowed-tools: Bash(bash:*)
---

# RosPanel Xray Lab & Configuration Validation

Используйте этот навык при разработке, отладке или модификации генератора конфигураций Xray, маршрутизации трафика, мульти-хоп цепочек или протоколов VLESS/REALITY/AmneziaWG (`internal/xray`, `internal/sub`).

---

## 1. Запуск интеграционных тестов с реальным Xray-core

Пакет `internal/xray` содержит полноценные тесты, которые проверяют не только синтаксис Go-структур, но и реальную валидность конфигураций с помощью запуска бинарника Xray.

### 1.1. Переменная окружения `RP_XRAY_BIN`
Если переменная `RP_XRAY_BIN` не задана, тесты, требующие запущенного ядра Xray, автоматически пропускаются (`t.Skip`).

Для полноценного прогона укажите путь к бинарнику:
```bash
RP_XRAY_BIN=$(which xray 2>/dev/null || echo "/usr/local/bin/xray") go test -v ./internal/xray/...
```

### 1.2. Таргетированные тесты мульти-хопа
Для проверки цепочек Multi-Hop и обратных маршрутов запускайте:
```bash
# Проверка базовых цепочек мульти-хопа (вход -> транзит -> выход):
RP_XRAY_BIN=/usr/local/bin/xray go test -v ./internal/xray/ -run TestMultiHopReal

# Проверка защищенных обратных маршрутов (reverse multi-hop):
RP_XRAY_BIN=/usr/local/bin/xray go test -v ./internal/xray/ -run TestMultiHopReverseReal
```

---

## 2. Чеклист проверки генерируемой конфигурации Xray

При добавлении нового транспорта или изменении шаблонов конфигурации проверяйте следующие критерии:

### 2.1. Детерминизм и отсутствие «висячих» секций
- [ ] Если в базе данных отключен REALITY или протокол деактивирован, секция `streamSettings.realitySettings` **не должна** попадать в JSON с пустыми полями.
- [ ] Все сгенерированные `tag` в секциях `inbounds` и `outbounds` должны иметь соответствующие правила в `routing.rules`.

### 2.2. Мульти-хоп цепочки (VLESS Multi-Hop)
- [ ] Входная нода содержит inbound с протоколом VLESS и портом из диапазона `28443–28506`.
- [ ] Трафик направляется в соответствующий outbound транзитной ноды через `routing.rules`.
- [ ] На выходной ноде трафик передается в `outbound: direct` или `outbound: block` (при утечке/сбое транзита).
- [ ] Отсутствуют петли маршрутизации (routing loops).

### 2.3. Порт 443 и разделение по SNI (Decoy + REALITY)
- [ ] При совмещении порта 443 inbound REALITY направляет чужой трафик (fallback) на локальный decoy-сервер.
- [ ] В ответах decoy отключен заголовок `Alt-Svc: h3` (HTTP/3 рекламирование).
- [ ] Проверено отсутствие коллизий со встроенными портами панели.

---

## 3. Ручная валидация сгенерированного JSON

Если требуется проверить конкретный сгенерированный файл конфигурации вручную через утилиту Xray:
```bash
xray -test -config /path/to/test-config.json
```
Успешный вывод должен содержать: `Configuration OK`.
Любые предупреждения о неизвестных полях или синтаксических ошибках являются блокирующими дефектами.

