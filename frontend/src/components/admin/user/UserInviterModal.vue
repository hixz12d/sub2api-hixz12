<template>
  <BaseDialog :show="show" :title="t('admin.users.inviter.title')" width="narrow" @close="handleClose">
    <div v-if="user" class="space-y-5">
      <div class="flex items-center gap-3 rounded-xl bg-gray-50 p-4 dark:bg-dark-700">
        <div class="flex h-10 w-10 items-center justify-center rounded-full bg-primary-100"><span class="text-lg font-medium text-primary-700">{{ user.email.charAt(0).toUpperCase() }}</span></div>
        <div class="min-w-0 flex-1">
          <p class="truncate font-medium text-gray-900 dark:text-gray-100">{{ user.email }}</p>
          <p class="text-sm text-gray-500 dark:text-gray-400">#{{ user.id }}<span v-if="user.username"> · {{ user.username }}</span></p>
        </div>
      </div>

      <!-- 当前邀请人 -->
      <div>
        <label class="input-label">{{ t('admin.users.inviter.current') }}</label>
        <div v-if="loading" class="flex items-center gap-2 py-2 text-sm text-gray-500 dark:text-gray-400">
          <span class="h-4 w-4 animate-spin rounded-full border-2 border-primary-500 border-t-transparent"></span>
          {{ t('common.loading') }}
        </div>
        <div v-else-if="loadFailed" class="flex items-center justify-between gap-3 rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700 dark:border-red-800 dark:bg-red-900/20 dark:text-red-300">
          <span>{{ t('admin.users.inviter.loadFailed') }}</span>
          <button type="button" class="btn btn-secondary btn-sm" @click="loadInviter">{{ t('common.retry') }}</button>
        </div>
        <div
          v-else-if="current?.inviter"
          class="flex items-center justify-between gap-3 rounded-lg border border-gray-200 bg-gray-50 px-3 py-2 dark:border-dark-600 dark:bg-dark-800"
          data-test="inviter-current"
        >
          <div class="min-w-0 text-sm">
            <p class="truncate">
              <span class="font-medium text-gray-900 dark:text-white">{{ current.inviter.email }}</span>
              <span v-if="current.inviter.username" class="ml-1 text-xs text-gray-500 dark:text-dark-400">({{ current.inviter.username }})</span>
              <span class="ml-1 font-mono text-xs text-gray-500 dark:text-dark-400">#{{ current.inviter.id }}</span>
            </p>
            <p v-if="current.bound_at" class="mt-0.5 text-xs text-gray-500 dark:text-dark-400">
              {{ t('admin.users.inviter.boundAt') }}: {{ formatDateTime(current.bound_at) }}
            </p>
          </div>
          <button
            type="button"
            class="shrink-0 rounded-lg px-2.5 py-1 text-sm font-medium text-red-600 hover:bg-red-50 disabled:opacity-50 dark:text-red-400 dark:hover:bg-red-900/20"
            :disabled="submitting"
            data-test="inviter-unbind"
            @click="showUnbindConfirm = true"
          >
            {{ t('admin.users.inviter.unbind') }}
          </button>
        </div>
        <p v-else class="py-2 text-sm text-gray-500 dark:text-gray-400" data-test="inviter-unbound">{{ t('admin.users.inviter.unbound') }}</p>
      </div>

      <!-- 设置 / 改绑 -->
      <div>
        <label class="input-label">{{ current?.inviter ? t('admin.users.inviter.change') : t('admin.users.inviter.set') }}</label>
        <div
          v-if="selectedUser"
          class="flex items-center justify-between gap-3 rounded-md border border-primary-200 bg-primary-50 px-3 py-2 dark:border-primary-700/50 dark:bg-primary-900/20"
          data-test="inviter-selected"
        >
          <div class="min-w-0 truncate text-sm">
            <span class="font-mono text-gray-500 dark:text-dark-400">#{{ selectedUser.id }}</span>
            <span class="ml-2 font-medium text-gray-900 dark:text-white">{{ selectedUser.email }}</span>
            <span v-if="selectedUser.username" class="ml-1 text-xs text-gray-500 dark:text-dark-400">({{ selectedUser.username }})</span>
          </div>
          <button
            type="button"
            class="text-lg leading-none text-gray-400 hover:text-red-600 disabled:opacity-50"
            :title="t('admin.users.inviter.reselect')"
            :disabled="submitting"
            @click="clearSelection"
          >
            ×
          </button>
        </div>
        <template v-else>
          <input
            v-model="query"
            type="text"
            class="input"
            :placeholder="t('admin.users.inviter.searchPlaceholder')"
            :disabled="loading || loadFailed"
            data-test="inviter-search"
            @input="onQueryInput"
          />
          <div
            v-if="results.length > 0"
            class="mt-1 max-h-48 overflow-y-auto rounded-lg border border-gray-200 dark:border-dark-700"
          >
            <button
              v-for="item in results"
              :key="item.id"
              type="button"
              class="w-full px-3 py-1.5 text-left text-sm text-gray-900 hover:bg-gray-100 dark:text-white dark:hover:bg-dark-800"
              data-test="inviter-option"
              @click="selectUser(item)"
            >
              <span class="font-mono text-xs text-gray-500 dark:text-dark-400">#{{ item.id }}</span>
              {{ item.email }}
              <span v-if="item.username" class="text-xs text-gray-500 dark:text-dark-400">({{ item.username }})</span>
            </button>
          </div>
          <p v-else-if="searched" class="input-hint">{{ t('admin.users.inviter.noUserFound') }}</p>
        </template>
      </div>

      <p class="text-xs leading-relaxed text-gray-500 dark:text-gray-400">{{ t('admin.users.inviter.hint') }}</p>
    </div>

    <template #footer>
      <div class="flex justify-end gap-3">
        <button type="button" class="btn btn-secondary" :disabled="submitting" @click="handleClose">{{ t('common.cancel') }}</button>
        <button
          type="button"
          class="btn btn-primary"
          :disabled="!selectedUser || submitting || loading"
          data-test="inviter-save"
          @click="save"
        >
          {{ submitting ? t('common.saving') : t('admin.users.inviter.save') }}
        </button>
      </div>
    </template>
  </BaseDialog>

  <ConfirmDialog
    :show="showUnbindConfirm"
    :title="t('admin.users.inviter.unbindTitle')"
    :message="t('admin.users.inviter.unbindConfirm', { email: user?.email ?? '', inviter: current?.inviter?.email ?? '' })"
    :confirm-text="t('admin.users.inviter.unbind')"
    danger
    @confirm="unbind"
    @cancel="showUnbindConfirm = false"
  />
</template>

<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { affiliatesAPI, type SimpleUser, type UserInviterInfo } from '@/api/admin/affiliates'
import { extractI18nErrorMessage } from '@/utils/apiError'
import { formatDateTime } from '@/utils/format'
import type { AdminUser } from '@/types'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'

const props = defineProps<{ show: boolean, user: AdminUser | null }>()
const emit = defineEmits(['close', 'success'])
const { t } = useI18n()
const appStore = useAppStore()

const current = ref<UserInviterInfo | null>(null)
const loading = ref(false)
const loadFailed = ref(false)
const submitting = ref(false)
const showUnbindConfirm = ref(false)

const query = ref('')
const results = ref<SimpleUser[]>([])
const searched = ref(false)
const selectedUser = ref<SimpleUser | null>(null)

let searchTimer: ReturnType<typeof setTimeout> | null = null
let searchSeq = 0
let loadSeq = 0

function showError(error: unknown) {
  appStore.showError(extractI18nErrorMessage(error, t, 'admin.users.inviter.errors', t('common.error')))
}

function clearSearchTimer() {
  if (searchTimer) {
    clearTimeout(searchTimer)
    searchTimer = null
  }
}

async function loadInviter() {
  if (!props.user) return
  const seq = ++loadSeq
  loading.value = true
  loadFailed.value = false
  try {
    const info = await affiliatesAPI.getUserInviter(props.user.id)
    if (seq !== loadSeq) return
    current.value = info
  } catch (error) {
    if (seq !== loadSeq) return
    loadFailed.value = true
    showError(error)
  } finally {
    if (seq === loadSeq) loading.value = false
  }
}

function onQueryInput() {
  clearSearchTimer()
  searched.value = false
  const q = query.value.trim()
  if (!q) {
    searchSeq++
    results.value = []
    return
  }
  searchTimer = setTimeout(() => {
    void searchUsers(q)
  }, 300)
}

async function searchUsers(q: string) {
  const seq = ++searchSeq
  try {
    const users = await affiliatesAPI.lookupUsers(q)
    if (seq !== searchSeq) return
    // 不能把用户自己设为邀请人
    results.value = (users || []).filter((u) => u.id !== props.user?.id)
    searched.value = true
  } catch (error) {
    if (seq !== searchSeq) return
    showError(error)
  }
}

function selectUser(item: SimpleUser) {
  clearSearchTimer()
  searchSeq++
  selectedUser.value = item
  query.value = ''
  results.value = []
  searched.value = false
}

function clearSelection() {
  selectedUser.value = null
}

function resetForm() {
  clearSearchTimer()
  searchSeq++
  loadSeq++
  current.value = null
  loading.value = false
  loadFailed.value = false
  submitting.value = false
  showUnbindConfirm.value = false
  query.value = ''
  results.value = []
  searched.value = false
  selectedUser.value = null
}

function handleClose() {
  if (submitting.value) return
  emit('close')
}

async function save() {
  if (!props.user || !selectedUser.value) return
  submitting.value = true
  try {
    current.value = await affiliatesAPI.setUserInviter(props.user.id, selectedUser.value.id)
    selectedUser.value = null
    appStore.showSuccess(t('admin.users.inviter.saveSuccess'))
    emit('success')
    emit('close')
  } catch (error) {
    showError(error)
  } finally {
    submitting.value = false
  }
}

async function unbind() {
  showUnbindConfirm.value = false
  if (!props.user) return
  submitting.value = true
  try {
    current.value = await affiliatesAPI.clearUserInviter(props.user.id)
    appStore.showSuccess(t('admin.users.inviter.unbindSuccess'))
    emit('success')
  } catch (error) {
    showError(error)
  } finally {
    submitting.value = false
  }
}

watch(
  () => props.show,
  (show) => {
    resetForm()
    if (show) void loadInviter()
  },
)

onBeforeUnmount(clearSearchTimer)
</script>
