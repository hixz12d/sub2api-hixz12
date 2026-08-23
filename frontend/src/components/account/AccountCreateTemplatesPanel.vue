<template>
  <div class="card" data-testid="account-create-templates-panel">
    <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
      <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
        {{ t('admin.settings.features.accountCreateTemplates.title') }}
      </h2>
      <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
        {{ t('admin.settings.features.accountCreateTemplates.description') }}
      </p>
    </div>
    <div class="space-y-4 p-6">
      <p class="text-xs text-gray-500 dark:text-gray-400">
        {{ t('admin.settings.features.accountCreateTemplates.manageHint') }}
      </p>
      <div v-if="loading" class="text-sm text-gray-500 dark:text-gray-400">
        {{ t('common.loading') }}
      </div>
      <div
        v-else-if="templates.length === 0"
        class="rounded-lg border border-dashed border-gray-200 px-4 py-6 text-sm text-gray-500 dark:border-dark-600 dark:text-gray-400"
      >
        {{ t('admin.settings.features.accountCreateTemplates.empty') }}
      </div>
      <div v-else class="space-y-3">
        <div
          v-for="item in templates"
          :key="item.id"
          class="rounded-lg border border-gray-200 p-4 dark:border-dark-600"
          :data-testid="`account-create-template-row-${item.id}`"
        >
          <div class="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
            <div class="min-w-0 flex-1 space-y-2">
              <div class="flex flex-wrap items-center gap-2">
                <input
                  v-model="item.name"
                  type="text"
                  class="input max-w-xs"
                  maxlength="80"
                  :aria-label="t('admin.accounts.createTemplate.name')"
                />
                <span class="rounded bg-gray-100 px-2 py-0.5 text-xs text-gray-600 dark:bg-dark-700 dark:text-gray-300">
                  {{ item.platform }} / {{ item.type }}
                </span>
                <span
                  v-if="item.is_default"
                  class="rounded bg-primary-50 px-2 py-0.5 text-xs text-primary-700 dark:bg-primary-900/30 dark:text-primary-300"
                >
                  {{ t('admin.accounts.createTemplate.defaultBadge') }}
                </span>
              </div>
              <p class="text-xs text-gray-500 dark:text-gray-400">
                {{ previewOf(item) }}
              </p>
              <label class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
                <input
                  v-model="item.include_groups"
                  type="checkbox"
                  class="rounded border-gray-300 text-primary-600 focus:ring-primary-500"
                />
                {{ t('admin.accounts.createTemplate.includeGroups') }}
              </label>
            </div>
            <div class="flex flex-wrap gap-2">
              <button
                type="button"
                class="btn btn-secondary text-sm"
                :disabled="savingId === item.id"
                @click="saveItem(item)"
              >
                {{ t('common.save') }}
              </button>
              <button
                type="button"
                class="btn btn-secondary text-sm"
                :disabled="savingId === item.id || item.is_default"
                @click="setDefault(item)"
              >
                {{ t('admin.accounts.createTemplate.setDefault') }}
              </button>
              <button
                type="button"
                class="btn btn-secondary text-sm text-red-600 hover:text-red-700"
                :disabled="savingId === item.id"
                @click="removeItem(item)"
              >
                {{ t('common.delete') }}
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import { useAppStore } from '@/stores'
import { extractApiErrorMessage } from '@/utils/apiError'
import type { AccountPlatform, AccountType, AdminGroup, Proxy } from '@/types'
import {
  buildAccountCreateTemplatePreview,
  normalizeAccountCreateTemplateValues,
  type AccountCreateTemplate,
} from './accountCreateTemplate'

const { t } = useI18n()
const appStore = useAppStore()
const templates = ref<AccountCreateTemplate[]>([])
const proxies = ref<Proxy[]>([])
const groups = ref<AdminGroup[]>([])
const loading = ref(false)
const savingId = ref('')

const previewOf = (item: AccountCreateTemplate) =>
  buildAccountCreateTemplatePreview(normalizeAccountCreateTemplateValues(item.values), {
    includeGroups: item.include_groups,
    proxies: proxies.value,
    groups: groups.value,
  })

const load = async () => {
  loading.value = true
  try {
    const [templateResp, proxyResp, groupResp] = await Promise.all([
      adminAPI.settings.listAccountCreateTemplates(),
      adminAPI.proxies.list().catch(() => ({ items: [] as Proxy[] })),
      adminAPI.groups.getAll().catch(() => [] as AdminGroup[]),
    ])
    templates.value = (templateResp.items ?? []).map((item) => ({
      ...item,
      platform: item.platform as AccountPlatform,
      type: item.type as AccountType,
      values: normalizeAccountCreateTemplateValues(item.values),
    }))
    proxies.value = proxyResp.items ?? []
    groups.value = Array.isArray(groupResp) ? groupResp : []
  } catch (err) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  } finally {
    loading.value = false
  }
}

const saveItem = async (item: AccountCreateTemplate, extra: Partial<AccountCreateTemplate> = {}) => {
  savingId.value = item.id
  try {
    const next = { ...item, ...extra }
    await adminAPI.settings.updateAccountCreateTemplate(item.id, {
      name: next.name.trim(),
      platform: next.platform,
      type: next.type,
      is_default: next.is_default,
      include_groups: next.include_groups,
      values: next.include_groups
        ? next.values
        : { ...next.values, group_ids: [] },
    })
    await load()
    appStore.showSuccess(t('admin.accounts.createTemplate.saveSuccess'))
  } catch (err) {
    appStore.showError(extractApiErrorMessage(err, t('admin.accounts.createTemplate.saveFailed')))
  } finally {
    savingId.value = ''
  }
}

const setDefault = (item: AccountCreateTemplate) => {
  void saveItem(item, { is_default: true })
}

const removeItem = async (item: AccountCreateTemplate) => {
  savingId.value = item.id
  try {
    await adminAPI.settings.deleteAccountCreateTemplate(item.id)
    await load()
    appStore.showSuccess(t('admin.accounts.createTemplate.deleteSuccess'))
  } catch (err) {
    appStore.showError(extractApiErrorMessage(err, t('admin.accounts.createTemplate.deleteFailed')))
  } finally {
    savingId.value = ''
  }
}

onMounted(() => {
  void load()
})
</script>
