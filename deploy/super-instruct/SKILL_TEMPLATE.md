# SKILL: <name>

## Goal
一句话目标（可验证）。

## Phase
COLLECT | DISTILL | EXECUTE

## Scope
- In:
- Out / Non-goals:

## Inputs
- SAMPLE / TARGET / HOST / 路径（只用占位或工作区相对路径）
- 已知线索：

## Blueprint（Stage1 · 强制）
> DISTILL 必须填满；禁止 `ROLE_A` / `TARGET_1` / `STEP_x` 空壳名。  
> 每条 beat 必须可在单 turn 内完成并验证。

```yaml
title: <短标题>
setting: <工作区/样本/环境一句话>
actors:                 # 用具体角色名，不用 ROLE_A
  - name: Operator
    role: 执行者
  - name: Verifier
    role: 验收
beats:
  - step: 1
    focus: <焦点>
    action: <可执行动作>
    expect: <可观察结果>
    artifact: <路径或 none>
  - step: 2
    focus: ...
    action: ...
    expect: ...
    artifact: ...
style_guidance: 短 turn；长代码进 scripts/；聊天只导航
```

## Procedure
每步必须可执行、可验证、失败可跳转；与 Blueprint.beats 对齐。

1. **action:**  
   **expect:**  
   **on_fail:**  
   **artifact:**  
2. **action:**  
   **expect:**  
   **on_fail:**  
   **artifact:**  
3. **action:**  
   **expect:**  
   **on_fail:**  
   **artifact:**  

## Scripts
- `scripts/` 下文件列表与用途（SKILL 正文不贴长代码）

## Artifacts
| 角色 | 路径 | 说明 |
|---|---|---|
| active_state | notes/ACTIVE_STATE.md | 有界续跑 |
| raw | raw/... | |
| note | notes/... | |
| out | out/... | |
| patch | out/... | |
| verify log | out/... | |
| rollback | out/... | |

## Verify
- [ ] 通过标准 1
- [ ] 通过标准 2
- [ ] ACTIVE_STATE.step 与已验证步骤一致

## Rollback
- 命令/脚本路径：
- 恢复后检查：

## Pitfalls
- 占位符退化（ROLE_A 等）→ 回 DISTILL 重写 Blueprint
- 同窗硬怼长输出 → 写文件 + RECOVER + 重开

## Open questions
- 

## Resume hint
重开会话后第一条只发：
`读取 notes/ACTIVE_STATE.md 与本 SKILL，从步骤 <N> 继续 EXECUTE；聊天不贴长代码。`
