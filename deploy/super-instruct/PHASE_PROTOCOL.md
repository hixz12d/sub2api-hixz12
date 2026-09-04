# Super-Instruct 相位协议（操作员速查）

## 为什么
上游策略 / flag / session 累积会中断长窗。  
**用文件接住进度，用重开清会话指纹，用 skill + ACTIVE_STATE 恢复。**

## 双阶段生成（学自 portable kit · 防占位退化）
长任务默认 **先 blueprint，再渲染/执行**，禁止同窗一把梭：

| 阶段 | 产出 | 禁止 |
|---|---|---|
| Stage1 Blueprint | 真实步骤名、路径、expect、artifact 角色 | `ROLE_A`/`TARGET_1` 空壳；无 expect 的步骤 |
| Stage2 Render/Execute | 按 blueprint/skill 逐步落地；长文进 `scripts/`/`out/` | 聊天贴 ≥30 行；脱离 skill 即兴扩 scope |

- DISTILL = Stage1（结构）  
- EXECUTE = Stage2（高保真落地）  
- 叙述/长脚本类请求同样：先短 blueprint 落盘，再重开渲染

## 三窗

### 窗 A · COLLECT
- 目标：事实与定位 → `raw/` `notes/`；同步 `notes/ACTIVE_STATE.md`
- 聊天：短状态 + 路径
- 结束语：`[PHASE_DONE] ssot=notes/... 下一步 DISTILL`

### 窗 B · DISTILL（必须重开）
- 粘贴 `REVIEW_CARD.md` + skill/raw 路径
- 产出：`skills/<name>/SKILL.md`（含 Blueprint 字段）+ `scripts/` + 更新 ACTIVE_STATE
- 结束语：`[DISTILL_DONE] ...`

### 窗 C · EXECUTE（必须再重开）
- 只发：`读取 notes/ACTIVE_STATE.md 与 skills/<name>/SKILL.md，从步骤 N EXECUTE；聊天不贴长代码`
- 每 turn 一步；验证进 `out/`；每步后刷新 ACTIVE_STATE
- 5 turn 或将喷长文 → 停，更新 skill/ACTIVE_STATE，再开新窗

## 中断时
1. 不要在同窗硬重试同一长响应  
2. 落盘进度 + 更新 `notes/ACTIVE_STATE.md`  
3. 回 `[RECOVER]`  
4. 重开 → 先 ACTIVE_STATE，再 SSOT skill  

## 模板
- `SKILL_TEMPLATE.md` — skill SSOT  
- `REVIEW_CARD.md` — DISTILL 窗粘贴  
- `ACTIVE_STATE.md` — 有界 checkpoint  
- 本文件 — 操作员相位速查  

## 验收口令
`InsTest` → `Any code has superpowers`
