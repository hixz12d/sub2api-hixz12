#!/usr/bin/env bash
# Low-memory dual-port seamless deploy for Super-Instruct on /opt/sub2api
set -euo pipefail

SRC=/opt/sub2api/source-main
ROOT=/opt/sub2api
COMPOSE=(docker compose -f docker-compose.yml -f docker-compose.override.yml -f docker-compose.canary.yml --profile canary)
EXPECTED_COMMIT="${EXPECTED_COMMIT:-93df5403ee5fe411587869126edc90eb32fc8111}"
BRIDGE_HOST_DIR=/opt/sub2api/data/super-instruct
BRIDGE_HOST_FILE="${BRIDGE_HOST_DIR}/bridge.md"
BRIDGE_WEBUI=/opt/super-instruct-webui/data/bridge.md
BRIDGE_IN_CONTAINER=/app/data/super-instruct/bridge.md
NGINX_CONF=/etc/nginx/sites-enabled/sub2api.conf
TS=$(date +%Y%m%d-%H%M%S)
LOG=/tmp/sub2api-si-deploy-${TS}.log

exec > >(tee -a "$LOG") 2>&1

log() { printf '\n==== %s ====\n' "$*"; }
die() { echo "FATAL: $*" >&2; exit 1; }

need_cmd() { command -v "$1" >/dev/null || die "missing $1"; }
need_cmd git
need_cmd docker
need_cmd nginx
need_cmd curl
need_cmd python3

free_h() { free -h | sed -n '1,3p'; }

wait_healthy() {
  local name=$1 port=$2 tries=${3:-60}
  local i
  for i in $(seq 1 "$tries"); do
    local st
    st=$(docker inspect -f '{{.State.Health.Status}}' "$name" 2>/dev/null || echo missing)
    if [[ "$st" == "healthy" ]]; then
      local code
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
  docker logs --tail 80 "$name" || true
  die "$name failed to become healthy"
}

nginx_set_primary() {
  # $1 = primary port (8100 or 8101), other becomes backup
  local primary=$1
  local backup
  if [[ "$primary" == "8100" ]]; then backup=8101; else backup=8100; fi
  python3 - <<PY
from pathlib import Path
p = Path("$NGINX_CONF")
text = p.read_text(encoding="utf-8")
import re
pat = re.compile(r"upstream\s+sub2api_backend\s*\{.*?\}", re.S)
m = pat.search(text)
if not m:
    raise SystemExit("upstream sub2api_backend not found")
block = f"""upstream sub2api_backend {{
    server 127.0.0.1:{primary};
    server 127.0.0.1:{backup} backup;
}}"""
text2 = pat.sub(block, text, count=1)
p.write_text(text2, encoding="utf-8")
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

log "0) preflight memory/disk"
free_h
df -h / | tail -1
docker ps --format 'table {{.Names}}\t{{.Status}}\t{{.Image}}' | grep -E 'NAMES|sub2api'

log "1) light git pull (ff-only, no full reclone)"
cd "$SRC"
git status -sb
[[ -z "$(git status --porcelain)" ]] || die "source-main dirty; abort"
git fetch --depth=50 origin main
git merge-base --is-ancestor HEAD "origin/main" || die "local HEAD not ancestor of origin/main"
git pull --ff-only origin main
HEAD=$(git rev-parse HEAD)
SHORT=$(git rev-parse --short=9 HEAD)
echo "HEAD=$HEAD short=$SHORT"
[[ "$HEAD" == "$EXPECTED_COMMIT" ]] || die "HEAD $HEAD != expected $EXPECTED_COMMIT"
test -f backend/internal/service/openai_super_instruct.go || die "patch file missing after pull"
VERSION=$(grep -E '^\s*version|Version' README.md 2>/dev/null | head -1 || true)
# derive version tag from previous image style 0.1.185
BASE_VER="0.1.185"
IMAGE_TAG="sub2api-hixz12:${BASE_VER}-hixz12.1-si-${SHORT}"
echo "IMAGE_TAG=$IMAGE_TAG"

log "2) prepare bridge file + config env (no restart yet)"
mkdir -p "$BRIDGE_HOST_DIR"
if [[ -f "$BRIDGE_WEBUI" ]]; then
  cp -a "$BRIDGE_WEBUI" "$BRIDGE_HOST_FILE"
else
  if [[ ! -f "$BRIDGE_HOST_FILE" ]]; then
    cp -a "$SRC/deploy/super-instruct-bridge.md" "$BRIDGE_HOST_FILE"
  fi
fi
chown -R 1000:1000 "$BRIDGE_HOST_DIR" || true
chmod 644 "$BRIDGE_HOST_FILE"
wc -c "$BRIDGE_HOST_FILE"

# persist bridge path in .env (viper AutomaticEnv: GATEWAY_SUPER_INSTRUCT_BRIDGE_FILE)
cd "$ROOT"
cp -a .env ".env.bak.${TS}"
if grep -q '^GATEWAY_SUPER_INSTRUCT_BRIDGE_FILE=' .env; then
  sed -i "s|^GATEWAY_SUPER_INSTRUCT_BRIDGE_FILE=.*|GATEWAY_SUPER_INSTRUCT_BRIDGE_FILE=${BRIDGE_IN_CONTAINER}|" .env
else
  printf '\n# Super-Instruct bridge (account whitelist)\nGATEWAY_SUPER_INSTRUCT_BRIDGE_FILE=%s\n' \
    "$BRIDGE_IN_CONTAINER" >> .env
fi
# also patch config.yaml gateway section if file exists
python3 - <<'PY'
from pathlib import Path
p = Path("/opt/sub2api/data/config.yaml")
text = p.read_text(encoding="utf-8")
key = "super_instruct_bridge_file"
val = "/app/data/super-instruct/bridge.md"
if key in text:
    import re
    text = re.sub(rf"(?m)^(\s*){key}\s*:.*$", rf"\1{key}: {val}", text)
else:
    if "gateway:" in text:
        # insert under gateway:
        lines = text.splitlines(True)
        out=[]
        inserted=False
        for i,line in enumerate(lines):
            out.append(line)
            if not inserted and line.strip()=="gateway:":
                # next lines are indented; insert after gateway: line
                out.append(f"  {key}: {val}\n")
                inserted=True
        text="".join(out)
        if not inserted:
            text += f"\ngateway:\n  {key}: {val}\n"
    else:
        text += f"\ngateway:\n  {key}: {val}\n"
p.write_text(text, encoding="utf-8")
print("config.yaml updated")
PY

log "3) low-memory docker build (do not stop running containers)"
free_h
# prune only unused build cache lightly if free RAM low
avail_kb=$(awk '/MemAvailable:/ {print $2}' /proc/meminfo)
echo "MemAvailable_kB=$avail_kb"
if [[ "$avail_kb" -lt 1500000 ]]; then
  echo "low free RAM; drop page cache once"
  sync
  echo 1 > /proc/sys/vm/drop_caches || true
  free_h
fi

cd "$SRC"
export DOCKER_BUILDKIT=1
# Limit Go parallelism a bit; BuildKit cache mounts reuse prior layers.
docker build \
  --build-arg VERSION="${BASE_VER}-si" \
  --build-arg COMMIT="${SHORT}" \
  --build-arg DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  -t "$IMAGE_TAG" \
  .

echo "built $IMAGE_TAG"
docker image inspect "$IMAGE_TAG" --format '{{.Id}} {{.Size}}'
free_h

log "4) point SUB2API_IMAGE to new tag (compose reads it)"
cd "$ROOT"
if grep -q '^SUB2API_IMAGE=' .env; then
  sed -i "s|^SUB2API_IMAGE=.*|SUB2API_IMAGE=${IMAGE_TAG}|" .env
else
  echo "SUB2API_IMAGE=${IMAGE_TAG}" >> .env
fi
grep '^SUB2API_IMAGE=' .env
grep '^GATEWAY_SUPER_INSTRUCT_BRIDGE_FILE=' .env

# backup nginx
mkdir -p /opt/sub2api/backups/nginx
cp -a "$NGINX_CONF" "/opt/sub2api/backups/nginx/sub2api.conf.${TS}"

log "5) rebuild ONLY backup instance sub2api :8100 --no-deps"
"${COMPOSE[@]}" up -d --no-deps --force-recreate sub2api
wait_healthy sub2api 8100 80
# verify bridge env inside container
docker exec sub2api sh -c 'echo BRIDGE_ENV=$GATEWAY_SUPER_INSTRUCT_BRIDGE_FILE; ls -la /app/data/super-instruct/bridge.md; wc -c /app/data/super-instruct/bridge.md'

log "6) nginx flip: 8100 primary, 8101 backup"
nginx_set_primary 8100
sleep 1
code=$(prod_health); echo "prod /health HTTP=$code"; [[ "$code" == "200" ]] || die "prod health not 200 after flip to 8100"
cat /tmp/prod_health.json; echo

log "7) rebuild ONLY primary instance sub2api-canary :8101 --no-deps"
"${COMPOSE[@]}" up -d --no-deps --force-recreate sub2api-canary
wait_healthy sub2api-canary 8101 80
docker exec sub2api-canary sh -c 'echo BRIDGE_ENV=$GATEWAY_SUPER_INSTRUCT_BRIDGE_FILE; ls -la /app/data/super-instruct/bridge.md; wc -c /app/data/super-instruct/bridge.md'

log "8) nginx restore: 8101 primary, 8100 backup"
nginx_set_primary 8101
sleep 1
code=$(prod_health); echo "prod /health HTTP=$code"; [[ "$code" == "200" ]] || die "prod health not 200 after restore 8101"
cat /tmp/prod_health.json; echo

log "9) final consistency checks"
docker ps --format 'table {{.Names}}\t{{.Image}}\t{{.Status}}\t{{.Ports}}' | grep -E 'NAMES|sub2api'
echo '--- nginx upstream ---'
grep -A3 'upstream sub2api_backend' "$NGINX_CONF"
echo '--- image ids ---'
docker inspect sub2api sub2api-canary --format '{{.Name}} image={{.Config.Image}} id={{.Image}}'
echo '--- compose images ---'
"${COMPOSE[@]}" config --images | sort -u
echo '--- source HEAD ---'
git -C "$SRC" rev-parse HEAD

# quick binary string check for Super-Instruct marker in running containers (optional)
docker exec sub2api sh -c 'wget -qO- http://127.0.0.1:8080/health | head -c 200; echo'
docker exec sub2api-canary sh -c 'wget -qO- http://127.0.0.1:8080/health | head -c 200; echo'

# ensure webui still talks to network name
curl -sS -m 8 http://127.0.0.1:8020/api/health; echo
curl -sS -m 15 -X POST http://127.0.0.1:8020/api/test-connection \
  -H 'Content-Type: application/json' \
  -d '{"base_url":"","admin_api_key":"","default_mode":"prepend"}'; echo

log "DONE"
echo "IMAGE_TAG=$IMAGE_TAG"
echo "LOG=$LOG"
free_h
