<template>
  <AppLayout>
    <div class="mx-auto max-w-[1320px]">
      <HomeHero
        class="tile-glide"
        style="--n: 0"
        :name="displayName"
        :today-requests="stats?.today_requests ?? 0"
        :today-cost="stats?.today_actual_cost ?? 0"
      />

      <!-- 1 + 2：钱包磁贴 + 快捷操作（没有 Key 时换成三步引导） -->
      <div class="tile-grid mt-6 grid-cols-2 lg:mt-7 lg:grid-cols-[2fr_1fr_1fr_1fr]">
        <div class="tile-glide col-span-2 min-h-[150px] lg:col-span-1 lg:min-h-[172px]" style="--n: 1">
          <HomeWalletCard
            :balance="balance"
            :frozen="frozen"
            :today-cost="stats?.today_actual_cost ?? 0"
            :week-cost="weekCost"
            :wallet-path="walletPath"
            :loading="statsLoading && !stats"
          />
        </div>
        <HomeQuickActions
          class="tile-glide"
          style="--n: 2"
          :onboarding="isNewUser"
          :can-recharge="canRecharge"
          :wallet-path="walletPath"
          :endpoint="endpoint"
        />
      </div>
      <HomeBlockError v-if="statsError" class="mt-2" @retry="loadStats" />

      <!-- 3 + 4：用量数据 + 趋势 -->
      <section class="mt-10 lg:mt-12">
        <div class="mb-3.5 flex flex-wrap items-end justify-between gap-4">
          <h2 class="text-[26px] font-light leading-none tracking-[-0.02em] text-ink lg:text-[30px]">{{ t('userHome.usage.title') }}</h2>
          <div class="flex gap-1" role="group" :aria-label="t('userHome.usage.title')">
            <button
              v-for="p in periods"
              :key="p"
              type="button"
              class="h-9 px-4 text-sm font-semibold transition-colors"
              :class="period === p ? 'bg-money text-money-ink' : 'bg-surface-tile text-ink-2 hover:bg-surface-tile-2 hover:text-ink'"
              :aria-pressed="period === p"
              @click="setPeriod(p)"
            >
              {{ t(`userHome.usage.periods.${p}`) }}
            </button>
          </div>
        </div>
        <HomeBlockError v-if="snapshotError" class="mb-2" @retry="loadSnapshot" />
        <HomeStatGrid
          :requests="totals.requests"
          :tokens="totals.tokens"
          :cost="totals.cost"
          :speed="speed"
          :loading="snapshotLoading && !snapshot"
          :speed-loading="logsLoading && !logs.length"
        />
        <HomeTrendChart class="mt-2" :buckets="buckets" :loading="snapshotLoading && !snapshot" />
      </section>

      <!-- 5：按分组明细 -->
      <section class="mt-10 lg:mt-12">
        <h2 class="mb-3.5 text-[26px] font-light leading-none tracking-[-0.02em] text-ink lg:text-[30px]">{{ t('userHome.groups.title') }}</h2>
        <HomeGroupTable
          v-if="!snapshotError"
          :groups="snapshot?.groups ?? []"
          :loading="snapshotLoading && !snapshot"
          :color-of="groupColor"
        />
      </section>

      <!-- 6：最近 5 条使用记录 -->
      <section class="mt-10 lg:mt-12">
        <div class="mb-3.5 flex items-end justify-between gap-4">
          <h2 class="text-[26px] font-light leading-none tracking-[-0.02em] text-ink lg:text-[30px]">{{ t('userHome.recent.title') }}</h2>
          <router-link to="/usage" class="inline-flex items-center gap-1.5 text-[15px] font-semibold text-ink-2 transition-colors hover:text-ink">
            {{ t('userHome.recent.viewAll') }}
            <Icon name="arrowRight" size="sm" />
          </router-link>
        </div>
        <HomeBlockError v-if="logsError" @retry="loadLogs" />
        <HomeRecentUsage
          v-else
          :logs="logs.slice(0, 5)"
          :loading="logsLoading && !logs.length"
          :color-of="groupColor"
          :group-names="groupNames"
        />
      </section>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import HomeHero from '@/components/user/home/HomeHero.vue'
import HomeWalletCard from '@/components/user/home/HomeWalletCard.vue'
import HomeQuickActions from '@/components/user/home/HomeQuickActions.vue'
import HomeStatGrid from '@/components/user/home/HomeStatGrid.vue'
import HomeTrendChart, { type HomeTrendBucket } from '@/components/user/home/HomeTrendChart.vue'
import HomeGroupTable from '@/components/user/home/HomeGroupTable.vue'
import HomeRecentUsage from '@/components/user/home/HomeRecentUsage.vue'
import HomeBlockError from '@/components/user/home/HomeBlockError.vue'
import { medianTokensPerSecond, type HomePeriod } from '@/components/user/home/homeFormat'
import { usageAPI, type UserDashboardStats, type UsageDashboardSnapshotV2Response } from '@/api/usage'
import { useAppStore, useAuthStore } from '@/stores'
import { usePaymentStore } from '@/stores/payment'
import { FeatureFlags, isFeatureFlagEnabled } from '@/utils/featureFlags'
import { getExternalRechargeEntries } from '@/utils/externalRecharge'
import { formatDateLocalInput, getBrowserTimeZone } from '@/utils/format'
import type { UsageLog } from '@/types'

const { t } = useI18n()
const authStore = useAuthStore()
const appStore = useAppStore()
const paymentStore = usePaymentStore()

const periods: HomePeriod[] = ['today', 'week', 'month']
const period = ref<HomePeriod>('today')

const user = computed(() => authStore.user)
const displayName = computed(() => user.value?.username || user.value?.email?.split('@')[0] || '')
const balance = computed(() => Number(user.value?.balance || 0))
const frozen = computed(() => Number(user.value?.frozen_balance || 0))

const endpoint = computed(() => appStore.cachedPublicSettings?.api_base_url || window.location.origin)

const canRecharge = computed(() =>
  (isFeatureFlagEnabled(FeatureFlags.payment) && paymentStore.purchaseEntryAvailable) ||
  getExternalRechargeEntries(appStore.cachedPublicSettings?.custom_menu_items).length > 0
)
const walletPath = computed(() => (canRecharge.value ? '/wallet/recharge' : '/wallet'))

// ---------- 今日概况 + Key 数量 ----------
const stats = ref<UserDashboardStats | null>(null)
const statsLoading = ref(false)
const statsError = ref(false)
const isNewUser = computed(() => stats.value != null && stats.value.total_api_keys === 0)

async function loadStats() {
  statsLoading.value = true
  statsError.value = false
  try {
    const [res] = await Promise.all([usageAPI.getDashboardStats(), authStore.refreshUser().catch(() => undefined)])
    stats.value = res
  } catch (error) {
    console.error('Failed to load dashboard stats:', error)
    statsError.value = true
  } finally {
    statsLoading.value = false
  }
}

// ---------- 时间段 ----------
function rangeOf(p: HomePeriod) {
  const end = new Date()
  const days = p === 'today' ? 0 : p === 'week' ? 6 : 29
  const start = new Date(end.getTime() - days * 86400000)
  return { start_date: formatDateLocalInput(start), end_date: formatDateLocalInput(end) }
}

// ---------- 趋势 + 分组（snapshot-v2） ----------
const snapshot = ref<UsageDashboardSnapshotV2Response | null>(null)
const snapshotLoading = ref(false)
const snapshotError = ref(false)
let snapshotSeq = 0

async function loadSnapshot() {
  const seq = ++snapshotSeq
  snapshotLoading.value = true
  snapshotError.value = false
  try {
    const res = await usageAPI.getDashboardSnapshotV2({
      ...rangeOf(period.value),
      granularity: period.value === 'today' ? 'hour' : 'day',
      timezone: getBrowserTimeZone(),
      include_trend: true,
      include_model_stats: false,
      include_group_stats: true
    })
    if (seq !== snapshotSeq) return
    snapshot.value = res
  } catch (error) {
    if (seq !== snapshotSeq) return
    console.error('Failed to load dashboard snapshot:', error)
    snapshotError.value = true
    snapshot.value = null
  } finally {
    if (seq === snapshotSeq) snapshotLoading.value = false
  }
}

const totals = computed(() => {
  const trend = snapshot.value?.trend ?? []
  return trend.reduce(
    (acc, p) => ({ requests: acc.requests + p.requests, tokens: acc.tokens + p.total_tokens, cost: acc.cost + p.actual_cost }),
    { requests: 0, tokens: 0, cost: 0 }
  )
})

// 补齐空档：今天按 0–23 时，7/30 天按日期
const buckets = computed((): HomeTrendBucket[] => {
  const trend = snapshot.value?.trend ?? []
  const byKey = new Map(trend.map((p) => [p.date, p]))
  const pad = (n: number) => String(n).padStart(2, '0')
  if (period.value === 'today') {
    const day = formatDateLocalInput(new Date())
    const nowHour = new Date().getHours()
    // 还没到的小时留空，不画成 0
    return Array.from({ length: 24 }, (_, h) => {
      const p = byKey.get(`${day} ${pad(h)}:00`)
      if (h > nowHour) return { label: pad(h), tokens: null, cost: null }
      return { label: pad(h), tokens: p?.total_tokens ?? 0, cost: p?.actual_cost ?? 0 }
    })
  }
  const days = period.value === 'week' ? 7 : 30
  const now = Date.now()
  return Array.from({ length: days }, (_, i) => {
    const d = new Date(now - (days - 1 - i) * 86400000)
    const p = byKey.get(formatDateLocalInput(d))
    return { label: `${d.getMonth() + 1}/${d.getDate()}`, tokens: p?.total_tokens ?? 0, cost: p?.actual_cost ?? 0 }
  })
})

// ---------- 近 7 天花费（给“还能用几天”） ----------
const weekCost = ref<number | null>(null)

async function loadWeekCost() {
  try {
    const res = await usageAPI.getDashboardSnapshotV2({
      ...rangeOf('week'),
      granularity: 'day',
      timezone: getBrowserTimeZone(),
      include_trend: true,
      include_model_stats: false,
      include_group_stats: false
    })
    weekCost.value = (res.trend ?? []).reduce((sum, p) => sum + p.actual_cost, 0)
  } catch (error) {
    console.error('Failed to load 7-day spend:', error)
    weekCost.value = null
  }
}

// ---------- 使用记录（平均速度 + 最近 5 条共用） ----------
const logs = ref<UsageLog[]>([])
const logsLoading = ref(false)
const logsError = ref(false)
let logsSeq = 0

async function loadLogs() {
  const seq = ++logsSeq
  logsLoading.value = true
  logsError.value = false
  try {
    const res = await usageAPI.query({
      ...rangeOf(period.value),
      timezone: getBrowserTimeZone(),
      page: 1,
      page_size: 200,
      sort_by: 'created_at',
      sort_order: 'desc'
    })
    if (seq !== logsSeq) return
    logs.value = res.items ?? []
  } catch (error) {
    if (seq !== logsSeq) return
    console.error('Failed to load usage logs:', error)
    logsError.value = true
    logs.value = []
  } finally {
    if (seq === logsSeq) logsLoading.value = false
  }
}

const speed = computed(() => medianTokensPerSecond(logs.value))

// ---------- 分组识别色与名称（分组表和最近使用共用） ----------
const SERIES = ['bg-series-1', 'bg-series-2', 'bg-series-3', 'bg-series-4', 'bg-series-5']
const groupOrder = computed(() =>
  [...(snapshot.value?.groups ?? [])].sort((a, b) => (b.actual_cost ?? 0) - (a.actual_cost ?? 0)).map((g) => g.group_id)
)
function groupColor(groupId: number): string {
  const idx = groupOrder.value.indexOf(groupId)
  return SERIES[(idx >= 0 ? idx : Math.abs(groupId)) % SERIES.length]
}
const groupNames = computed(() =>
  Object.fromEntries((snapshot.value?.groups ?? []).map((g) => [g.group_id, g.group_name]))
)

function setPeriod(p: HomePeriod) {
  if (period.value === p) return
  period.value = p
  void loadSnapshot()
  void loadLogs()
}

onMounted(() => {
  if (isFeatureFlagEnabled(FeatureFlags.payment)) void paymentStore.fetchConfig()
  void loadStats()
  void loadSnapshot()
  void loadWeekCost()
  void loadLogs()
})
</script>
