# Changelog

## Unreleased
- Stage 4: nodes run Xray (embedded) from the desired state the panel renders; restart-free user/inbound updates; sequenced traffic batches applied exactly once; subscriptions use the node address.
- Removed from the panel: local Xray, the old HTTP multi-node, Telegram/Discord/email, LDAP, WARP/Nord/PIA, MTProto, AmneziaWG, sponsors.
- TUIC inbounds are temporarily rejected until the TUIC engine runs on nodes.
- Stage 3: node link. Internal CA, one-time enrollment tokens, gRPC + mTLS service (`:8444`), heartbeat, node metrics on the dashboard and Nodes page.
- UI languages reduced to ru-RU and en-US.
- Stage 2: repository skeleton on top of 3x-ui v3.11.0.
