<template>
  <BaseDialog
    :show="show"
    :title="t('admin.resetCards.issue.title')"
    width="wide"
    @close="requestClose"
  >
    <div class="space-y-5">
      <!-- 步骤条 -->
      <ol class="flex flex-wrap items-center gap-x-1 gap-y-2 text-xs">
        <li v-for="(label, index) in stepLabels" :key="index" class="flex items-center gap-1">
          <button
            type="button"
            :disabled="index + 1 > maxReachedStep || submitting"
            :class="[
              'flex items-center gap-1.5 rounded-full px-2.5 py-1 font-medium transition-colors',
              index + 1 === step
                ? 'bg-primary-600 text-white dark:bg-primary-500'
                : index + 1 < step
                  ? 'text-primary-700 hover:bg-primary-50 dark:text-primary-300 dark:hover:bg-primary-900/20'
                  : 'text-gray-400 dark:text-dark-500',
              index + 1 <= maxReachedStep && index + 1 !== step ? 'cursor-pointer' : 'cursor-default'
            ]"
            @click="goToStep(index + 1)"
          >
            <span class="tabular-nums">{{ index + 1 }}</span>
            <span>{{ label }}</span>
          </button>
          <Icon
            v-if="index < stepLabels.length - 1"
            name="chevronRight"
            size="xs"
            class="text-gray-300 dark:text-dark-500"
          />
        </li>
      </ol>

      <!-- 第 1 步：分组 -->
      <section v-if="step === 1" class="space-y-2">
        <div v-if="groupsLoading" class="flex items-center gap-2 py-6 text-sm text-gray-500 dark:text-dark-400">
          <Icon name="refresh" size="sm" class="animate-spin" />
          {{ t('common.loading') }}
        </div>
        <template v-else>
          <GroupSelector v-model="form.group_ids" :groups="groups" />
          <p class="text-xs text-gray-500 dark:text-dark-400">{{ t('admin.resetCards.issue.groupsHint') }}</p>
          <p v-if="errors.group_ids" class="text-xs text-red-600 dark:text-red-400">{{ errors.group_ids }}</p>
        </template>
      </section>

      <!-- 第 2 步：比例与上限 -->
      <section v-else-if="step === 2" class="grid gap-4 sm:grid-cols-2">
        <div>
          <label class="input-label" for="reset-card-percent">{{ t('admin.resetCards.issue.percentLabel') }}</label>
          <input
            id="reset-card-percent"
            v-model.number="form.percent"
            type="number"
            min="0"
            max="100"
            step="0.01"
            :class="['input', errors.percent && 'border-red-500 dark:border-red-500']"
          />
          <p v-if="errors.percent" class="mt-1 text-xs text-red-600 dark:text-red-400">{{ errors.percent }}</p>
          <p v-else class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ t('admin.resetCards.issue.percentHint') }}</p>
        </div>
        <div>
          <label class="input-label" for="reset-card-max">{{ t('admin.resetCards.issue.maxAmountLabel') }}</label>
          <input
            id="reset-card-max"
            v-model.number="form.max_amount"
            type="number"
            min="0"
            step="0.01"
            :class="['input', errors.max_amount && 'border-red-500 dark:border-red-500']"
          />
          <p v-if="errors.max_amount" class="mt-1 text-xs text-red-600 dark:text-red-400">{{ errors.max_amount }}</p>
          <p v-else class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ t('admin.resetCards.issue.maxAmountHint') }}</p>
        </div>
      </section>

      <!-- 第 3 步：有效期 -->
      <section v-else-if="step === 3" class="space-y-3">
        <div class="grid gap-4 sm:grid-cols-2">
          <div>
            <label class="input-label" for="reset-card-card-days">{{ t('admin.resetCards.issue.cardValidDaysLabel') }}</label>
            <input
              id="reset-card-card-days"
              v-model.number="form.card_valid_days"
              type="number"
              min="1"
              max="90"
              step="1"
              :class="['input', errors.card_valid_days && 'border-red-500 dark:border-red-500']"
            />
            <p v-if="errors.card_valid_days" class="mt-1 text-xs text-red-600 dark:text-red-400">
              {{ errors.card_valid_days }}
            </p>
          </div>
          <div>
            <label class="input-label" for="reset-card-credit-days">{{ t('admin.resetCards.issue.creditValidDaysLabel') }}</label>
            <input
              id="reset-card-credit-days"
              v-model.number="form.credit_valid_days"
              type="number"
              min="1"
              max="30"
              step="1"
              :class="['input', errors.credit_valid_days && 'border-red-500 dark:border-red-500']"
            />
            <p v-if="errors.credit_valid_days" class="mt-1 text-xs text-red-600 dark:text-red-400">
              {{ errors.credit_valid_days }}
            </p>
          </div>
        </div>
        <p class="text-xs text-gray-500 dark:text-dark-400">
          {{ t('admin.resetCards.issue.validityHint', { cardDays: displayDays(form.card_valid_days), creditDays: displayDays(form.credit_valid_days) }) }}
        </p>
      </section>

      <!-- 第 4 步：发卡对象 -->
      <section v-else-if="step === 4" class="space-y-3">
        <div class="grid gap-3 sm:grid-cols-2" role="radiogroup">
          <label
            v-for="mode in targetModes"
            :key="mode.value"
            :class="[
              'flex cursor-pointer gap-3 rounded-lg border p-3 transition-colors',
              form.target_mode === mode.value
                ? 'border-primary-500 ring-1 ring-primary-500'
                : 'border-gray-200 hover:border-gray-300 dark:border-dark-600 dark:hover:border-dark-500'
            ]"
          >
            <input
              v-model="form.target_mode"
              type="radio"
              name="reset-card-target"
              :value="mode.value"
              class="mt-0.5 h-4 w-4 shrink-0 border-gray-300 text-primary-600 focus:ring-primary-500 dark:border-dark-500"
            />
            <span class="min-w-0">
              <span class="block text-sm font-medium text-gray-900 dark:text-white">{{ mode.label }}</span>
              <span class="mt-0.5 block text-xs text-gray-500 dark:text-dark-400">{{ mode.desc }}</span>
            </span>
          </label>
        </div>

        <label
          v-if="form.target_mode === 'all'"
          class="flex cursor-pointer items-start gap-2 rounded-lg bg-gray-50 px-3 py-2.5 text-sm text-gray-700 dark:bg-dark-700 dark:text-gray-200"
        >
          <input
            v-model="form.only_active"
            type="checkbox"
            class="mt-0.5 h-4 w-4 shrink-0 rounded border-gray-300 text-primary-600 focus:ring-primary-500 dark:border-dark-500"
          />
          <span>{{ t('admin.resetCards.issue.onlyActive') }}</span>
        </label>

        <div v-else class="space-y-2">
          <OpenAIFastPolicyUserSelector v-model="form.user_ids" />
          <p class="text-xs text-gray-500 dark:text-dark-400">
            {{ t('admin.resetCards.issue.selectedUsers', { count: form.user_ids.length }) }}
          </p>
          <p v-if="errors.user_ids" class="text-xs text-red-600 dark:text-red-400">{{ errors.user_ids }}</p>
        </div>
      </section>

      <!-- 第 5 步：预览 -->
      <section v-else class="space-y-4">
        <div class="rounded-xl border border-gray-200 p-4 dark:border-dark-600">
          <div v-if="previewLoading" class="flex items-center gap-2 text-sm text-gray-500 dark:text-dark-400">
            <Icon name="refresh" size="sm" class="animate-spin" />
            {{ t('admin.resetCards.issue.previewLoading') }}
          </div>
          <div v-else-if="previewError" class="flex flex-wrap items-center gap-3 text-sm text-red-600 dark:text-red-400">
            <Icon name="exclamationCircle" size="sm" />
            <span>{{ previewError }}</span>
            <button type="button" class="btn btn-secondary btn-sm" @click="runPreview">
              {{ t('admin.resetCards.issue.retry') }}
            </button>
          </div>
          <div v-else-if="preview" class="space-y-1.5">
            <p class="text-2xl font-semibold tabular-nums text-gray-900 dark:text-white">
              {{ t('admin.resetCards.issue.recipients', { count: preview.recipient_count }) }}
            </p>
            <p class="text-sm text-gray-600 dark:text-gray-300">
              {{ t('admin.resetCards.issue.maxCost', { amount: formatYuan(preview.estimated_max_cost) }) }}
            </p>
            <p
              v-if="preview.recipient_count === 0"
              class="flex items-start gap-1.5 pt-1 text-sm text-amber-700 dark:text-amber-300"
            >
              <Icon name="exclamationTriangle" size="sm" class="mt-0.5 shrink-0" />
              <span>{{ zeroRecipientReason }}</span>
            </p>
          </div>
        </div>

        <div>
          <h4 class="mb-2 text-sm font-medium text-gray-900 dark:text-white">{{ t('admin.resetCards.issue.summary') }}</h4>
          <dl class="divide-y divide-gray-100 rounded-lg border border-gray-200 text-sm dark:divide-dark-700 dark:border-dark-600">
            <div class="flex flex-col gap-1 px-3 py-2 sm:flex-row sm:gap-4">
              <dt class="w-28 shrink-0 text-gray-500 dark:text-dark-400">{{ t('admin.resetCards.issue.summaryGroups') }}</dt>
              <dd class="flex flex-wrap gap-1">
                <GroupBadge
                  v-for="group in selectedGroups"
                  :key="group.id"
                  :name="group.name"
                  :platform="group.platform"
                  :show-rate="false"
                  :truncate="false"
                />
              </dd>
            </div>
            <div class="flex flex-col gap-1 px-3 py-2 sm:flex-row sm:gap-4">
              <dt class="w-28 shrink-0 text-gray-500 dark:text-dark-400">{{ t('admin.resetCards.issue.summaryRule') }}</dt>
              <dd class="text-gray-900 dark:text-white">
                {{ t('admin.resetCards.percentAndCap', { percent: formatPercent(form.percent), cap: formatYuan(form.max_amount) }) }}
              </dd>
            </div>
            <div class="flex flex-col gap-1 px-3 py-2 sm:flex-row sm:gap-4">
              <dt class="w-28 shrink-0 text-gray-500 dark:text-dark-400">{{ t('admin.resetCards.issue.summaryValidity') }}</dt>
              <dd class="text-gray-900 dark:text-white">
                {{ t('admin.resetCards.validity', { cardDays: form.card_valid_days, creditDays: form.credit_valid_days }) }}
              </dd>
            </div>
            <div class="flex flex-col gap-1 px-3 py-2 sm:flex-row sm:gap-4">
              <dt class="w-28 shrink-0 text-gray-500 dark:text-dark-400">{{ t('admin.resetCards.issue.summaryTarget') }}</dt>
              <dd class="text-gray-900 dark:text-white">{{ targetSummary }}</dd>
            </div>
          </dl>
        </div>

        <div>
          <label class="input-label" for="reset-card-name">{{ t('admin.resetCards.issue.nameLabel') }}</label>
          <input
            id="reset-card-name"
            v-model="form.name"
            type="text"
            maxlength="100"
            :placeholder="t('admin.resetCards.issue.namePlaceholder')"
            :class="['input', errors.name && 'border-red-500 dark:border-red-500']"
          />
          <p v-if="errors.name" class="mt-1 text-xs text-red-600 dark:text-red-400">{{ errors.name }}</p>
        </div>
      </section>
    </div>

    <template #footer>
      <div class="flex flex-wrap items-center justify-between gap-3">
        <span class="text-xs text-gray-400 dark:text-dark-500">
          {{ t('admin.resetCards.issue.stepOf', { current: step, total: TOTAL_STEPS }) }}
        </span>
        <div class="flex flex-wrap justify-end gap-2">
          <button type="button" class="btn btn-secondary" :disabled="submitting" @click="requestClose">
            {{ t('common.cancel') }}
          </button>
          <button v-if="step > 1" type="button" class="btn btn-secondary" :disabled="submitting" @click="prevStep">
            {{ t('admin.resetCards.issue.prev') }}
          </button>
          <button v-if="step < TOTAL_STEPS" type="button" class="btn btn-primary" @click="nextStep">
            {{ t('admin.resetCards.issue.next') }}
          </button>
          <button
            v-else
            type="button"
            class="btn btn-primary"
            :disabled="!canSubmit"
            @click="submit"
          >
            <Icon v-if="submitting" name="refresh" size="sm" class="mr-1 animate-spin" />
            {{ submitting ? t('admin.resetCards.issue.submitting') : t('admin.resetCards.issue.submit') }}
          </button>
        </div>
      </div>
    </template>
  </BaseDialog>

  <ConfirmDialog
    :show="showDiscardConfirm"
    :title="t('admin.resetCards.issue.discardTitle')"
    :message="t('admin.resetCards.issue.discardMessage')"
    :confirm-text="t('admin.resetCards.issue.discardConfirm')"
    :cancel-text="t('common.cancel')"
    danger
    @confirm="discardAndClose"
    @cancel="showDiscardConfirm = false"
  />
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { adminAPI } from '@/api/admin'
import { formatDateLocalInput } from '@/utils/format'
import { extractApiErrorMessage } from '@/utils/apiError'
import type { AdminGroup } from '@/types'
import type {
  ResetCardBatch,
  ResetCardBatchPreviewRequest,
  ResetCardBatchPreviewResponse,
  ResetCardTargetMode
} from '@/types/resetCard'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import GroupSelector from '@/components/common/GroupSelector.vue'
import GroupBadge from '@/components/common/GroupBadge.vue'
import Icon from '@/components/icons/Icon.vue'
import OpenAIFastPolicyUserSelector from '@/views/admin/settings/OpenAIFastPolicyUserSelector.vue'

const props = defineProps<{ show: boolean }>()
const emit = defineEmits<{
  (e: 'close'): void
  (e: 'created', batch: ResetCardBatch): void
}>()

const { t } = useI18n()
const appStore = useAppStore()

const TOTAL_STEPS = 5
const MAX_USERS = 2000
const MAX_NAME_LENGTH = 100
const DEFAULTS = {
  percent: 30,
  card_valid_days: 14,
  credit_valid_days: 7
}

interface IssueForm {
  name: string
  group_ids: number[]
  percent: number | ''
  max_amount: number | ''
  card_valid_days: number | ''
  credit_valid_days: number | ''
  target_mode: ResetCardTargetMode
  only_active: boolean
  user_ids: number[]
}

type FieldKey = 'group_ids' | 'percent' | 'max_amount' | 'card_valid_days' | 'credit_valid_days' | 'user_ids' | 'name'

function defaultName(): string {
  return t('admin.resetCards.issue.defaultName', { date: formatDateLocalInput(new Date()) })
}

function freshForm(): IssueForm {
  return {
    name: defaultName(),
    group_ids: [],
    percent: DEFAULTS.percent,
    max_amount: '',
    card_valid_days: DEFAULTS.card_valid_days,
    credit_valid_days: DEFAULTS.credit_valid_days,
    target_mode: 'all',
    only_active: true,
    user_ids: []
  }
}

const form = reactive<IssueForm>(freshForm())
const errors = reactive<Partial<Record<FieldKey, string>>>({})
const step = ref(1)
const maxReachedStep = ref(1)
const submitting = ref(false)
const showDiscardConfirm = ref(false)

const groups = ref<AdminGroup[]>([])
const groupsLoading = ref(false)

const preview = ref<ResetCardBatchPreviewResponse | null>(null)
const previewLoading = ref(false)
const previewError = ref('')
let previewSequence = 0

const stepLabels = computed(() => [
  t('admin.resetCards.issue.steps.groups'),
  t('admin.resetCards.issue.steps.rule'),
  t('admin.resetCards.issue.steps.validity'),
  t('admin.resetCards.issue.steps.target'),
  t('admin.resetCards.issue.steps.preview')
])

const targetModes = computed(() => [
  {
    value: 'all' as const,
    label: t('admin.resetCards.issue.targetAll'),
    desc: t('admin.resetCards.issue.targetAllDesc')
  },
  {
    value: 'users' as const,
    label: t('admin.resetCards.issue.targetUsers'),
    desc: t('admin.resetCards.issue.targetUsersDesc')
  }
])

const selectedGroups = computed(() => {
  const byId = new Map(groups.value.map((g) => [g.id, g]))
  return form.group_ids.map((id) => {
    const g = byId.get(id)
    return { id, name: g?.name || t('admin.resetCards.deletedGroup'), platform: g?.platform }
  })
})

const targetSummary = computed(() => {
  if (form.target_mode === 'users') {
    return t('admin.resetCards.issue.summaryTargetUsers', { count: form.user_ids.length })
  }
  return form.only_active
    ? t('admin.resetCards.issue.summaryTargetAllActive')
    : t('admin.resetCards.issue.summaryTargetAll')
})

const zeroRecipientReason = computed(() =>
  form.target_mode === 'all' && form.only_active
    ? t('admin.resetCards.issue.zeroRecipientsActive')
    : t('admin.resetCards.issue.zeroRecipients')
)

const canSubmit = computed(
  () =>
    !submitting.value &&
    !previewLoading.value &&
    !previewError.value &&
    preview.value !== null &&
    preview.value.recipient_count > 0
)

// 填过内容（和打开时的默认值不同）才需要关闭前确认
const isDirty = computed(
  () =>
    step.value > 1 ||
    form.group_ids.length > 0 ||
    form.percent !== DEFAULTS.percent ||
    form.max_amount !== '' ||
    form.card_valid_days !== DEFAULTS.card_valid_days ||
    form.credit_valid_days !== DEFAULTS.credit_valid_days ||
    form.target_mode !== 'all' ||
    !form.only_active ||
    form.user_ids.length > 0
)

function displayDays(value: number | ''): string {
  return value === '' ? '—' : String(value)
}

function formatPercent(value: number | ''): string {
  return value === '' ? '—' : String(Number(Number(value).toFixed(2)))
}

function formatYuan(value: number | '' | null | undefined): string {
  if (value === '' || value === null || value === undefined) return '—'
  const n = Number(value)
  return Number.isInteger(n) ? `$${n}` : `$${n.toFixed(2)}`
}

function clearErrors() {
  for (const key of Object.keys(errors) as FieldKey[]) delete errors[key]
}

function isNumber(value: number | ''): value is number {
  return typeof value === 'number' && Number.isFinite(value)
}

function validateStep(target: number): boolean {
  clearErrors()
  if (target === 1) {
    if (form.group_ids.length === 0) errors.group_ids = t('admin.resetCards.issue.errors.groupsRequired')
  } else if (target === 2) {
    if (!isNumber(form.percent) || form.percent <= 0 || form.percent > 100) {
      errors.percent = t('admin.resetCards.issue.errors.percentRange')
    }
    if (!isNumber(form.max_amount) || form.max_amount <= 0) {
      errors.max_amount = t('admin.resetCards.issue.errors.maxAmountRequired')
    }
  } else if (target === 3) {
    const c = form.card_valid_days
    if (!isNumber(c) || !Number.isInteger(c) || c < 1 || c > 90) {
      errors.card_valid_days = t('admin.resetCards.issue.errors.cardValidDaysRange')
    }
    const d = form.credit_valid_days
    if (!isNumber(d) || !Number.isInteger(d) || d < 1 || d > 30) {
      errors.credit_valid_days = t('admin.resetCards.issue.errors.creditValidDaysRange')
    }
  } else if (target === 4) {
    if (form.target_mode === 'users') {
      if (form.user_ids.length === 0) {
        errors.user_ids = t('admin.resetCards.issue.errors.usersRequired')
      } else if (form.user_ids.length > MAX_USERS) {
        errors.user_ids = t('admin.resetCards.issue.errors.usersTooMany', { max: MAX_USERS })
      }
    }
  } else if (target === 5) {
    const name = form.name.trim()
    if (!name) errors.name = t('admin.resetCards.issue.errors.nameRequired')
    else if ([...name].length > MAX_NAME_LENGTH) {
      errors.name = t('admin.resetCards.issue.errors.nameTooLong', { max: MAX_NAME_LENGTH })
    }
  }
  return Object.keys(errors).length === 0
}

function enterStep(target: number) {
  step.value = target
  if (target > maxReachedStep.value) maxReachedStep.value = target
  if (target === TOTAL_STEPS) runPreview()
}

function nextStep() {
  if (!validateStep(step.value)) return
  enterStep(step.value + 1)
}

function prevStep() {
  clearErrors()
  if (step.value > 1) step.value -= 1
}

function goToStep(target: number) {
  if (submitting.value || target === step.value || target > maxReachedStep.value) return
  if (target < step.value) {
    clearErrors()
    step.value = target
    return
  }
  // 往后跳：前面每一步都要先通过校验
  for (let s = step.value; s < target; s++) {
    if (!validateStep(s)) {
      step.value = s
      return
    }
  }
  enterStep(target)
}

function buildPreviewRequest(): ResetCardBatchPreviewRequest {
  const request: ResetCardBatchPreviewRequest = {
    group_ids: [...form.group_ids],
    percent: Number(form.percent),
    max_amount: Number(form.max_amount),
    target_mode: form.target_mode,
    only_active: form.target_mode === 'all' ? form.only_active : false
  }
  if (form.target_mode === 'users') request.user_ids = [...form.user_ids]
  return request
}

async function runPreview() {
  const sequence = ++previewSequence
  preview.value = null
  previewError.value = ''
  previewLoading.value = true
  try {
    const res = await adminAPI.resetCards.previewBatch(buildPreviewRequest())
    if (sequence !== previewSequence) return
    preview.value = res
  } catch (error) {
    if (sequence !== previewSequence) return
    previewError.value = extractApiErrorMessage(error, t('admin.resetCards.issue.previewFailed'))
  } finally {
    if (sequence === previewSequence) previewLoading.value = false
  }
}

async function submit() {
  if (!canSubmit.value) return
  if (!validateStep(5)) return
  submitting.value = true
  try {
    const batch = await adminAPI.resetCards.createBatch({
      ...buildPreviewRequest(),
      name: form.name.trim(),
      card_valid_days: Number(form.card_valid_days),
      credit_valid_days: Number(form.credit_valid_days)
    })
    appStore.showSuccess(t('admin.resetCards.issue.success', { count: batch.stats?.issued ?? 0 }))
    resetState()
    emit('created', batch)
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('admin.resetCards.issue.failed')))
  } finally {
    submitting.value = false
  }
}

async function loadGroups() {
  groupsLoading.value = true
  try {
    groups.value = await adminAPI.groups.getAll()
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('admin.resetCards.issue.groupsLoadFailed')))
  } finally {
    groupsLoading.value = false
  }
}

function resetState() {
  Object.assign(form, freshForm())
  clearErrors()
  step.value = 1
  maxReachedStep.value = 1
  previewSequence++
  preview.value = null
  previewError.value = ''
  previewLoading.value = false
  showDiscardConfirm.value = false
}

function requestClose() {
  if (submitting.value) return
  if (isDirty.value) {
    showDiscardConfirm.value = true
    return
  }
  resetState()
  emit('close')
}

function discardAndClose() {
  resetState()
  emit('close')
}

watch(
  () => props.show,
  (visible) => {
    if (!visible) return
    resetState()
    loadGroups()
  }
)

// 改了分组或发卡对象，旧的用户校验提示就不再准确
watch(
  () => [form.group_ids.length, form.user_ids.length, form.target_mode],
  () => {
    if (errors.group_ids && form.group_ids.length > 0) delete errors.group_ids
    if (errors.user_ids && (form.target_mode === 'all' || form.user_ids.length > 0)) delete errors.user_ids
  }
)
</script>
