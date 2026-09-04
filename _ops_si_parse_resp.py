#!/usr/bin/env python3
import json
from pathlib import Path

raw = Path("/tmp/si_resp.json").read_text(encoding="utf-8", errors="replace")
print("bytes", len(raw))
print(raw[:2500])
try:
    d = json.loads(raw)
except Exception as e:
    print("json err", e)
    raise SystemExit(1)

texts = []
if isinstance(d, dict):
    if d.get("output_text"):
        texts.append(str(d.get("output_text")))
    for item in d.get("output") or []:
        if not isinstance(item, dict):
            continue
        for c in item.get("content") or []:
            if isinstance(c, dict) and c.get("text"):
                texts.append(str(c["text"]))
    for ch in d.get("choices") or []:
        msg = ch.get("message") or {}
        if msg.get("content"):
            texts.append(str(msg["content"]))
    for k in ("status", "model", "id"):
        if k in d:
            print(k, d.get(k))
    if d.get("error"):
        print("error", d.get("error"))

joined = " | ".join(texts)
print("EXTRACTED:", joined[:800])
if "FOUND_CANARY" in joined:
    print("RESULT=FOUND_CANARY")
elif "NO_CANARY" in joined:
    print("RESULT=NO_CANARY")
else:
    print("RESULT=UNKNOWN")
