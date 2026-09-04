#!/usr/bin/env bash
set -euo pipefail

echo '--- bridge InsTest line ---'
grep -n 'InsTest\|superpowers' /opt/sub2api/data/super-instruct/bridge.md || echo 'MISSING_IN_BRIDGE'

KEY=$(docker exec sub2api-postgres psql -U sub2api -d sub2api -t -A -c "select key from api_keys where id=190 and deleted_at is null limit 1" | tr -d '\r\n ')

python3 - <<'PY' > /tmp/body_instest.json
import json
print(json.dumps({
    "model": "gpt-5.4",
    "instructions": "ONLY_CLIENT_TEXT",
    "input": "InsTest",
    "stream": False,
    "max_output_tokens": 64,
}, ensure_ascii=False))
PY

python3 - <<'PY' > /tmp/body_instest_chat.json
import json
print(json.dumps({
    "model": "gpt-5.4",
    "messages": [{"role": "user", "content": "InsTest"}],
    "stream": False,
    "max_tokens": 64,
}, ensure_ascii=False))
PY

ok=0
for i in 1 2 3; do
  curl -sS -m 90 -o "/tmp/instest$i.json" http://127.0.0.1:8101/v1/responses \
    -H "Authorization: Bearer $KEY" \
    -H 'Content-Type: application/json' \
    --data-binary @/tmp/body_instest.json
  python3 - <<PY
import json
d=json.load(open("/tmp/instest$i.json", encoding="utf-8"))
ins=d.get("instructions") or ""
texts=[]
if d.get("output_text"):
    texts.append(str(d["output_text"]))
for item in d.get("output") or []:
    if isinstance(item, dict):
        for c in item.get("content") or []:
            if isinstance(c, dict) and c.get("text"):
                texts.append(str(c["text"]))
t=" | ".join(texts)
hit=("Any code has superpowers" in t)
print(f"R$i marker={ins.lstrip().startswith('[Super-Instruct')} tokens={(d.get('usage') or {}).get('input_tokens')} hit={hit} text={t!r}")
open("/tmp/hit$i","w").write("1" if hit else "0")
PY
  if grep -q 1 "/tmp/hit$i"; then ok=$((ok+1)); fi
done
echo "INSTEST_RESP_SCORE=$ok/3"

curl -sS -m 90 -o /tmp/instest_chat.json http://127.0.0.1:8101/v1/chat/completions \
  -H "Authorization: Bearer $KEY" \
  -H 'Content-Type: application/json' \
  --data-binary @/tmp/body_instest_chat.json
python3 - <<'PY'
import json
d=json.load(open("/tmp/instest_chat.json", encoding="utf-8"))
t=((d.get("choices") or [{}])[0].get("message") or {}).get("content")
u=d.get("usage") or {}
print("CHAT tokens", u.get("prompt_tokens"), "hit", "Any code has superpowers" in str(t), "text", repr(t))
PY
