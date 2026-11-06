# Как подключить узел к панели

**RU** · [EN](#english)

Узел сам подключается к панели (исходящее соединение gRPC + mTLS). Подключение — это регистрация по одноразовому токену, после которой узел получает сертификат и держит связь с панелью.

## 0. Что должно быть готово
| | Панель | Узел |
|---|---|---|
| Docker + compose | да | да |
| Домен | `panel.example.com` → IP сервера панели (без «оранжевого облака» Cloudflare: порт 8444 через прокси не пройдёт) | публичный IP или домен — он попадёт в подписки клиентов |
| Открытые порты | 443 (веб), **8444/tcp** (связь с узлами), 2096 (подписки, если нужны) | порты инбаундов (задаются в панели) |
| Исходящий доступ | — | на `panel.example.com:443` (регистрация) и `:8444` (постоянная связь) |

Панель уже запущена (см. [README](../README.md)), и вы можете войти в неё по `https://panel.example.com/<путь>/`.

## 1. Создайте узел в панели
1. **Узлы → Добавить**.
2. Имя (например `de-1`) и **публичный адрес узла** — домен или IP сервера, где будет работать узел. Он подставляется в ссылки подписки клиентов.
3. Нажмите «Создать и получить токен» и скопируйте токен. Он показывается один раз и действует 1 час. Узел в списке получит статус «ожидает регистрации».

## 2. Зарегистрируйте и запустите узел
На сервере узла (только Docker, клонировать ничего не нужно):
```
mkdir node && cd node
curl -O https://raw.githubusercontent.com/dest1256/d256-node/main/docker-compose.yml
echo 'ТОКЕН' | docker compose run --rm -T d256-node enroll https://panel.example.com/<путь-панели>/ -
docker compose up -d
```
Правила для адреса в `enroll`:
- Берите **публичный адрес панели из браузера**, вместе с веб-путём (`webBasePath`), если он не `/`. Например `https://panel.example.com/mjhdftghjdhjdcfhmxcdis/`.
- Именно этот домен узел потом использует для постоянной связи (`panel.example.com:8444`), поэтому он должен указывать прямо на сервер панели.
- Токен передавайте через `echo ... |` и `-`, а не аргументом: так он не попадёт в историю команд и `ps`.

В ответ вы увидите `Enrolled as node N. Panel link: panel.example.com:8444`. Ключ узла создан на самом узле и никуда не передавался; рядом с compose-файлом появилась папка `data`.

## 3. Проверьте подключение
- В панели узел должен стать «в сети», появятся версия узла и Xray, задержка и метрики сервера.
- На узле: `docker compose logs -f d256-node` (в норме нет повторяющихся `link down`) и `docker compose exec d256-node d256-node status`.

## 4. Используйте узел
1. **Инбаунды → Добавить** → выберите этот узел, протокол и порт.
2. Добавьте клиента в инбаунд или привяжите существующего (**Клиенты → изменить → входящие**).
3. Откройте порт инбаунда в файрволе узла.
4. Ссылку подписки берите в карточке клиента (кнопка «i»).

## Если что-то не так
| Симптом | Причина и решение |
|---|---|
| `enroll`: отказ с кодом 403 (токен не подходит) | Токен уже использован, старше часа или скопирован не полностью. В панели у узла «Новый токен». |
| `enroll`: «too many attempts» (429) | Слишком много неудачных попыток с одного адреса. Подождите и повторите с верным токеном. |
| `enroll`: `refusing plain http` | Адрес панели указан как `http://`. Используйте `https://`. |
| `enroll`: 404 или не тот ответ | Не указан веб-путь панели (`/<путь>/`) или он указан неточно. |
| Узел зарегистрирован, но «не в сети»; в логах `link down ... connection refused / i/o timeout` | Порт 8444 закрыт на сервере панели или домен идёт через прокси CDN. Проверка с узла: `nc -vz panel.example.com 8444`. |
| В логах `certificate` / `unknown authority` | Узел зарегистрирован в другой панели или панель пересоздана (потеряна папка `db`). Удалите `data` на узле, создайте узел заново и зарегистрируйте новым токеном. |
| Подписка отвечает 404 | У клиента нет ни одного включённого инбаунда (шаг 4). |

## Перерегистрация, замена и удаление
- **Новый токен** (узел → меню): старый сертификат перестаёт приниматься, узел отключается, пока его не зарегистрируют заново (`rm -rf data`, затем `enroll`).
- **Удаление узла:** если на нём есть инбаунды, отметьте «Также удалить инбаунды этого узла и их клиентов» или сначала удалите/перенесите инбаунды.
- **Бэкап узла не нужен:** при потере папки `data` его регистрируют заново. Недоставленный офлайн-журнал при этом теряется.

---

## English
The node dials the panel (outbound gRPC + mTLS). Connecting = enrolling with a one-time token; the node then gets a certificate and keeps the link up.

**Prerequisites.** Docker on both servers. The panel domain must point straight at the panel server (no CDN proxy: port 8444 will not pass). Open on the panel: 443, **8444/tcp**, 2096 (subscriptions). The node needs outbound access to `panel.example.com:443` and `:8444`; open the inbound ports you configure in the panel on the node.

1. **Panel:** *Nodes → Add*, name and the node's **public address** (used in client subscriptions) → copy the one-time token (shown once, valid 1 hour).
2. **Node server:**
   ```
   mkdir node && cd node
   curl -O https://raw.githubusercontent.com/dest1256/d256-node/main/docker-compose.yml
   echo 'TOKEN' | docker compose run --rm -T d256-node enroll https://panel.example.com/<panel-base-path>/ -
   docker compose up -d
   ```
   Use the panel's public URL **including its web base path** (it is also the host the node later dials on `:8444`). Pass the token via stdin, not as an argument.
3. **Check:** the node turns "online" in the panel; `docker compose logs -f d256-node`, `docker compose exec d256-node d256-node status`.
4. **Use it:** *Inbounds → Add* on this node, attach clients, open the inbound port on the node, take the subscription link from the client card.

Troubleshooting: an expired/used token → *New token* on the node; `refusing plain http` → use `https://`; enroll 404 → wrong base path; "offline" with `connection refused/timeout` → port 8444 is closed or proxied by a CDN (`nc -vz panel.example.com 8444`); certificate errors → the panel was recreated, delete `data` on the node and enroll again with a new token; subscription 404 → the client has no enabled inbound.
