<template>
  <section class="min-w-0 space-y-4" :aria-label="tr('title')" :aria-busy="loading || busy">
    <div class="flex flex-wrap items-center justify-between gap-3 border-b border-gray-200 pb-3 dark:border-dark-600">
      <h2 class="text-lg font-semibold">{{ tr('title') }}</h2>
      <div class="flex items-center gap-2">
        <input ref="fileInput" type="file" accept=".json" class="hidden" @change="importFile" />
        <button class="btn btn-secondary inline-flex items-center gap-2" :disabled="busy" @click="fileInput?.click()">
          <Icon name="upload" size="sm" />{{ tr('import') }}
        </button>
        <button class="btn btn-secondary h-10 w-10 p-2" :title="tr('refresh')" :aria-label="tr('refresh')" :disabled="loading || busy" @click="reload">
          <Icon name="refresh" size="sm" />
        </button>
      </div>
    </div>
    <p v-if="error" role="alert" class="break-words text-sm text-red-600 dark:text-red-400">{{ error }}</p>
    <p v-if="busy" role="status" class="text-sm text-gray-500">{{ tr('processing') }}</p>
    <p v-if="loading && !items.length" role="status" class="py-8 text-center text-sm">{{ tr('loading') }}</p>
    <p v-else-if="!items.length && !error" class="py-8 text-center text-sm text-gray-500">{{ tr('empty') }}</p>
    <ul class="divide-y divide-gray-200 dark:divide-dark-600">
      <li v-for="item in items" :key="item.id" class="grid min-w-0 gap-3 py-4 md:grid-cols-[minmax(0,1fr)_auto]">
        <div class="min-w-0 space-y-1">
          <h3 class="break-all text-sm font-semibold">{{ item.benchmark_id }}</h3>
          <div class="flex flex-wrap gap-x-3 gap-y-1 text-xs text-gray-500 dark:text-gray-400">
            <span>{{ item.version }}</span><span>{{ item.mode }}</span>
            <span :class="item.state === 'approved' ? 'text-emerald-600 dark:text-emerald-400' : item.state === 'withdrawn' ? 'text-red-600 dark:text-red-400' : ''">{{ tr(item.state) }}</span>
          </div>
          <code class="block break-all text-xs text-gray-500">SHA256 {{ item.sha256 }}</code>
          <p v-for="channel in channels.filter(c => c.release_id === item.id)" :key="channel.name" class="break-all text-xs text-gray-500">
            {{ tr('channel') }}: {{ channel.name }} · {{ tr('revision') }} {{ channel.revision }}
          </p>
        </div>
        <div class="flex flex-wrap items-start gap-2">
          <button v-if="item.state === 'candidate'" class="btn btn-secondary inline-flex items-center gap-1" :disabled="busy || loading" @click="run(() => benchmarkAPI.approve(item.id, operation.signal))">
            <Icon name="checkCircle" size="sm" />{{ tr('approve') }}
          </button>
          <button v-if="item.state === 'approved'" class="btn btn-secondary inline-flex items-center gap-1" :disabled="busy || loading" @click="openAction(item, 'activate')">
            <Icon name="check" size="sm" />{{ tr('activate') }}
          </button>
          <button v-if="item.state !== 'withdrawn'" class="btn btn-secondary inline-flex items-center gap-1" :disabled="busy || loading" @click="openAction(item, 'withdraw')">
            <Icon name="ban" size="sm" />{{ tr('withdraw') }}
          </button>
        </div>
      </li>
    </ul>
    <div class="flex items-center justify-end gap-3 border-t border-gray-200 pt-3 dark:border-dark-600">
      <button class="btn btn-secondary h-10 w-10 p-2" :disabled="page <= 1 || loading || busy" :title="tr('previous')" :aria-label="tr('previous')" @click="changePage(-1)"><Icon name="chevronLeft" size="sm" /></button>
      <span class="min-w-8 text-center text-sm">{{ page }}</span>
      <button class="btn btn-secondary h-10 w-10 p-2" :disabled="!hasMore || loading || busy" :title="tr('next')" :aria-label="tr('next')" @click="changePage(1)"><Icon name="chevronRight" size="sm" /></button>
    </div>
    <BaseDialog :show="selected !== null" :title="tr(action)" width="narrow" @close="closeAction">
      <div class="space-y-4">
        <p class="break-all text-sm font-medium">{{ selected?.benchmark_id }} / {{ selected?.version }}</p>
        <template v-if="action === 'activate'">
          <label for="benchmark-channel" class="block text-sm">{{ tr('channel') }}</label>
          <input id="benchmark-channel" v-model="channelName" list="benchmark-channels" maxlength="200" class="input w-full" :disabled="busy" />
          <datalist id="benchmark-channels"><option v-for="c in channels" :key="c.name" :value="c.name" /></datalist>
          <p class="text-xs text-gray-500">{{ tr('revision') }}: {{ expectedRevision }}</p>
        </template>
        <template v-else>
          <label for="benchmark-reason" class="block text-sm">{{ tr('reason') }}</label>
          <textarea id="benchmark-reason" v-model="reason" rows="3" maxlength="500" class="input w-full" :disabled="busy" />
        </template>
        <p v-if="error" role="alert" class="text-sm text-red-600">{{ error }}</p>
      </div>
      <template #footer>
        <div class="flex justify-end gap-2">
          <button class="btn btn-secondary" :disabled="busy" @click="closeAction">{{ tr('cancel') }}</button>
          <button class="btn btn-primary" :disabled="busy || (action === 'activate' ? !channelName.trim() : !reason.trim())" @click="confirmAction">{{ tr('confirm') }}</button>
        </div>
      </template>
    </BaseDialog>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import { benchmarkAPI, type BenchmarkChannel, type BenchmarkRelease } from '@/api/admin/benchmarks'

const { t } = useI18n()
const tr = (key: string) => t(`channelMonitorV2.benchmarks.${key}`)
const items = ref<BenchmarkRelease[]>([])
const channels = ref<BenchmarkChannel[]>([])
const page = ref(1)
const hasMore = ref(false)
const loading = ref(false)
const busy = ref(false)
const error = ref('')
const fileInput = ref<HTMLInputElement | null>(null)
const selected = ref<BenchmarkRelease | null>(null)
const action = ref<'activate' | 'withdraw'>('activate')
const channelName = ref('')
const reason = ref('')
const expectedRevision = computed(() => channels.value.find(c => c.name === channelName.value.trim())?.revision ?? 0)
const operation = new AbortController()
let listController: AbortController | null = null
let alive = true

function errorText(err: unknown) {
  const failure = err as { status?: number; response?: { status?: number } }
  const status = failure?.status ?? failure?.response?.status
  return tr(status === 409 ? 'conflict' : status === 503 ? 'unavailable' : status === 403 ? 'forbidden' : 'failed')
}
async function reload() {
  listController?.abort()
  const ctrl = new AbortController()
  listController = ctrl
  loading.value = true
  error.value = ''
  try {
    const result = await benchmarkAPI.list(page.value, ctrl.signal)
    if (!alive || ctrl.signal.aborted || listController !== ctrl) return
    items.value = result.items
    channels.value = result.channels
    hasMore.value = result.has_more
  } catch (err) {
    if (alive && !ctrl.signal.aborted && listController === ctrl) error.value = errorText(err)
  } finally {
    if (alive && listController === ctrl) loading.value = false
  }
}
async function run(work: () => Promise<unknown>) {
  if (busy.value || !alive) return
  busy.value = true
  error.value = ''
  try {
    await work()
    if (!alive) return
    selected.value = null
    await reload()
  } catch (err) {
    if (alive) error.value = errorText(err)
  } finally {
    if (alive) busy.value = false
  }
}
function importFile(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (file) void run(() => benchmarkAPI.stage(file, operation.signal))
}
function changePage(delta: number) {
  page.value += delta
  items.value = []
  void reload()
}
function openAction(item: BenchmarkRelease, next: 'activate' | 'withdraw') {
  selected.value = item
  action.value = next
  channelName.value = item.benchmark_id
  reason.value = ''
  error.value = ''
}
function closeAction() { if (!busy.value) selected.value = null }
function confirmAction() {
  const id = selected.value?.id
  if (!id) return
  const revision = expectedRevision.value
  void run(() => action.value === 'activate'
    ? benchmarkAPI.activate(id, channelName.value.trim(), revision, operation.signal)
    : benchmarkAPI.withdraw(id, reason.value.trim(), operation.signal))
}
onMounted(reload)
onUnmounted(() => { alive = false; operation.abort(); listController?.abort() })
</script>
