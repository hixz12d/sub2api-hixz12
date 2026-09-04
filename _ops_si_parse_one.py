#!/usr/bin/env python3
import json
import sys
from pathlib import Path

path = Path(sys.argv[1] if len(sys.argv) > 1 else "/tmp/si_resp.json")
raw = path.read_text(encoding="utf-8", errors="replace") if path.exists() else ""
print("bytes", len(raw))
try:
    d = json.loads(raw)
except Exception:
    print(raw[:1000])
    raise SystemExit(1)

ins = d.get("instructions")
if isinstance(ins, str):
    print("instructions_prefix:", repr(ins[:100]))
    print("has_marker", "[Super-Instruct" in ins)
    print("has_client", ("ONLY_CLIENT_TEXT" in ins) or ("CLIENT_INSTRUCTIONS" in ins))
    print("starts_with_bridge", ins.lstrip().startswith("[Super-Instruct") or ins.lstrip().startswith("CANARY_MARKER"))
else:
    print("instructions_type", type(ins).__name__, "present", "instructions" in d)

texts = []
if d.get("output_text"):
    texts.append(str(d.get("output_text")))
for item in d.get("output") or []:
    if isinstance(item, dict):
        for c in item.get("content") or []:
            if isinstance(c, dict) and c.get("text"):
                texts.append(str(c["text"]))
for ch in d.get("choices") or []:
    msg = ch.get("message") or {}
    if msg.get("content"):
        texts.append(str(msg["content"]))
if texts:
    print("text", " | ".join(texts)[:300])
if d.get("error"):
    print("error", d.get("error"))
for k in ("status", "model", "id"):
    if k in d:
        print(k, d.get(k))
