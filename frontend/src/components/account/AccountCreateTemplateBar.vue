<template>
  <div
    class="border-t border-gray-200 pt-4 dark:border-dark-600"
    data-testid="account-create-template-bar"
  >
    <div class="mb-3">
      <h3 class="input-label mb-0 text-base font-semibold">
        {{ t('admin.accounts.createTemplate.title') }}
      </h3>
      <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
        {{ t('admin.accounts.createTemplate.hint') }}
      </p>
    </div>

    <div class="flex flex-col gap-3 lg:flex-row lg:items-center">
      <div class="min-w-0 flex-1">
        <Select
          v-model="selectedId"
          data-testid="account-create-template-select"
          :options="selectOptions"
          :placeholder="t('admin.accounts.createTemplate.placeholder')"
        />
      </div>
      <div class="flex flex-wrap gap-2">
        <button
          type="button"
          class="btn btn-secondary text-sm"
          :disabled="!selectedTemplate || saving"
          data-testid="account-create-template-save"
          @click="openSave(false)"
        >
          {{ t('admin.accounts.createTemplate.save') }}
        </button>
        <button
          type="button"
          class="btn btn-secondary text-sm"
          :disabled="saving"
          data-testid="account-create-template-save-as"
          @click="openSave(true)"
        >
          {{ t('admin.accounts.createTemplate.saveAs') }}
        </button>
      </div>
    </div>

    <label class="mt-3 flex items-start gap-2 text-sm text-gray-700 dark:text-gray-300">
      <input
        v-model="applyGroups"
        type="checkbox"
        class="mt-0.5 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
        data-testid="account-create-template-apply-groups"
      />
      <span>
        {{ t('admin.accounts.createTemplate.applyGroups') }}
        <span class="block text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.accounts.createTemplate.applyGroupsHint') }}
        </span>
      </span>
    </label>

    <p
      v-if="previewText"
      class="mt-2 text-xs text-gray-500 dark:text-gray-400"
      data-testid="account-create-template-preview"
    >
      {{ previewText }}
    </p>
  </div>

  <AccountCreateTemplateSaveDialog
    :show="saveDialog.show"
    :title="saveDialogTitle"
    :initial-name="saveDialog.name"
    :initial-include-groups="saveDialog.includeGroups"
    :initial-is-default="saveDialog.isDefault"
    :submitting="saving"
    @close="saveDialog.show = false"
    @confirm="handleSaveConfirm"
  />
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import Select from '@/components/common/Select.vue'
import type { AccountPlatform, AccountType, AdminGroup, Proxy } from '@/types'
import AccountCreateTemplateSaveDialog from './AccountCreateTemplateSaveDialog.vue'
import {
  ACCOUNT_CREATE_TEMPLATE_NONE,
  applyAccountCreateTemplateGroups,
  buildAccountCreateTemplatePreview,
  filterAccountCreateTemplates,
  findDefaultAccountCreateTemplate,
  normalizeAccountCreateTemplateValues,
  type AccountCreateTemplate,
  type AccountCreateTemplateValues,
} from './accountCreateTemplate'

const props = defineProps<{
  platform: AccountPlatform
  accountType: AccountType
  proxies: Proxy[]
  groups: AdminGroup[]
  autoApply?: boolean
  active?: boolean
  resetToken?: string | number | null
  getSnapshot: () => AccountCreateTemplateValues
  applySnapshot: (values: AccountCreateTemplateValues, options: { includeGroups: boolean }) => void
}>()

const { t } = useI18n()
const appStore = useAppStore()
const templates = ref<AccountCreateTemplate[]>([])
const selectedId = ref(ACCOUNT_CREATE_TEMPLATE_NONE)
const applyGroups = ref(false)
const saving = ref(false)
const skipNextSelectApply = ref(false)
const userPickedNoneForScope = ref('')
const saveDialog = reactive({
  show: false,
  asNew: true,
  name: '',
  includeGroups: false,
  isDefault: false,
})

const scopedTemplates = computed(() =>
  filterAccountCreateTemplates(templates.value, props.platform, props.accountType),
)

const selectedTemplate = computed(
  () => scopedTemplates.value.find((item) => item.id === selectedId.value) ?? null,
)

const selectOptions = computed(() => [
  { value: ACCOUNT_CREATE_TEMPLATE_NONE, label: t('admin.accounts.createTemplate.none') },
  ...scopedTemplates.value.map((item) => ({
    value: item.id,
    label: item.is_default
      ? t('admin.accounts.createTemplate.defaultOption', { name: item.name })
      : item.name,
  })),
])

const previewText = computed(() => {
  const values = selectedTemplate.value
    ? normalizeAccountCreateTemplateValues(selectedTemplate.value.values)
    : null
  if (!values) return ''
  return t('admin.accounts.createTemplate.preview', {
    summary: buildAccountCreateTemplatePreview(values, {
      includeGroups: applyGroups.value,
      proxies: props.proxies,
      groups: props.groups,
    }),
  })
})

const saveDialogTitle = computed(() =>
  saveDialog.asNew
    ? t('admin.accounts.createTemplate.saveAsTitle')
    : t('admin.accounts.createTemplate.saveTitle'),
)

const scopeKey = computed(() => `${props.platform}:${props.accountType}`)

const loadTemplates = async () => {
  try {
    const { items } = await adminAPI.settings.listAccountCreateTemplates()
    templates.value = (items ?? []).map((item) => ({
      ...item,
      platform: item.platform as AccountPlatform,
      type: item.type as AccountType,
      values: normalizeAccountCreateTemplateValues(item.values),
    }))
  } catch (err) {
    templates.value = []
    console.warn('load account create templates failed', err)
  }
}

const applyTemplate = (template: AccountCreateTemplate | null) => {
  if (!template) return
  const values = normalizeAccountCreateTemplateValues(template.values)
  props.applySnapshot(values, { includeGroups: applyGroups.value })
}

const syncSelectionForScope = (reason: 'scope' | 'reload') => {
  const scoped = scopedTemplates.value
  const stillValid = scoped.some((item) => item.id === selectedId.value)
  if (stillValid && reason === 'reload') return
  if (userPickedNoneForScope.value === scopeKey.value && reason === 'scope') {
    selectedId.value = ACCOUNT_CREATE_TEMPLATE_NONE
    applyGroups.value = false
    return
  }
  const fallback = props.autoApply
    ? findDefaultAccountCreateTemplate(templates.value, props.platform, props.accountType)
    : null
  skipNextSelectApply.value = true
  selectedId.value = fallback?.id ?? ACCOUNT_CREATE_TEMPLATE_NONE
  applyGroups.value = fallback?.include_groups === true
  if (fallback && props.autoApply) {
    applyTemplate(fallback)
  }
}

watch(
  () => [props.platform, props.accountType],
  () => {
    syncSelectionForScope('scope')
  },
)

watch(
  () => props.resetToken,
  (token, previous) => {
    if (token === previous) return
    userPickedNoneForScope.value = ''
    syncSelectionForScope('scope')
  },
)

watch(
  () => props.active,
  (active) => {
    if (active && selectedTemplate.value) {
      applyTemplate(selectedTemplate.value)
    }
  },
)

watch(selectedId, (id, previous) => {
  if (skipNextSelectApply.value) {
    skipNextSelectApply.value = false
    return
  }
  if (id === ACCOUNT_CREATE_TEMPLATE_NONE) {
    userPickedNoneForScope.value = scopeKey.value
    applyGroups.value = false
    return
  }
  userPickedNoneForScope.value = ''
  const template = selectedTemplate.value
  if (!template) return
  if (previous === ACCOUNT_CREATE_TEMPLATE_NONE || previous !== id) {
    applyGroups.value = template.include_groups
  }
  applyTemplate(template)
})

watch(
  () => applyGroups.value,
  () => {
    if (selectedTemplate.value) {
      applyTemplate(selectedTemplate.value)
    }
  },
)

const openSave = (asNew: boolean) => {
  const current = selectedTemplate.value
  saveDialog.asNew = asNew || !current
  saveDialog.name = asNew ? '' : (current?.name ?? '')
  saveDialog.includeGroups = current?.include_groups ?? props.getSnapshot().group_ids.length > 0
  saveDialog.isDefault = current?.is_default ?? scopedTemplates.value.length === 0
  saveDialog.show = true
}

const handleSaveConfirm = async (payload: {
  name: string
  includeGroups: boolean
  isDefault: boolean
}) => {
  saving.value = true
  try {
    const values = {
      ...normalizeAccountCreateTemplateValues(props.getSnapshot()),
      group_ids: applyAccountCreateTemplateGroups(
        props.getSnapshot().group_ids,
        normalizeAccountCreateTemplateValues(props.getSnapshot()),
        payload.includeGroups,
      ),
    }
    if (!payload.includeGroups) {
      values.group_ids = []
    }
    const body = {
      name: payload.name,
      platform: props.platform,
      type: props.accountType,
      is_default: payload.isDefault,
      include_groups: payload.includeGroups,
      values,
    }
    const current = selectedTemplate.value
    const saved = saveDialog.asNew || !current
      ? await adminAPI.settings.createAccountCreateTemplate(body)
      : await adminAPI.settings.updateAccountCreateTemplate(current.id, body)
    await loadTemplates()
    skipNextSelectApply.value = true
    selectedId.value = saved.id
    applyGroups.value = saved.include_groups
    saveDialog.show = false
    appStore.showSuccess(t('admin.accounts.createTemplate.saveSuccess'))
  } catch (err) {
    appStore.showError(extractApiErrorMessage(err, t('admin.accounts.createTemplate.saveFailed')))
  } finally {
    saving.value = false
  }
}

loadTemplates().then(() => {
  syncSelectionForScope('reload')
})
</script>
