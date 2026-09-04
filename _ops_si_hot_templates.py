#!/usr/bin/env python3
"""Hot-sync Super-Instruct bridge + templates only (no image rebuild)."""
from pathlib import Path
import hashlib
import os
import sys
import time

import paramiko

sys.stdout.reconfigure(encoding="utf-8", errors="replace")

HOST = "156.238.254.8"
USER = "root"
PASSWORD = os.environ.get("SUB2API_SSH_PASSWORD")
BASE = Path(r"C:/Projects/VPS/速维云-美国-8H8G-三网/sub2api-hixz12-main")
REMOTE_SRC = "/opt/sub2api/source-main"
REMOTE_BRIDGE_DIR = "/opt/sub2api/data/super-instruct"
REMOTE_WEBUI = "/opt/super-instruct-webui/data/bridge.md"

FILES = [
    ("deploy/super-instruct-bridge.md", f"{REMOTE_SRC}/deploy/super-instruct-bridge.md"),
    ("deploy/super-instruct/SKILL_TEMPLATE.md", f"{REMOTE_SRC}/deploy/super-instruct/SKILL_TEMPLATE.md"),
    ("deploy/super-instruct/REVIEW_CARD.md", f"{REMOTE_SRC}/deploy/super-instruct/REVIEW_CARD.md"),
    ("deploy/super-instruct/PHASE_PROTOCOL.md", f"{REMOTE_SRC}/deploy/super-instruct/PHASE_PROTOCOL.md"),
    ("deploy/super-instruct/ACTIVE_STATE.md", f"{REMOTE_SRC}/deploy/super-instruct/ACTIVE_STATE.md"),
    ("_ops_si_cloud_audit_deploy.sh", f"{REMOTE_SRC}/_ops_si_cloud_audit_deploy.sh"),
]


def ensure_dir(sftp: paramiko.SFTPClient, remote: str) -> None:
    parts = remote.strip("/").split("/")
    cur = ""
    for p in parts:
        cur += "/" + p
        try:
            sftp.stat(cur)
        except FileNotFoundError:
            try:
                sftp.mkdir(cur)
            except OSError:
                pass


def main() -> int:
    if not PASSWORD:
        print("SUB2API_SSH_PASSWORD is required", file=sys.stderr)
        return 2
    for rel, _ in FILES:
        local = BASE / rel
        if not local.is_file():
            print("MISSING", local)
            return 1

    ssh = paramiko.SSHClient()
    ssh.set_missing_host_key_policy(paramiko.AutoAddPolicy())
    ssh.connect(HOST, username=USER, password=PASSWORD, timeout=30)
    sftp = ssh.open_sftp()

    for rel, remote in FILES:
        local = BASE / rel
        data = local.read_bytes().replace(b"\r\n", b"\n")
        ensure_dir(sftp, str(Path(remote).as_posix().rsplit("/", 1)[0]))
        with sftp.file(remote, "wb") as f:
            f.write(data)
        digest = hashlib.md5(data).hexdigest()
        print(f"uploaded {rel} bytes={len(data)} md5={digest}")

    sftp.chmod(f"{REMOTE_SRC}/_ops_si_cloud_audit_deploy.sh", 0o700)
    sftp.close()

    cmd = r'''
set -euo pipefail
SRC=/opt/sub2api/source-main
B=/opt/sub2api/data/super-instruct
mkdir -p "$B/templates"
# backup
ts=$(date +%Y%m%d-%H%M%S)
cp -a "$B/bridge.md" "/tmp/bridge.bak.$ts" 2>/dev/null || true
cp -a "$SRC/deploy/super-instruct-bridge.md" "$B/bridge.md"
cp -a "$SRC/deploy/super-instruct/SKILL_TEMPLATE.md" "$B/templates/SKILL_TEMPLATE.md"
cp -a "$SRC/deploy/super-instruct/REVIEW_CARD.md" "$B/templates/REVIEW_CARD.md"
cp -a "$SRC/deploy/super-instruct/PHASE_PROTOCOL.md" "$B/templates/PHASE_PROTOCOL.md"
cp -a "$SRC/deploy/super-instruct/ACTIVE_STATE.md" "$B/templates/ACTIVE_STATE.md"
if [[ -d /opt/super-instruct-webui/data ]]; then
  cp -a "$B/bridge.md" /opt/super-instruct-webui/data/bridge.md || true
fi
# force mtime for hot reload
touch "$B/bridge.md"
sleep 1
echo '=== bridge head ==='
head -n 25 "$B/bridge.md"
echo '=== markers ==='
grep -n 'execution-first\|ACTIVE_STATE\|Blueprint\|双阶段' "$B/bridge.md" | head -20
echo '=== templates ==='
ls -la "$B/templates"
md5sum "$B/bridge.md" "$B/templates"/*
# container view
docker exec sub2api head -n 20 /app/data/super-instruct/bridge.md
docker exec sub2api ls -la /app/data/super-instruct/templates
docker exec sub2api-canary head -n 8 /app/data/super-instruct/bridge.md || true
# keep settings off
docker exec sub2api-postgres psql -U sub2api -d sub2api -t -A -c \
  "UPDATE settings SET value='false', updated_at=NOW() WHERE key='cyber_session_block_enabled'; SELECT key,value FROM settings WHERE key LIKE 'cyber_session_block%';"
# InsTest both ports
KEY=$(docker exec sub2api-postgres psql -U sub2api -d sub2api -t -A -c \
  "select key from api_keys where id=190 and deleted_at is null limit 1" | tr -d '\r\n ')
python3 - <<'P' > /tmp/body_instest.json
import json
print(json.dumps({
  "model":"gpt-5.4",
  "instructions":"ONLY_CLIENT_TEXT",
  "input":"InsTest",
  "stream":False,
  "max_output_tokens":32
}, ensure_ascii=False))
P
ok=0
for port in 8100 8101; do
  curl -sS -m 90 -o "/tmp/it_${port}.json" "http://127.0.0.1:${port}/v1/responses" \
    -H "Authorization: Bearer $KEY" -H 'Content-Type: application/json' \
    --data-binary @/tmp/body_instest.json || true
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
phase = ("执行纪律" in ins) or ("ACTIVE_STATE" in ins) or ("Blueprint" in ins) or ("会话相位" in ins)
exact = (t == "Any code has superpowers")
print(f"port=${port} phase_markers={phase} exact={exact} text={t!r} ins_has_exec={'执行纪律' in ins} ins_has_active={'ACTIVE_STATE' in ins}")
open(f"/tmp/ok_${port}", "w").write("1" if exact and phase else "0")
PY
  if grep -q 1 "/tmp/ok_${port}"; then ok=$((ok+1)); fi
done
echo "INSTEST_OK=$ok/2"
curl -sS -m 8 -o /dev/null -w 'prod=%{http_code}\n' --resolve sub2api.xiaozhudf2026.foo:443:127.0.0.1 https://sub2api.xiaozhudf2026.foo/health
[[ "$ok" == "2" ]]
'''
    print("running remote hot sync...")
    _i, o, e = ssh.exec_command(cmd, timeout=180)
    out = o.read().decode("utf-8", "replace")
    err = e.read().decode("utf-8", "replace")
    code = o.channel.recv_exit_status()
    print(out)
    if err.strip():
        print("ERR", err[-2000:])
    print("exit", code)
    ssh.close()
    return code


if __name__ == "__main__":
    raise SystemExit(main())
