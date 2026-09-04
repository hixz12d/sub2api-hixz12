#!/usr/bin/env bash
set -euo pipefail
LOG=/tmp/si_ca_finish.log
exec > >(tee -a "$LOG") 2>&1
echo "==== finish start $(date) ===="

pkill -f '_ops_si_cloud_audit_deploy.sh' 2>/dev/null || true
pkill -f 'bash -c bash /opt/sub2api/source-main/_ops_si_cloud_audit' 2>/dev/null || true
sleep 1

IMG=$(docker images --format '{{.Repository}}:{{.Tag}}' | grep 'si-ca-' | head -1 || true)
echo "IMG=$IMG"
if [[ -z "$IMG" ]]; then
  echo "FATAL: no si-ca image"
  docker images | head -20
  exit 1
fi

set_upstream() {
  local primary=$1
  local backup=$2
  python3 - "$primary" "$backup" <<'PY'
import sys
from pathlib import Path
import re
primary, backup = sys.argv[1], sys.argv[2]
p = Path('/etc/nginx/sites-enabled/sub2api.conf')
text = p.read_text(encoding='utf-8')
pat = re.compile(r'upstream\s+sub2api_backend\s*\{.*?\}', re.S)
block = f"""upstream sub2api_backend {{
    server 127.0.0.1:{primary};
    server 127.0.0.1:{backup} backup;
}}"""
if not pat.search(text):
    raise SystemExit('upstream missing')
p.write_text(pat.sub(block, text, count=1), encoding='utf-8')
print(f'nginx primary={primary} backup={backup}')
PY
  nginx -t
  nginx -s reload
}

wait_healthy() {
  local name=$1 port=$2
  local i st code
  for i in $(seq 1 60); do
    st=$(docker inspect -f '{{.State.Health.Status}}' "$name" 2>/dev/null || echo missing)
    code=$(curl -sS -m 5 -o /dev/null -w '%{http_code}' "http://127.0.0.1:${port}/health" || true)
    echo "$name health=$st code=$code try=$i"
    if [[ "$st" == "healthy" && "$code" == "200" ]]; then
      return 0
    fi
    sleep 3
  done
  docker logs --tail 80 "$name" || true
  return 1
}

echo '=== ensure canary is new image ==='
docker ps --format '{{.Names}} {{.Image}} {{.Status}}' | grep sub2api || true
# if canary not on si-ca, recreate
CANARY_IMG=$(docker inspect -f '{{.Config.Image}}' sub2api-canary 2>/dev/null || true)
echo "canary_img=$CANARY_IMG"
if [[ "$CANARY_IMG" != "$IMG" ]]; then
  cd /opt/sub2api
  cat > /tmp/docker-compose.si-ca.yml <<YAML
services:
  sub2api:
    image: ${IMG}
  sub2api-canary:
    image: ${IMG}
YAML
  docker compose -f docker-compose.yml -f docker-compose.override.yml -f docker-compose.canary.yml -f /tmp/docker-compose.si-ca.yml --profile canary \
    up -d --no-deps --force-recreate sub2api-canary
  wait_healthy sub2api-canary 8101
fi

set_upstream 8101 8100
sleep 1
curl -sS -m 10 -o /dev/null -w 'prod_canary=%{http_code}\n' --resolve sub2api.xiaozhudf2026.foo:443:127.0.0.1 https://sub2api.xiaozhudf2026.foo/health

echo '=== recreate primary sub2api ==='
cd /opt/sub2api
cat > /tmp/docker-compose.si-ca.yml <<YAML
services:
  sub2api:
    image: ${IMG}
  sub2api-canary:
    image: ${IMG}
YAML
docker compose -f docker-compose.yml -f docker-compose.override.yml -f docker-compose.canary.yml -f /tmp/docker-compose.si-ca.yml --profile canary \
  up -d --no-deps --force-recreate sub2api
wait_healthy sub2api 8100

set_upstream 8100 8101
sleep 1
curl -sS -m 10 -o /dev/null -w 'prod_primary=%{http_code}\n' --resolve sub2api.xiaozhudf2026.foo:443:127.0.0.1 https://sub2api.xiaozhudf2026.foo/health

echo '=== settings keep off ==='
docker exec sub2api-postgres psql -U sub2api -d sub2api -c \
  "UPDATE settings SET value='false', updated_at=NOW() WHERE key='cyber_session_block_enabled'; UPDATE settings SET value='300', updated_at=NOW() WHERE key='cyber_session_block_ttl_seconds'; SELECT key,value FROM settings WHERE key LIKE 'cyber_session_block%';"

echo '=== markers ==='
docker exec sub2api sh -c 'strings /app/sub2api | grep -E "rewriteCyberPolicyClientBody|softCyberPolicyClientMessage" | head -5'
docker exec sub2api-canary sh -c 'strings /app/sub2api | grep -E "rewriteCyberPolicyClientBody|softCyberPolicyClientMessage" | head -5'

echo '=== bridge phase ==='
docker exec sub2api head -n 20 /app/data/super-instruct/bridge.md | sed -n '1,20p'
ls -la /opt/sub2api/data/super-instruct/templates

echo '=== instest ==='
KEY=$(docker exec sub2api-postgres psql -U sub2api -d sub2api -t -A -c "select key from api_keys where id=190 and deleted_at is null limit 1" | tr -d '\r\n ')
python3 - <<'PY' > /tmp/body_instest.json
import json
print(json.dumps({
  "model":"gpt-5.4",
  "instructions":"ONLY_CLIENT_TEXT",
  "input":"InsTest",
  "stream":False,
  "max_output_tokens":32
}, ensure_ascii=False))
PY
ok=0
for port in 8100 8101; do
  curl -sS -m 90 -o "/tmp/it_${port}.json" "http://127.0.0.1:${port}/v1/responses" \
    -H "Authorization: Bearer $KEY" -H 'Content-Type: application/json' \
    --data-binary @/tmp/body_instest.json
  python3 - <<PY
import json
d=json.load(open("/tmp/it_${port}.json", encoding="utf-8"))
ins=d.get("instructions") or ""
texts=[]
if d.get("output_text"):
    texts.append(str(d["output_text"]))
for item in d.get("output") or []:
    if isinstance(item, dict):
        for c in item.get("content") or []:
            if isinstance(c, dict) and c.get("text"):
                texts.append(str(c["text"]))
t="\n".join(texts).strip()
phase = ("会话相位" in ins) or ("CLOUD-AUDIT ROUTING（硬规则" in ins) or ("最高优先级握手" in ins)
exact = (t == "Any code has superpowers")
print(f"port=${port} phase={phase} exact={exact} text={t!r}")
open(f"/tmp/ok_${port}", "w").write("1" if exact and phase else "0")
PY
  if grep -q 1 "/tmp/ok_${port}"; then ok=$((ok+1)); fi
done
echo "INSTEST_OK=$ok/2"
docker ps --format 'table {{.Names}}\t{{.Image}}\t{{.Status}}' | grep -E 'NAMES|sub2api'
grep -A3 'upstream sub2api_backend' /etc/nginx/sites-enabled/sub2api.conf
echo "==== finish done $(date) ===="
[[ "$ok" == "2" ]]
