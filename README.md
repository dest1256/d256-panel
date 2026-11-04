# d256 panel

**RU** · [EN](#english)

Веб-панель управления Xray-узлами. Форк [3x-ui](https://github.com/MHSanaei/3x-ui) v3.11.0 (MHSanaei и контрибьюторы), изменения © dest1256, лицензия GPL-3.0 (см. [LICENSE](LICENSE), [NOTICE](NOTICE)).
Интерфейс 3x-ui сохранён (вид, навигация, тёмная тема, RU/EN), но панель **не запускает Xray**: она единственный источник правды и единственная БД, а Xray работает на узлах
([d256-node](https://github.com/dest1256/d256-node)), которые сами подключаются к панели по gRPC + mTLS. Архитектура: [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md).

## Что умеет
- **Узлы:** «Узлы → Добавить» выдаёт одноразовый токен регистрации; узел получает клиентский сертификат от внутреннего CA панели. Статус, задержка, версии, метрики (CPU, RAM, swap, диск, сеть, сокеты, аптайм) на дашборде и на странице узла.
- **Конфиг:** панель собирает конфиг Xray для каждого узла и шлёт его с номером версии; пользователи и inbound'ы добавляются без перезапуска Xray. Шаблон роутинга/аутбаундов можно задать для каждого узла отдельно.
- **Учёт:** трафик по клиентам доставляется ровно один раз, в том числе после разрыва связи (журнал на узле). Лимиты трафика, срок и **лимит устройств** узел применяет сам, даже без панели.
- **Обновление узлов в один клик:** на странице «Узлы» видно, что вышла новая версия узла (она включает Xray); «Обновить узел» просит узел скачать подписанный релиз, проверить подпись и перезапуститься. Настройка ключа подписи и выпуск релизов — в [README узла](https://github.com/dest1256/d256-node#readme).
- **Подписки** раздаёт только панель, адреса в них — адреса узлов.
- **Клиент → «i»:** графики трафика (1ч/24ч/7д/30д/свой период, по узлам и inbound'ам), активные подключения (IP, домен, порт, узел, inbound, байты, автообновление), домены и IP за период (сортировка, поиск, фильтр по узлу, CSV), устройства.
- **Импорт `x-ui.db`:** «Узлы → Импорт из x-ui.db» добавляет inbound'ы, клиентов и трафик из базы 3x-ui на выбранный узел. Существующее не заменяется; неподдерживаемые протоколы (MTProto, AmneziaWG, TUIC), дубликаты e-mail и занятые порты пропускаются с отчётом.

## Запуск
Нужен только Docker и уже работающий reverse proxy (например Caddy) в docker-сети `Caddy-net` (`docker network create Caddy-net`, если её ещё нет). Клонировать репозиторий не нужно: достаточно файла [docker-compose.yml](docker-compose.yml), панель собирается прямо с GitHub.
```
mkdir panel && cd panel
curl -O https://raw.githubusercontent.com/dest1256/d256-panel/main/docker-compose.yml
docker compose up -d --build
```
Рядом с compose-файлом появятся папки: `db` (база, логи, внутренний CA — **их нужно бэкапить**) и `cert` (необязательные TLS-сертификаты). Имя сети можно поменять переменной `PROXY_NETWORK` (файл `.env`, пример — `.env.example`).
В вашем Caddy добавьте (имя контейнера — `d256-panel`):
```
panel.example.com        { reverse_proxy d256-panel:2053 }
panel.example.com:2096   { reverse_proxy d256-panel:2096 }   # подписки
```
Порт `8444` (gRPC для узлов) панель публикует сама, откройте его в файрволе.
Логин и пароль администратора создаются при первом старте и показываются один раз: `docker compose logs d256-panel | grep Username`. Сменить: `docker compose exec d256-panel /app/d256-panel setting -username ... -password ...`.
Подключение узла — в [README узла](https://github.com/dest1256/d256-node#readme).

### Переменные окружения
| Переменная | По умолчанию | Назначение |
|---|---|---|
| `PROXY_NETWORK` | `Caddy-net` | docker-сеть вашего reverse proxy |
| `XUI_NODELINK_PORT` | `8444` | порт gRPC для узлов |
| `XUI_CONN_RETENTION_DAYS` | `30` | сколько хранить почасовые данные по доменам/IP до свёртки в дневные |
| `XUI_CONN_DAILY_RETENTION_DAYS` | `365` | сколько хранить дневные данные |
| `XUI_TRAFFIC_RETENTION_DAYS` | `90` | сколько хранить данные графиков трафика |
| `XUI_NODE_RELEASE_REPO` | `dest1256/d256-node` | репозиторий GitHub, в котором панель ищет новые версии узла |

## Обновление и бэкап
Обновление: `docker compose up -d --build` (подтянет свежий код с GitHub; для конкретной версии задайте `D256_PANEL_SRC=https://github.com/dest1256/d256-panel.git#v1.0.0`); миграции БД выполняются при старте.
Бэкап: папка `db` — вся постоянная информация, включая внутренний CA. Потеряв её, придётся заново регистрировать все узлы.

## Разработка и проверки (только Docker)
```
./dev make verify         # линтеры, форматирование, типы, тесты, сборка, govulncheck
./dev make proto          # перегенерировать gRPC-код
make smoke                # сквозной тест: панель + узел + клиент Xray
make integration          # docker compose: панель + 2 узла + 2 клиента, обрыв связи, досылка журнала
make update-e2e           # самообновление узла: чужая подпись отвергается, подписанный релиз ставится
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
- **One-click node updates:** the Nodes page shows when a newer node release (it includes Xray) exists; "Update node" asks the node to download the signed release, verify the signature and restart. Key setup and releases are described in the node README.
- **Subscriptions** are served by the panel only, with node addresses.
- **Client → "i":** traffic charts (1h/24h/7d/30d/custom, by node and inbound), live connections (IP, domain, port, node, inbound, bytes, auto-refresh), domains and IPs per period (sort, search, node filter, CSV), devices.
- **`x-ui.db` import:** *Nodes → Import from x-ui.db* adds the inbounds, clients and traffic of a 3x-ui database to the chosen node. Nothing existing is replaced; unsupported protocols (MTProto, AmneziaWG, TUIC), duplicate e-mails and busy ports are skipped and reported.

### Run
Docker and an existing reverse proxy (e.g. Caddy) on the docker network `Caddy-net` are required. No clone needed: only [docker-compose.yml](docker-compose.yml); the panel is built from GitHub.
```
mkdir panel && cd panel
curl -O https://raw.githubusercontent.com/dest1256/d256-panel/main/docker-compose.yml
docker compose up -d --build
```
Next to the compose file you get `db` (database, logs, the internal CA — **back it up**) and `cert` (optional TLS certificates). The network name is `PROXY_NETWORK` (`.env`, see `.env.example`).
In your Caddy: `panel.example.com { reverse_proxy d256-panel:2053 }` and `panel.example.com:2096 { reverse_proxy d256-panel:2096 }` (subscriptions). The node gRPC port `8444` is published by the panel itself (open it in your firewall). The admin login is generated on first start and shown once: `docker compose logs d256-panel | grep Username`; change it with `docker compose exec d256-panel /app/d256-panel setting -username ... -password ...`.
Environment: `PROXY_NETWORK`, `XUI_NODELINK_PORT` (8444), `XUI_CONN_RETENTION_DAYS` (30), `XUI_CONN_DAILY_RETENTION_DAYS` (365), `XUI_TRAFFIC_RETENTION_DAYS` (90).

### Update and backup
Update: `docker compose up -d --build` (pulls the latest source; pin a version with `D256_PANEL_SRC=https://github.com/dest1256/d256-panel.git#v1.0.0`); DB migrations run on start. Backup: the `db` folder holds everything including the internal CA; if it is lost, every node must be enrolled again.

### Development and checks (Docker only)
`./dev make verify`, `./dev make proto`, `make smoke` (panel + node + client), `make integration` (compose: panel + 2 nodes + 2 clients, outage and exactly-once redelivery). They use the images `d256-panel:local` / `d256-node:local` (`make image` in both repositories, side by side) and internet access.

### Security and limitations
- Nodes dial in; the panel only accepts certificates it issued and only the one whose serial is recorded for the node. Enrollment tokens are single-use and stored hashed.
- Everything from a node is validated (foreign clients/inbounds, bad names, unrealistic values, oversized messages are dropped). A node can only distort its **own** data.
- The node's cache and journal are encrypted with a key derived from the node's private key stored on the same disk: protection against tampering and casual reading, **not** against root on the node server — run nodes on servers you trust. There is no separate "protected mode".
- Live connections and devices are held in the panel's memory and are empty after a restart until the next node snapshot (seconds).
- Not supported: MTProto, AmneziaWG, TUIC; Telegram/Discord/e-mail/LDAP and WARP/Nord/PIA were removed.
