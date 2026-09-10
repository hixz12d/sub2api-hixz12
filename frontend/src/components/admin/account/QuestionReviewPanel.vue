<template>
  <section class="min-w-0 space-y-3 border-t border-gray-200 pt-4 dark:border-dark-500">
    <div class="flex items-center justify-between gap-2">
      <h3 class="text-sm font-semibold">{{ t('admin.accounts.questionReview.title') }}</h3>
      <button type="button" class="btn btn-secondary p-2" :disabled="busy" :title="t('admin.accounts.questionReview.reload')" @click="load()"><Icon name="refresh" size="sm" /></button>
    </div>
    <p v-if="error" role="alert" class="text-sm text-red-600">{{ error }}</p>
    <p v-if="!busy && !records.length" class="text-sm text-gray-500">{{ t('admin.accounts.questionReview.empty') }}</p>
    <select v-if="records.length" v-model="selected" class="input w-full min-w-0" :disabled="busy || saving" :aria-label="t('admin.accounts.questionReview.title')" @change="loadHistory()">
      <option v-for="record in records" :key="record.id" :value="record.id">{{ record.request_model }} · {{ new Date(record.created_at).toLocaleString() }}</option>
    </select>
    <div class="flex justify-end gap-2">
      <button type="button" class="btn btn-secondary p-2" :disabled="busy || saving || page === 1" :title="t('admin.accounts.questionReview.previous')" @click="page--; load()"><Icon name="chevronLeft" size="sm" /></button>
      <button type="button" class="btn btn-secondary p-2" :disabled="busy || saving || !hasMore" :title="t('admin.accounts.questionReview.next')" @click="page++; load()"><Icon name="chevronRight" size="sm" /></button>
    </div>
    <template v-if="current">
      <div class="text-xs text-gray-500">{{ t(`admin.accounts.questionReview.${current.transport_state}`) }}</div>
      <pre class="max-h-32 overflow-auto whitespace-pre-wrap break-words text-sm">{{ current.prompt }}</pre>
      <pre class="max-h-48 overflow-auto whitespace-pre-wrap break-words border-l-2 border-gray-300 pl-3 text-sm">{{ current.answer }}</pre>
      <form class="space-y-2" @submit.prevent="save">
        <label class="block text-sm">{{ t('admin.accounts.questionReview.verdict') }}
          <select v-model="verdict" class="input mt-1 w-full" :disabled="saving || !historyReady">
            <option v-for="value in verdicts" :key="value" :value="value">{{ t(`admin.accounts.questionReview.${value}`) }}</option>
          </select>
        </label>
        <label class="block text-sm">{{ t('admin.accounts.questionReview.reason') }}
          <textarea v-model="reason" class="input mt-1 w-full" rows="2" maxlength="2000" required :disabled="saving || !historyReady" />
        </label>
        <button type="submit" class="btn btn-primary" :disabled="saving || !historyReady || !reason.trim() || reasonBytes > 2000">{{ t('admin.accounts.questionReview.save') }}</button>
      </form>
      <ol class="max-h-48 divide-y divide-gray-200 overflow-auto text-sm dark:divide-dark-500">
        <li v-for="review in history" :key="review.id" class="space-y-1 py-2">
          <div>{{ t(`admin.accounts.questionReview.${review.verdict}`) }} · #{{ review.revision }} · {{ review.reviewed_by }} · {{ new Date(review.created_at).toLocaleString() }}</div>
          <p class="whitespace-pre-wrap break-words text-gray-500">{{ review.reason }}</p>
        </li>
      </ol>
      <button v-if="historyMore" type="button" class="btn btn-secondary" :disabled="busy || saving" @click="loadHistory(true)">{{ t('admin.accounts.questionReview.more') }}</button>
    </template>
    <p v-if="accountDisabled" role="status" class="text-sm">{{ t('admin.accounts.questionReview.disabled') }}</p>
    <button v-else-if="historyReady && history[0]?.verdict === 'degraded'" type="button" class="btn btn-secondary" :disabled="saving || disabling" @click="disableTarget = accountId">{{ t('admin.accounts.questionReview.disable') }}</button>
    <ConfirmDialog :show="disableTarget !== null" :title="t('admin.accounts.questionReview.disable')" :message="t('admin.accounts.questionReview.disableConfirm', { id: disableTarget })" :danger="true" @cancel="disableTarget = null" @confirm="disableAccount" />
  </section>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Icon } from '@/components/icons'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import { toggleStatus } from '@/api/admin/accounts'
import { questionAPI, type QuestionRecord, type QuestionReview, type QuestionVerdict } from '@/api/admin/questionReviews'
const props = defineProps<{ accountId: number; refreshToken?: number }>()
const { t } = useI18n()
const emit = defineEmits<{ (event: 'account-updated'): void }>()
const disableTarget = ref<number | null>(null)
const disabling = ref(false)
const accountDisabled = ref(false)
async function disableAccount() {
  const id = disableTarget.value
  if (id === null || id !== props.accountId || disabling.value) return
  disableTarget.value = null; disabling.value = true
  try {
    await toggleStatus(id, 'inactive')
    if (id === props.accountId) accountDisabled.value = true
    emit('account-updated')
  } catch { error.value = t('admin.accounts.questionReview.disableFailed') }
  finally { disabling.value = false }
}
watch(() => props.accountId, () => { disableTarget.value = null; accountDisabled.value = false })
const records = ref<QuestionRecord[]>([])
const selected = ref('')
const current = computed(() => records.value.find(record => record.id === selected.value))
const page = ref(1)
const hasMore = ref(false)
const busy = ref(false)
const saving = ref(false)
const error = ref('')
const history = ref<QuestionReview[]>([])
const historyPage = ref(1)
const historyMore = ref(false)
const historyReady = ref(false)
const verdicts: QuestionVerdict[] = ['unlabeled', 'normal', 'degraded']
const verdict = ref<QuestionVerdict>('unlabeled')
const reason = ref('')
const reasonBytes = computed(() => new TextEncoder().encode(reason.value.trim()).length)
let controller: AbortController | undefined
let historyController: AbortController | undefined
async function load() {
  controller?.abort()
  const request = new AbortController(); controller = request
  historyController?.abort(); historyReady.value = false
  busy.value = true; error.value = ''
  try {
    const result = await questionAPI.list(props.accountId, page.value, request.signal)
    if (request.signal.aborted) return
    records.value = result.items; hasMore.value = result.has_more
    selected.value = result.items[0]?.id || ''
    if (selected.value) await loadHistory()
  } catch { if (!request.signal.aborted) error.value = t('admin.accounts.questionReview.loadFailed') }
  finally { if (!request.signal.aborted) busy.value = false }
}
async function loadHistory(append = false) {
  historyController?.abort()
  const request = new AbortController(); historyController = request
  const id = selected.value
  if (!append) { history.value = []; historyPage.value = 1; reason.value = ''; historyReady.value = false }
  try {
    const result = await questionAPI.history(id, append ? historyPage.value + 1 : 1, request.signal)
    if (request.signal.aborted || id !== selected.value) return
    history.value = append ? [...history.value, ...result.items] : result.items
    historyPage.value = result.page; historyMore.value = result.has_more
    verdict.value = history.value[0]?.verdict || 'unlabeled'; historyReady.value = true
  } catch { if (!request.signal.aborted) error.value = t('admin.accounts.questionReview.loadFailed') }
}
async function save() {
  if (!historyReady.value || saving.value || !reason.value.trim() || reasonBytes.value > 2000) return
  saving.value = true; error.value = ''
  const id = selected.value
  try {
    await questionAPI.review(id, verdict.value, reason.value.trim(), history.value[0]?.revision || 0)
    window.dispatchEvent(new Event('question-assessment-changed'))
    if (id === selected.value) await loadHistory()
  } catch { error.value = t('admin.accounts.questionReview.saveFailed'); historyReady.value = false }
  finally { saving.value = false }
}
watch(() => [props.accountId, props.refreshToken], () => { page.value = 1; void load() }, { immediate: true })
onBeforeUnmount(() => { controller?.abort(); historyController?.abort() })
</script>
