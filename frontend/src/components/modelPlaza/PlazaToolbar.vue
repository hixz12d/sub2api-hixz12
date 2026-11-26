<template>
  <div ref="toolbarRoot" class="space-y-2.5">
    <!-- Row 1: Main single-row toolbar on lg+ -->
    <div class="flex flex-wrap items-center gap-2 lg:flex-nowrap">
      <!-- Group single-select switcher (no numeric ID ever displayed) -->
      <div class="relative max-w-full shrink-0">
        <button
          type="button"
          class="inline-flex h-10 max-w-full items-center gap-2 rounded-none bg-surface-tile px-3 text-sm font-medium text-ink transition-colors duration-150 hover:bg-surface-tile-2 sm:max-w-[300px]"
          :disabled="groups.length === 0"
          :aria-expanded="groupOpen"
          :aria-label="t('modelPlaza.toolbar.groupSelectorLabel')"
          @click="toggleMenu('group')"
        >
          <span
            v-if="activeGroup"
            class="h-3.5 w-3.5 shrink-0 rounded-none"
            :class="groupColorClass(activeGroup.id)"
            aria-hidden="true"
          />
          <span class="truncate font-semibold">
            {{ activeGroup?.name || t('modelPlaza.toolbar.groupSelectorLabel') }}
          </span>
          <span
            v-if="activeGroup"
            class="shrink-0 font-mono text-xs text-ink-2 tabular-nums"
          >
            {{ formatRate(activeGroup) }}×
          </span>
          <span
            v-if="activeGroup?.is_authorized === false"
            class="hidden shrink-0 items-center gap-1 bg-warn/15 px-1.5 py-0.5 text-[11px] font-medium text-warn sm:inline-flex"
          >
            <Icon name="lock" size="xs" class="h-3 w-3" aria-hidden="true" />
            <span>{{ activeGroup.showcase_badge || t('modelPlaza.toolbar.groupUnauthorizedBadge') }}</span>
          </span>
          <span
            v-else-if="activeGroup?.showcase_badge"
            class="hidden shrink-0 bg-surface-tile-2 px-1.5 py-0.5 text-[11px] font-medium text-ink-2 sm:inline-flex"
          >
            {{ activeGroup.showcase_badge }}
          </span>
          <span
            v-else-if="activeGroup?.is_exclusive"
            class="hidden shrink-0 bg-surface-tile-2 px-1.5 py-0.5 text-[11px] font-medium text-ink-2 sm:inline-flex"
          >
            {{ t('modelPlaza.badges.exclusive') }}
          </span>
          <span
            v-else-if="activeGroup?.subscription_type === 'subscription'"
            class="hidden shrink-0 bg-surface-tile-2 px-1.5 py-0.5 text-[11px] font-medium text-ink-2 sm:inline-flex"
          >
            {{ t('modelPlaza.badges.subscription') }}
          </span>
          <Icon
            v-if="groups.length > 1"
            name="chevronDown"
            size="xs"
            class="h-3.5 w-3.5 shrink-0 text-ink-3 transition-transform duration-150"
            :class="{ 'rotate-180': groupOpen }"
            aria-hidden="true"
          />
        </button>

        <!-- Interactive Group Dropdown -->
        <div
          v-if="groupOpen && groups.length > 0"
          role="listbox"
          :aria-label="t('modelPlaza.toolbar.groupSelectorLabel')"
          class="absolute left-0 top-full z-40 mt-1 max-h-[340px] w-[300px] max-w-[calc(100vw-2rem)] overflow-y-auto rounded-none border border-rule bg-surface py-1 sm:w-[360px]"
        >
          <button
            v-for="g in groups"
            :key="g.name"
            type="button"
            role="option"
            :aria-selected="g.name === selectedGroupName"
            class="flex w-full items-center justify-between gap-2 px-3 py-2.5 text-left text-sm transition-colors duration-150"
            :class="
              g.name === selectedGroupName
                ? 'bg-surface-tile font-semibold text-ink'
                : 'text-ink-2 hover:bg-surface-tile hover:text-ink'
            "
            @click="selectGroup(g.name)"
          >
            <div class="flex min-w-0 flex-1 flex-wrap items-center gap-1.5">
              <span
                class="h-3.5 w-3.5 shrink-0 rounded-none"
                :class="groupColorClass(g.id)"
                aria-hidden="true"
              />
              <span class="truncate">{{ g.name }}</span>
              <span
                v-if="g.is_authorized === false"
                class="inline-flex shrink-0 items-center gap-0.5 bg-warn/15 px-1.5 py-0.5 text-[11px] font-medium text-warn"
              >
                <Icon name="lock" size="xs" class="h-3 w-3" aria-hidden="true" />
                <span>{{ g.showcase_badge || t('modelPlaza.toolbar.groupUnauthorizedBadge') }}</span>
              </span>
              <span
                v-else-if="g.showcase_badge"
                class="shrink-0 bg-surface-tile-2 px-1.5 py-0.5 text-[11px] font-medium text-ink-2"
              >
                {{ g.showcase_badge }}
              </span>
              <span
                v-else-if="g.is_exclusive"
                class="shrink-0 bg-surface-tile-2 px-1.5 py-0.5 text-[11px] font-medium text-ink-2"
              >
                {{ t('modelPlaza.badges.exclusive') }}
              </span>
              <span
                v-if="g.subscription_type === 'subscription'"
                class="shrink-0 bg-surface-tile-2 px-1.5 py-0.5 text-[11px] font-medium text-ink-2"
              >
                {{ t('modelPlaza.badges.subscription') }}
              </span>
            </div>

            <div class="flex shrink-0 items-center gap-2 text-xs tabular-nums">
              <span class="font-mono text-ink-2">{{ formatRate(g) }}×</span>
              <span
                v-if="hasSearch"
                class="min-w-[28px] text-right font-semibold"
                :class="getGroupMatchCount(g) > 0 ? 'text-money-text' : 'text-ink-3'"
              >
                ({{ getGroupMatchCount(g) }})
              </span>
              <span v-else class="text-ink-3">
                {{ t('modelPlaza.toolbar.groupModelCount', { count: getGroupBaseCount(g) }) }}
              </span>
              <Icon
                v-if="g.name === selectedGroupName"
                name="check"
                size="xs"
                class="h-3.5 w-3.5 text-money-text"
                aria-hidden="true"
              />
            </div>
          </button>
        </div>
      </div>

      <!-- Search input -->
      <div class="relative min-w-[160px] flex-1">
        <Icon
          name="search"
          size="sm"
          class="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-ink-3"
          aria-hidden="true"
        />
        <input
          :value="search"
          type="text"
          :placeholder="t('modelPlaza.toolbar.searchPlaceholder')"
          class="input h-10 rounded-none pl-9 pr-8 text-sm"
          @input="emit('update:search', ($event.target as HTMLInputElement).value)"
        />
        <button
          v-if="search"
          type="button"
          class="absolute right-2 top-1/2 inline-flex h-6 w-6 -translate-y-1/2 items-center justify-center rounded-none text-ink-3 transition-colors duration-150 hover:bg-surface-tile hover:text-ink"
          :aria-label="t('modelPlaza.toolbar.clearSearch')"
          :title="t('modelPlaza.toolbar.clearSearch')"
          @click="emit('update:search', '')"
        >
          <Icon name="x" size="xs" class="h-3.5 w-3.5" aria-hidden="true" />
        </button>
      </div>

      <!-- Filter Popover -->
      <div class="relative shrink-0">
        <button
          type="button"
          class="inline-flex h-10 items-center gap-1.5 rounded-none px-3 text-sm font-medium transition-colors duration-150"
          :class="
            activeFilterCount > 0 || filterOpen
              ? 'bg-surface-tile-2 font-semibold text-money-text'
              : 'bg-surface-tile text-ink-2 hover:bg-surface-tile-2 hover:text-ink'
          "
          :aria-expanded="filterOpen"
          @click="toggleMenu('filter')"
        >
          <Icon name="filter" size="sm" aria-hidden="true" />
          <span class="whitespace-nowrap">
            {{
              activeFilterCount > 0
                ? t('modelPlaza.toolbar.filterActiveCount', { count: activeFilterCount })
                : t('modelPlaza.toolbar.filterButton')
            }}
          </span>
        </button>

        <div
          v-if="filterOpen"
          class="absolute left-0 top-full z-40 mt-1 w-[310px] max-w-[calc(100vw-2rem)] rounded-none border border-rule bg-surface p-4 sm:left-auto sm:right-0 sm:w-[360px]"
        >
          <!-- Capabilities multi-select -->
          <div>
            <div class="mb-2 text-xs font-medium text-ink-3">
              {{ t('modelPlaza.toolbar.filterCapabilities') }}
            </div>
            <div class="flex flex-wrap gap-1.5">
              <button
                v-for="cap in resolvedCapabilities"
                :key="cap"
                type="button"
                class="rounded-none px-2.5 py-1 text-xs font-medium transition-colors duration-150"
                :class="
                  selectedCapabilities.includes(cap)
                    ? 'bg-money text-money-ink hover:bg-money-hover'
                    : 'bg-surface-tile text-ink-2 hover:bg-surface-tile-2 hover:text-ink'
                "
                @click="toggleCapability(cap)"
              >
                {{ capabilityLabel(cap) }}
              </button>
            </div>
          </div>

          <!-- Billing type single-select -->
          <div class="mt-4">
            <div class="mb-2 text-xs font-medium text-ink-3">
              {{ t('modelPlaza.toolbar.filterBillingType') }}
            </div>
            <div class="flex flex-wrap gap-1.5">
              <button
                v-for="opt in billingOptions"
                :key="opt.value"
                type="button"
                class="rounded-none px-2.5 py-1 text-xs font-medium transition-colors duration-150"
                :class="
                  billingType === opt.value
                    ? 'bg-money text-money-ink hover:bg-money-hover'
                    : 'bg-surface-tile text-ink-2 hover:bg-surface-tile-2 hover:text-ink'
                "
                @click="emit('update:billingType', opt.value)"
              >
                {{ opt.label }}
              </button>
            </div>
          </div>

          <!-- Quick checkboxes -->
          <div class="mt-4">
            <div class="mb-2 text-xs font-medium text-ink-3">
              {{ t('modelPlaza.toolbar.filterQuick') }}
            </div>
            <div class="space-y-2">
              <label class="flex cursor-pointer items-center gap-2 text-xs text-ink">
                <input
                  type="checkbox"
                  :checked="onlyHighCache"
                  class="h-4 w-4 rounded-none border-rule accent-[rgb(var(--c-money))]"
                  @change="emit('update:onlyHighCache', ($event.target as HTMLInputElement).checked)"
                />
                <span>{{ t('modelPlaza.toolbar.onlyHighCache') }}</span>
              </label>
              <label class="flex cursor-pointer items-center gap-2 text-xs text-ink">
                <input
                  type="checkbox"
                  :checked="onlyVerifiedConsistency"
                  class="h-4 w-4 rounded-none border-rule accent-[rgb(var(--c-money))]"
                  @change="
                    emit('update:onlyVerifiedConsistency', ($event.target as HTMLInputElement).checked)
                  "
                />
                <span>{{ t('modelPlaza.toolbar.onlyVerifiedConsistency') }}</span>
              </label>
            </div>
          </div>

          <!-- Popover footer -->
          <div class="mt-4 flex items-center justify-between border-t border-rule pt-3">
            <button
              type="button"
              class="btn btn-ghost btn-sm"
              :disabled="activeFilterCount === 0"
              @click="handleResetFilters"
            >
              {{ t('modelPlaza.toolbar.resetFilters') }}
            </button>
            <button
              type="button"
              class="btn btn-primary btn-sm"
              @click="filterOpen = false"
            >
              {{ t('modelPlaza.toolbar.applyFilters') }}
            </button>
          </div>
        </div>
      </div>

      <!-- Sort dropdown -->
      <div class="relative shrink-0">
        <button
          type="button"
          class="inline-flex h-10 items-center gap-1.5 rounded-none bg-surface-tile px-3 text-sm font-medium text-ink-2 transition-colors duration-150 hover:bg-surface-tile-2 hover:text-ink"
          :aria-expanded="sortOpen"
          :aria-label="t('modelPlaza.toolbar.sortLabel')"
          @click="toggleMenu('sort')"
        >
          <Icon name="sort" size="sm" aria-hidden="true" />
          <span class="whitespace-nowrap text-ink">{{ currentSortLabel }}</span>
          <Icon
            name="chevronDown"
            size="xs"
            class="h-3.5 w-3.5 text-ink-3 transition-transform duration-150"
            :class="{ 'rotate-180': sortOpen }"
            aria-hidden="true"
          />
        </button>

        <div
          v-if="sortOpen"
          role="listbox"
          :aria-label="t('modelPlaza.toolbar.sortLabel')"
          class="absolute right-0 top-full z-40 mt-1 w-48 max-w-[calc(100vw-2rem)] rounded-none border border-rule bg-surface py-1"
        >
          <button
            v-for="opt in sortOptions"
            :key="opt.value"
            type="button"
            role="option"
            :aria-selected="sortBy === opt.value"
            class="flex w-full items-center justify-between px-3 py-2 text-left text-sm transition-colors duration-150"
            :class="
              sortBy === opt.value
                ? 'bg-surface-tile font-semibold text-money-text'
                : 'text-ink-2 hover:bg-surface-tile hover:text-ink'
            "
            @click="selectSort(opt.value)"
          >
            <span>{{ opt.label }}</span>
            <Icon
              v-if="sortBy === opt.value"
              name="check"
              size="xs"
              class="h-3.5 w-3.5 text-money-text"
              aria-hidden="true"
            />
          </button>
        </div>
      </div>

      <!-- View segmented switcher (md+ only, hidden on mobile) -->
      <div
        class="tabs hidden shrink-0 md:inline-flex"
        role="group"
        :aria-label="t('modelPlaza.toolbar.viewSwitchLabel')"
      >
        <button
          type="button"
          class="tab inline-flex h-10 items-center gap-1.5 px-3 py-0 text-sm"
          :class="{ 'tab-active': viewMode === 'card' }"
          :aria-pressed="viewMode === 'card'"
          @click="emit('update:viewMode', 'card')"
        >
          <Icon name="grid" size="sm" aria-hidden="true" />
          <span>{{ t('modelPlaza.toolbar.viewCard') }}</span>
        </button>
        <button
          type="button"
          class="tab inline-flex h-10 items-center gap-1.5 px-3 py-0 text-sm"
          :class="{ 'tab-active': viewMode === 'table' }"
          :aria-pressed="viewMode === 'table'"
          @click="emit('update:viewMode', 'table')"
        >
          <Icon name="table" size="sm" aria-hidden="true" />
          <span>{{ t('modelPlaza.toolbar.viewTable') }}</span>
        </button>
      </div>

      <!-- Batch copy all filtered model IDs -->
      <button
        type="button"
        class="inline-flex h-10 shrink-0 items-center gap-1.5 rounded-none px-3 text-sm font-medium transition-colors duration-150 active:scale-[0.985] disabled:cursor-not-allowed disabled:opacity-50"
        :class="
          batchCopied
            ? 'bg-ok text-white'
            : 'bg-surface-tile text-ink hover:bg-surface-tile-2'
        "
        :disabled="filteredModelIds.length === 0"
        :title="t('modelPlaza.toolbar.copyAllModelsTooltip', { count: filteredModelIds.length })"
        :aria-label="t('modelPlaza.toolbar.copyAllModels')"
        @click="handleCopyAll"
      >
        <Icon :name="batchCopied ? 'check' : 'copy'" size="sm" aria-hidden="true" />
        <span class="hidden whitespace-nowrap xl:inline">
          {{
            batchCopied
              ? t('modelPlaza.toolbar.copiedAllModels', { count: batchCopiedCount })
              : t('modelPlaza.toolbar.copyAllModels')
          }}
        </span>
      </button>
    </div>

    <!-- Row 2: Horizontal scrollable vendor shortcut strip + result summary -->
    <div v-if="resolvedVendors.length > 0" class="flex items-center justify-between gap-3">
      <div class="scrollbar-hide flex min-w-0 flex-1 items-center gap-1.5 overflow-x-auto whitespace-nowrap">
        <button
          type="button"
          class="inline-flex h-8 shrink-0 items-center gap-1.5 rounded-none px-3 text-xs font-semibold transition-colors duration-150"
          :class="
            selectedVendor === 'all'
              ? 'bg-money text-money-ink hover:bg-money-hover'
              : 'bg-surface-tile text-ink-2 hover:bg-surface-tile-2 hover:text-ink'
          "
          @click="emit('update:selectedVendor', 'all')"
        >
          <span>{{ t('modelPlaza.toolbar.vendorAll') }}</span>
          <span
            class="tabular-nums"
            :class="selectedVendor === 'all' ? 'text-money-dim' : 'text-ink-3'"
          >
            {{ totalVendorModels }}
          </span>
        </button>

        <button
          v-for="vendor in resolvedVendors"
          :key="vendor.key"
          type="button"
          class="inline-flex h-8 shrink-0 items-center gap-1.5 rounded-none px-3 text-xs font-semibold transition-colors duration-150"
          :class="
            selectedVendor === vendor.key
              ? 'bg-money text-money-ink hover:bg-money-hover'
              : 'bg-surface-tile text-ink-2 hover:bg-surface-tile-2 hover:text-ink'
          "
          @click="emit('update:selectedVendor', vendor.key)"
        >
          <ModelIcon
            v-if="vendorModelProbe(vendor.key)"
            :model="vendorModelProbe(vendor.key)!"
            size="14px"
            :class="
              selectedVendor === vendor.key
                ? 'brightness-0 invert'
                : vendorNeedsDarkInvert(vendor.key)
                  ? 'dark:invert'
                  : ''
            "
            aria-hidden="true"
          />
          <Icon v-else name="cube" size="xs" class="h-3.5 w-3.5" aria-hidden="true" />
          <span>{{ vendorLabel(vendor.key) }}</span>
          <span
            class="tabular-nums"
            :class="selectedVendor === vendor.key ? 'text-money-dim' : 'text-ink-3'"
          >
            {{ vendor.count }}
          </span>
        </button>
      </div>

      <span
        v-if="effectiveTotalInGroup > 0"
        class="hidden shrink-0 whitespace-nowrap text-xs text-ink-3 tabular-nums sm:inline"
      >
        {{
          t('modelPlaza.toolbar.resultSummary', {
            shown: shownEntryCount,
            total: effectiveTotalInGroup
          })
        }}
      </span>
    </div>

    <!-- Cross-group search smart banner: Case A (current group truly has 0 matches, other groups matched) -->
    <div
      v-if="showCrossGroupEmptyBanner"
      class="rounded-none border-l-2 border-money bg-surface-tile px-4 py-3.5"
      role="status"
    >
      <div class="flex items-center gap-2 text-sm font-medium text-ink">
        <Icon name="search" size="sm" class="shrink-0 text-money-text" aria-hidden="true" />
        <span>
          {{
            t('modelPlaza.toolbar.crossGroupEmptyTitle', {
              query: trimmedSearch,
              count: totalOtherMatches
            })
          }}
        </span>
      </div>
      <div class="mt-2.5 flex flex-wrap items-center gap-2">
        <button
          v-for="item in otherGroupMatches"
          :key="item.groupName"
          type="button"
          class="inline-flex items-center gap-1.5 rounded-none bg-surface px-3 py-1.5 text-xs font-semibold text-ink transition-colors duration-150 hover:bg-surface-tile-2"
          @click="selectGroup(item.groupName)"
        >
          <span
            class="h-3 w-3 shrink-0 rounded-none"
            :class="groupColorClass(item.groupId)"
            aria-hidden="true"
          />
          <span>
            {{
              t('modelPlaza.toolbar.switchToGroup', {
                group: item.groupName,
                count: item.count
              })
            }}
          </span>
          <Icon name="arrowRight" size="xs" class="h-3.5 w-3.5 text-money-text" aria-hidden="true" />
        </button>
      </div>
    </div>

    <!-- Cross-group search smart banner: Case B (current group has results, other groups also matched) -->
    <div
      v-else-if="showCrossGroupAlsoFoundBanner"
      class="flex flex-wrap items-center gap-x-2 gap-y-1 rounded-none bg-surface-tile px-3.5 py-2 text-xs text-ink-2"
      role="status"
    >
      <Icon name="infoCircle" size="xs" class="h-3.5 w-3.5 shrink-0 text-money-text" aria-hidden="true" />
      <span>
        {{
          t('modelPlaza.toolbar.crossGroupAlsoFound', {
            query: trimmedSearch,
            count: totalOtherMatches
          })
        }}
      </span>
      <button
        v-for="item in otherGroupMatches"
        :key="item.groupName"
        type="button"
        class="inline-flex items-center gap-1 rounded-none bg-surface-tile-2 px-2 py-0.5 font-semibold text-money-text transition-colors duration-150 hover:bg-surface"
        @click="selectGroup(item.groupName)"
      >
        <span>
          {{
            t('modelPlaza.toolbar.switchToGroup', {
              group: item.groupName,
              count: item.count
            })
          }}
        </span>
        <Icon name="arrowRight" size="xs" class="h-3 w-3" aria-hidden="true" />
      </button>
    </div>
  </div>
</template>

<script lang="ts">
export type PlazaSortBy =
  | 'default'
  | 'price_asc'
  | 'price_desc'
  | 'cache_desc'
  | 'ref_multiplier_asc'
  | 'sla_desc'
  | 'ttft_asc'

export type PlazaViewMode = 'card' | 'table'

export type PlazaBillingFilter = 'all' | 'token' | 'per_request' | 'image' | 'video'

export interface PlazaVendorOption {
  key: string
  count: number
}
</script>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import ModelIcon from '@/components/common/ModelIcon.vue'
import { useClipboard } from '@/composables/useClipboard'
import { groupSeriesClass } from '@/utils/groupSeriesColor'
import {
  KNOWN_CAPABILITY_KEYS,
  KNOWN_VENDOR_KEYS,
  inferVendorKey,
  needsDarkInvert
} from '@/utils/plazaFormat'
import type { ModelPlazaGroup } from '@/api/modelPlaza'

interface Props {
  groups: ModelPlazaGroup[]
  selectedGroupName: string
  search: string
  selectedVendor: string
  selectedCapabilities: string[]
  billingType: PlazaBillingFilter
  onlyHighCache: boolean
  onlyVerifiedConsistency: boolean
  sortBy: PlazaSortBy
  viewMode: PlazaViewMode
  filteredModelIds: string[]
  /** Optional map of groupName -> matched entry count when searching. Falls back to in-memory scan if omitted. */
  groupMatchCounts?: Record<string, number>
  /** Optional map of groupName -> total folded entry count when not searching. */
  groupTotalCounts?: Record<string, number>
  /** Optional vendor chips for current group. Falls back to deriving from active group models. */
  vendors?: PlazaVendorOption[]
  /** Optional capability keys shown in filter popover. */
  availableCapabilities?: string[]
  /** Optional total visible entries in current group (defaults to activeGroup.models.length). */
  totalInGroup?: number
  /** Optional count of currently shown entries (defaults to filteredModelIds.length). */
  shownCount?: number
}

const props = defineProps<Props>()

const emit = defineEmits<{
  'update:selectedGroupName': [value: string]
  'update:search': [value: string]
  'update:selectedVendor': [value: string]
  'update:selectedCapabilities': [value: string[]]
  'update:billingType': [value: PlazaBillingFilter]
  'update:onlyHighCache': [value: boolean]
  'update:onlyVerifiedConsistency': [value: boolean]
  'update:sortBy': [value: PlazaSortBy]
  'update:viewMode': [value: PlazaViewMode]
  'reset-filters': []
}>()

const { t } = useI18n()
const { copyToClipboard } = useClipboard()

const toolbarRoot = ref<HTMLElement | null>(null)
const groupOpen = ref(false)
const filterOpen = ref(false)
const sortOpen = ref(false)

const batchCopied = ref(false)
const batchCopiedCount = ref(0)
let batchCopyTimer: ReturnType<typeof setTimeout> | null = null

const DEFAULT_CAPABILITY_ORDER = [
  'reasoning',
  'tools',
  'vision',
  'long_context',
  'image',
  'audio',
  'video',
  'prompt_caching',
  'web_search',
  'code'
]

const VENDOR_ICON_PROBES: Record<string, string> = {
  openai: 'gpt-4o',
  anthropic: 'claude-sonnet',
  google: 'gemini-2.5-pro',
  xai: 'grok-3',
  deepseek: 'deepseek-r1',
  qwen: 'qwen-max',
  zhipu: 'glm-4',
  meta: 'llama-3',
  mistral: 'mistral-large',
  moonshot: 'kimi-k2',
  minimax: 'minimax-01',
  doubao: 'doubao-pro',
  cohere: 'command-r'
}

const orderedGroupIds = computed(() => props.groups.map((g) => g.id))

const activeGroup = computed(
  () => props.groups.find((g) => g.name === props.selectedGroupName) ?? props.groups[0] ?? null
)

const effectiveTotalInGroup = computed(
  () => props.totalInGroup ?? activeGroup.value?.models.length ?? 0
)

const shownEntryCount = computed(() => props.shownCount ?? props.filteredModelIds.length)

const trimmedSearch = computed(() => props.search.trim())
const hasSearch = computed(() => trimmedSearch.value.length > 0)

function groupColorClass(groupId: number): string {
  return groupSeriesClass(groupId, orderedGroupIds.value)
}

function formatRate(group: ModelPlazaGroup): string {
  const rate = group.user_rate_multiplier ?? group.rate_multiplier
  return String(Number(rate.toPrecision(6)))
}

function toggleMenu(target: 'group' | 'filter' | 'sort') {
  if (target === 'group' && props.groups.length <= 1) return
  groupOpen.value = target === 'group' ? !groupOpen.value : false
  filterOpen.value = target === 'filter' ? !filterOpen.value : false
  sortOpen.value = target === 'sort' ? !sortOpen.value : false
}

function closeAllMenus() {
  groupOpen.value = false
  filterOpen.value = false
  sortOpen.value = false
}

function selectGroup(name: string) {
  emit('update:selectedGroupName', name)
  groupOpen.value = false
}

function selectSort(value: PlazaSortBy) {
  emit('update:sortBy', value)
  sortOpen.value = false
}

function toggleCapability(cap: string) {
  const exists = props.selectedCapabilities.includes(cap)
  const next = exists
    ? props.selectedCapabilities.filter((c) => c !== cap)
    : [...props.selectedCapabilities, cap]
  emit('update:selectedCapabilities', next)
}

function handleResetFilters() {
  emit('update:selectedCapabilities', [])
  emit('update:billingType', 'all')
  emit('update:onlyHighCache', false)
  emit('update:onlyVerifiedConsistency', false)
  emit('reset-filters')
}

const activeFilterCount = computed(() => {
  let count = props.selectedCapabilities.length
  if (props.billingType !== 'all') count += 1
  if (props.onlyHighCache) count += 1
  if (props.onlyVerifiedConsistency) count += 1
  return count
})

const resolvedCapabilities = computed<string[]>(() => {
  if (props.availableCapabilities && props.availableCapabilities.length > 0) {
    return props.availableCapabilities
  }
  const found = new Set<string>()
  for (const m of activeGroup.value?.models ?? []) {
    for (const c of m.metadata?.capabilities ?? []) {
      const normalized = c.trim().toLowerCase()
      if (normalized) found.add(normalized)
    }
  }
  if (found.size === 0) {
    return DEFAULT_CAPABILITY_ORDER
  }
  const ordered = DEFAULT_CAPABILITY_ORDER.filter((c) => found.has(c))
  for (const c of found) {
    if (!ordered.includes(c)) ordered.push(c)
  }
  return ordered
})

function capabilityLabel(cap: string): string {
  const normalized = cap.trim().toLowerCase()
  if (KNOWN_CAPABILITY_KEYS.has(normalized)) {
    return t(`modelPlaza.capabilities.${normalized}`)
  }
  return cap
}

const billingOptions = computed<Array<{ value: PlazaBillingFilter; label: string }>>(() => [
  { value: 'all', label: t('modelPlaza.toolbar.billingAll') },
  { value: 'token', label: t('modelPlaza.toolbar.billingToken') },
  { value: 'per_request', label: t('modelPlaza.toolbar.billingRequest') },
  { value: 'image', label: t('modelPlaza.toolbar.billingImage') },
  { value: 'video', label: t('modelPlaza.toolbar.billingVideo') }
])

const sortOptions = computed<Array<{ value: PlazaSortBy; label: string }>>(() => [
  { value: 'default', label: t('modelPlaza.toolbar.sortDefault') },
  { value: 'price_asc', label: t('modelPlaza.toolbar.sortPriceAsc') },
  { value: 'price_desc', label: t('modelPlaza.toolbar.sortPriceDesc') },
  { value: 'cache_desc', label: t('modelPlaza.toolbar.sortCacheDesc') },
  { value: 'ref_multiplier_asc', label: t('modelPlaza.toolbar.sortRefMultiplierAsc') },
  { value: 'sla_desc', label: t('modelPlaza.toolbar.sortSlaDesc') },
  { value: 'ttft_asc', label: t('modelPlaza.toolbar.sortTtftAsc') }
])

const currentSortLabel = computed(
  () => sortOptions.value.find((o) => o.value === props.sortBy)?.label ?? t('modelPlaza.toolbar.sortDefault')
)

const resolvedVendors = computed<PlazaVendorOption[]>(() => {
  if (props.vendors) {
    return props.vendors
  }
  const counts = new Map<string, number>()
  for (const m of activeGroup.value?.models ?? []) {
    const key = inferVendorKey(m.name, m.platform, m.metadata?.vendor)
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

const totalVendorModels = computed(() =>
  resolvedVendors.value.reduce((sum, item) => sum + item.count, 0)
)

function vendorModelProbe(vendorKey: string): string | null {
  return VENDOR_ICON_PROBES[vendorKey.toLowerCase()] ?? null
}

function vendorNeedsDarkInvert(vendorKey: string): boolean {
  const probe = vendorModelProbe(vendorKey)
  return probe ? needsDarkInvert(probe) : false
}

function vendorLabel(vendorKey: string): string {
  const normalized = vendorKey.trim().toLowerCase()
  if (KNOWN_VENDOR_KEYS.has(normalized)) {
    return t(`modelPlaza.vendors.${normalized}`)
  }
  return vendorKey
}

function fallbackMatchCount(group: ModelPlazaGroup, queryLower: string): number {
  let matched = 0
  for (const m of group.models) {
    if (m.name.toLowerCase().includes(queryLower)) {
      matched += 1
      continue
    }
    if (m.metadata?.display_name?.toLowerCase().includes(queryLower)) {
      matched += 1
      continue
    }
    if (m.metadata?.description?.toLowerCase().includes(queryLower)) {
      matched += 1
      continue
    }
    const vendorKey = inferVendorKey(m.name, m.platform, m.metadata?.vendor)
    if (vendorKey.includes(queryLower) || vendorLabel(vendorKey).toLowerCase().includes(queryLower)) {
      matched += 1
      continue
    }
    const capsMatch = (m.metadata?.capabilities ?? []).some((cap) => {
      const norm = cap.toLowerCase()
      return norm.includes(queryLower) || capabilityLabel(cap).toLowerCase().includes(queryLower)
    })
    if (capsMatch) matched += 1
  }
  return matched
}

function getGroupBaseCount(group: ModelPlazaGroup): number {
  if (props.groupTotalCounts && group.name in props.groupTotalCounts) {
    return props.groupTotalCounts[group.name] ?? 0
  }
  return group.models.length
}

function getGroupMatchCount(group: ModelPlazaGroup): number {
  if (!hasSearch.value) return getGroupBaseCount(group)
  if (props.groupMatchCounts && group.name in props.groupMatchCounts) {
    return props.groupMatchCounts[group.name] ?? 0
  }
  return fallbackMatchCount(group, trimmedSearch.value.toLowerCase())
}

const currentGroupRawMatchCount = computed(() =>
  activeGroup.value ? getGroupMatchCount(activeGroup.value) : 0
)

const otherGroupMatches = computed(() => {
  if (!hasSearch.value) return []
  return props.groups
    .filter((g) => g.name !== props.selectedGroupName)
    .map((g) => ({
      groupId: g.id,
      groupName: g.name,
      count: getGroupMatchCount(g)
    }))
    .filter((item) => item.count > 0)
    .sort((a, b) => b.count - a.count)
})

const totalOtherMatches = computed(() =>
  otherGroupMatches.value.reduce((sum, item) => sum + item.count, 0)
)

// Avoid false "not found in current group" banner when current group actually has matches
// that are simply filtered out by the currently active vendor chip.
const showCrossGroupEmptyBanner = computed(
  () =>
    hasSearch.value &&
    shownEntryCount.value === 0 &&
    currentGroupRawMatchCount.value === 0 &&
    otherGroupMatches.value.length > 0
)

const showCrossGroupAlsoFoundBanner = computed(
  () =>
    hasSearch.value &&
    !showCrossGroupEmptyBanner.value &&
    otherGroupMatches.value.length > 0 &&
    (shownEntryCount.value > 0 || currentGroupRawMatchCount.value > 0)
)

async function handleCopyAll() {
  const count = props.filteredModelIds.length
  if (count === 0) return
  const ok = await copyToClipboard(
    props.filteredModelIds.join(','),
    t('modelPlaza.toolbar.copiedAllModels', { count })
  )
  if (ok) {
    batchCopied.value = true
    batchCopiedCount.value = count
    if (batchCopyTimer) clearTimeout(batchCopyTimer)
    batchCopyTimer = setTimeout(() => {
      batchCopied.value = false
    }, 2000)
  }
}

function handleDocumentClick(event: MouseEvent) {
  if (!groupOpen.value && !filterOpen.value && !sortOpen.value) return
  const target = event.target as Node | null
  if (target && toolbarRoot.value?.contains(target)) return
  closeAllMenus()
}

function handleDocumentKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape') {
    closeAllMenus()
  }
}

onMounted(() => {
  document.addEventListener('click', handleDocumentClick, true)
  document.addEventListener('keydown', handleDocumentKeydown)
})

onBeforeUnmount(() => {
  document.removeEventListener('click', handleDocumentClick, true)
  document.removeEventListener('keydown', handleDocumentKeydown)
  if (batchCopyTimer) clearTimeout(batchCopyTimer)
})
</script>
