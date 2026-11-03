# d256 panel

**RU** · [EN](#english)

Веб-панель управления Xray-узлами. Форк [3x-ui](https://github.com/MHSanaei/3x-ui) v3.11.0 (MHSanaei и контрибьюторы), изменения © dest1256, лицензия GPL-3.0.
Интерфейс 3x-ui сохранён, но панель **не запускает Xray**: она единственный источник правды и единственная БД, а Xray работает на узлах
([d256-node](https://github.com/dest1256/d256-node)), которые сами подключаются к панели по gRPC + mTLS.

> Статус: разработка (этап 2 из 8). Архитектура: [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md).

## Разработка (только Docker)
На хосте ничего, кроме Docker, ставить не нужно.
```
./dev make verify     # линтеры + тесты + сборка (или `make verify`, если на хосте есть make)
./dev make proto      # перегенерировать gRPC-код
./dev bash            # оболочка в dev-контейнере
```

## Запуск
```
cp .env.example .env   # задайте PANEL_DOMAIN
docker compose up -d --build
```
Панель — за Caddy (TLS). gRPC-порт для узлов `8444` панель слушает сама.

## Обновление и бэкап
Обновление: `git pull && docker compose up -d --build`. Бэкап: том `panel-data` (`/data/x-ui.db`).

---

## English
Web control panel for Xray nodes. A fork of [3x-ui](https://github.com/MHSanaei/3x-ui) v3.11.0 (MHSanaei and contributors); changes © dest1256; GPL-3.0.
The 3x-ui UI is kept, but the panel **does not run Xray**: it is the single source of truth and the only database, while Xray runs on
[d256-node](https://github.com/dest1256/d256-node) instances that dial in over gRPC + mTLS.

> Status: work in progress (stage 2 of 8). Design: [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md).

**Development (Docker only):** `./dev make verify`, `./dev make proto`, `./dev bash`.
**Run:** `cp .env.example .env` (set `PANEL_DOMAIN`), then `docker compose up -d --build`. Caddy terminates TLS for the UI; the node gRPC port `8444` is served by the panel itself.
**Update:** `git pull && docker compose up -d --build`. **Backup:** the `panel-data` volume (`/data/x-ui.db`).
