from pathlib import Path
import paramiko
import os
import sys

sys.stdout.reconfigure(encoding="utf-8", errors="replace")
base = Path(r"C:/Projects/VPS/速维云-美国-8H8G-三网/sub2api-hixz12-main")
password = os.environ.get("SUB2API_SSH_PASSWORD")
if not password:
    raise RuntimeError("SUB2API_SSH_PASSWORD is required")

ssh = paramiko.SSHClient()
ssh.set_missing_host_key_policy(paramiko.AutoAddPolicy())
ssh.connect("156.238.254.8", username="root", password=password, timeout=30)
sftp = ssh.open_sftp()


def ensure_dir(remote: str) -> None:
    parts = remote.strip("/").split("/")
    cur = ""
    for p in parts:
        cur += "/" + p
        try:
            sftp.stat(cur)
        except FileNotFoundError:
            try:
                sftp.mkdir(cur)
            except Exception:
                pass


files = [
    "deploy/super-instruct-bridge.md",
    "deploy/super-instruct/SKILL_TEMPLATE.md",
    "deploy/super-instruct/REVIEW_CARD.md",
    "deploy/super-instruct/PHASE_PROTOCOL.md",
    "deploy/super-instruct/ACTIVE_STATE.md",
    "backend/internal/handler/openai_gateway_handler.go",
    "backend/internal/service/openai_gateway_upstream_errors.go",
    "backend/internal/service/openai_cyber_policy.go",
    "_ops_si_cloud_audit_deploy.sh",
]
remote_root = "/opt/sub2api/source-main"
for rel in files:
    local = base / rel
    remote = remote_root + "/" + rel.replace("\\", "/")
    ensure_dir(str(Path(remote).parent).replace("\\", "/"))
    data = local.read_bytes().replace(b"\r\n", b"\n")
    with sftp.file(remote, "wb") as f:
        f.write(data)
    print("uploaded", rel, "bytes", len(data))

sftp.chmod(remote_root + "/_ops_si_cloud_audit_deploy.sh", 0o700)
sftp.close()

print("starting deploy...")
_stdin, stdout, stderr = ssh.exec_command(
    "bash /opt/sub2api/source-main/_ops_si_cloud_audit_deploy.sh", timeout=1200
)
out = stdout.read().decode("utf-8", "replace")
err = stderr.read().decode("utf-8", "replace")
print(out[-12000:])
if err.strip():
    print("ERR", err[-3000:])
print("exit", stdout.channel.recv_exit_status())
ssh.close()
