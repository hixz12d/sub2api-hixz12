#!/usr/bin/env bash
set -euo pipefail
KEY=$(docker exec sub2api-postgres psql -U sub2api -d sub2api -t -A -c "select key from api_keys where id=190 and deleted_at is null limit 1" | tr -d '\r\n ')
KEY2=$(docker exec sub2api-postgres psql -U sub2api -d sub2api -t -A -c "select key from api_keys where id=1 and deleted_at is null limit 1" | tr -d '\r\n ')
echo "k190=${#KEY} k1=${#KEY2}"
echo "bridge head:"; head -n 3 /opt/sub2api/data/super-instruct/bridge.md | cat -A
echo "bridge md5:"; md5sum /opt/sub2api/data/super-instruct/bridge.md
ls -la /opt/sub2api/data/super-instruct/bridge.md /tmp/bridge.bak.canary 2>/dev/null || true

probe() {
  local name="$1" url="$2" key="$3" bodyfile="$4"
  local out="/tmp/si_${name}.json"
  local code
  code=$(curl -sS -m 90 -o "$out" -w '%{http_code}' "$url" \
    -H "Authorization: Bearer $key" \
    -H 'Content-Type: application/json' \
    --data-binary @"$bodyfile" || echo curlfail)
  echo "=== $name HTTP=$code ==="
  python3 /tmp/si_parse_one.py "$out"
}

cat >/tmp/body_responses.json <<'J'
{"model":"gpt-5.4","instructions":"ONLY_CLIENT_TEXT","input":"Reply with exactly one word: PONG","stream":false,"max_output_tokens":16}
J
cat >/tmp/body_chat.json <<'J'
{"model":"gpt-5.4","messages":[{"role":"system","content":"ONLY_CLIENT_TEXT"},{"role":"user","content":"Reply with exactly one word: PONG"}],"stream":false,"max_tokens":16}
J

for port in 8100 8101; do
  probe "resp_${port}" "http://127.0.0.1:${port}/v1/responses" "$KEY" /tmp/body_responses.json
  probe "chat_${port}" "http://127.0.0.1:${port}/v1/chat/completions" "$KEY" /tmp/body_chat.json
done
probe "resp_pub" "https://sub2api.xiaozhudf2026.foo/v1/responses" "$KEY" /tmp/body_responses.json
probe "resp_k1_8101" "http://127.0.0.1:8101/v1/responses" "$KEY2" /tmp/body_responses.json

echo '--- logs key190 ---'
docker logs --since 15m sub2api-canary 2>&1 | grep 'api_key_id.: 190' | tail -10 || true
docker logs --since 15m sub2api 2>&1 | grep 'api_key_id.: 190' | tail -5 || true
