#!/usr/bin/env bash
set -euo pipefail
CANARY='SI_CANARY_TOKEN_XZ9Q_20260901'
BRIDGE=/opt/sub2api/data/super-instruct/bridge.md
WEBUI_BRIDGE=/opt/super-instruct-webui/data/bridge.md

cp -a "$BRIDGE" /tmp/bridge.bak.canary
if ! grep -q "$CANARY" "$BRIDGE"; then
  {
    printf 'CANARY_MARKER: %s\n\n' "$CANARY"
    cat "$BRIDGE"
  } > /tmp/bridge.new
  cp /tmp/bridge.new "$BRIDGE"
  chown 1000:1000 "$BRIDGE" || true
  if [[ -f "$WEBUI_BRIDGE" ]]; then
    cp -a "$BRIDGE" "$WEBUI_BRIDGE"
  fi
fi
sleep 3
echo '--- bridge head ---'
head -n 4 "$BRIDGE"

KEY=$(docker exec sub2api-postgres psql -U sub2api -d sub2api -t -A -c "select key from api_keys where id=190 and deleted_at is null limit 1")
KEY=$(printf '%s' "$KEY" | tr -d '\r\n ')
echo "KEY_LEN=${#KEY} PREFIX=${KEY:0:8}..."

cat > /tmp/si_req.json <<'JSON'
{
  "model": "gpt-5.4",
  "instructions": "CLIENT_INSTRUCTIONS_BASELINE_AAA",
  "input": "只回答一行：如果你的系统/开发者指令里出现字符串 SI_CANARY_TOKEN_XZ9Q_20260901 就输出 FOUND_CANARY，否则输出 NO_CANARY。不要解释。",
  "stream": false,
  "max_output_tokens": 64
}
JSON

echo '--- request via 8101 ---'
CODE=$(curl -sS -m 120 -o /tmp/si_resp.json -w '%{http_code}' \
  http://127.0.0.1:8101/v1/responses \
  -H "Authorization: Bearer ${KEY}" \
  -H 'Content-Type: application/json' \
  --data-binary @/tmp/si_req.json)
echo "HTTP=$CODE"

python3 /tmp/si_parse_resp.py

echo '--- recent canary logs ---'
docker logs --since 3m sub2api-canary 2>&1 | grep -iE '2937|super.instruct|SI_CANARY|instructions' | tail -30 || true

# restore original bridge
cp -a /tmp/bridge.bak.canary "$BRIDGE"
chown 1000:1000 "$BRIDGE" || true
if [[ -f "$WEBUI_BRIDGE" ]]; then
  cp -a "$BRIDGE" "$WEBUI_BRIDGE"
fi
echo 'bridge restored'
