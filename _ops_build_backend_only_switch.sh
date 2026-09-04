#!/usr/bin/env bash
# Continue seamless dual-port switch after low-mem backend-only image build.
set -euo pipefail

SRC=/opt/sub2api/source-main
ROOT=/opt/sub2api
COMPOSE=(docker compose -f docker-compose.yml -f docker-compose.override.yml -f docker-compose.canary.yml --profile canary)
EXPECTED_COMMIT="${EXPECTED_COMMIT:-93df5403ee5fe411587869126edc90eb32fc8111}"
NGINX_CONF=/etc/nginx/sites-enabled/sub2api.conf
TS=$(date +%Y%m%d-%H%M%S)
LOG=/tmp/sub2api-si-backend-only-${TS}.log
BASE_VER=0.1.185

exec > >(tee -a "$LOG") 2>&1
log() { printf '\n==== %s ====\n' "$*"; }
die() { echo "FATAL: $*" >&2; exit 1; }

wait_healthy() {
  local name=$1 port=$2 tries=${3:-80}
  local i st code
  for i in $(seq 1 "$tries"); do
    st=$(docker inspect -f '{{.State.Health.Status}}' "$name" 2>/dev/null || echo missing)
    if [[ "$st" == "healthy" ]]; then
      code=$(curl -sS -m 5 -o /tmp/h_${name}.json -w '%{http_code}' "http://127.0.0.1:${port}/health" || true)
      if [[ "$code" == "200" ]]; then
        echo "$name healthy + /health 200"
        return 0
      fi
      echo "$name docker-healthy but /health=$code (try $i)"
    else
      echo "$name health=$st (try $i/$tries)"
    fi
    sleep 3
  done
  docker logs --tail 100 "$name" || true
  die "$name failed healthy"
}

nginx_set_primary() {
  local primary=$1 backup
  if [[ "$primary" == "8100" ]]; then backup=8101; else backup=8100; fi
  python3 - <<PY
from pathlib import Path
import re
p = Path("$NGINX_CONF")
text = p.read_text(encoding="utf-8")
pat = re.compile(r"upstream\s+sub2api_backend\s*\{.*?\}", re.S)
if not pat.search(text):
    raise SystemExit("upstream missing")
block = f"""upstream sub2api_backend {{
    server 127.0.0.1:{primary};
    server 127.0.0.1:{backup} backup;
}}"""
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

log "0) preflight"
free -h | sed -n '1,3p'
df -h / | tail -1
cd "$SRC"
HEAD=$(git rev-parse HEAD)
SHORT=$(git rev-parse --short=9 HEAD)
echo "HEAD=$HEAD"
[[ "$HEAD" == "$EXPECTED_COMMIT" ]] || die "unexpected HEAD"
test -f backend/internal/service/openai_super_instruct.go || die "patch missing"
test -f backend/internal/web/dist/index.html || die "frontend dist missing — upload first"
test -f Dockerfile.backend-only || die "Dockerfile.backend-only missing"
IMAGE_TAG="sub2api-hixz12:${BASE_VER}-hixz12.1-si-${SHORT}"
echo "IMAGE_TAG=$IMAGE_TAG"

# ensure bridge/config already present
test -f /opt/sub2api/data/super-instruct/bridge.md || die "bridge.md missing"
grep -q 'super_instruct_bridge_file' /opt/sub2api/data/config.yaml || die "config.yaml missing bridge key"

log "1) low-mem backend-only docker build"
export DOCKER_BUILDKIT=1
export GOMAXPROCS=2
sync
# drop caches if available mem low
avail_kb=$(awk '/MemAvailable:/ {print $2}' /proc/meminfo)
echo "MemAvailable_kB=$avail_kb"
if [[ "$avail_kb" -lt 2000000 ]]; then
  echo 1 > /proc/sys/vm/drop_caches || true
fi
free -h | sed -n '1,3p'

docker build \
  -f Dockerfile.backend-only \
  --build-arg VERSION="${BASE_VER}-si" \
  --build-arg COMMIT="${SHORT}" \
  --build-arg DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  -t "$IMAGE_TAG" \
  .

docker image inspect "$IMAGE_TAG" --format 'id={{.Id}} size={{.Size}}'
# smoke: binary contains Super-Instruct marker string
docker run --rm --entrypoint sh "$IMAGE_TAG" -c 'strings /app/sub2api | grep -F "[Super-Instruct" | head -3'
docker run --rm --entrypoint sh "$IMAGE_TAG" -c 'strings /app/sub2api | grep -F "super_instruct" | head -5'

log "2) persist image tag"
cd "$ROOT"
cp -a .env ".env.bak.backendonly.${TS}"
if grep -q '^SUB2API_IMAGE=' .env; then
  sed -i "s|^SUB2API_IMAGE=.*|SUB2API_IMAGE=${IMAGE_TAG}|" .env
else
  echo "SUB2API_IMAGE=${IMAGE_TAG}" >> .env
fi
grep '^SUB2API_IMAGE=' .env
mkdir -p /opt/sub2api/backups/nginx
cp -a "$NGINX_CONF" "/opt/sub2api/backups/nginx/sub2api.conf.${TS}"

log "3) rebuild backup sub2api :8100 only"
"${COMPOSE[@]}" up -d --no-deps --force-recreate sub2api
wait_healthy sub2api 8100 90
docker exec sub2api sh -c 'grep -n super_instruct /app/data/config.yaml; ls -la /app/data/super-instruct/bridge.md; wc -c /app/data/super-instruct/bridge.md'
docker inspect sub2api --format 'image={{.Config.Image}} id={{.Image}}'

log "4) nginx flip -> 8100 primary"
nginx_set_primary 8100
sleep 1
code=$(prod_health); echo "prod HTTP=$code"; [[ "$code" == "200" ]] || die "prod health fail after 8100 flip"
cat /tmp/prod_health.json; echo

log "5) rebuild primary sub2api-canary :8101 only"
"${COMPOSE[@]}" up -d --no-deps --force-recreate sub2api-canary
wait_healthy sub2api-canary 8101 90
docker exec sub2api-canary sh -c 'grep -n super_instruct /app/data/config.yaml; ls -la /app/data/super-instruct/bridge.md'
docker inspect sub2api-canary --format 'image={{.Config.Image}} id={{.Image}}'

log "6) nginx restore -> 8101 primary"
nginx_set_primary 8101
sleep 1
code=$(prod_health); echo "prod HTTP=$code"; [[ "$code" == "200" ]] || die "prod health fail after restore"
cat /tmp/prod_health.json; echo

log "7) final checks"
docker ps --format 'table {{.Names}}\t{{.Image}}\t{{.Status}}\t{{.Ports}}' | grep -E 'NAMES|sub2api'
echo '--- upstream ---'
grep -A3 'upstream sub2api_backend' "$NGINX_CONF"
echo '--- image match ---'
id0=$(docker inspect -f '{{.Image}}' sub2api)
id1=$(docker inspect -f '{{.Image}}' sub2api-canary)
echo "sub2api=$id0"
echo "canary=$id1"
[[ "$id0" == "$id1" ]] || die "image ids differ"
"${COMPOSE[@]}" config --images | sort -u
git -C "$SRC" rev-parse HEAD
curl -sS -m 8 http://127.0.0.1:8020/api/health; echo
curl -sS -m 15 -X POST http://127.0.0.1:8020/api/test-connection \
  -H 'Content-Type: application/json' \
  -d '{"base_url":"","admin_api_key":"","default_mode":"prepend"}'; echo
# public pages
curl -sS -m 10 -o /dev/null -w 'public_home=%{http_code}\n' \
  --resolve sub2api.xiaozhudf2026.foo:443:127.0.0.1 \
  https://sub2api.xiaozhudf2026.foo/
curl -sS -m 10 -o /dev/null -w 'public_si=%{http_code}\n' \
  --resolve sub2api.xiaozhudf2026.foo:443:127.0.0.1 \
  https://sub2api.xiaozhudf2026.foo/super-instruct/

log "DONE"
echo "IMAGE_TAG=$IMAGE_TAG LOG=$LOG"
free -h | sed -n '1,3p'
