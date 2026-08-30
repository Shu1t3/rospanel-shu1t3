# Happ Subscriptions Integration & Decryption

Пакет `internal/happ` реализует интеграцию внешних подписок формата **Happ Proxy Subscription** в РосПанель:
1. Загрузку подписок по HTTP/HTTPS (с поддержкой plain URI-списков, Base64-списков и зашифрованных ссылок `happ://crypt*`).
2. Нативную расшифровку форматов `happ://crypt`, `happ://crypt2`, `happ://crypt3`, `happ://crypt4` и `happ://crypt5`.
3. Парсинг конфигураций прокси-узлов (`vless://`, `vmess://`, `trojan://`, `ss://`, `hysteria2://`).
4. Автоматическую генерацию исходящих соединений (Xray Outbounds `happ-<id>`).
5. Экспорт подключений в клиентские подписки (Universal Links, Sing-Box JSON, Clash/Mihomo YAML, HTML) с поддержкой групп доступа (`happ:<id>`).
6. Автоматическое фоновое обновление каждые 59 минут.

---

## Форматы шифрования Happ

- **happ://crypt/** — RSA-1024 (PKCS#1 v1.5).
- **happ://crypt2/** — RSA-4096 (PKCS#1 v1.5).
- **happ://crypt3/** — RSA-4096 (PKCS#1 v1.5).
- **happ://crypt4/** — RSA-4096 (PKCS#1 v1.5).
- **happ://crypt5/** — Гибридная схема: RSA PKCS#1 v1.5 (расшифровка сессионного ключа из таблицы ключей) + ChaCha20-Poly1305 (расшифровка полезной нагрузки).

---

## Источники, благодарности и проверка обновлений

Спецификация криптографических схем Happ и ключи основаны на исследовании разработчика **LeeeeT**:
- **Онлайн-декриптор**: [https://leeeet.dev/happ-decryptor/](https://leeeet.dev/happ-decryptor/)
- **Репозиторий с исходным кодом**: [https://github.com/LeeeeT/happ-decryptor](https://github.com/LeeeeT/happ-decryptor)

При появлении новых версий схем Happ (`happ://crypt6/...` и новее) сверяйтесь с репозиторием [LeeeeT/happ-decryptor](https://github.com/LeeeeT/happ-decryptor) для обновления ключей и алгоритмов в `internal/happ/decrypt.go`.
