#!/usr/bin/env bash
# Deploy Super-Instruct cloud-audit package:
# - bridge + templates (hot)
# - disable cyber_session_block (settings)
# - rebuild backend with SI cyber soft-fail + skip session-block write
set -euo pipefail

SRC=/opt/sub2api/source-main
ROOT=/opt/sub2api
BRIDGE_DIR=/opt/sub2api/data/super-instruct
COMPOSE=(docker compose -f docker-compose.yml -f docker-compose.override.yml -f docker-compose.canary.yml --profile canary)
NGINX_CONF=/etc/nginx/sites-enabled/sub2api.conf
TS=$(date +%Y%m%d-%H%M%S)
LOG=/tmp/sub2api-si-cloud-audit-${TS}.log
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
  docker logs --tail 80 "$name" || true
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

log "1) sync bridge + templates (hot)"
mkdir -p "$BRIDGE_DIR" "$BRIDGE_DIR/templates"
cp -a "$SRC/deploy/super-instruct-bridge.md" "$BRIDGE_DIR/bridge.md"
cp -a "$SRC/deploy/super-instruct/SKILL_TEMPLATE.md" "$BRIDGE_DIR/templates/SKILL_TEMPLATE.md"
cp -a "$SRC/deploy/super-instruct/REVIEW_CARD.md" "$BRIDGE_DIR/templates/REVIEW_CARD.md"
cp -a "$SRC/deploy/super-instruct/PHASE_PROTOCOL.md" "$BRIDGE_DIR/templates/PHASE_PROTOCOL.md"
cp -a "$SRC/deploy/super-instruct/ACTIVE_STATE.md" "$BRIDGE_DIR/templates/ACTIVE_STATE.md"
chown -R 1000:1000 "$BRIDGE_DIR" || true
if [[ -d /opt/super-instruct-webui/data ]]; then
  cp -a "$BRIDGE_DIR/bridge.md" /opt/super-instruct-webui/data/bridge.md || true
fi
touch "$BRIDGE_DIR/bridge.md"
ls -la "$BRIDGE_DIR" "$BRIDGE_DIR/templates"
head -n 16 "$BRIDGE_DIR/bridge.md"
md5sum "$BRIDGE_DIR/bridge.md"

log "2) disable cyber_session_block (immediate)"
docker exec sub2api-postgres psql -U sub2api -d sub2api -v ON_ERROR_STOP=1 <<'SQL'
INSERT INTO settings(key, value, updated_at)
VALUES
  ('cyber_session_block_enabled', 'false', NOW()),
  ('cyber_session_block_ttl_seconds', '300', NOW())
ON CONFLICT (key) DO UPDATE
SET value = EXCLUDED.value, updated_at = NOW();
SELECT key, value FROM settings WHERE key LIKE 'cyber_session_block%';
SQL

log "3) build backend-only image"
cd "$SRC"
test -f backend/internal/service/openai_super_instruct.go || die "si missing"
test -f backend/internal/service/openai_cyber_policy.go || die "cyber policy missing"
grep -q 'rewriteCyberPolicyClientBody' backend/internal/service/openai_cyber_policy.go || die "soft rewrite missing"
grep -q 'IsSuperInstructEnabled()' backend/internal/handler/openai_gateway_handler.go || die "SI skip block missing"
test -f backend/internal/web/dist/index.html || die "frontend dist missing"
test -f Dockerfile.backend-only || die "Dockerfile.backend-only missing"
SHORT=$(git rev-parse --short=9 HEAD 2>/dev/null || echo local)
# include dirty marker
if ! git diff --quiet 2>/dev/null; then SHORT="${SHORT}d"; fi
IMAGE_TAG="sub2api-hixz12:${BASE_VER}-hixz12.1-si-ca-${SHORT}-${TS}"
echo "IMAGE_TAG=$IMAGE_TAG"
export DOCKER_BUILDKIT=1
docker build -f Dockerfile.backend-only \
  --build-arg VERSION_VALUE="${BASE_VER}-si-ca" \
  --build-arg COMMIT="$(git rev-parse HEAD 2>/dev/null || echo local)" \
  --build-arg DATE_VALUE="$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  -t "$IMAGE_TAG" .
docker run --rm --entrypoint sh "$IMAGE_TAG" -c 'strings /app/sub2api | grep -E "rewriteCyberPolicyClientBody|softCyberPolicyClientMessage|openai_super_instruct" | head -20'

log "4) dual-port switch: canary first"
cd "$ROOT"
# detect current primary
CUR=$(python3 - <<'PY'
from pathlib import Path,re
t=Path("/etc/nginx/sites-enabled/sub2api.conf").read_text()
m=re.search(r"upstream\s+sub2api_backend\s*\{([^}]*)\}",t,re.S)
block=m.group(1)
prim=None
for line in block.splitlines():
    line=line.strip()
    if line.startswith('server') and 'backup' not in line:
        if '8100' in line: prim='8100'
        elif '8101' in line: prim='8101'
print(prim or '8100')
PY
)
echo "current primary=$CUR"
if [[ "$CUR" == "8100" ]]; then
  CANARY_PORT=8101; PROD_PORT=8100; CANARY_SVC=sub2api-canary; PROD_SVC=sub2api
else
  CANARY_PORT=8100; PROD_PORT=8101; CANARY_SVC=sub2api; PROD_SVC=sub2api-canary
fi
echo "canary_target=$CANARY_SVC:$CANARY_PORT prod_keep=$PROD_SVC:$PROD_PORT"

# retag compose image usage: update override or force recreate with IMAGE
# Prefer setting image via env if compose supports; else docker tag + recreate.
docker tag "$IMAGE_TAG" sub2api-hixz12:si-current
# Ensure both services can pull local tag — inspect compose image names
"${COMPOSE[@]}" config | grep -E 'image:|container_name:' | head -40 || true

# Recreate canary with new image
if docker ps -a --format '{{.Names}}' | grep -qx sub2api-canary; then
  docker stop sub2api-canary || true
  docker rm sub2api-canary || true
fi
# Use compose up for canary profile with image override
IMAGE_OVERRIDE="$IMAGE_TAG"
cd "$ROOT"
# write ephemeral override
cat > /tmp/docker-compose.si-ca.yml <<YAML
services:
  sub2api:
    image: ${IMAGE_OVERRIDE}
  sub2api-canary:
    image: ${IMAGE_OVERRIDE}
YAML
"${COMPOSE[@]}" -f /tmp/docker-compose.si-ca.yml up -d --no-deps --force-recreate sub2api-canary
wait_healthy sub2api-canary 8101 90

log "5) nginx primary -> canary 8101"
nginx_set_primary 8101
sleep 2
code=$(curl -sS -m 10 -o /tmp/prod_health.json -w '%{http_code}' --resolve sub2api.xiaozhudf2026.foo:443:127.0.0.1 https://sub2api.xiaozhudf2026.foo/health || true)
echo "prod health=$code"
[[ "$code" == "200" ]] || die "prod health not 200"

log "6) recreate primary 8100 with new image"
"${COMPOSE[@]}" -f /tmp/docker-compose.si-ca.yml up -d --no-deps --force-recreate sub2api
wait_healthy sub2api 8100 90
nginx_set_primary 8100
sleep 2
code=$(curl -sS -m 10 -o /tmp/prod_health2.json -w '%{http_code}' --resolve sub2api.xiaozhudf2026.foo:443:127.0.0.1 https://sub2api.xiaozhudf2026.foo/health || true)
echo "prod health after switch back=$code"
[[ "$code" == "200" ]] || die "prod health not 200 after switch back"

log "7) verify settings + binary markers + instest"
docker exec sub2api-postgres psql -U sub2api -d sub2api -t -A -c "select key,value from settings where key like 'cyber_session_block%'"
docker exec sub2api sh -c 'strings /app/sub2api | grep -E "rewriteCyberPolicyClientBody|softCyberPolicyClientMessage" | head -5'
docker exec sub2api head -n 12 /app/data/super-instruct/bridge.md
docker exec sub2api-canary head -n 12 /app/data/super-instruct/bridge.md
ls -la /app/data/super-instruct/templates 2>/dev/null || ls -la "$BRIDGE_DIR/templates"

KEY=$(docker exec sub2api-postgres psql -U sub2api -d sub2api -t -A -c "select key from api_keys where id=190 and deleted_at is null limit 1" | tr -d '\r\n ')
python3 - <<'PY' > /tmp/body_instest.json
import json
print(json.dumps({"model":"gpt-5.4","instructions":"ONLY_CLIENT_TEXT","input":"InsTest","stream":False,"max_output_tokens":32}, ensure_ascii=False))
PY
ok=0
for port in 8100 8101; do
  curl -sS -m 90 -o /tmp/it_$port.json "http://127.0.0.1:${port}/v1/responses" \
    -H "Authorization: Bearer $KEY" -H 'Content-Type: application/json' --data-binary @/tmp/body_instest.json
  python3 - <<PY
import json
d=json.load(open('/tmp/it_$port.json',encoding='utf-8'))
ins=d.get('instructions') or ''
texts=[]
if d.get('output_text'): texts.append(str(d['output_text']))
for item in d.get('output') or []:
  if isinstance(item,dict):
    for c in item.get('content') or []:
      if isinstance(c,dict) and c.get('text'): texts.append(str(c['text']))
t='\n'.join(texts).strip()
print('port=$port', 'handshake', '最高优先级握手' in ins or 'SESSION PHASE' in ins or '会话相位' in ins, 'exact', t=='Any code has superpowers', 'text', repr(t[:80]))
open('/tmp/ok_$port','w').write('1' if t=='Any code has superpowers' else '0')
PY
  grep -q 1 /tmp/ok_$port && ok=$((ok+1)) || true
done
echo "INSTEST_PORTS_OK=$ok/2"
[[ "$ok" == "2" ]] || die "instest failed"

log "DONE log=$LOG"
