#!/usr/bin/env bash
set -euo pipefail
ROOT=/opt/sub2api
COMPOSE=(docker compose -f docker-compose.yml -f docker-compose.override.yml -f docker-compose.canary.yml --profile canary)
NGINX_CONF=/etc/nginx/sites-enabled/sub2api.conf
TS=$(date +%Y%m%d-%H%M%S)
LOG=/tmp/sub2api-si-finish-${TS}.log
exec > >(tee -a "$LOG") 2>&1

die(){ echo "FATAL: $*"; exit 1; }

wait_healthy() {
  local name=$1 port=$2
  local i st code
  for i in $(seq 1 90); do
    st=$(docker inspect -f '{{.State.Health.Status}}' "$name" 2>/dev/null || echo missing)
    code=$(curl -sS -m 5 -o "/tmp/h_${name}.json" -w '%{http_code}' "http://127.0.0.1:${port}/health" || true)
    echo "$name health=$st http=$code try=$i"
    if [[ "$st" == "healthy" && "$code" == "200" ]]; then
      return 0
    fi
    sleep 3
  done
  docker logs --tail 80 "$name" || true
  die "$name not healthy"
}

nginx_set_primary() {
  local primary=$1
  local backup
  if [[ "$primary" == "8100" ]]; then
    backup=8101
  else
    backup=8100
  fi
  python3 - "$NGINX_CONF" "$primary" "$backup" <<'PY'
from pathlib import Path
import re
import sys

conf, primary, backup = sys.argv[1], sys.argv[2], sys.argv[3]
p = Path(conf)
text = p.read_text(encoding="utf-8")
pat = re.compile(r"upstream\s+sub2api_backend\s*\{.*?\}", re.S)
if not pat.search(text):
    raise SystemExit("upstream missing")
block = (
    "upstream sub2api_backend {\n"
    f"    server 127.0.0.1:{primary};\n"
    f"    server 127.0.0.1:{backup} backup;\n"
    "}"
)
p.write_text(pat.sub(block, text, count=1), encoding="utf-8")
print(f"nginx upstream primary={primary} backup={backup}")
PY
  nginx -t
  nginx -s reload
}

prod_health() {
  curl -sS -m 10 -o /tmp/prod_health.json -w '%{http_code}' \
    --resolve sub2api.xiaozhudf2026.foo:443:127.0.0.1 \
    https://sub2api.xiaozhudf2026.foo/health || true
}

echo "==== state before ===="
docker ps --format 'table {{.Names}}\t{{.Image}}\t{{.Status}}' | grep sub2api || true
grep -A3 'upstream sub2api_backend' "$NGINX_CONF"
curl -sS -m 5 http://127.0.0.1:8100/health; echo
curl -sS -m 5 http://127.0.0.1:8101/health; echo

cd "$ROOT"
grep '^SUB2API_IMAGE=' .env
IMAGE_TAG=$(grep '^SUB2API_IMAGE=' .env | cut -d= -f2)
test -n "$IMAGE_TAG"
docker image inspect "$IMAGE_TAG" >/dev/null

mkdir -p /opt/sub2api/backups/nginx
cp -a "$NGINX_CONF" "/opt/sub2api/backups/nginx/sub2api.conf.finish.${TS}"

echo "==== flip nginx to 8100 primary (new image) ===="
nginx_set_primary 8100
sleep 1
code=$(prod_health)
echo "prod HTTP=$code"
[[ "$code" == "200" ]] || die "prod fail after flip"
cat /tmp/prod_health.json; echo
grep -A3 'upstream sub2api_backend' "$NGINX_CONF"

echo "==== rebuild canary 8101 only ===="
"${COMPOSE[@]}" up -d --no-deps --force-recreate sub2api-canary
wait_healthy sub2api-canary 8101
docker exec sub2api-canary sh -c 'grep -n super_instruct /app/data/config.yaml; ls -la /app/data/super-instruct/bridge.md; wc -c /app/data/super-instruct/bridge.md'
docker inspect sub2api-canary --format 'image={{.Config.Image}} id={{.Image}}'

echo "==== restore nginx 8101 primary ===="
nginx_set_primary 8101
sleep 1
code=$(prod_health)
echo "prod HTTP=$code"
[[ "$code" == "200" ]] || die "prod fail after restore"
cat /tmp/prod_health.json; echo

echo "==== final ===="
docker ps --format 'table {{.Names}}\t{{.Image}}\t{{.Status}}\t{{.Ports}}' | grep -E 'NAMES|sub2api' || true
grep -A3 'upstream sub2api_backend' "$NGINX_CONF"
id0=$(docker inspect -f '{{.Image}}' sub2api)
id1=$(docker inspect -f '{{.Image}}' sub2api-canary)
echo "sub2api=$id0"
echo "canary=$id1"
[[ "$id0" == "$id1" ]] || die "image mismatch"
img0=$(docker inspect -f '{{.Config.Image}}' sub2api)
img1=$(docker inspect -f '{{.Config.Image}}' sub2api-canary)
echo "tags $img0 / $img1"
[[ "$img0" == "$img1" ]] || die "tag mismatch"
"${COMPOSE[@]}" config --images | sort -u
git -C /opt/sub2api/source-main rev-parse HEAD

curl -sS -m 8 http://127.0.0.1:8020/api/health; echo
curl -sS -m 15 -X POST http://127.0.0.1:8020/api/test-connection \
  -H 'Content-Type: application/json' \
  -d '{"base_url":"","admin_api_key":"","default_mode":"prepend"}'; echo
curl -sS -m 10 -o /dev/null -w 'public_home=%{http_code}\n' \
  --resolve sub2api.xiaozhudf2026.foo:443:127.0.0.1 \
  https://sub2api.xiaozhudf2026.foo/
curl -sS -m 10 -o /dev/null -w 'public_health=%{http_code}\n' \
  --resolve sub2api.xiaozhudf2026.foo:443:127.0.0.1 \
  https://sub2api.xiaozhudf2026.foo/health
curl -sS -m 10 -o /dev/null -w 'public_si=%{http_code}\n' \
  --resolve sub2api.xiaozhudf2026.foo:443:127.0.0.1 \
  https://sub2api.xiaozhudf2026.foo/super-instruct/

docker exec sub2api sh -c 'strings /app/sub2api | grep -F "openai_super_instruct.go" | head -1'
docker exec sub2api-canary sh -c 'strings /app/sub2api | grep -F "openai_super_instruct.go" | head -1'

echo "DONE LOG=$LOG"
free -h | sed -n '1,3p'
