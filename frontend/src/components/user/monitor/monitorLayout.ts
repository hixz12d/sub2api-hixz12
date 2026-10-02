/** 渠道行与表头共用的桌面端列宽：图标 | 名称 | 状态 | 延迟 | 可用率 | 时间轴 */
export const MONITOR_ROW_GRID = 'md:grid-cols-[20px_minmax(0,1fr)_88px_84px_96px_minmax(0,2.4fr)]'

/** 可用率分档：≥95% 正常，80–95% 提醒，<80% 异常 */
export function availabilityClass(pct: number | null): string {
  if (pct == null || Number.isNaN(pct)) return 'text-ink-3'
  if (pct >= 95) return 'text-ok'
  if (pct >= 80) return 'text-warn'
  return 'text-bad'
}

/** 百分比去掉多余的小数 0：100 → "100"，99.5 → "99.5"，99.823 → "99.82" */
export function formatAvailabilityPct(pct: number | null): string | null {
  if (pct == null || Number.isNaN(pct)) return null
  return String(Number(pct.toFixed(2)))
}

/** 纯配额模式的主模型是占位符 "quota"，没有真实对话延迟 */
export const QUOTA_MODEL_PLACEHOLDER = 'quota'
