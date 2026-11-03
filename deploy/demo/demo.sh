#!/usr/bin/env bash
# Demo: starts a panel (http://localhost:2053), one node and an Xray client, creates a VLESS inbound
# with a client, pushes some traffic and leaves everything running.
# Needs images d256-panel:local and d256-node:local and outbound internet access.
#   bash deploy/demo/demo.sh          start
#   bash deploy/demo/demo.sh stop     remove containers and volumes
set -euo pipefail
export MSYS_NO_PATHCONV=1
NET=d256demo
PANEL=d256demo-panel
NODE=d256demo-node
CLIENT=d256demo-client
UUID=6b0b0e3e-41b2-4f1c-9a16-0d5c1c1f3a11
C="docker run --rm --network $NET curlimages/curl -s"

stop() {
  docker rm -f $PANEL $NODE $CLIENT d256demo-hold >/dev/null 2>&1 || true
  docker volume rm -f $PANEL-data $NODE-data >/dev/null 2>&1 || true
  docker network rm $NET >/dev/null 2>&1 || true
}
if [ "${1:-}" = stop ]; then stop; echo "demo removed"; exit 0; fi
stop
docker network create $NET >/dev/null

docker run -d --name $PANEL --network $NET -p 2053:2053 -p 2096:2096 -p 8444:8444 -v $PANEL-data:/data d256-panel:local >/dev/null
TOK=""
for _ in $(seq 1 30); do
  TOK=$(docker exec $PANEL /app/d256-panel setting -getApiToken 2>/dev/null | awk '/apiToken:/{print $2}') && [ -n "$TOK" ] && break
  sleep 1
done
API="http://$PANEL:2053/panel/api"
json() { $C -H "Authorization: Bearer $TOK" -H 'Content-Type: application/json' "$@"; }
quote() { printf '%s' "$1" | sed 's/"/\\"/g; s/^/"/; s/$/"/'; }

# A known admin login for the demo (a fresh panel generates a random one).
docker exec $PANEL /app/d256-panel setting -username admin -password demo-admin-256 >/dev/null

RES=$(json -X POST -d '{"name":"demo-node","address":"'$NODE'"}' $API/nodes/create)
NODE_ID=$(echo "$RES" | sed 's/.*"nodeId":\([0-9]*\).*/\1/')
ENR=$(echo "$RES" | sed 's/.*"token":"\([^"]*\)".*/\1/')
docker run --rm --network $NET -v $NODE-data:/var/lib/d256-node d256-node:local enroll --allow-insecure-http http://$PANEL:2053 "$ENR"
docker run -d --name $NODE --network $NET -v $NODE-data:/var/lib/d256-node d256-node:local run >/dev/null

SETTINGS='{"clients":[{"id":"'$UUID'","email":"alice@demo","flow":"","enable":true,"limitIp":0,"totalGB":0,"expiryTime":0,"tgId":"","subId":"demosub1","reset":0}],"decryption":"none","fallbacks":[]}'
STREAM='{"network":"tcp","security":"none","tcpSettings":{"acceptProxyProtocol":false,"header":{"type":"none"}}}'
SNIFF='{"enabled":true,"destOverride":["http","tls"],"metadataOnly":false,"routeOnly":false}'
BODY=$(printf '{"up":0,"down":0,"total":0,"remark":"demo","enable":true,"expiryTime":0,"listen":"","port":20443,"protocol":"vless","settings":%s,"streamSettings":%s,"sniffing":%s,"nodeId":%s}' \
  "$(quote "$SETTINGS")" "$(quote "$STREAM")" "$(quote "$SNIFF")" "$NODE_ID")
json -X POST -d "$BODY" $API/inbounds/add >/dev/null

for _ in $(seq 1 40); do
  json $API/nodes/list | grep -q '"xrayState":"running"' && break
  sleep 1
done

CFG=./.demo-client.json
cat > "$CFG" <<EOF
{"log":{"loglevel":"warning"},
 "inbounds":[{"listen":"0.0.0.0","port":1080,"protocol":"socks","settings":{"udp":false}}],
 "outbounds":[{"protocol":"vless","settings":{"vnext":[{"address":"$NODE","port":20443,"users":[{"id":"$UUID","encryption":"none"}]}]},"streamSettings":{"network":"tcp"}}]}
EOF
docker create --name $CLIENT --network $NET teddysun/xray >/dev/null
docker cp "$CFG" $CLIENT:/etc/xray/config.json
rm -f "$CFG"
docker start $CLIENT >/dev/null
sleep 3
for site in example.com example.org example.net; do
  for _ in 1 2; do
    docker run --rm --network $NET curlimages/curl -s -o /dev/null --max-time 20 -x socks5h://$CLIENT:1080 http://$site/ || true
  done
done
# One connection stays open so that it shows up under "Active connections".
docker run -d --name d256demo-hold --network $NET curlimages/curl sh -c "sleep 1200 | curl -s -T - --max-time 1200 -x socks5h://$CLIENT:1080 telnet://example.com:80" >/dev/null
sleep 8

echo
echo "Panel:    http://localhost:2053/   login: admin / demo-admin-256"
echo "Node:     demo-node (online), VLESS inbound on :20443, client alice@demo"
echo "Open Clients -> alice@demo -> (i) -> Connections and traffic."
echo "Subscription: open Clients -> alice@demo -> subscription link (served on :2096)"
echo "Stop:     bash deploy/demo/demo.sh stop"
