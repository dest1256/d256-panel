# d256 panel

**RU** · [EN](#english)

Веб-панель управления Xray-узлами. Форк [3x-ui](https://github.com/MHSanaei/3x-ui) v3.11.0 (MHSanaei и контрибьюторы), изменения © dest1256, лицензия GPL-3.0 (см. [LICENSE](LICENSE), [NOTICE](NOTICE)).
Интерфейс 3x-ui сохранён (вид, навигация, тёмная тема, RU/EN), но панель **не запускает Xray**: она единственный источник правды и единственная БД, а Xray работает на узлах
([d256-node](https://github.com/dest1256/d256-node)), которые сами подключаются к панели по gRPC + mTLS. Архитектура: [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md).

## Что умеет
- **Узлы:** «Узлы → Добавить» выдаёт одноразовый токен регистрации; узел получает клиентский сертификат от внутреннего CA панели. Статус, задержка, версии, метрики (CPU, RAM, swap, диск, сеть, сокеты, аптайм) на дашборде и на странице узла.
- **Конфиг:** панель собирает конфиг Xray для каждого узла и шлёт его с номером версии; пользователи и inbound'ы добавляются без перезапуска Xray. Шаблон роутинга/аутбаундов можно задать для каждого узла отдельно.
- **Учёт:** трафик по клиентам доставляется ровно один раз, в том числе после разрыва связи (журнал на узле). Лимиты трафика, срок и **лимит устройств** узел применяет сам, даже без панели.
- **Подписки** раздаёт только панель, адреса в них — адреса узлов.
- **Клиент → «i»:** графики трафика (1ч/24ч/7д/30д/свой период, по узлам и inbound'ам), активные подключения (IP, домен, порт, узел, inbound, байты, автообновление), домены и IP за период (сортировка, поиск, фильтр по узлу, CSV), устройства.
- **Импорт `x-ui.db`:** «Узлы → Импорт из x-ui.db» добавляет inbound'ы, клиентов и трафик из базы 3x-ui на выбранный узел. Существующее не заменяется; неподдерживаемые протоколы (MTProto, AmneziaWG, TUIC), дубликаты e-mail и занятые порты пропускаются с отчётом.

## Запуск
Нужен только Docker.
```
cp .env.example .env     # задайте PANEL_DOMAIN
docker compose up -d --build
```
Панель — за Caddy (автоматический TLS на вашем домене). gRPC-порт для узлов `8444` панель слушает сама (откройте его в файрволе).
Логин и пароль администратора создаются при первом старте и показываются один раз: `docker compose logs panel | grep Username`. Сменить: `docker compose exec panel /app/d256-panel setting -username ... -password ...`.
Подключение узла — в [README узла](https://github.com/dest1256/d256-node#readme).

### Переменные окружения
| Переменная | По умолчанию | Назначение |
|---|---|---|
| `PANEL_DOMAIN` | — | домен панели для Caddy |
| `XUI_NODELINK_PORT` | `8444` | порт gRPC для узлов |
| `XUI_CONN_RETENTION_DAYS` | `30` | сколько хранить почасовые данные по доменам/IP до свёртки в дневные |
| `XUI_CONN_DAILY_RETENTION_DAYS` | `365` | сколько хранить дневные данные |
| `XUI_TRAFFIC_RETENTION_DAYS` | `90` | сколько хранить данные графиков трафика |

## Обновление и бэкап
Обновление: `git pull && docker compose up -d --build`; миграции БД выполняются при старте.
Бэкап: том `panel-data` (`/data/x-ui.db`) — вся постоянная информация, включая внутренний CA. Потеряв том, придётся заново регистрировать все узлы.

## Разработка и проверки (только Docker)
```
./dev make verify         # линтеры, форматирование, типы, тесты, сборка, govulncheck
./dev make proto          # перегенерировать gRPC-код
make smoke                # сквозной тест: панель + узел + клиент Xray
make integration          # docker compose: панель + 2 узла + 2 клиента, обрыв связи, досылка журнала
```
`smoke` и `integration` используют образы `d256-panel:local` и `d256-node:local` (`make image` в обоих репозиториях; `d256-node` должен лежать рядом) и выход в интернет.

## Безопасность и ограничения
- Узел подключается сам; панель принимает только сертификаты, выданные ей, и только тот, серийный номер которого записан у узла. Токен регистрации одноразовый и хранится как хеш.
- Данные от узла проверяются: чужие клиенты и inbound'ы, недопустимые имена, нереалистичные значения и слишком большие сообщения отбрасываются. Узел с честным ключом может искажать только **свои** данные.
- Кэш и журнал на узле зашифрованы ключом, выведенным из приватного ключа узла, лежащего на том же диске. Это защита от подмены и случайного чтения, **не** от человека с root на сервере узла — ставьте узлы на серверы, которым доверяете. Отдельного «защищённого режима» нет.
- Список активных подключений и устройств хранится в памяти панели и после её перезапуска пуст до первого снимка узла (секунды).
- Не поддерживаются: MTProto, AmneziaWG, TUIC; Telegram/Discord/e-mail/LDAP и WARP/Nord/PIA удалены из панели.

---

## English
Web control panel for Xray nodes. A fork of [3x-ui](https://github.com/MHSanaei/3x-ui) v3.11.0 (MHSanaei and contributors); changes © dest1256; GPL-3.0 (see [LICENSE](LICENSE), [NOTICE](NOTICE)).
The 3x-ui UI is kept (look, navigation, dark theme, RU/EN), but the panel **does not run Xray**: it is the single source of truth and the only database, while Xray runs on
[d256-node](https://github.com/dest1256/d256-node) instances that dial in over gRPC + mTLS. Design: [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) (Russian).

### Features
- **Nodes:** *Nodes → Add* issues a one-time enrollment token; the node gets a client certificate from the panel's internal CA. Status, latency, versions and host metrics on the dashboard and the node page.
- **Config:** the panel renders each node's Xray config and sends it with a version number; users and inbounds are added without restarting Xray. Routing/outbound templates can be set per node.
- **Accounting:** per-client usage is delivered exactly once, also after an outage (journal on the node). Traffic quota, expiry and the **device limit** are enforced by the node itself, even offline.
- **Subscriptions** are served by the panel only, with node addresses.
- **Client → "i":** traffic charts (1h/24h/7d/30d/custom, by node and inbound), live connections (IP, domain, port, node, inbound, bytes, auto-refresh), domains and IPs per period (sort, search, node filter, CSV), devices.
- **`x-ui.db` import:** *Nodes → Import from x-ui.db* adds the inbounds, clients and traffic of a 3x-ui database to the chosen node. Nothing existing is replaced; unsupported protocols (MTProto, AmneziaWG, TUIC), duplicate e-mails and busy ports are skipped and reported.

### Run
Docker is the only requirement.
```
cp .env.example .env     # set PANEL_DOMAIN
docker compose up -d --build
```
Caddy terminates TLS for the UI. The node gRPC port `8444` is served by the panel itself (open it in your firewall). The admin login is generated on first start and shown once: `docker compose logs panel | grep Username`; change it with `docker compose exec panel /app/d256-panel setting -username ... -password ...`. Connecting a node: see the node's README.
Environment: `PANEL_DOMAIN`, `XUI_NODELINK_PORT` (8444), `XUI_CONN_RETENTION_DAYS` (30), `XUI_CONN_DAILY_RETENTION_DAYS` (365), `XUI_TRAFFIC_RETENTION_DAYS` (90).

### Update and backup
Update: `git pull && docker compose up -d --build` (DB migrations run on start). Backup: the `panel-data` volume (`/data/x-ui.db`) holds everything including the internal CA; if it is lost, every node must be enrolled again.

### Development and checks (Docker only)
`./dev make verify`, `./dev make proto`, `make smoke` (panel + node + client), `make integration` (compose: panel + 2 nodes + 2 clients, outage and exactly-once redelivery). They use the images `d256-panel:local` / `d256-node:local` (`make image` in both repositories, side by side) and internet access.

### Security and limitations
- Nodes dial in; the panel only accepts certificates it issued and only the one whose serial is recorded for the node. Enrollment tokens are single-use and stored hashed.
- Everything from a node is validated (foreign clients/inbounds, bad names, unrealistic values, oversized messages are dropped). A node can only distort its **own** data.
- The node's cache and journal are encrypted with a key derived from the node's private key stored on the same disk: protection against tampering and casual reading, **not** against root on the node server — run nodes on servers you trust. There is no separate "protected mode".
- Live connections and devices are held in the panel's memory and are empty after a restart until the next node snapshot (seconds).
- Not supported: MTProto, AmneziaWG, TUIC; Telegram/Discord/e-mail/LDAP and WARP/Nord/PIA were removed.
