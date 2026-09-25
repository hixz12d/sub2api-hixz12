<template>
  <BaseDialog :show="show" :title="t('accountCompare.title')" width="full" @close="close">
    <div class="space-y-4">
      <p class="text-sm text-gray-500">{{ t('accountCompare.hint') }}</p>
      <div class="grid gap-3 md:grid-cols-[1fr_280px_180px]">
        <label class="block text-sm">{{ t('accountCompare.prompt') }}
          <textarea v-model="prompt" class="input mt-1" rows="3" maxlength="4096" :disabled="running" />
        </label>
        <label class="block text-sm">{{ t('accountCompare.model') }}
          <input v-model="model" class="input mt-1" maxlength="200" :disabled="running" placeholder="gpt-5.4" />
        </label>
        <label class="block text-sm">{{ t('accountCompare.effort') }}
          <select v-model="effort" data-effort class="input mt-1" :disabled="running">
            <option v-for="level in efforts" :key="level" :value="level">{{ effortLabel(level) }}</option>
          </select>
        </label>
      </div>
      <div class="flex flex-wrap items-center justify-between gap-3">
        <span class="text-sm text-gray-500">{{ completed }} / {{ cards.length }}</span>
        <div class="flex gap-2">
          <button v-if="running" type="button" class="btn btn-danger" @click="stop">{{ t('accountCompare.stop') }}</button>
          <button type="button" class="btn btn-primary" :disabled="running || !validInput || !cards.length || cards.some(c => c.saving)" @click="startAll">{{ t('accountCompare.start') }}</button>
        </div>
      </div>
      <div class="grid items-start gap-4 md:grid-cols-2 xl:grid-cols-3">
        <article v-for="card in cards" :key="card.id" :data-account-id="card.id" class="min-w-0 overflow-hidden rounded-xl border border-gray-200 dark:border-dark-600">
          <header class="border-b border-gray-200 bg-gray-50 p-3 dark:border-dark-600 dark:bg-dark-800">
            <div class="flex items-center justify-between gap-2">
              <h3 class="truncate font-semibold" :title="card.name">{{ card.name }}</h3>
              <span class="shrink-0 text-xs" :class="statusClass(card.status)" role="status">{{ t(`accountCompare.${card.status}`) }}</span>
            </div>
            <div class="mt-1 flex flex-wrap justify-between gap-2 text-xs text-gray-500">
              <span>{{ card.actualModel || card.requestModel || model }} · {{ effortLabel(card.requestModel ? card.requestEffort : effort) }}</span>
              <span v-if="card.startedAt">{{ t('accountCompare.elapsed', { seconds: elapsed(card) }) }}</span>
            </div>
          </header>
          <pre class="h-64 overflow-auto whitespace-pre-wrap break-words p-3 text-sm leading-relaxed">{{ card.answer || t('accountCompare.empty') }}</pre>
          <p v-if="card.error" role="alert" class="px-3 pb-3 text-sm text-red-500">{{ card.error }}</p>
          <p v-if="card.status === 'success' && !card.recordId" role="alert" class="px-3 pb-3 text-xs text-amber-600">{{ t('admin.accounts.questionReview.recordFailed') }}</p>
          <details v-if="card.requestPrompt" class="border-t p-3 text-xs dark:border-dark-600">
            <summary class="cursor-pointer text-gray-500">{{ t('accountCompare.prompt') }}</summary>
            <p class="mt-2 whitespace-pre-wrap break-words">{{ card.requestPrompt }}</p>
          </details>
          <form v-if="card.recordId" class="space-y-2 border-t p-3 dark:border-dark-600" @submit.prevent="saveReview(card)">
            <label class="block text-xs">{{ t('admin.accounts.questionReview.verdict') }}
              <select v-model="card.verdict" class="input mt-1" :disabled="card.saving">
                <option v-for="verdict in verdicts" :key="verdict" :value="verdict">{{ t(`admin.accounts.questionReview.${verdict}`) }}</option>
              </select>
            </label>
            <textarea v-model="card.reason" class="input text-sm" rows="2" maxlength="2000" :aria-label="t('admin.accounts.questionReview.reason')" :placeholder="t('admin.accounts.questionReview.reason')" :disabled="card.saving" />
            <p v-if="card.reviewError" role="alert" class="text-xs text-red-500">{{ card.reviewError }}</p>
            <button v-if="card.reviewError" type="button" class="btn btn-secondary text-xs" :disabled="card.saving" @click="reloadReview(card)">{{ t('accountCompare.reloadReview') }}</button>
            <p v-if="card.reviewSaved" class="text-xs text-green-600">{{ t('accountCompare.reviewSaved') }}</p>
            <button type="submit" class="btn btn-secondary text-xs" :disabled="card.saving || !card.reason.trim() || utf8Length(card.reason.trim()) > 2000">{{ t('admin.accounts.questionReview.save') }}</button>
          </form>
          <div class="flex justify-end border-t p-2 dark:border-dark-600">
            <button type="button" class="btn btn-secondary text-xs" :disabled="running || card.saving || !validInput" @click="retry(card)">{{ t('accountCompare.retry') }}</button>
          </div>
        </article>
      </div>
    </div>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import { adminAPI } from '@/api/admin'
import { questionAPI, type QuestionVerdict } from '@/api/admin/questionReviews'
import { streamAccountQuestion } from '@/utils/accountQuestionStream'

type State = 'waiting' | 'running' | 'success' | 'failed' | 'cancelled'
interface Card {
  id: number; name: string; status: State; answer: string; error: string
  requestModel: string; actualModel: string; requestPrompt: string; requestEffort: string
  startedAt: number; finishedAt: number; recordId: string
  verdict: QuestionVerdict; reason: string; revision: number; saving: boolean
  reviewError: string; reviewSaved: boolean
}
const props = defineProps<{ show: boolean; accountIds: number[]; knownAccounts: { id: number; name: string }[] }>()
const emit = defineEmits<{ close: []; reviewed: [] }>()
const { t } = useI18n()
const prompt = ref("don't search the internet, who is Thibault Sottiaux on X")
const model = ref('gpt-5.4')
// Empty keeps the upstream default; values mirror the backend whitelist.
const efforts = ['', 'none', 'minimal', 'low', 'medium', 'high', 'xhigh'] as const
const effort = ref<string>('')
const effortLabel = (level: string) => level || t('accountCompare.effortDefault')
const running = ref(false)
const cards = ref<Card[]>([])
const now = ref(Date.now())
const verdicts: QuestionVerdict[] = ['unlabeled', 'normal', 'degraded']
const utf8Length = (value: string) => new TextEncoder().encode(value).length
const validInput = computed(() => Boolean(prompt.value.trim() && model.value.trim()) && utf8Length(prompt.value) <= 4096 && !prompt.value.includes('\0'))
const completed = computed(() => cards.value.filter(c => !['waiting', 'running'].includes(c.status)).length)
const elapsed = (card: Card) => ((Math.max(card.startedAt, card.finishedAt || now.value) - card.startedAt) / 1000).toFixed(1)
const statusClass = (state: State) => ({ success: 'text-green-600', failed: 'text-red-500', running: 'text-primary-500', waiting: 'text-gray-500', cancelled: 'text-amber-600' })[state]
let generation = 0
let clock: ReturnType<typeof setInterval> | undefined
const controllers = new Map<number, AbortController>()
function newCard(id: number): Card {
  return { id, name: props.knownAccounts.find(a => a.id === id)?.name || `#${id}`, status: 'waiting', answer: '', error: '', requestModel: '', actualModel: '', requestPrompt: '', requestEffort: '', startedAt: 0, finishedAt: 0, recordId: '', verdict: 'unlabeled', reason: '', revision: 0, saving: false, reviewError: '', reviewSaved: false }
}
function stop() {
  generation++
  for (const controller of controllers.values()) controller.abort()
  controllers.clear()
  for (const card of cards.value) {
    if (card.status === 'running' || card.status === 'waiting') { card.status = 'cancelled'; card.finishedAt = Date.now() }
  }
  running.value = false
  clearInterval(clock)
}
function close() { stop(); emit('close') }
watch(() => props.show, show => {
  stop()
  if (show) cards.value = [...new Set(props.accountIds)].map(newCard)
}, { immediate: true })
onBeforeUnmount(stop)

async function runCard(card: Card, run: number, requestModel: string, requestPrompt: string, requestEffort: string) {
  Object.assign(card, { status: 'running', answer: '', error: '', actualModel: '', requestModel, requestPrompt, requestEffort, startedAt: Date.now(), finishedAt: 0, recordId: '', revision: 0, verdict: 'unlabeled', reason: '', reviewError: '', reviewSaved: false })
  const controller = new AbortController()
  controllers.set(card.id, controller)
  let timedOut = false
  let failed = false
  let succeeded = false
  const timeout = setTimeout(() => { timedOut = true; controller.abort() }, 120_000)
  const current = () => generation === run && !controller.signal.aborted
  try {
    if (card.name === `#${card.id}`) {
      const account = await adminAPI.accounts.getById(card.id)
      if (!current()) { if (timedOut) throw new Error('timeout'); return }
      card.name = account.name
    }
    await streamAccountQuestion(card.id, requestModel, requestPrompt, controller.signal, event => {
      if (!current()) return
      if (event.type === 'test_start' && event.model) card.actualModel = event.model
      if (event.type === 'content' && event.text) {
        if (card.answer.length + event.text.length > 256 * 1024) throw new Error(t('accountCompare.answerTooLarge'))
        card.answer += event.text
      }
      if (event.type === 'error' || (event.type === 'test_complete' && !event.success)) {
        failed = true
        card.error = event.error || t('admin.accounts.testFailed')
      }
      if (event.type === 'test_complete' && event.success) succeeded = true
      if (event.type === 'question_record' && event.saved && event.record_id) card.recordId = event.record_id
    }, requestEffort)
    if (current()) card.status = failed || !succeeded ? 'failed' : 'success'
  } catch (error) {
    if (generation !== run) return
    card.status = controller.signal.aborted && !timedOut ? 'cancelled' : 'failed'
    const message = error instanceof Error ? error.message : t('common.unknownError')
    card.error = timedOut ? t('accountCompare.timeout') : message === 'ACCOUNT_TEST_INCOMPLETE' ? t('accountCompare.incomplete') : message
  } finally {
    clearTimeout(timeout)
    if (generation === run) { card.finishedAt = Date.now(); controllers.delete(card.id) }
  }
}
async function runBatch(targets: Card[], requestModel: string, requestPrompt: string, requestEffort: string) {
  if (running.value || !validInput.value || targets.some(c => c.saving)) return
  const run = ++generation
  running.value = true
  let next = 0
  now.value = Date.now()
  clock = setInterval(() => { now.value = Date.now() }, 200)
  for (const card of targets) Object.assign(card, newCard(card.id), { name: card.name, requestModel, requestPrompt, requestEffort })
  const worker = async () => {
    while (generation === run && next < targets.length) await runCard(targets[next++], run, requestModel, requestPrompt, requestEffort)
  }
  try { await Promise.all(Array.from({ length: Math.min(3, targets.length) }, worker)) }
  finally { if (generation === run) { running.value = false; clearInterval(clock) } }
}
function startAll() { void runBatch(cards.value, model.value.trim(), prompt.value, effort.value) }
// A card that already ran retries with its original model, question and effort.
function retry(card: Card) {
  if (card.requestModel) void runBatch([card], card.requestModel, card.requestPrompt, card.requestEffort)
  else void runBatch([card], model.value.trim(), prompt.value, effort.value)
}
async function reloadReview(card: Card) {
  if (card.saving || !card.recordId) return
  card.saving = true
  try {
    const history = await questionAPI.history(card.recordId)
    const latest = history.items[0]
    card.revision = latest?.revision ?? 0
    card.verdict = latest?.verdict ?? 'unlabeled'
    card.reason = latest?.reason ?? ''
    card.reviewError = ''
    card.reviewSaved = false
  } catch { card.reviewError = t('admin.accounts.questionReview.loadFailed') }
  finally { card.saving = false }
}
async function saveReview(card: Card) {
  if (card.saving || !card.recordId || !card.reason.trim()) return
  const record = card.recordId
  card.saving = true
  card.reviewError = ''
  card.reviewSaved = false
  try {
    const review = await questionAPI.review(record, card.verdict, card.reason.trim(), card.revision)
    if (card.recordId !== record) return
    card.revision = review.revision
    card.reviewSaved = true
    emit('reviewed')
  } catch (error: any) {
    card.reviewError = error.response?.status === 409 ? t('accountCompare.reviewConflict') : t('admin.accounts.questionReview.saveFailed')
  } finally { card.saving = false }
}
</script>
