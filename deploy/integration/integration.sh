#!/usr/bin/env bash
# Integration test: one panel, two nodes, two Xray clients (docker compose).
# Checks: enrollment of both nodes, per-node isolation of clients and usage, per-destination accounting
# with the node name, a panel outage with journaling on both nodes, and exactly-once redelivery
# (no loss, no duplicates) afterwards.
# Needs images d256-panel:local and d256-node:local and outbound internet access.
set -euo pipefail
export MSYS_NO_PATHCONV=1
cd "$(dirname "$0")"
DC="docker compose -f docker-compose.yml"
NET=d256it_default
UUID1=6b0b0e3e-41b2-4f1c-9a16-0d5c1c1f3a11
UUID2=0c1c4b34-7d68-4d6a-8d5e-3f7a1a1b9c22
C="docker run --rm --network $NET curlimages/curl -s"

cleanup() { $DC --profile nodes --profile clients down -v --remove-orphans >/dev/null 2>&1 || true; }
trap cleanup EXIT
cleanup

$DC up -d panel >/dev/null
TOK=""
for _ in $(seq 1 30); do
  TOK=$($DC exec -T panel /app/d256-panel setting -getApiToken 2>/dev/null | awk '/apiToken:/{print $2}') && [ -n "$TOK" ] && break
  sleep 1
done
[ -n "$TOK" ] || { echo "panel did not start"; exit 1; }
API="http://panel:2053/panel/api"
json() { $C -H "Authorization: Bearer $TOK" -H 'Content-Type: application/json' "$@"; }
# example.com does not answer with exactly the same number of bytes every time, so compare with a tolerance
# that is far below one request (about 1 KB): a lost or doubled delivery would still be caught.
near() { local d=$(( $1 - $2 )); [ "${d#-}" -le "${3:-60}" ]; }
quote() { printf '%s' "$1" | sed 's/"/\\"/g; s/^/"/; s/$/"/'; }
fail() { echo "FAIL: $*"; $DC logs --tail 15 2>&1 | tail -45; exit 1; }

add_node() { # name -> prints "id token"
  local res
  res=$(json -X POST -d '{"name":"'$1'","address":"'$1'"}' $API/nodes/create)
  echo "$(echo "$res" | sed 's/.*"nodeId":\([0-9]*\).*/\1/') $(echo "$res" | sed 's/.*"token":"\([^"]*\)".*/\1/')"
}

echo "== two nodes enroll"
read -r ID1 TOK1 <<<"$(add_node node1)"
read -r ID2 TOK2 <<<"$(add_node node2)"
$DC --profile nodes run --rm --no-deps node1 enroll --allow-insecure-http http://panel:2053 "$TOK1" | tail -2
$DC --profile nodes run --rm --no-deps node2 enroll --allow-insecure-http http://panel:2053 "$TOK2" | tail -2

echo "== a spent enrollment token is refused"
if $DC --profile nodes run --rm --no-deps node2 enroll --allow-insecure-http http://panel:2053 "$TOK2" >/dev/null 2>&1; then
  fail "a one-time token was accepted twice"
fi
$DC --profile nodes up -d node1 node2 >/dev/null

add_inbound() { # nodeId port uuid email subid
  local settings stream sniff body
  settings='{"clients":[{"id":"'$3'","email":"'$4'","flow":"","enable":true,"limitIp":0,"totalGB":0,"expiryTime":0,"tgId":"","subId":"'$5'","reset":0}],"decryption":"none","fallbacks":[]}'
  stream='{"network":"tcp","security":"none","tcpSettings":{"acceptProxyProtocol":false,"header":{"type":"none"}}}'
  sniff='{"enabled":true,"destOverride":["http","tls"],"metadataOnly":false,"routeOnly":false}'
  body=$(printf '{"up":0,"down":0,"total":0,"remark":"it-%s","enable":true,"expiryTime":0,"listen":"","port":%s,"protocol":"vless","settings":%s,"streamSettings":%s,"sniffing":%s,"nodeId":%s}' \
    "$1" "$2" "$(quote "$settings")" "$(quote "$stream")" "$(quote "$sniff")" "$1")
  json -X POST -d "$body" $API/inbounds/add | grep -q '"success":true' || fail "inbound for node $1 was not created"
}
echo "== one inbound and one client per node"
add_inbound $ID1 20443 $UUID1 alice@it itsub1
add_inbound $ID2 20444 $UUID2 bob@it itsub2

for _ in $(seq 1 40); do
  L=$(json $API/nodes/list)
  [ "$(echo "$L" | grep -o '"xrayState":"running"' | wc -l)" = 2 ] && break
  sleep 1
done
[ "$(echo "$L" | grep -o '"xrayState":"running"' | wc -l)" = 2 ] || fail "both nodes must run Xray"
echo "both nodes are running Xray"

$DC --profile clients up -d client1 client2 >/dev/null
sleep 3
req() { $C -o /dev/null -w "%{http_code}" --max-time 20 -x socks5h://$1:1080 http://example.com/; }

total_of() { # email -> up+down on the panel
  json $API/inbounds/list | grep -o '"email":"'$1'"[^}]*' | grep -oE '"(up|down)":[0-9]+' | cut -d: -f2 | awk '{s+=$1} END{print s+0}'
}

echo "== traffic through each node is billed to its own client"
for n in 1 2; do for _ in 1 2; do [ "$(req client$n)" = 200 ] || fail "request via client$n"; done; done
A=0; B=0
for _ in $(seq 1 30); do
  A=$(total_of alice@it); B=$(total_of bob@it)
  [ "$A" -gt 0 ] && [ "$B" -gt 0 ] && break
  sleep 1
done
sleep 6
A=$(total_of alice@it); B=$(total_of bob@it)
PER=$((A / 2))
echo "alice=$A bob=$B bytes per request=$PER"
[ "$PER" -gt 0 ] && near "$A" "$B" && near $((PER * 2)) "$A" || fail "accounting before the outage is wrong"

echo "== per-destination accounting names the node"
for who in "alice@it node1" "bob@it node2"; do
  set -- $who
  D=$(json -G --data-urlencode "email=$1" --data-urlencode period=1h $API/clientInsights/destinations)
  echo "$D" | grep -q '"dest":"example.com"' && echo "$D" | grep -q '"node":"'$2'"' || fail "destinations of $1: $D"
done

echo "== the panel goes away; both nodes keep serving and journal usage"
$DC stop panel >/dev/null
sleep 3
for n in 1 2; do for _ in 1 2 3; do [ "$(req client$n)" = 200 ] || fail "offline request via client$n"; done; done
sleep 8
for n in 1 2; do
  J=$($DC --profile nodes exec -T node$n /usr/local/bin/d256-node status | awk '/journal:/{print $2}')
  echo "node$n journal: ${J:-0} bytes"
  [ "${J:-0}" -gt 0 ] || fail "node$n did not journal"
done

echo "== the panel is back; every journal is delivered exactly once"
$DC start panel >/dev/null
sleep 5
for _ in $(seq 1 60); do
  A=$(total_of alice@it 2>/dev/null || echo 0); B=$(total_of bob@it 2>/dev/null || echo 0)
  near "$A" $((PER * 5)) && near "$B" $((PER * 5)) && break
  sleep 1
done
sleep 12 # a duplicate delivery, if any, would show up now
A=$(total_of alice@it); B=$(total_of bob@it)
echo "alice=$A bob=$B expected=$((PER * 5)) each"
near "$A" $((PER * 5)) && near "$B" $((PER * 5)) || fail "usage was lost or counted twice"

echo "INTEGRATION OK"
