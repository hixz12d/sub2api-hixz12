/**
 * 重置卡（消费返还卡）用户端状态。
 *
 * 只在新版界面、且后台开启 reset_card_enabled 时工作；不启用时所有方法都不发请求。
 * 拉取时机由 App 决定（登录后、切页时调用 fetch），退出登录时调用 reset。
 */

import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { resetCardsAPI } from '@/api/resetCards'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'
import { isClassicUi } from '@/utils/uiStyle'
import type {
  ResetCard,
  ResetCardErrorReason,
  ResetCardEstimate,
  ResetCardUseResponse
} from '@/types/resetCard'

const THROTTLE_MS = 60 * 1000
const DAY_MS = 24 * 60 * 60 * 1000

// 与顶栏、钱包余额卡同一个货币符号：返还的钱直接进余额，单位必须一致
export const RESET_CARD_CURRENCY = '$'

/** 金额统一保留两位小数 */
export function formatResetMoney(value: number | null | undefined): string {
  const n = Number(value)
  return `${RESET_CARD_CURRENCY}${(Number.isFinite(n) ? n : 0).toFixed(2)}`
}

/** 上限这类"规则数字"：整数不带小数（最高 $50），否则两位小数 */
export function formatResetLimit(value: number | null | undefined): string {
  const n = Number(value)
  if (!Number.isFinite(n)) return `${RESET_CARD_CURRENCY}0`
  return Number.isInteger(n) ? `${RESET_CARD_CURRENCY}${n}` : formatResetMoney(n)
}

/** 比例：30 → 30，12.5 → 12.5 */
export function formatResetPercent(value: number | null | undefined): string {
  const n = Number(value)
  if (!Number.isFinite(n)) return '0'
  return String(Math.round(n * 100) / 100)
}

/**
 * 距离到期还差几个日历日（按本地日期算）：0 = 今天到期，1 = 明天到期；已过期返回 -1。
 */
export function daysUntil(iso: string | null | undefined, now = Date.now()): number {
  if (!iso) return -1
  const end = new Date(iso)
  if (Number.isNaN(end.getTime()) || end.getTime() <= now) return -1
  const startOfDay = (d: Date) => new Date(d.getFullYear(), d.getMonth(), d.getDate()).getTime()
  // round 消除夏令时带来的 ±1 小时
  return Math.round((startOfDay(end) - startOfDay(new Date(now))) / DAY_MS)
}

/** "11 月 21 日" / "Nov 21" */
export function formatResetMonthDay(iso: string | null | undefined, locale: string): string {
  if (!iso) return ''
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return ''
  return new Intl.DateTimeFormat(locale.startsWith('zh') ? 'zh-CN' : 'en-US', {
    month: 'short',
    day: 'numeric'
  }).format(d)
}

/** 从接口错误里取出重置卡业务错误码（放在 reason 字段） */
export function resetCardErrorReason(err: unknown): ResetCardErrorReason | string | undefined {
  if (err && typeof err === 'object' && 'reason' in err) {
    const reason = (err as { reason?: unknown }).reason
    return typeof reason === 'string' ? reason : undefined
  }
  return undefined
}

export const useResetCardsStore = defineStore('resetCards', () => {
  const appStore = useAppStore()
  const authStore = useAuthStore()

  // State
  const cards = ref<ResetCard[]>([])
  const unusedCount = ref(0)
  const limitedCreditRemaining = ref(0)
  const popupCardIds = ref<number[]>([])
  const loading = ref(false)
  const loaded = ref(false)
  const error = ref(false)

  let lastFetchTime = 0
  let fetchGeneration = 0
  // 本次会话已标记过弹窗的卡：接口失败或列表还没刷新时，也不再重复弹
  let seenIds = new Set<number>()

  // 经典界面不提供任何重置卡入口；isClassicUi 只在整页刷新时变化，读一次即可
  const classicUi = isClassicUi()
  const enabled = computed(
    () => appStore.cachedPublicSettings?.reset_card_enabled === true && !classicUi
  )

  /** 生效中、还有剩余的限时额度里，最早的到期时间（钱包余额拆分显示用） */
  const limitedEarliestExpiresAt = computed<string | null>(() => {
    let earliest: string | null = null
    for (const card of cards.value) {
      const credit = card.credit
      if (!credit || credit.status !== 'active' || credit.remaining_amount <= 0) continue
      if (!earliest || new Date(credit.expires_at) < new Date(earliest)) earliest = credit.expires_at
    }
    return earliest
  })

  // Actions
  async function fetch(force = false): Promise<void> {
    if (!enabled.value) return
    const now = Date.now()
    if (!force && lastFetchTime > 0 && now - lastFetchTime < THROTTLE_MS) return

    // 先占住时间戳，避免并发重复请求
    lastFetchTime = now
    const generation = ++fetchGeneration
    loading.value = true
    try {
      const data = await resetCardsAPI.list()
      if (generation !== fetchGeneration) return
      cards.value = data.cards ?? []
      unusedCount.value = Number(data.unused_count) || 0
      limitedCreditRemaining.value = Number(data.limited_credit_remaining) || 0
      popupCardIds.value = (data.popup_card_ids ?? []).filter((id) => !seenIds.has(id))
      loaded.value = true
      error.value = false
    } catch (err) {
      if (generation !== fetchGeneration) return
      // 失败时允许下次立即重试
      lastFetchTime = 0
      error.value = true
      console.error('Failed to fetch reset cards:', err)
    } finally {
      if (generation === fetchGeneration) loading.value = false
    }
  }

  /** 实时重算一张可用卡的预估，并写回列表里的这张卡 */
  async function preview(id: number): Promise<ResetCardEstimate> {
    const estimate = await resetCardsAPI.preview(id)
    const card = cards.value.find((c) => c.id === id)
    if (card && card.status === 'unused') card.estimate = estimate
    return estimate
  }

  /**
   * 使用卡。成功后在后台刷新列表和用户余额（不等它们完成，卡面动画可以立刻开始）；
   * 失败时原样抛出错误，调用方用 resetCardErrorReason(err) 取错误码（如 RESET_CARD_NOTHING_TO_REFUND）。
   */
  async function use(id: number): Promise<ResetCardUseResponse> {
    const result = await resetCardsAPI.use(id)
    void Promise.allSettled([fetch(true), authStore.refreshUser()])
    return result
  }

  /** 标记已弹过窗：本地立即移除，接口失败也不再在本次会话里重复弹 */
  async function markPopupSeen(ids: number[]): Promise<void> {
    const unique = [...new Set(ids)].filter((id) => !seenIds.has(id))
    if (unique.length === 0) return
    unique.forEach((id) => seenIds.add(id))
    popupCardIds.value = popupCardIds.value.filter((id) => !seenIds.has(id))
    try {
      await resetCardsAPI.markPopupSeen(unique)
    } catch (err) {
      console.error('Failed to mark reset card popup as seen:', err)
    }
  }

  function reset(): void {
    fetchGeneration++
    cards.value = []
    unusedCount.value = 0
    limitedCreditRemaining.value = 0
    popupCardIds.value = []
    loading.value = false
    loaded.value = false
    error.value = false
    lastFetchTime = 0
    seenIds = new Set()
  }

  return {
    // State
    cards,
    unusedCount,
    limitedCreditRemaining,
    popupCardIds,
    loading,
    loaded,
    error,
    // Getters
    enabled,
    limitedEarliestExpiresAt,
    // Actions
    fetch,
    preview,
    use,
    markPopupSeen,
    reset
  }
})
