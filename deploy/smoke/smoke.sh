#!/usr/bin/env bash
# End-to-end smoke test: panel + node + an Xray client pushing real traffic through the node.
# Needs images d256-panel:local and d256-node:local (make image) and outbound internet access.
set -euo pipefail
export MSYS_NO_PATHCONV=1
NET=d256smoke
PANEL=d256smoke-panel
NODE=d256smoke-node
CLIENT=d256smoke-client
UUID=6b0b0e3e-41b2-4f1c-9a16-0d5c1c1f3a11
UUID2=0c1c4b34-7d68-4d6a-8d5e-3f7a1a1b9c22
PORT=20443
C="docker run --rm --network $NET curlimages/curl -s"
CFG=./.smoke-client.json

cleanup() {
  rm -f $CFG
  docker rm -f $PANEL $NODE $CLIENT >/dev/null 2>&1 || true
  docker volume rm -f $PANEL-data $NODE-data >/dev/null 2>&1 || true
  docker network rm $NET >/dev/null 2>&1 || true
}
trap cleanup EXIT
cleanup
docker network create $NET >/dev/null

docker run -d --name $PANEL --network $NET -v $PANEL-data:/data d256-panel:local >/dev/null
TOK=""
for i in $(seq 1 30); do
  TOK=$(docker exec $PANEL /app/d256-panel setting -getApiToken 2>/dev/null | awk '/apiToken:/{print $2}') && [ -n "$TOK" ] && break
  sleep 1
done
API="http://$PANEL:2053/panel/api"
AUTH="Authorization: Bearer $TOK"
json() { $C -H "$AUTH" -H 'Content-Type: application/json' "$@"; }
quote() { printf '%s' "$1" | sed 's/"/\\"/g; s/^/"/; s/$/"/'; }

echo "== create node"
RES=$(json -X POST -d '{"name":"smoke-1","address":"'$NODE'"}' $API/nodes/create)
NODE_ID=$(echo "$RES" | sed 's/.*"nodeId":\([0-9]*\).*/\1/')
ENR=$(echo "$RES" | sed 's/.*"token":"\([^"]*\)".*/\1/')

echo "== enroll + run node"
docker run --rm --network $NET -v $NODE-data:/var/lib/d256-node d256-node:local enroll --allow-insecure-http http://$PANEL:2053 "$ENR"
docker run -d --name $NODE --network $NET -v $NODE-data:/var/lib/d256-node d256-node:local run >/dev/null

echo "== create inbound + client on the node"
SETTINGS='{"clients":[{"id":"'$UUID'","email":"alice@smoke","flow":"","enable":true,"limitIp":0,"totalGB":0,"expiryTime":0,"tgId":"","subId":"smokesub1","reset":0}],"decryption":"none","fallbacks":[]}'
STREAM='{"network":"tcp","security":"none","tcpSettings":{"acceptProxyProtocol":false,"header":{"type":"none"}}}'
SNIFF='{"enabled":true,"destOverride":["http","tls"],"metadataOnly":false,"routeOnly":false}'
BODY=$(printf '{"up":0,"down":0,"total":0,"remark":"smoke","enable":true,"expiryTime":0,"listen":"","port":%s,"protocol":"vless","settings":%s,"streamSettings":%s,"sniffing":%s,"nodeId":%s}' \
  $PORT "$(quote "$SETTINGS")" "$(quote "$STREAM")" "$(quote "$SNIFF")" $NODE_ID)
json -X POST -d "$BODY" $API/inbounds/add | head -c 160; echo

echo "== wait for the node to apply the config"
L=""
for i in $(seq 1 30); do
  L=$(json $API/nodes/list)
  if echo "$L" | grep -q '"appliedVersion":[1-9]' && echo "$L" | grep -q '"xrayState":"running"'; then break; fi
  sleep 1
done
echo "$L" | head -c 600; echo

echo "== client traffic through the node"
cat > $CFG <<EOF
{"log":{"loglevel":"warning"},
 "inbounds":[{"listen":"0.0.0.0","port":1080,"protocol":"socks","settings":{"udp":false}}],
 "outbounds":[{"protocol":"vless","settings":{"vnext":[{"address":"$NODE","port":$PORT,"users":[{"id":"$UUID","encryption":"none"}]}]},"streamSettings":{"network":"tcp"}}]}
EOF
docker create --name $CLIENT --network $NET teddysun/xray >/dev/null
docker cp $CFG $CLIENT:/etc/xray/config.json
docker start $CLIENT >/dev/null
sleep 3
for n in 1 2; do
  docker run --rm --network $NET curlimages/curl -s -o /dev/null -w "HTTP %{http_code}, %{size_download} bytes\n" --max-time 20 -x socks5h://$CLIENT:1080 http://example.com/
done

echo "== panel accounting (waits for the next 5s batch)"
T=""
for i in $(seq 1 20); do
  T=$(json $API/inbounds/list)
  if echo "$T" | grep -q '"email":"alice@smoke"' && echo "$T" | grep -qE '"up":[1-9]'; then break; fi
  sleep 1
done
echo "$T" | grep -o '"clientStats":\[[^]]*\]' | head -c 400; echo
echo "$T" | grep -qE '"(up|down)":[1-9][0-9]*' || { echo "ACCOUNTING FAILED"; docker logs $NODE 2>&1 | tail -20; exit 1; }

echo "== subscription uses the node address"
SUBPATH=$(json -X POST $API/setting/all | grep -o '"subPath":"[^"]*"' | cut -d'"' -f4)
RAW=$($C http://$PANEL:2096${SUBPATH}smokesub1)
SUB=$(echo "$RAW" | base64 -d 2>/dev/null || echo "$RAW")
echo "$SUB" | head -c 300; echo
echo "$SUB" | grep -q "@$NODE:$PORT" || { echo "SUB FAILED: expected the node host in the link"; exit 1; }

echo "== adding a client is applied without restarting Xray"
STARTS_BEFORE=$(docker logs $NODE 2>&1 | grep -c "xray started" || true)
json -X POST -d '{"client":{"email":"bob@smoke","id":"'$UUID2'","enable":true,"subId":"smokesub2"},"inboundIds":[1]}' $API/clients/add | head -c 200; echo
sleep 6
STARTS_AFTER=$(docker logs $NODE 2>&1 | grep -c "xray started" || true)
echo "xray starts before/after: $STARTS_BEFORE/$STARTS_AFTER"
docker logs $NODE 2>&1 | grep "desired state applied" | tail -3
[ "$STARTS_BEFORE" = "$STARTS_AFTER" ] || { echo "HOT UPDATE FAILED: xray was restarted"; exit 1; }
docker run --rm --network $NET curlimages/curl -s -o /dev/null -w "alice still connected: HTTP %{http_code}\n" --max-time 20 -x socks5h://$CLIENT:1080 http://example.com/

echo "SMOKE OK"
