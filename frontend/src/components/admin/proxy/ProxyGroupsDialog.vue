<template>
  <BaseDialog :show="show" :title="t('proxyGroups.manage')" width="extra-wide" @close="close">
    <p class="mb-3 text-sm text-gray-500">{{ t('proxyGroups.hint') }}</p>
    <p class="mb-4 text-sm text-gray-500">{{ t('proxyGroups.selectedHint') }}</p>
    <p v-if="error" role="alert" class="mb-3 text-sm text-red-500">{{ error }}</p>
    <div v-if="loading" class="py-8 text-center">{{ t('common.loading') }}</div>
    <div v-else class="grid gap-5 md:grid-cols-[220px_1fr]">
      <div class="space-y-2">
        <button class="btn btn-primary w-full" :disabled="busy" @click="edit(null)">{{ t('proxyGroups.create') }}</button>
        <button v-for="group in groups" :key="group.id" class="w-full rounded-lg border p-3 text-left dark:border-dark-600" :class="editingId === group.id && 'border-primary-500 bg-primary-50 dark:bg-primary-900/20'" :disabled="busy" @click="edit(group)">
          <span class="block font-medium">{{ group.name }}</span>
          <span class="text-xs text-gray-500">{{ t('proxyGroups.count', { count: group.proxy_ids.length }) }}</span>
        </button>
      </div>
      <form class="space-y-4" @submit.prevent="save">
        <label class="block"><span class="input-label">{{ t('proxyGroups.name') }}</span><input v-model="name" class="input" maxlength="100" required :placeholder="t('proxyGroups.nameHint')" :disabled="busy" /></label>
        <label class="block"><span class="input-label">{{ t('proxyGroups.limit') }}</span><input v-model.number="limit" type="number" min="1" max="10000" step="1" required class="input" :disabled="busy" /></label>
        <div class="flex items-center justify-between gap-2">
          <span class="font-medium">{{ t('proxyGroups.members') }} ({{ memberIds.length }})</span>
          <button v-if="selectedIds.length" type="button" class="btn btn-secondary text-xs" :disabled="busy" @click="memberIds = [...new Set([...memberIds, ...selectedIds])]">{{ t('proxyGroups.addSelected', { count: selectedIds.length }) }}</button>
        </div>
        <input v-model="search" class="input" :placeholder="t('admin.proxies.searchProxies')" />
        <div class="max-h-72 overflow-y-auto rounded-lg border p-2 dark:border-dark-600">
          <label v-for="proxy in filteredProxies" :key="proxy.id" class="flex cursor-pointer items-center gap-3 rounded p-2 hover:bg-gray-50 dark:hover:bg-dark-700">
            <input v-model="memberIds" type="checkbox" :value="proxy.id" :disabled="busy" />
            <span class="min-w-0 flex-1"><span class="block truncate">{{ proxy.name }}</span><span class="text-xs text-gray-500">{{ proxy.host }}:{{ proxy.port }} · {{ groupName(proxy.id) }} · {{ proxy.status }}</span></span>
          </label>
        </div>
        <div class="flex justify-end gap-2">
          <button v-if="editingId" type="button" class="btn btn-danger" :disabled="busy" @click="confirmDelete = true">{{ t('common.delete') }}</button>
          <button type="submit" class="btn btn-primary" :disabled="busy || !name.trim()">{{ t('common.save') }}</button>
        </div>
      </form>
    </div>
  </BaseDialog>
  <ConfirmDialog :show="confirmDelete" :title="t('common.delete')" :message="t('proxyGroups.deleteConfirm')" :loading="busy" @confirm="remove" @cancel="confirmDelete = false" />
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import { adminAPI } from '@/api/admin'
import { deleteProxyGroup, listProxyGroups, saveProxyGroup, type ProxyGroup } from '@/api/admin/proxyGroups'
import { useAppStore } from '@/stores/app'
import type { Proxy } from '@/types'

const props = defineProps<{ show: boolean; selectedIds: number[] }>()
const emit = defineEmits<{ close: []; saved: [] }>()
const { t } = useI18n()
const app = useAppStore()
const groups = ref<ProxyGroup[]>([])
const proxies = ref<Proxy[]>([])
const editingId = ref<number | null>(null)
const name = ref('')
const limit = ref(2)
const memberIds = ref<number[]>([])
const search = ref('')
const error = ref('')
const loading = ref(false)
const busy = ref(false)
const confirmDelete = ref(false)
let generation = 0
const filteredProxies = computed(() => proxies.value.filter(p => `${p.name} ${p.host}`.toLowerCase().includes(search.value.toLowerCase())))
const groupName = (id: number) => groups.value.find(g => g.proxy_ids.includes(id))?.name || t('proxyGroups.ungrouped')
const message = (e: any) => e.response?.data?.message || e.message || t('proxyGroups.saveFailed')
function edit(group: ProxyGroup | null) {
  editingId.value = group?.id ?? null
  name.value = group?.name ?? ''
  limit.value = group?.max_accounts_per_proxy ?? 2
  memberIds.value = [...(group?.proxy_ids ?? props.selectedIds)]
  search.value = ''
}
function close() { if (!busy.value) emit('close') }
watch(() => props.show, async show => {
  const current = ++generation
  if (!show) return
  loading.value = true
  error.value = ''
  try {
    const result = await listProxyGroups()
    const all: Proxy[] = []
    for (let page = 1; ; page++) {
      const data = await adminAPI.proxies.list(page, 100)
      if (generation !== current) return
      all.push(...data.items)
      if (page >= data.pages || data.items.length === 0) break
    }
    if (generation !== current) return
    groups.value = result
    proxies.value = all
    edit(null)
  } catch (e) { if (generation === current) error.value = message(e) }
  finally { if (generation === current) loading.value = false }
}, { immediate: true })
async function save() {
  if (busy.value) return
  busy.value = true
  error.value = ''
  try {
    const saved = await saveProxyGroup(editingId.value, { name: name.value.trim(), max_accounts_per_proxy: limit.value, proxy_ids: memberIds.value })
    groups.value = await listProxyGroups()
    edit(groups.value.find(g => g.id === saved.id) ?? null)
    app.showSuccess(t('proxyGroups.saved'))
    emit('saved')
  } catch (e) { error.value = message(e) }
  finally { busy.value = false }
}
async function remove() {
  if (busy.value || editingId.value === null) return
  busy.value = true
  error.value = ''
  try {
    await deleteProxyGroup(editingId.value)
    groups.value = await listProxyGroups()
    confirmDelete.value = false
    edit(null)
    app.showSuccess(t('proxyGroups.deleted'))
    emit('saved')
  } catch (e) { error.value = message(e) }
  finally { busy.value = false }
}
</script>
