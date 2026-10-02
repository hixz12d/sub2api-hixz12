// 新仪表盘共用的格式化与计算
import type { GroupStat, UsageLog } from '@/types'
import { usageTokensPerSecond } from '@/utils/usageThroughput'

export type HomePeriod = 'today' | 'week' | 'month'

// 站内金额沿用 $ 计价（与顶栏余额一致）
export function formatHomeMoney(value: number | null | undefined, digits = 2): string {
  const v = Number(value ?? 0)
  if (!Number.isFinite(v)) return '$0.00'
  if (v !== 0 && Math.abs(v) < 0.01) return `$${v.toFixed(4)}`
  return `$${v.toFixed(digits)}`
}

// 1.2K / 3.4M，整数位不带多余的 .0
export function formatHomeTokens(value: number | null | undefined): string {
  const v = Number(value ?? 0)
  const trim = (s: string) => s.replace(/\.0$/, '')
  if (v >= 1e9) return `${trim((v / 1e9).toFixed(1))}B`
  if (v >= 1e6) return `${trim((v / 1e6).toFixed(1))}M`
  if (v >= 1e3) return `${trim((v / 1e3).toFixed(v >= 1e5 ? 0 : 1))}K`
  return String(Math.round(v))
}

export function formatHomeCount(value: number | null | undefined): string {
  return Math.round(Number(value ?? 0)).toLocaleString('en-US')
}

const MIN_HIT_RATE_REQUESTS = 10

// 缓存命中率 = 读缓存 ÷（普通输入 + 读缓存 + 写缓存）；请求太少或分母为 0 时返回 null
export function groupCacheHitRate(stat: GroupStat): number | null {
  if ((stat.requests ?? 0) < MIN_HIT_RATE_REQUESTS) return null
  const read = stat.cache_read_tokens ?? 0
  const denom = (stat.input_tokens ?? 0) + read + (stat.cache_creation_tokens ?? 0)
  if (denom <= 0) return null
  return read / denom
}

const MIN_SPEED_SAMPLES = 5

// 平均速度：有效样本的中位数；样本不足时返回 null
export function medianTokensPerSecond(logs: UsageLog[]): number | null {
  const values = logs
    .map((log) => usageTokensPerSecond(log))
    .filter((v): v is number => v != null && Number.isFinite(v))
    .sort((a, b) => a - b)
  if (values.length < MIN_SPEED_SAMPLES) return null
  const mid = Math.floor(values.length / 2)
  return values.length % 2 ? values[mid] : (values[mid - 1] + values[mid]) / 2
}

// 读取主题 CSS 变量（RGB 三元组）给 Chart.js 用
export function themeColor(name: string, alpha = 1): string {
  const raw = getComputedStyle(document.documentElement).getPropertyValue(`--c-${name}`).trim()
  const parts = raw.split(/\s+/).map(Number)
  if (parts.length !== 3 || parts.some((n) => !Number.isFinite(n))) return `rgba(128, 128, 128, ${alpha})`
  return `rgba(${parts[0]}, ${parts[1]}, ${parts[2]}, ${alpha})`
}
