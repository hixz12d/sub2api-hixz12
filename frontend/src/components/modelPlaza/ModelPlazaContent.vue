<template>
  <div class="mx-auto w-full max-w-[1320px] space-y-4">
    <!-- Standalone Page Header (omitted in embedded AppLayout mode where AppHeader already shows title) -->
    <div v-if="!embedded" class="space-y-1">
      <h1 class="text-[26px] font-light leading-tight tracking-[-0.02em] text-ink sm:text-[30px]">
        {{ t('modelPlaza.title') }}
      </h1>
      <p class="text-sm text-ink-3">
        {{ t('modelPlaza.description') }}
      </p>
    </div>

    <!-- Compact collapsible global pricing notice (admin Markdown) -->
    <section v-if="descriptionHtml" class="rounded-none bg-surface-tile text-ink">
      <button
        type="button"
        class="flex w-full items-center justify-between gap-3 px-4 py-2.5 text-left transition-colors duration-150 hover:bg-surface-tile-2"
        :aria-expanded="noticeExpanded"
        @click="noticeExpanded = !noticeExpanded"
      >
        <span class="flex min-w-0 items-center gap-2 text-xs font-medium text-ink-2">
          <Icon name="infoCircle" size="xs" class="h-3.5 w-3.5 shrink-0 text-money-text" aria-hidden="true" />
          <span class="truncate">{{ t('modelPlaza.common.noticeTitle') }}</span>
        </span>
        <span class="inline-flex shrink-0 items-center gap-1 text-xs text-ink-3">
          <span>
            {{
              noticeExpanded
                ? t('modelPlaza.common.collapseNotice')
                : t('modelPlaza.common.expandNotice')
            }}
          </span>
          <Icon
            name="chevronDown"
            size="xs"
            class="h-3.5 w-3.5 transition-transform duration-150"
            :class="{ 'rotate-180': noticeExpanded }"
            aria-hidden="true"
          />
        </span>
      </button>
      <div
        v-if="noticeExpanded"
        class="plaza-description border-t border-rule px-4 py-3 text-sm text-ink-2"
        v-html="descriptionHtml"
      />
    </section>

    <!-- Compact one-line guest hint -->
    <p
      v-if="!isAuthenticated"
      class="flex items-center gap-1.5 text-xs text-ink-3"
    >
      <Icon name="infoCircle" size="xs" class="h-3.5 w-3.5 shrink-0" aria-hidden="true" />
      <span>{{ t('modelPlaza.anonymousHint') }}</span>
    </p>

    <!-- Loading Skeleton -->
    <template v-if="loading">
      <PlazaStrengthBar :summary="null" :can-open-monitor="canOpenMonitor" loading />
      <div class="space-y-2.5">
        <div class="flex flex-wrap gap-2">
          <div class="h-10 w-44 animate-pulse rounded-none bg-surface-tile" />
          <div class="h-10 min-w-[180px] flex-1 animate-pulse rounded-none bg-surface-tile" />
          <div class="h-10 w-24 animate-pulse rounded-none bg-surface-tile" />
          <div class="h-10 w-28 animate-pulse rounded-none bg-surface-tile" />
        </div>
        <div class="flex gap-1.5 overflow-hidden">
          <div v-for="i in 5" :key="i" class="h-8 w-20 shrink-0 animate-pulse rounded-none bg-surface-tile" />
        </div>
      </div>
      <div
        v-if="effectiveViewMode === 'table'"
        class="space-y-1"
      >
        <div
          v-for="i in 6"
          :key="i"
          class="h-[54px] animate-pulse rounded-none bg-surface-tile"
        />
      </div>
      <div v-else class="tile-grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3">
        <div
          v-for="i in 6"
          :key="i"
          class="h-[246px] animate-pulse rounded-none bg-surface-tile p-[18px]"
        >
          <div class="h-5 w-2/3 bg-surface-tile-2" />
          <div class="mt-3 h-4 w-3/4 bg-surface-tile-2" />
          <div class="mt-3 h-[58px] w-full bg-surface-tile-2" />
          <div class="mt-3 h-9 w-full bg-surface-tile-2" />
          <div class="mt-4 h-4 w-1/2 bg-surface-tile-2" />
        </div>
      </div>
    </template>

    <!-- Error State -->
    <div
      v-else-if="error"
      class="flex flex-col items-center justify-center gap-3 rounded-none bg-surface-tile px-5 py-10 text-center"
      role="alert"
    >
      <div class="inline-flex items-center gap-2 text-sm font-medium text-bad">
        <Icon name="xCircle" size="md" aria-hidden="true" />
        <span>{{ t('modelPlaza.loadFailed') }}</span>
      </div>
      <button
        type="button"
        class="btn btn-secondary btn-sm rounded-none"
        @click="emit('retry')"
      >
        <Icon name="refresh" size="xs" aria-hidden="true" />
        <span>{{ t('modelPlaza.common.retry') }}</span>
      </button>
    </div>

    <!-- Main 3-Layer Content -->
    <template v-else>
      <!-- Layer 1: Site Strength Bar -->
      <PlazaStrengthBar
        v-if="sortedGroups.length > 0 || response?.site_summary"
        :summary="response?.site_summary ?? null"
        :can-open-monitor="canOpenMonitor"
      />

      <!-- Empty Plaza (no visible groups) -->
      <EmptyState
        v-if="!activeGroup"
        :title="t('modelPlaza.empty')"
      />

      <!-- Layer 2: Toolbar + Active View (Card Grid / Table) -->
      <template v-else>
        <PlazaToolbar
          :groups="sortedGroups"
          :selected-group-name="selectedGroupName"
          :search="searchQuery"
          :selected-vendor="selectedVendor"
          :selected-capabilities="selectedCapabilities"
          :billing-type="billingType"
          :only-high-cache="onlyHighCache"
          :only-verified-consistency="onlyVerifiedConsistency"
          :sort-by="sortBy"
          :view-mode="effectiveViewMode"
          :filtered-model-ids="filteredModelIds"
          :group-match-counts="groupMatchCounts"
          :vendors="vendorOptions"
          :total-in-group="activeGroup.models.length"
          @update:selected-group-name="handleSelectGroup"
          @update:search="searchQuery = $event"
          @update:selected-vendor="selectedVendor = $event"
          @update:selected-capabilities="selectedCapabilities = $event"
          @update:billing-type="billingType = $event"
          @update:only-high-cache="onlyHighCache = $event"
          @update:only-verified-consistency="onlyVerifiedConsistency = $event"
          @update:sort-by="sortBy = $event"
          @update:view-mode="handleViewModeChange"
          @reset-filters="handleResetFilters"
        />

        <!-- Group has no models configured at all -->
        <EmptyState
          v-if="activeGroup.models.length === 0 && !hasCrossGroupFallback"
          :title="t('modelPlaza.detail.noModels')"
        />

        <!-- Desktop Table View (only when viewport >= 768px and viewMode === 'table') -->
        <PlazaModelTable
          v-else-if="effectiveViewMode === 'table'"
          :group="activeGroup"
          :models="sortedFilteredModels"
          :sort-by="sortBy"
          :hide-empty-state="hasCrossGroupFallback"
          @update:sort-by="sortBy = $event"
          @select-model="handleSelectModel"
        />

        <!-- Card Grid View (default on desktop; forced 1-column on <768px) -->
        <PlazaModelCardGrid
          v-else
          :group="activeGroup"
          :models="sortedFilteredModels"
          :hide-empty-state="hasCrossGroupFallback"
          @select-model="handleSelectModel"
        />
      </template>

      <!-- Layer 3: Model Detail Drawer -->
      <PlazaModelDetailDrawer
        :show="drawerOpen"
        :model="drawerModel"
        :group-name="selectedGroupName"
        :all-groups="sortedGroups"
        @close="handleCloseDrawer"
        @switch-group="handleDrawerSwitchGroup"
      />
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { marked } from 'marked'
import DOMPurify from 'dompurify'
import Icon from '@/components/icons/Icon.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import PlazaStrengthBar from './PlazaStrengthBar.vue'
import PlazaToolbar, {
  type PlazaBillingFilter,
  type PlazaSortBy,
  type PlazaVendorOption,
  type PlazaViewMode
} from './PlazaToolbar.vue'
import PlazaModelCardGrid from './PlazaModelCardGrid.vue'
import PlazaModelTable from './PlazaModelTable.vue'
import PlazaModelDetailDrawer from './PlazaModelDetailDrawer.vue'
import { resolveIntervalPrices } from '@/utils/pricing'
import { isChannelMonitorRouteEnabled } from '@/utils/featureFlags'
import {
  BILLING_MODE_IMAGE,
  BILLING_MODE_TOKEN,
  BILLING_MODE_VIDEO,
  type BillingMode
} from '@/constants/channel'
import type {
  ModelPlazaGroup,
  ModelPlazaResponse,
  PlazaMetricSampleState,
  PlazaModel
} from '@/api/modelPlaza'
import type { UserPricingInterval } from '@/api/channels'
import { useAuthStore } from '@/stores/auth'

const VIEW_STORAGE_KEY = 'sub2api:model-plaza:view'
const MOBILE_MEDIA_QUERY = '(max-width: 767px)'

const KNOWN_CAPABILITY_KEYS = new Set([
  'reasoning',
  'tools',
  'vision',
  'audio',
  'image',
  'video',
  'prompt_caching',
  'web_search',
  'long_context',
  'structured_outputs',
  'code',
  'embedding',
  'realtime'
])

const KNOWN_VENDOR_KEYS = new Set([
  'all',
  'openai',
  'anthropic',
  'google',
  'xai',
  'deepseek',
  'qwen',
  'zhipu',
  'meta',
  'mistral',
  'moonshot',
  'minimax',
  'doubao',
  'cohere',
  'other'
])

const props = defineProps<{
  response: ModelPlazaResponse | null
  loading: boolean
  error?: boolean
  /** Embedded mode inside AppLayout: omits standalone page header. */
  embedded?: boolean
  /** URL-synced group name (never numeric group ID). */
  selectedGroup?: string
  /** URL-synced active model name for the detail drawer. */
  activeModel?: string
  /** Explicit view mode from URL query; takes priority over localStorage. */
  preferredView?: PlazaViewMode | null
}>()

const emit = defineEmits<{
  'update:selectedGroup': [groupName: string]
  'update:activeModel': [modelName: string]
  'update:viewMode': [mode: PlazaViewMode]
  retry: []
}>()

const { t } = useI18n()
const authStore = useAuthStore()

const isAuthenticated = computed(() => authStore.isAuthenticated)
const canOpenMonitor = computed(() => isAuthenticated.value && isChannelMonitorRouteEnabled())

// Collapsible admin notice
const noticeExpanded = ref(false)
const descriptionHtml = computed(() => {
  const md = props.response?.description?.trim()
  if (!md) return ''
  return DOMPurify.sanitize(marked.parse(md) as string)
})

// View preference (localStorage + URL override + <768px forced single-column card)
function readSavedViewMode(): PlazaViewMode {
  try {
    const saved = localStorage.getItem(VIEW_STORAGE_KEY)
    if (saved === 'card' || saved === 'table') return saved
  } catch {
    // ignore storage errors
  }
  return 'card'
}

function persistViewMode(mode: PlazaViewMode) {
  try {
    localStorage.setItem(VIEW_STORAGE_KEY, mode)
  } catch {
    // ignore storage errors
  }
}

const desktopViewMode = ref<PlazaViewMode>(props.preferredView ?? readSavedViewMode())
const isMobile = ref(false)
let mobileMql: MediaQueryList | null = null

function syncMobileBreakpoint() {
  if (mobileMql) {
    isMobile.value = mobileMql.matches
  } else if (typeof window !== 'undefined') {
    isMobile.value = window.innerWidth < 768
  }
}

function handleMediaChange(event: MediaQueryListEvent) {
  isMobile.value = event.matches
}

onMounted(() => {
  if (typeof window !== 'undefined' && typeof window.matchMedia === 'function') {
    mobileMql = window.matchMedia(MOBILE_MEDIA_QUERY)
    syncMobileBreakpoint()
    if (typeof mobileMql.addEventListener === 'function') {
      mobileMql.addEventListener('change', handleMediaChange)
    } else if (typeof mobileMql.addListener === 'function') {
      mobileMql.addListener(handleMediaChange)
    }
  } else {
    syncMobileBreakpoint()
  }
})

onBeforeUnmount(() => {
  if (mobileMql) {
    if (typeof mobileMql.removeEventListener === 'function') {
      mobileMql.removeEventListener('change', handleMediaChange)
    } else if (typeof mobileMql.removeListener === 'function') {
      mobileMql.removeListener(handleMediaChange)
    }
  }
})

watch(
  () => props.preferredView,
  (next) => {
    if (next === 'card' || next === 'table') {
      desktopViewMode.value = next
    }
  }
)

const effectiveViewMode = computed<PlazaViewMode>(() =>
  isMobile.value ? 'card' : desktopViewMode.value
)

function handleViewModeChange(mode: PlazaViewMode) {
  desktopViewMode.value = mode
  persistViewMode(mode)
  emit('update:viewMode', mode)
}

// Group ordering & selection
function effectiveRate(g: ModelPlazaGroup): number {
  return g.user_rate_multiplier ?? g.rate_multiplier
}

const sortedGroups = computed<ModelPlazaGroup[]>(() =>
  [...(props.response?.groups ?? [])].sort(
    (a, b) => effectiveRate(a) - effectiveRate(b) || a.name.localeCompare(b.name)
  )
)

const selectedGroupName = ref('')
const drawerModelName = ref('')

// Toolbar filters & sort state
const searchQuery = ref('')
const selectedVendor = ref('all')
const selectedCapabilities = ref<string[]>([])
const billingType = ref<PlazaBillingFilter>('all')
const onlyHighCache = ref(false)
const onlyVerifiedConsistency = ref(false)
const sortBy = ref<PlazaSortBy>('default')

function reconcileSelectionFromProps() {
  const groups = sortedGroups.value
  if (groups.length === 0) {
    selectedGroupName.value = ''
    drawerModelName.value = ''
    return
  }

  const requestedGroup = props.selectedGroup?.trim() || ''
  const requestedModel =
    props.activeModel !== undefined ? props.activeModel.trim() : drawerModelName.value

  let nextGroup = requestedGroup
    ? groups.find((g) => g.name === requestedGroup)
    : undefined

  let nextModelName = ''
  if (requestedModel) {
    if (nextGroup && nextGroup.models.some((m) => m.name === requestedModel)) {
      nextModelName = requestedModel
    } else {
      const fallbackGroup = groups.find((g) =>
        g.models.some((m) => m.name === requestedModel)
      )
      if (fallbackGroup) {
        nextGroup = fallbackGroup
        nextModelName = requestedModel
      }
    }
  }

  if (!nextGroup) {
    nextGroup =
      groups.find((g) => g.name === selectedGroupName.value) ?? groups[0]
  }

  selectedGroupName.value = nextGroup.name
  drawerModelName.value = nextModelName

  if (props.selectedGroup !== undefined && requestedGroup !== nextGroup.name) {
    emit('update:selectedGroup', nextGroup.name)
  }
  if (props.activeModel !== undefined && requestedModel !== nextModelName) {
    emit('update:activeModel', nextModelName)
  }
}

watch(
  () => [sortedGroups.value, props.selectedGroup, props.activeModel] as const,
  () => {
    reconcileSelectionFromProps()
  },
  { immediate: true }
)

const activeGroup = computed<ModelPlazaGroup | null>(
  () => sortedGroups.value.find((g) => g.name === selectedGroupName.value) ?? sortedGroups.value[0] ?? null
)

const drawerModel = computed<PlazaModel | null>(() => {
  if (!drawerModelName.value || !activeGroup.value) return null
  return activeGroup.value.models.find((m) => m.name === drawerModelName.value) ?? null
})

const drawerOpen = computed(() => Boolean(drawerModel.value))

// Vendor inference (aligned with PlazaToolbar)
function inferVendorKey(model: PlazaModel): string {
  const metaVendor = model.metadata?.vendor?.trim().toLowerCase()
  if (metaVendor) return metaVendor
  const name = model.name.toLowerCase()
  if (name.includes('claude')) return 'anthropic'
  if (
    name.startsWith('gpt') ||
    name.startsWith('o1') ||
    name.startsWith('o3') ||
    name.startsWith('o4') ||
    name.includes('chatgpt')
  ) {
    return 'openai'
  }
  if (name.includes('gemini') || name.includes('imagen') || name.includes('veo')) return 'google'
  if (name.includes('grok')) return 'xai'
  if (name.includes('deepseek')) return 'deepseek'
  if (name.includes('qwen') || name.includes('qwq')) return 'qwen'
  if (name.includes('glm')) return 'zhipu'
  if (name.includes('llama')) return 'meta'
  if (name.includes('mistral') || name.includes('mixtral') || name.includes('codestral')) return 'mistral'
  if (name.includes('kimi') || name.includes('moonshot')) return 'moonshot'
  if (name.includes('minimax') || name.includes('abab')) return 'minimax'
  if (name.includes('doubao')) return 'doubao'
  if (name.includes('command')) return 'cohere'
  return 'other'
}

const vendorOptions = computed<PlazaVendorOption[]>(() => {
  const counts = new Map<string, number>()
  for (const m of activeGroup.value?.models ?? []) {
    const key = inferVendorKey(m)
    counts.set(key, (counts.get(key) ?? 0) + 1)
  }
  return [...counts.entries()]
    .map(([key, count]) => ({ key, count }))
    .sort((a, b) => {
      if (a.key === 'other') return 1
      if (b.key === 'other') return -1
      return b.count - a.count || a.key.localeCompare(b.key)
    })
})

function capabilityLabel(cap: string): string {
  const normalized = cap.trim().toLowerCase()
  if (KNOWN_CAPABILITY_KEYS.has(normalized)) {
    return t(`modelPlaza.capabilities.${normalized}`)
  }
  return cap
}

function vendorLabel(vendorKey: string): string {
  const normalized = vendorKey.trim().toLowerCase()
  if (KNOWN_VENDOR_KEYS.has(normalized)) {
    return t(`modelPlaza.vendors.${normalized}`)
  }
  return vendorKey
}

function matchesSearch(model: PlazaModel, queryLower: string): boolean {
  if (!queryLower) return true
  if (model.name.toLowerCase().includes(queryLower)) return true
  if (model.metadata?.display_name?.toLowerCase().includes(queryLower)) return true
  if (model.metadata?.description?.toLowerCase().includes(queryLower)) return true
  const vendorKey = inferVendorKey(model)
  if (vendorKey.includes(queryLower) || vendorLabel(vendorKey).toLowerCase().includes(queryLower)) {
    return true
  }
  return (model.metadata?.capabilities ?? []).some((cap) => {
    const norm = cap.trim().toLowerCase()
    return norm.includes(queryLower) || capabilityLabel(cap).toLowerCase().includes(queryLower)
  })
}

function matchesPopoverFilters(model: PlazaModel): boolean {
  if (selectedCapabilities.value.length > 0) {
    const modelCaps = new Set(
      (model.metadata?.capabilities ?? []).map((c) => c.trim().toLowerCase()).filter(Boolean)
    )
    if (!selectedCapabilities.value.every((req) => modelCaps.has(req.trim().toLowerCase()))) {
      return false
    }
  }

  if (billingType.value !== 'all') {
    const mode = (model.pricing?.billing_mode || BILLING_MODE_TOKEN) as BillingMode
    if (mode !== billingType.value) return false
  }

  if (onlyHighCache.value) {
    const cacheVal = validMetricValue(
      model.metrics?.cache_read_ratio,
      model.metrics?.cache_state
    )
    if (cacheVal == null || cacheVal < 0.7) return false
  }

  if (onlyVerifiedConsistency.value) {
    if (model.metrics?.consistency_state !== 'verified') return false
  }

  return true
}

const trimmedSearchLower = computed(() => searchQuery.value.trim().toLowerCase())
const hasSearch = computed(() => trimmedSearchLower.value.length > 0)

// Cross-group search match counts across all visible groups
const groupMatchCounts = computed<Record<string, number>>(() => {
  const counts: Record<string, number> = {}
  const q = trimmedSearchLower.value
  for (const group of sortedGroups.value) {
    if (!q) {
      counts[group.name] = group.models.length
      continue
    }
    let matched = 0
    for (const model of group.models) {
      if (matchesSearch(model, q) && matchesPopoverFilters(model)) {
        matched += 1
      }
    }
    counts[group.name] = matched
  }
  return counts
})

// Reset selectedVendor if switching to a group where it is absent or hides search results
watch(selectedGroupName, () => {
  if (selectedVendor.value === 'all') return
  const models = activeGroup.value?.models ?? []
  const hasVendorInGroup = models.some((m) => inferVendorKey(m) === selectedVendor.value)
  if (!hasVendorInGroup) {
    selectedVendor.value = 'all'
    return
  }
  if (hasSearch.value) {
    const hasMatchWithVendor = models.some(
      (m) =>
        inferVendorKey(m) === selectedVendor.value &&
        matchesSearch(m, trimmedSearchLower.value) &&
        matchesPopoverFilters(m)
    )
    if (!hasMatchWithVendor) {
      selectedVendor.value = 'all'
    }
  }
})

// Sorting helpers (missing / low-sample values always sort to the end)
function sortByContext(intervals: UserPricingInterval[]): UserPricingInterval[] {
  return [...intervals].sort((a, b) => a.min_tokens - b.min_tokens)
}

function validMetricValue(
  val: number | null | undefined,
  state: PlazaMetricSampleState | null | undefined
): number | null {
  if (state != null && state !== 'valid') return null
  if (val == null || !Number.isFinite(val)) return null
  return val
}

function computeSortPrice(model: PlazaModel, group: ModelPlazaGroup): number | null {
  const mode = (model.pricing?.billing_mode || BILLING_MODE_TOKEN) as BillingMode
  const rate = effectiveRate(group)

  if (mode === BILLING_MODE_TOKEN) {
    const intervals = model.pricing?.intervals?.length
      ? sortByContext(model.pricing.intervals).map((iv) => resolveIntervalPrices(iv, model.pricing!))
      : []
    const base = intervals[0]
    const rawIn = base ? base.input_price : (model.pricing?.input_price ?? null)
    const rawOut = base ? base.output_price : (model.pricing?.output_price ?? null)
    const paidIn = rawIn != null && Number.isFinite(rawIn) ? rawIn * rate : null
    const paidOut = rawOut != null && Number.isFinite(rawOut) ? rawOut * rate : null
    return paidIn ?? paidOut
  }

  let reqRate = rate
  if (mode === BILLING_MODE_VIDEO && group.video_rate_independent) {
    reqRate = Math.max(0, group.video_rate_multiplier ?? 1)
  } else if (mode === BILLING_MODE_IMAGE && group.image_rate_independent) {
    reqRate = group.image_rate_multiplier ?? 1
  }

  const reqIntervals = (model.pricing?.intervals ?? []).filter((iv) => iv.per_request_price != null)
  const rawReqPrice =
    reqIntervals.length > 0
      ? reqIntervals[0].per_request_price
      : (model.pricing?.per_request_price ?? null)
  return rawReqPrice != null && Number.isFinite(rawReqPrice) ? rawReqPrice * reqRate : null
}

function compareNullable(a: number | null, b: number | null, dir: 'asc' | 'desc'): number {
  if (a == null && b == null) return 0
  if (a == null) return 1
  if (b == null) return -1
  return dir === 'asc' ? a - b : b - a
}

const sortedFilteredModels = computed<PlazaModel[]>(() => {
  const group = activeGroup.value
  if (!group) return []

  const q = trimmedSearchLower.value
  const vendor = selectedVendor.value

  const filtered = group.models.filter((model) => {
    if (q && !matchesSearch(model, q)) return false
    if (vendor !== 'all' && inferVendorKey(model) !== vendor) return false
    return matchesPopoverFilters(model)
  })

  const currentSort = sortBy.value
  if (currentSort === 'default') return filtered

  const list = [...filtered]
  list.sort((a, b) => {
    switch (currentSort) {
      case 'price_asc':
        return (
          compareNullable(computeSortPrice(a, group), computeSortPrice(b, group), 'asc') ||
          a.name.localeCompare(b.name)
        )
      case 'price_desc':
        return (
          compareNullable(computeSortPrice(a, group), computeSortPrice(b, group), 'desc') ||
          a.name.localeCompare(b.name)
        )
      case 'cache_desc':
        return (
          compareNullable(
            validMetricValue(a.metrics?.cache_read_ratio, a.metrics?.cache_state),
            validMetricValue(b.metrics?.cache_read_ratio, b.metrics?.cache_state),
            'desc'
          ) || a.name.localeCompare(b.name)
        )
      case 'ref_multiplier_asc':
        return (
          compareNullable(
            validMetricValue(a.metrics?.reference_multiplier, a.metrics?.reference_multiplier_state),
            validMetricValue(b.metrics?.reference_multiplier, b.metrics?.reference_multiplier_state),
            'asc'
          ) || a.name.localeCompare(b.name)
        )
      case 'sla_desc':
        return (
          compareNullable(
            validMetricValue(a.metrics?.sla_24h, a.metrics?.sla_state),
            validMetricValue(b.metrics?.sla_24h, b.metrics?.sla_state),
            'desc'
          ) || a.name.localeCompare(b.name)
        )
      case 'ttft_asc':
        return (
          compareNullable(
            validMetricValue(a.metrics?.ttft_p50_ms, a.metrics?.ttft_state),
            validMetricValue(b.metrics?.ttft_p50_ms, b.metrics?.ttft_state),
            'asc'
          ) || a.name.localeCompare(b.name)
        )
      default:
        return 0
    }
  })
  return list
})

const filteredModelIds = computed(() => sortedFilteredModels.value.map((m) => m.name))

const hasCrossGroupFallback = computed(() => {
  if (!hasSearch.value || sortedFilteredModels.value.length > 0) return false
  return sortedGroups.value.some(
    (g) => g.name !== selectedGroupName.value && (groupMatchCounts.value[g.name] ?? 0) > 0
  )
})

// Event handlers
function handleSelectGroup(groupName: string) {
  if (!groupName || groupName === selectedGroupName.value) return
  const target = sortedGroups.value.find((g) => g.name === groupName)
  if (!target) return
  selectedGroupName.value = target.name
  emit('update:selectedGroup', target.name)
  if (drawerModelName.value && !target.models.some((m) => m.name === drawerModelName.value)) {
    drawerModelName.value = ''
    emit('update:activeModel', '')
  }
}

function handleResetFilters() {
  selectedCapabilities.value = []
  billingType.value = 'all'
  onlyHighCache.value = false
  onlyVerifiedConsistency.value = false
}

function handleSelectModel(model: PlazaModel) {
  drawerModelName.value = model.name
  emit('update:activeModel', model.name)
}

function handleCloseDrawer() {
  drawerModelName.value = ''
  emit('update:activeModel', '')
}

function handleDrawerSwitchGroup(targetGroupName: string) {
  const target = sortedGroups.value.find((g) => g.name === targetGroupName)
  if (!target) return
  selectedGroupName.value = target.name
  emit('update:selectedGroup', target.name)
  if (drawerModelName.value && !target.models.some((m) => m.name === drawerModelName.value)) {
    drawerModelName.value = ''
    emit('update:activeModel', '')
  }
}
</script>

<style scoped>
.plaza-description {
  line-height: 1.65;
  overflow-wrap: anywhere;
}

.plaza-description :deep(h1),
.plaza-description :deep(h2),
.plaza-description :deep(h3) {
  @apply mb-2 mt-3 font-semibold text-ink first:mt-0;
}

.plaza-description :deep(p) {
  @apply mb-2 text-ink-2 last:mb-0;
}

.plaza-description :deep(a) {
  @apply text-money-text underline underline-offset-4 hover:opacity-80;
}

.plaza-description :deep(ul) {
  @apply mb-2 list-disc pl-5;
}

.plaza-description :deep(ol) {
  @apply mb-2 list-decimal pl-5;
}

.plaza-description :deep(li) {
  @apply mb-0.5 text-ink-2;
}

.plaza-description :deep(code) {
  @apply rounded-none bg-surface-tile-2 px-1.5 py-0.5 font-mono text-xs text-money-text;
}

.plaza-description :deep(blockquote) {
  @apply my-2 border-l-2 border-rule pl-3 text-ink-3;
}
</style>
