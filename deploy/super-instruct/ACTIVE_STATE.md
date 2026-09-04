# ACTIVE_STATE（有界 checkpoint · 无密钥）

> 本文件是 **会话压缩 / 中断 / 重开** 后的最小续跑状态。  
> 不含 token、cookie、密钥、完整 payload、大段 hex。  
> 权威仍是当前用户请求；本文件只恢复 **已验证进度**，不发明新目标。

## schema

```yaml
phase: COLLECT | DISTILL | EXECUTE
skill_path: skills/<name>/SKILL.md   # 无则空
step: <N>                            # Procedure 下一步（1-based）
last_verify: <短结果或路径>           # 最近一次字面验证
next_action: <一句可执行下一动>
ssot_paths:
  - notes/...
  - raw/...
  - skills/<name>/SKILL.md
  - out/...
updated_at: <ISO8601 或本地时间>
```

## 写入时机（模型必须遵守）
- 完成一个可验证步骤后
- 发出 `[PHASE_DONE]` / `[RECOVER]` / `[DISTILL_DONE]` 前
- 预计将 compact / 窗将尽 / 将喷长文前
- 每窗第 3 个实质 turn 至少更新一次

## 重开第一条（操作员）
```text
读取 notes/ACTIVE_STATE.md 与其 skill_path；从 step 继续；聊天不贴长代码。
```

## 示例
```yaml
phase: EXECUTE
skill_path: skills/sample-re/SKILL.md
step: 3
last_verify: out/verify-step2.log exit=0
next_action: 运行 scripts/step3_patch.py 并写 out/verify-step3.log
ssot_paths:
  - skills/sample-re/SKILL.md
  - skills/sample-re/scripts/step3_patch.py
  - out/verify-step2.log
updated_at: 2026-09-02T00:00:00+08:00
```
