# Changelog

## Unreleased
- docker-compose: builds from GitHub, data in ./db and ./cert next to the compose file, joins an external reverse-proxy network (no bundled Caddy).
- Stage 7-8: import of a 3x-ui x-ui.db into a node (Nodes -> Import); security tests (anonymous/restricted-token access, CSRF, SQL injection, CSV formulas, hostile uploads); docker compose integration test (panel + 2 nodes + 2 clients); RU/EN READMEs; the inert LDAP settings tab is removed. The protected node mode was dropped by decision.
- Stage 6: connection accounting. Per-destination usage (client x domain/IP x hour) is stored with retention and rollups; client info dialog shows traffic charts by node/inbound, live connections, domains and IPs (sort, search, node filter, CSV) and devices; device limit is enforced from the limits snapshot.
- Stage 5: node offline mode. The panel sends client limit snapshots and accepts batches with time spans; usage is journaled on the node and delivered exactly once after an outage.
- Per-node Xray templates (routing, outbounds, DNS) with fallback to the global template; template selector on the Xray page.
- TUIC removed from the product (Hysteria stays: it is an Xray protocol).
- Stage 4: nodes run Xray (embedded) from the desired state the panel renders; restart-free user/inbound updates; sequenced traffic batches applied exactly once; subscriptions use the node address.
- Removed from the panel: local Xray, the old HTTP multi-node, Telegram/Discord/email, LDAP, WARP/Nord/PIA, MTProto, AmneziaWG, sponsors.
- Stage 3: node link. Internal CA, one-time enrollment tokens, gRPC + mTLS service (`:8444`), heartbeat, node metrics on the dashboard and Nodes page.
- UI languages reduced to ru-RU and en-US.
- Stage 2: repository skeleton on top of 3x-ui v3.11.0.
