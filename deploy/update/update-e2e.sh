#!/usr/bin/env bash
# End-to-end test of the node self-update, with a throw-away signing key and a local release server:
#   - a node built as v0.0.1 (its update source points at the local server) connects to the panel
#   - the panel asks it to update to a release signed with a foreign key -> refused, node unchanged
#   - the panel asks it to update to a properly signed v0.0.2 -> the node installs it, restarts into it,
#     reports v0.0.2 and Xray runs the panel's config again; it also survives a container restart
# Needs the repositories d256-panel (this one) and d256-node side by side, the image d256-panel:local,
# and outbound internet access (golang/nginx/curl images). Nothing is written outside ./.update-e2e.
set -euo pipefail
export MSYS_NO_PATHCONV=1
cd "$(dirname "$0")/../.."
PANEL_DIR=$(pwd)
NODE_SRC=${NODE_SRC:-../d256-node}
W=./.update-e2e
NET=d256upd
PANEL=d256upd-panel
NODE=d256upd-node
REL=d256upd-rel
UUID=6b0b0e3e-41b2-4f1c-9a16-0d5c1c1f3a11
C="docker run --rm --network $NET curlimages/curl -s"

cleanup() {
  docker rm -f $PANEL $NODE $REL >/dev/null 2>&1 || true
  docker volume rm -f $PANEL-data $NODE-data >/dev/null 2>&1 || true
  docker network rm $NET >/dev/null 2>&1 || true
  rm -rf $W
}
trap cleanup EXIT
cleanup
mkdir -p $W/src/d256-node $W/rel
fail() { echo "FAIL: $*"; docker logs --tail 25 $NODE 2>&1 | tail -30; exit 1; }

echo "== prepare a node source tree with a throw-away signing key"
(cd "$NODE_SRC" && tar --exclude=.git --exclude=build --exclude=data -cf - .) | tar -xf - -C $W/src/d256-node
mkdir -p $W/src/d256-panel && cp -r proto $W/src/d256-panel/proto
GO="docker run --rm -v $PANEL_DIR/$W/src:/src -w /src/d256-node -e GOFLAGS=-buildvcs=false -v d256-cache:/cache -e GOCACHE=/cache/go-build -e GOMODCACHE=/cache/go-mod golang:1.27"
GOOD=$($GO go run ./cmd/d256-sign keygen 2>/dev/null)
cp $W/src/d256-node/internal/update/signing.pub $W/good.pub
BAD=$($GO go run ./cmd/d256-sign keygen 2>/dev/null)
cp $W/good.pub $W/src/d256-node/internal/update/signing.pub
[ -n "$GOOD" ] && [ -n "$BAD" ] && [ "$GOOD" != "$BAD" ] || fail "key generation"

echo "== build the signed releases (v0.0.2 good, v0.0.3 signed with a foreign key)"
for v in v0.0.2 v0.0.3; do
  d=$W/rel/dest1256/d256-node/releases/download/$v
  mkdir -p $d
  $GO sh -c "CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags '-w -s -X github.com/dest1256/d256-node/internal/version.Version=$v' -o /src/../src/.rel-$v ./cmd/d256-node"
  mv $W/src/.rel-$v $d/d256-node_linux_amd64
  key=$GOOD; [ $v = v0.0.3 ] && key=$BAD
  docker run --rm -v $PANEL_DIR/$W/src:/src -v $PANEL_DIR/$d:/rel -w /src/d256-node -e D256_SIGNING_KEY="$key" \
    -v d256-cache:/cache -e GOCACHE=/cache/go-build -e GOMODCACHE=/cache/go-mod golang:1.27 \
    go run ./cmd/d256-sign sign $v linux amd64 /rel/d256-node_linux_amd64
done
docker network create $NET >/dev/null
docker create --name $REL --network $NET --network-alias relsrv nginx:alpine >/dev/null
docker cp $W/rel/dest1256 $REL:/usr/share/nginx/html/
docker start $REL >/dev/null

echo "== build the v0.0.1 node image (embeds the throw-away public key and the local release server)"
(cd $W/src/d256-node && docker build -q -t d256-node:e2e --build-arg VERSION=v0.0.1 --build-arg UPDATE_BASE_URL=http://relsrv --build-context panel=../d256-panel . >/dev/null)

echo "== panel + node"
docker run -d --name $PANEL --network $NET -v $PANEL-data:/data d256-panel:local >/dev/null
TOK=""
for _ in $(seq 1 30); do
  TOK=$(docker exec $PANEL /app/d256-panel setting -getApiToken 2>/dev/null | awk '/apiToken:/{print $2}') && [ -n "$TOK" ] && break
  sleep 1
done
API="http://$PANEL:2053/panel/api"
json() { $C -H "Authorization: Bearer $TOK" -H 'Content-Type: application/json' "$@"; }
quote() { printf '%s' "$1" | sed 's/"/\\"/g; s/^/"/; s/$/"/'; }
RES=$(json -X POST -d '{"name":"upd-1","address":"'$NODE'"}' $API/nodes/create)
NODE_ID=$(echo "$RES" | sed 's/.*"nodeId":\([0-9]*\).*/\1/')
ENR=$(echo "$RES" | sed 's/.*"token":"\([^"]*\)".*/\1/')
docker run --rm --network $NET -v $NODE-data:/var/lib/d256-node d256-node:e2e enroll --allow-insecure-http http://$PANEL:2053 "$ENR" >/dev/null
docker run -d --name $NODE --network $NET -v $NODE-data:/var/lib/d256-node d256-node:e2e run >/dev/null

SETTINGS='{"clients":[{"id":"'$UUID'","email":"u@upd","flow":"","enable":true,"limitIp":0,"totalGB":0,"expiryTime":0,"tgId":"","subId":"updsub","reset":0}],"decryption":"none","fallbacks":[]}'
STREAM='{"network":"tcp","security":"none","tcpSettings":{"acceptProxyProtocol":false,"header":{"type":"none"}}}'
SNIFF='{"enabled":false,"destOverride":["http","tls"],"metadataOnly":false,"routeOnly":false}'
BODY=$(printf '{"up":0,"down":0,"total":0,"remark":"upd","enable":true,"expiryTime":0,"listen":"","port":20443,"protocol":"vless","settings":%s,"streamSettings":%s,"sniffing":%s,"nodeId":%s}' \
  "$(quote "$SETTINGS")" "$(quote "$STREAM")" "$(quote "$SNIFF")" "$NODE_ID")
json -X POST -d "$BODY" $API/inbounds/add >/dev/null

live() { json $API/nodes/live; }
wait_for() { # description, grep pattern, seconds
  for _ in $(seq 1 "$3"); do live | grep -q "$2" && return 0; sleep 1; done
  fail "$1 (waited $3s for: $2)"
}
wait_for "node online with v0.0.1 and Xray running" '"nodeVersion":"v0.0.1".*"xrayState":"running"' 60
echo "node v0.0.1 is online and Xray is running"

echo "== an update signed with a foreign key is refused"
json -X POST -d '{"version":"v0.0.3"}' $API/nodes/upgrade/$NODE_ID | grep -q '"success":true' || fail "the request itself must be accepted"
wait_for "a refused update" '"upgradeState":"failed"' 60
live | grep -o '"upgradeError":"[^"]*"' | head -1
live | grep -q '"upgradeError":"[^"]*signature' || fail "the refusal must name the signature"
live | grep -q '"nodeVersion":"v0.0.1"' || fail "a refused update must leave the node on v0.0.1"

echo "== a malformed version never reaches the node"
json -X POST -d '{"version":"v1.0.0/../../x"}' $API/nodes/upgrade/$NODE_ID | grep -q '"success":false' || fail "a malformed version must be refused"

echo "== a correctly signed release is installed and the node restarts into it"
json -X POST -d '{"version":"v0.0.2"}' $API/nodes/upgrade/$NODE_ID | grep -q '"success":true' || fail "upgrade request"
wait_for "node on v0.0.2 with Xray running" '"nodeVersion":"v0.0.2".*"xrayState":"running"' 90
docker logs $NODE 2>&1 | grep -q "restarting into the installed update" || fail "the node did not log the hand-over"
docker logs $NODE 2>&1 | grep "d256-node starting" | tail -2

echo "== it stays on v0.0.2 after the container restarts"
docker restart $NODE >/dev/null
wait_for "v0.0.2 after a restart" '"nodeVersion":"v0.0.2".*"xrayState":"running"' 90
docker logs $NODE 2>&1 | grep -c "d256-node starting" | xargs echo "starts logged:"

echo "UPDATE E2E OK"
