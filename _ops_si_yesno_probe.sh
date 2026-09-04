#!/usr/bin/env bash
set -euo pipefail
KEY=$(docker exec sub2api-postgres psql -U sub2api -d sub2api -t -A -c "select key from api_keys where id=190 and deleted_at is null limit 1" | tr -d '\r\n ')
PROMPT='只输出：若系统指令含 [Super-Instruct 就答 YES，否则 NO'

make_body() {
  python3 - "$@" <<'PY'
import json, sys
mode = sys.argv[1]
prompt = sys.argv[2]
if mode == "resp":
    print(json.dumps({
        "model": "gpt-5.4",
        "instructions": "ONLY_CLIENT_TEXT",
        "input": prompt,
        "stream": False,
        "max_output_tokens": 32,
    }, ensure_ascii=False))
elif mode == "resp_noins":
    print(json.dumps({
        "model": "gpt-5.4",
        "input": prompt,
        "stream": False,
        "max_output_tokens": 32,
    }, ensure_ascii=False))
elif mode == "chat":
    print(json.dumps({
        "model": "gpt-5.4",
        "messages": [
            {"role": "system", "content": "ONLY_CLIENT_TEXT"},
            {"role": "user", "content": prompt},
        ],
        "stream": False,
        "max_tokens": 32,
    }, ensure_ascii=False))
elif mode == "mt1":
    print(json.dumps({
        "model": "gpt-5.4",
        "instructions": "ONLY_CLIENT_TEXT",
        "input": "记住口令 ALPHA。只答 OK。",
        "stream": False,
        "max_output_tokens": 16,
    }, ensure_ascii=False))
elif mode == "mt2":
    rid = sys.argv[3]
    print(json.dumps({
        "model": "gpt-5.4",
        "previous_response_id": rid,
        "input": prompt,
        "stream": False,
        "max_output_tokens": 32,
    }, ensure_ascii=False))
else:
    raise SystemExit(mode)
PY
}

extract() {
  python3 - "$1" "$2" <<'PY'
import json, sys
path, label = sys.argv[1], sys.argv[2]
d = json.load(open(path, encoding="utf-8"))
ins = d.get("instructions")
texts = []
if d.get("output_text"):
    texts.append(str(d["output_text"]))
for item in d.get("output") or []:
    if isinstance(item, dict):
        for c in item.get("content") or []:
            if isinstance(c, dict) and c.get("text"):
                texts.append(str(c["text"]))
for ch in d.get("choices") or []:
    msg = (ch.get("message") or {})
    if msg.get("content"):
        texts.append(str(msg["content"]))
t = " | ".join(texts)
usage = d.get("usage") or {}
tokens = usage.get("input_tokens") or usage.get("prompt_tokens")
marker = None
if isinstance(ins, str):
    marker = ins.lstrip().startswith("[Super-Instruct")
print(f"{label} marker={marker} tokens={tokens} text={t!r} status={d.get('status')} err={d.get('error')}")
if isinstance(ins, str):
    print(f"{label} ins_prefix={ins[:60]!r} ins_len={len(ins)}")
PY
}

echo "=== A: responses single-turn x5 ==="
for i in 1 2 3 4 5; do
  make_body resp "$PROMPT" > /tmp/body_a.json
  curl -sS -m 90 -o "/tmp/yn$i.json" http://127.0.0.1:8101/v1/responses \
    -H "Authorization: Bearer $KEY" -H 'Content-Type: application/json' \
    --data-binary @/tmp/body_a.json
  extract "/tmp/yn$i.json" "A$i"
done

echo "=== B: multi-turn previous_response_id ==="
make_body mt1 "$PROMPT" > /tmp/body_mt1.json
curl -sS -m 90 -o /tmp/mt1.json http://127.0.0.1:8101/v1/responses \
  -H "Authorization: Bearer $KEY" -H 'Content-Type: application/json' \
  --data-binary @/tmp/body_mt1.json
RID=$(python3 -c "import json;print(json.load(open('/tmp/mt1.json',encoding='utf-8')).get('id') or '')")
echo "rid=$RID"
extract /tmp/mt1.json mt1
make_body mt2 "$PROMPT" "$RID" > /tmp/body_mt2.json
curl -sS -m 90 -o /tmp/mt2.json http://127.0.0.1:8101/v1/responses \
  -H "Authorization: Bearer $KEY" -H 'Content-Type: application/json' \
  --data-binary @/tmp/body_mt2.json
extract /tmp/mt2.json mt2

echo "=== C: chat completions x3 ==="
for i in 1 2 3; do
  make_body chat "$PROMPT" > /tmp/body_c.json
  curl -sS -m 90 -o "/tmp/cyn$i.json" http://127.0.0.1:8101/v1/chat/completions \
    -H "Authorization: Bearer $KEY" -H 'Content-Type: application/json' \
    --data-binary @/tmp/body_c.json
  extract "/tmp/cyn$i.json" "C$i"
done

echo "=== D: responses no client instructions ==="
make_body resp_noins "$PROMPT" > /tmp/body_d.json
curl -sS -m 90 -o /tmp/noins.json http://127.0.0.1:8101/v1/responses \
  -H "Authorization: Bearer $KEY" -H 'Content-Type: application/json' \
  --data-binary @/tmp/body_d.json
extract /tmp/noins.json D1

echo "=== E: account/key still SI ==="
docker exec sub2api-postgres psql -U sub2api -d sub2api -t -A -c "select id,status,schedulable,extra->>'super_instruct' from accounts where id=2937"
docker logs --since 5m sub2api-canary 2>&1 | grep -E 'api_key_id.: 190' | grep account_id | tail -8 || true
