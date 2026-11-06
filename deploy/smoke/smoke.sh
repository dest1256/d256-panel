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
  docker rm -f $PANEL $NODE $CLIENT d256smoke-hold >/dev/null 2>&1 || true
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
# example.com does not answer with exactly the same number of bytes every time, so compare with a tolerance
# that is far below one request (about 1 KB): a lost or doubled delivery would still be caught.
near() { local d=$(( $1 - $2 )); [ "${d#-}" -le "${3:-60}" ]; }
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

echo "== client insights: destinations, live connections, devices, traffic series"
HOLD=d256smoke-hold
docker run -d --name $HOLD --network $NET curlimages/curl sh -c "sleep 25 | curl -s -T - --max-time 25 -x socks5h://$CLIENT:1080 telnet://example.com:80" >/dev/null
insights() { json -G --data-urlencode "email=alice@smoke" "$@"; }
DEST=""
for i in $(seq 1 20); do
  DEST=$(insights $API/clientInsights/destinations --data-urlencode period=1h)
  echo "$DEST" | grep -q '"dest":"example.com"' && break
  sleep 1
done
echo "$DEST" | head -c 300; echo
echo "$DEST" | grep -q '"dest":"example.com"' || { echo "DESTINATIONS FAILED"; docker logs $NODE 2>&1 | tail -10; exit 1; }
echo "$DEST" | grep -qE '"down":[1-9]' || { echo "DESTINATIONS HAVE NO BYTES"; exit 1; }
ACT=""
for i in $(seq 1 20); do
  ACT=$(insights $API/clientInsights/connections)
  echo "$ACT" | grep -q '"dest":"example.com"' && break
  sleep 1
done
echo "$ACT" | head -c 300; echo
echo "$ACT" | grep -q '"dest":"example.com"' || { echo "ACTIVE CONNECTIONS FAILED"; exit 1; }
echo "$ACT" | grep -q '"node":"smoke-1"' || { echo "ACTIVE CONNECTION HAS NO NODE NAME"; exit 1; }
DEV=$(insights $API/clientInsights/devices)
echo "$DEV" | head -c 200; echo
echo "$DEV" | grep -qE '"ip":"[0-9a-f.:]+"' || { echo "DEVICES FAILED"; exit 1; }
TR=$(insights $API/clientInsights/traffic --data-urlencode period=1h)
echo "$TR" | head -c 300; echo
echo "$TR" | grep -q '"name":"smoke-1"' || { echo "TRAFFIC SERIES FAILED"; exit 1; }
CSV=$(insights $API/clientInsights/destinations --data-urlencode period=1h --data-urlencode format=csv)
echo "$CSV" | head -3
echo "$CSV" | grep -q '^destination,network,node' || { echo "CSV FAILED"; exit 1; }
docker rm -f $HOLD >/dev/null 2>&1 || true

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

echo "== the panel goes away: traffic keeps flowing and is journaled on the node"
traffic_of() { json $API/inbounds/list | grep -o '"email":"alice@smoke"[^}]*' | grep -o '"\(up\|down\)":[0-9]*' | tr -d '"updown:' | paste -sd+ | bc 2>/dev/null || echo 0; }
sum_traffic() { json $API/inbounds/list | grep -o '"email":"alice@smoke"[^}]*' | grep -oE '"(up|down)":[0-9]+' | cut -d: -f2 | awk '{s+=$1} END{print s+0}'; }
sleep 7
BEFORE=$(sum_traffic)
PER_REQUEST=$((BEFORE / 3))
[ "$PER_REQUEST" -gt 0 ] || { echo "unexpected traffic before the outage: $BEFORE"; exit 1; }
echo "bytes per request: $PER_REQUEST (3 requests so far)"
docker stop $PANEL >/dev/null
sleep 3
for n in 1 2 3; do
  docker run --rm --network $NET curlimages/curl -s -o /dev/null -w "offline request: HTTP %{http_code}
" --max-time 20 -x socks5h://$CLIENT:1080 http://example.com/
done
sleep 7
docker exec $NODE /usr/local/bin/d256-node status | grep -E "panel link|journal"
JBYTES=$(docker exec $NODE /usr/local/bin/d256-node status | awk '/journal:/{print $2}')
[ "${JBYTES:-0}" -gt 0 ] || { echo "JOURNAL EMPTY: usage was not written to disk while offline"; exit 1; }

echo "== the panel is back: the journal is delivered exactly once"
docker start $PANEL >/dev/null
for i in $(seq 1 40); do
  AFTER=$(sum_traffic 2>/dev/null || echo 0)
  [ "${AFTER:-0}" -gt "$BEFORE" ] && break
  sleep 1
done
sleep 12
FINAL=$(sum_traffic)
echo "alice total: before=$BEFORE after-reconnect=$AFTER final=$FINAL"
[ "$AFTER" -gt "$BEFORE" ] || { echo "OFFLINE DELIVERY FAILED: nothing arrived"; docker logs $NODE 2>&1 | tail -10; exit 1; }
# six requests went through in total (2 + 1 + 3 offline): nothing lost, nothing counted twice
EXPECTED=$((6 * PER_REQUEST))
near "$FINAL" "$EXPECTED" || { echo "ACCOUNTING MISMATCH: got $FINAL, expected $EXPECTED"; exit 1; }
docker exec $NODE /usr/local/bin/d256-node status | grep -E "journal"

echo "== usage is attributed to the device that fetched the subscription from the same address"
HW=smoke-device-0001
HW8=$(printf '%s' "$HW" | sha256sum | cut -c1-8)
docker exec $CLIENT wget -q -O /dev/null --header="X-HWID: $HW" --header="X-Device-Model: SmokePhone" "http://$PANEL:2096${SUBPATH}smokesub1" || { echo "DEVICE FETCH FAILED"; exit 1; }
for n in 1 2; do
  docker run --rm --network $NET curlimages/curl -s -o /dev/null --max-time 20 -x socks5h://$CLIENT:1080 http://example.com/ || true
done
DEVS=""
for i in $(seq 1 30); do
  DEVS=$(json -G --data-urlencode "email=alice@smoke" --data-urlencode period=1h $API/clientInsights/destinations)
  echo "$DEVS" | grep -q '"short":"'$HW8'"' && break
  sleep 1
done
echo "$DEVS" | head -c 500; echo
echo "$DEVS" | grep -q '"short":"'$HW8'"' || { echo "DEVICE ATTRIBUTION FAILED"; exit 1; }
echo "$DEVS" | grep -q '"label":"SmokePhone"' || { echo "DEVICE LABEL MISSING"; exit 1; }
echo "$DEVS" | grep -qE '"lastSeen":[1-9][0-9]{9}' || { echo "LAST USE MISSING"; exit 1; }
json -G --data-urlencode "email=alice@smoke" $API/clientInsights/hwids | grep -q '"lastIp":"[0-9a-f.:]\+"' || { echo "DEVICE LIST FAILED"; exit 1; }

echo "SMOKE OK"
