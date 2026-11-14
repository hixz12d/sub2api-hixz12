<template>
  <BaseDialog
    :show="show"
    :title="current ? current.name : t('admin.resetCards.detail.title')"
    width="extra-wide"
    @close="handleClose"
  >
    <div class="space-y-5">
      <!-- 批次参数 -->
      <div v-if="current" class="space-y-3">
        <div class="flex flex-wrap items-center gap-x-4 gap-y-2 text-sm text-gray-600 dark:text-gray-300">
          <div class="flex flex-wrap gap-1">
            <GroupBadge
              v-for="group in current.groups"
              :key="group.id"
              :name="group.name || t('admin.resetCards.deletedGroup')"
              :platform="groupPlatforms?.[group.id]"
              :show-rate="false"
              :truncate="false"
            />
          </div>
          <span>
            {{ t('admin.resetCards.percentAndCap', { percent: formatPercent(current.percent), cap: formatYuan(current.max_amount) }) }}
          </span>
          <span>
            {{ t('admin.resetCards.validity', { cardDays: current.card_valid_days, creditDays: current.credit_valid_days }) }}
          </span>
          <span>{{ targetLabel }}</span>
          <span class="text-gray-400 dark:text-dark-500">{{ formatDateTime(current.created_at) }}</span>
        </div>

        <!-- 统计数字 -->
        <div class="grid grid-cols-2 gap-px overflow-hidden rounded-lg border border-gray-200 bg-gray-200 sm:grid-cols-4 lg:grid-cols-7 dark:border-dark-600 dark:bg-dark-600">
          <div v-for="item in statItems" :key="item.key" class="bg-white px-3 py-2.5 dark:bg-dark-800">
            <div class="text-xs text-gray-500 dark:text-dark-400">{{ item.label }}</div>
            <div :class="['mt-0.5 text-lg font-semibold tabular-nums', item.class]">{{ item.value }}</div>
          </div>
        </div>
      </div>

      <!-- 筛选 -->
      <div class="flex flex-wrap items-center gap-3">
        <div class="min-w-0 flex-1 sm:max-w-72">
          <input
            v-model="search"
            type="text"
            class="input"
            :placeholder="t('admin.resetCards.detail.searchPlaceholder')"
            @input="handleSearch"
          />
        </div>
        <Select v-model="statusFilter" :options="statusOptions" class="w-36" @change="reloadFromFirstPage" />
        <button
          type="button"
          class="btn btn-secondary"
          :disabled="cardsLoading"
          :title="t('common.refresh')"
          @click="refreshAll"
        >
          <Icon name="refresh" size="md" :class="cardsLoading ? 'animate-spin' : ''" />
        </button>
      </div>

      <!-- 卡列表 -->
      <DataTable :columns="columns" :data="cards" :loading="cardsLoading">
        <template #cell-user="{ row }">
          <div class="min-w-0">
            <div class="truncate font-medium text-gray-900 dark:text-white">{{ row.user.email }}</div>
            <div v-if="row.user.username" class="truncate text-xs text-gray-500 dark:text-dark-400">
              {{ row.user.username }}
            </div>
          </div>
        </template>

        <template #cell-status="{ row }">
          <span :class="['badge inline-flex items-center gap-1', statusMeta(row.status).badge]">
            <Icon :name="statusMeta(row.status).icon" size="xs" />
            {{ statusMeta(row.status).label }}
          </span>
        </template>

        <template #cell-expires_at="{ value }">
          <span class="whitespace-nowrap text-sm text-gray-500 dark:text-dark-400">{{ formatDateTime(value) }}</span>
        </template>

        <template #cell-used_at="{ value }">
          <span class="whitespace-nowrap text-sm text-gray-500 dark:text-dark-400">
            {{ value ? formatDateTime(value) : '—' }}
          </span>
        </template>

        <template #cell-refund_amount="{ value }">
          <span
            v-if="value !== null && value !== undefined"
            class="whitespace-nowrap text-sm font-medium tabular-nums text-emerald-600 dark:text-emerald-400"
          >
            {{ formatYuan(value, true) }}
          </span>
          <span v-else class="text-sm text-gray-400 dark:text-dark-500">—</span>
        </template>

        <template #cell-credit="{ row }">
          <span v-if="!row.credit" class="text-sm text-gray-400 dark:text-dark-500">—</span>
          <span
            v-else-if="row.credit.status === 'settled'"
            class="whitespace-nowrap text-sm text-gray-600 dark:text-gray-300"
          >
            {{ t('admin.resetCards.detail.creditSettled', { amount: formatYuan(row.credit.clawback_amount ?? 0, true) }) }}
          </span>
          <span v-else class="text-sm text-gray-600 dark:text-gray-300">
            {{
              t('admin.resetCards.detail.creditActive', {
                used: formatYuan(row.credit.used_amount, true),
                total: formatYuan(row.credit.amount, true),
                expires: formatDateTime(row.credit.expires_at)
              })
            }}
          </span>
        </template>

        <template #cell-actions="{ row }">
          <button
            v-if="row.status === 'unused'"
            type="button"
            class="btn btn-ghost btn-sm gap-1 text-red-600 dark:text-red-400"
            @click="revokingCard = row"
          >
            <Icon name="ban" size="sm" />
            {{ t('admin.resetCards.actions.revoke') }}
          </button>
          <span v-else class="text-sm text-gray-400 dark:text-dark-500">—</span>
        </template>

        <template #empty>
          <div class="py-8 text-center text-sm text-gray-500 dark:text-dark-400">
            {{ t('admin.resetCards.detail.noCards') }}
          </div>
        </template>
      </DataTable>

      <Pagination
        v-if="pagination.total > 0"
        :page="pagination.page"
        :total="pagination.total"
        :page-size="pagination.page_size"
        @update:page="handlePageChange"
        @update:pageSize="handlePageSizeChange"
      />
    </div>

    <template #footer>
      <div class="flex justify-end">
        <button type="button" class="btn btn-secondary" @click="handleClose">{{ t('common.close') }}</button>
      </div>
    </template>
  </BaseDialog>

  <ConfirmDialog
    :show="revokingCard !== null"
    :title="t('admin.resetCards.revokeCard.title')"
    :message="revokeCardMessage"
    :confirm-text="revokeSubmitting ? t('common.processing') : t('admin.resetCards.revokeCard.confirmButton')"
    :cancel-text="t('common.cancel')"
    danger
    @confirm="confirmRevokeCard"
    @cancel="revokingCard = null"
  />
</template>

<script setup lang="ts">
import { computed, onUnmounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { adminAPI } from '@/api/admin'
import { getPersistedPageSize } from '@/composables/usePersistedPageSize'
import { formatDateTime } from '@/utils/format'
import { extractApiErrorMessage } from '@/utils/apiError'
import type { GroupPlatform } from '@/types'
import type { AdminResetCard, ResetCardBatch, ResetCardStatus } from '@/types/resetCard'
import type { Column } from '@/components/common/types'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import Select from '@/components/common/Select.vue'
import GroupBadge from '@/components/common/GroupBadge.vue'
import Icon from '@/components/icons/Icon.vue'

const props = defineProps<{
  show: boolean
  /** 列表页里的批次数据，打开时先用它显示，再拉最新统计 */
  batch: ResetCardBatch | null
  /** 分组 ID → 平台，只用于分组徽标配色 */
  groupPlatforms?: Record<number, GroupPlatform>
}>()

const emit = defineEmits<{
  (e: 'close'): void
  /** 作废后通知列表页刷新统计 */
  (e: 'changed'): void
}>()

const { t } = useI18n()
const appStore = useAppStore()

const fresh = ref<ResetCardBatch | null>(null)
const current = computed(() => fresh.value ?? props.batch)

const cards = ref<AdminResetCard[]>([])
const cardsLoading = ref(false)
const search = ref('')
const statusFilter = ref<ResetCardStatus | ''>('')
const pagination = reactive({
  page: 1,
  page_size: getPersistedPageSize(),
  total: 0
})

const revokingCard = ref<AdminResetCard | null>(null)
const revokeSubmitting = ref(false)

const columns = computed<Column[]>(() => [
  { key: 'user', label: t('admin.resetCards.detail.columns.user') },
  { key: 'status', label: t('admin.resetCards.detail.columns.status') },
  { key: 'expires_at', label: t('admin.resetCards.detail.columns.expiresAt') },
  { key: 'used_at', label: t('admin.resetCards.detail.columns.usedAt') },
  { key: 'refund_amount', label: t('admin.resetCards.detail.columns.refund') },
  { key: 'credit', label: t('admin.resetCards.detail.columns.credit') },
  { key: 'actions', label: t('admin.resetCards.detail.columns.actions') }
])

const statusOptions = computed(() => [
  { value: '', label: t('admin.resetCards.status.all') },
  { value: 'unused', label: t('admin.resetCards.status.unused') },
  { value: 'used', label: t('admin.resetCards.status.used') },
  { value: 'expired', label: t('admin.resetCards.status.expired') },
  { value: 'revoked', label: t('admin.resetCards.status.revoked') }
])

const targetLabel = computed(() => {
  const b = current.value
  if (!b) return ''
  if (b.target_mode === 'users') return t('admin.resetCards.detail.targetUsers')
  return b.only_active ? t('admin.resetCards.detail.targetAllActive') : t('admin.resetCards.detail.targetAll')
})

const statItems = computed(() => {
  const s = current.value?.stats
  if (!s) return []
  return [
    { key: 'issued', label: t('admin.resetCards.stats.issued'), value: s.issued, class: 'text-gray-900 dark:text-white' },
    { key: 'unused', label: t('admin.resetCards.stats.unused'), value: s.unused, class: 'text-primary-600 dark:text-primary-400' },
    { key: 'used', label: t('admin.resetCards.stats.used'), value: s.used, class: 'text-emerald-600 dark:text-emerald-400' },
    { key: 'expired', label: t('admin.resetCards.stats.expired'), value: s.expired, class: 'text-gray-500 dark:text-dark-400' },
    { key: 'revoked', label: t('admin.resetCards.stats.revoked'), value: s.revoked, class: 'text-red-600 dark:text-red-400' },
    { key: 'refunded', label: t('admin.resetCards.stats.refundedTotal'), value: formatYuan(s.refunded_total, true), class: 'text-emerald-600 dark:text-emerald-400' },
    { key: 'clawback', label: t('admin.resetCards.stats.clawbackTotal'), value: formatYuan(s.clawback_total, true), class: 'text-gray-900 dark:text-white' }
  ]
})

const revokeCardMessage = computed(() => {
  const card = revokingCard.value
  if (!card) return ''
  return t('admin.resetCards.revokeCard.confirm', { user: card.user.email || card.user.username })
})

type StatusIcon = 'gift' | 'checkCircle' | 'clock' | 'ban'
const STATUS_META: Record<ResetCardStatus, { badge: string; icon: StatusIcon }> = {
  unused: { badge: 'badge-primary', icon: 'gift' },
  used: { badge: 'badge-success', icon: 'checkCircle' },
  expired: { badge: 'badge-gray', icon: 'clock' },
  revoked: { badge: 'badge-danger', icon: 'ban' }
}

function statusMeta(status: ResetCardStatus) {
  const meta = STATUS_META[status] ?? STATUS_META.expired
  return { ...meta, label: t(`admin.resetCards.status.${status}`) }
}

function formatPercent(value: number): string {
  return String(Number(Number(value).toFixed(2)))
}

function formatYuan(value: number | null | undefined, fixed = false): string {
  const n = Number(value ?? 0)
  if (!fixed && Number.isInteger(n)) return `$${n}`
  return `$${n.toFixed(2)}`
}

let cardsController: AbortController | null = null
let searchTimer: ReturnType<typeof setTimeout> | null = null

async function loadBatch() {
  const id = props.batch?.id
  if (!id) return
  try {
    const res = await adminAPI.resetCards.getBatch(id)
    if (props.show && props.batch?.id === id) fresh.value = res
  } catch (error) {
    if (props.show && props.batch?.id === id) {
      appStore.showError(extractApiErrorMessage(error, t('admin.resetCards.detail.loadFailed')))
    }
  }
}

async function loadCards() {
  const id = props.batch?.id
  if (!props.show || !id) return
  cardsController?.abort()
  const controller = new AbortController()
  cardsController = controller
  cardsLoading.value = true
  try {
    const res = await adminAPI.resetCards.listCards(
      id,
      pagination.page,
      pagination.page_size,
      {
        status: statusFilter.value || undefined,
        search: search.value.trim() || undefined
      },
      { signal: controller.signal }
    )
    if (controller.signal.aborted || cardsController !== controller) return
    cards.value = res.items || []
    pagination.total = res.total || 0
  } catch (error: any) {
    if (
      controller.signal.aborted ||
      cardsController !== controller ||
      error?.name === 'AbortError' ||
      error?.code === 'ERR_CANCELED'
    ) {
      return
    }
    appStore.showError(extractApiErrorMessage(error, t('admin.resetCards.detail.cardsLoadFailed')))
  } finally {
    if (cardsController === controller) {
      cardsLoading.value = false
      cardsController = null
    }
  }
}

function refreshAll() {
  loadBatch()
  loadCards()
}

function reloadFromFirstPage() {
  pagination.page = 1
  loadCards()
}

function handleSearch() {
  if (searchTimer) clearTimeout(searchTimer)
  searchTimer = setTimeout(reloadFromFirstPage, 300)
}

function handlePageChange(page: number) {
  pagination.page = page
  loadCards()
}

function handlePageSizeChange(pageSize: number) {
  pagination.page_size = pageSize
  pagination.page = 1
  loadCards()
}

async function confirmRevokeCard() {
  const card = revokingCard.value
  if (!card || revokeSubmitting.value) return
  revokeSubmitting.value = true
  try {
    await adminAPI.resetCards.revokeCard(card.id)
    appStore.showSuccess(t('admin.resetCards.revokeCard.success'))
    revokingCard.value = null
    refreshAll()
    emit('changed')
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('admin.resetCards.revokeCard.failed')))
  } finally {
    revokeSubmitting.value = false
  }
}

function resetState() {
  if (searchTimer) {
    clearTimeout(searchTimer)
    searchTimer = null
  }
  cardsController?.abort()
  cardsController = null
  cardsLoading.value = false
  fresh.value = null
  cards.value = []
  search.value = ''
  statusFilter.value = ''
  pagination.page = 1
  pagination.total = 0
  revokingCard.value = null
}

function handleClose() {
  resetState()
  emit('close')
}

watch(
  () => [props.show, props.batch?.id] as const,
  ([visible, id], old) => {
    const [wasVisible, oldId] = old ?? [false, undefined]
    if (!visible) {
      resetState()
      return
    }
    // 只在刚打开或换了批次时重新加载；列表页刷新同一批次的数据不重复请求
    if (!wasVisible || id !== oldId) {
      resetState()
      refreshAll()
    }
  }
)

onUnmounted(() => {
  if (searchTimer) clearTimeout(searchTimer)
  cardsController?.abort()
})
</script>
