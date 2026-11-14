<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <div class="space-y-3">
          <div
            v-if="showDisabledNotice"
            class="flex flex-wrap items-center gap-x-2 gap-y-1 rounded-lg border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-800 dark:border-amber-500/30 dark:bg-amber-500/10 dark:text-amber-300"
            role="status"
          >
            <Icon name="exclamationTriangle" size="sm" class="shrink-0" />
            <span>{{ t('admin.resetCards.disabledNotice') }}</span>
            <router-link
              to="/admin/settings"
              class="font-medium underline underline-offset-2 hover:no-underline"
            >
              {{ t('admin.resetCards.disabledNoticeLink') }}
            </router-link>
            <span class="text-xs opacity-80">{{ t('admin.resetCards.disabledNoticeHint') }}</span>
          </div>

          <div class="flex flex-wrap items-center gap-3">
            <p class="min-w-0 flex-1 text-sm text-gray-500 dark:text-dark-400">
              {{ t('admin.resetCards.description') }}
            </p>
            <div class="flex flex-wrap items-center justify-end gap-2">
              <button
                class="btn btn-secondary"
                :disabled="loading"
                :title="t('common.refresh')"
                @click="loadBatches"
              >
                <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
              </button>
              <button class="btn btn-primary" @click="showIssueDialog = true">
                <Icon name="plus" size="md" class="mr-1" />
                {{ t('admin.resetCards.issueButton') }}
              </button>
            </div>
          </div>
        </div>
      </template>

      <template #table>
        <DataTable :columns="columns" :data="batches" :loading="loading">
          <template #cell-name="{ row }">
            <button
              type="button"
              class="text-left font-medium text-gray-900 hover:text-primary-600 dark:text-white dark:hover:text-primary-400"
              @click="openDetail(row)"
            >
              {{ row.name }}
            </button>
          </template>

          <template #cell-groups="{ row }">
            <div class="flex max-w-xs flex-wrap gap-1">
              <GroupBadge
                v-for="group in row.groups"
                :key="group.id"
                :name="groupLabel(group)"
                :platform="groupPlatforms[group.id]"
                :show-rate="false"
                :truncate="false"
              />
            </div>
          </template>

          <template #cell-rule="{ row }">
            <span class="whitespace-nowrap text-sm text-gray-700 dark:text-gray-300">
              {{ t('admin.resetCards.percentAndCap', { percent: formatPercent(row.percent), cap: formatYuan(row.max_amount) }) }}
            </span>
          </template>

          <template #cell-validity="{ row }">
            <span class="whitespace-nowrap text-sm text-gray-700 dark:text-gray-300">
              {{ t('admin.resetCards.validity', { cardDays: row.card_valid_days, creditDays: row.credit_valid_days }) }}
            </span>
          </template>

          <template #cell-cards="{ row }">
            <div class="grid grid-cols-5 gap-x-3 text-xs">
              <div v-for="item in cardCountItems(row)" :key="item.key" class="whitespace-nowrap">
                <div class="text-gray-400 dark:text-dark-400">{{ item.label }}</div>
                <div :class="['font-semibold tabular-nums', item.class]">{{ item.value }}</div>
              </div>
            </div>
          </template>

          <template #cell-amounts="{ row }">
            <div class="whitespace-nowrap text-xs leading-5">
              <div>
                <span class="text-gray-400 dark:text-dark-400">{{ t('admin.resetCards.stats.refundedTotal') }}</span>
                <span class="ml-1.5 font-semibold tabular-nums text-emerald-600 dark:text-emerald-400">
                  {{ formatYuan(row.stats.refunded_total, true) }}
                </span>
              </div>
              <div>
                <span class="text-gray-400 dark:text-dark-400">{{ t('admin.resetCards.stats.clawbackTotal') }}</span>
                <span class="ml-1.5 font-semibold tabular-nums text-gray-700 dark:text-gray-300">
                  {{ formatYuan(row.stats.clawback_total, true) }}
                </span>
              </div>
            </div>
          </template>

          <template #cell-created_at="{ value }">
            <span class="whitespace-nowrap text-sm text-gray-500 dark:text-dark-400">
              {{ formatDateTime(value) }}
            </span>
          </template>

          <template #cell-actions="{ row }">
            <div class="flex items-center gap-1">
              <button
                type="button"
                class="btn btn-ghost btn-sm gap-1"
                @click="openDetail(row)"
              >
                <Icon name="eye" size="sm" />
                {{ t('admin.resetCards.actions.detail') }}
              </button>
              <button
                type="button"
                class="btn btn-ghost btn-sm gap-1 text-red-600 disabled:cursor-not-allowed disabled:opacity-40 dark:text-red-400"
                :disabled="row.stats.unused <= 0"
                :title="row.stats.unused <= 0 ? t('admin.resetCards.revokeBatch.nothing') : ''"
                @click="askRevokeBatch(row)"
              >
                <Icon name="ban" size="sm" />
                {{ t('admin.resetCards.actions.revokeUnused') }}
              </button>
            </div>
          </template>

          <template #empty>
            <div class="flex flex-col items-center gap-3 py-10 text-sm text-gray-500 dark:text-dark-400">
              <Icon name="gift" size="lg" class="text-gray-300 dark:text-dark-500" />
              <span>{{ t('admin.resetCards.empty') }}</span>
              <button class="btn btn-primary btn-sm" @click="showIssueDialog = true">
                {{ t('admin.resetCards.issueButton') }}
              </button>
            </div>
          </template>
        </DataTable>
      </template>

      <template #pagination>
        <Pagination
          v-if="pagination.total > 0"
          :page="pagination.page"
          :total="pagination.total"
          :page-size="pagination.page_size"
          @update:page="handlePageChange"
          @update:pageSize="handlePageSizeChange"
        />
      </template>
    </TablePageLayout>

    <ResetCardIssueDialog
      :show="showIssueDialog"
      @close="showIssueDialog = false"
      @created="handleCreated"
    />

    <ResetCardBatchDetailDialog
      :show="detailBatch !== null"
      :batch="detailBatch"
      :group-platforms="groupPlatforms"
      @close="detailBatch = null"
      @changed="loadBatches"
    />

    <ConfirmDialog
      :show="revokingBatch !== null"
      :title="t('admin.resetCards.revokeBatch.title')"
      :message="revokeBatchMessage"
      :confirm-text="revokeSubmitting ? t('common.processing') : t('admin.resetCards.revokeBatch.confirmButton')"
      :cancel-text="t('common.cancel')"
      danger
      @confirm="confirmRevokeBatch"
      @cancel="revokingBatch = null"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { adminAPI } from '@/api/admin'
import { getPersistedPageSize } from '@/composables/usePersistedPageSize'
import { formatDateTime } from '@/utils/format'
import { extractApiErrorMessage } from '@/utils/apiError'
import type { GroupPlatform } from '@/types'
import type { ResetCardBatch, ResetCardGroup } from '@/types/resetCard'
import type { Column } from '@/components/common/types'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import GroupBadge from '@/components/common/GroupBadge.vue'
import Icon from '@/components/icons/Icon.vue'
import ResetCardIssueDialog from '@/components/admin/resetCards/ResetCardIssueDialog.vue'
import ResetCardBatchDetailDialog from '@/components/admin/resetCards/ResetCardBatchDetailDialog.vue'

const { t } = useI18n()
const appStore = useAppStore()

const batches = ref<ResetCardBatch[]>([])
const loading = ref(false)
const pagination = reactive({
  page: 1,
  page_size: getPersistedPageSize(),
  total: 0
})

const showIssueDialog = ref(false)
const detailBatch = ref<ResetCardBatch | null>(null)
const revokingBatch = ref<ResetCardBatch | null>(null)
const revokeSubmitting = ref(false)
const groupPlatforms = ref<Record<number, GroupPlatform>>({})

// 公共设置还没加载到时不显示提示，避免闪一下
const showDisabledNotice = computed(
  () => appStore.cachedPublicSettings !== null && appStore.cachedPublicSettings.reset_card_enabled !== true
)

const columns = computed<Column[]>(() => [
  { key: 'name', label: t('admin.resetCards.columns.name') },
  { key: 'groups', label: t('admin.resetCards.columns.groups') },
  { key: 'rule', label: t('admin.resetCards.columns.rule') },
  { key: 'validity', label: t('admin.resetCards.columns.validity') },
  { key: 'cards', label: t('admin.resetCards.columns.cards') },
  { key: 'amounts', label: t('admin.resetCards.columns.amounts') },
  { key: 'created_at', label: t('admin.resetCards.columns.createdAt') },
  { key: 'actions', label: t('admin.resetCards.columns.actions') }
])

const revokeBatchMessage = computed(() => {
  const batch = revokingBatch.value
  if (!batch) return ''
  return t('admin.resetCards.revokeBatch.confirm', { name: batch.name, count: batch.stats.unused })
})

function groupLabel(group: ResetCardGroup): string {
  return group.name || t('admin.resetCards.deletedGroup')
}

function formatPercent(value: number): string {
  return String(Number(Number(value).toFixed(2)))
}

/** fixed=false 时整数不带小数（用于上限这类管理员填的整数），fixed=true 时固定两位小数 */
function formatYuan(value: number | null | undefined, fixed = false): string {
  const n = Number(value ?? 0)
  if (!fixed && Number.isInteger(n)) return `$${n}`
  return `$${n.toFixed(2)}`
}

function cardCountItems(batch: ResetCardBatch) {
  const s = batch.stats
  return [
    { key: 'issued', label: t('admin.resetCards.stats.issued'), value: s.issued, class: 'text-gray-900 dark:text-white' },
    { key: 'unused', label: t('admin.resetCards.stats.unused'), value: s.unused, class: 'text-primary-600 dark:text-primary-400' },
    { key: 'used', label: t('admin.resetCards.stats.used'), value: s.used, class: 'text-emerald-600 dark:text-emerald-400' },
    { key: 'expired', label: t('admin.resetCards.stats.expired'), value: s.expired, class: 'text-gray-500 dark:text-dark-400' },
    { key: 'revoked', label: t('admin.resetCards.stats.revoked'), value: s.revoked, class: 'text-red-600 dark:text-red-400' }
  ]
}

let abortController: AbortController | null = null

async function loadBatches() {
  abortController?.abort()
  const controller = new AbortController()
  abortController = controller
  loading.value = true
  try {
    const res = await adminAPI.resetCards.listBatches(pagination.page, pagination.page_size, {
      signal: controller.signal
    })
    if (controller.signal.aborted || abortController !== controller) return
    batches.value = res.items || []
    pagination.total = res.total || 0
    // 详情弹窗开着时，用新数据刷新它的统计
    if (detailBatch.value) {
      const fresh = batches.value.find((b) => b.id === detailBatch.value?.id)
      if (fresh) detailBatch.value = fresh
    }
  } catch (error: any) {
    if (
      controller.signal.aborted ||
      abortController !== controller ||
      error?.name === 'AbortError' ||
      error?.code === 'ERR_CANCELED'
    ) {
      return
    }
    appStore.showError(extractApiErrorMessage(error, t('admin.resetCards.failedToLoad')))
  } finally {
    if (abortController === controller) {
      loading.value = false
      abortController = null
    }
  }
}

async function loadGroupPlatforms() {
  try {
    const groups = await adminAPI.groups.getAllIncludingInactive()
    const map: Record<number, GroupPlatform> = {}
    for (const g of groups) map[g.id] = g.platform
    groupPlatforms.value = map
  } catch {
    // 只影响分组徽标的颜色，失败不提示
  }
}

function handlePageChange(page: number) {
  pagination.page = page
  loadBatches()
}

function handlePageSizeChange(pageSize: number) {
  pagination.page_size = pageSize
  pagination.page = 1
  loadBatches()
}

function openDetail(batch: ResetCardBatch) {
  detailBatch.value = batch
}

function handleCreated() {
  showIssueDialog.value = false
  pagination.page = 1
  loadBatches()
}

function askRevokeBatch(batch: ResetCardBatch) {
  if (batch.stats.unused <= 0) {
    appStore.showInfo(t('admin.resetCards.revokeBatch.nothing'))
    return
  }
  revokingBatch.value = batch
}

async function confirmRevokeBatch() {
  const batch = revokingBatch.value
  if (!batch || revokeSubmitting.value) return
  revokeSubmitting.value = true
  try {
    const res = await adminAPI.resetCards.revokeBatch(batch.id)
    appStore.showSuccess(t('admin.resetCards.revokeBatch.success', { count: res.revoked }))
    revokingBatch.value = null
    loadBatches()
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('admin.resetCards.revokeBatch.failed')))
  } finally {
    revokeSubmitting.value = false
  }
}

onMounted(() => {
  appStore.fetchPublicSettings().catch(() => {})
  loadBatches()
  loadGroupPlatforms()
})

onUnmounted(() => {
  abortController?.abort()
})
</script>
