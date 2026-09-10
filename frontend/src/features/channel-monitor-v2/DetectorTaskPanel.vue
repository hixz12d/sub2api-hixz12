<template>
  <section class="space-y-4 py-4">
    <h2 class="text-lg font-semibold">{{ text.title }}</h2>
    <p v-if="error" role="alert" class="text-sm text-red-600">{{ error }}</p>
    <form class="grid max-w-3xl grid-cols-1 gap-4 sm:grid-cols-2" @submit.prevent="preview">
      <label class="text-sm">{{ text.key }}<input v-model.number="keyID" type="number" min="1" step="1" required class="input mt-1 w-full" :disabled="busy || !!pendingKey" /></label>
      <label class="text-sm">{{ text.channel }}<select v-model="channel" required class="input mt-1 w-full" :disabled="busy || !!pendingKey"><option v-for="item in channels" :key="item.name" :value="item.name">{{ item.name }}</option></select></label>
      <label class="text-sm">{{ text.model }}<input v-model="model" required maxlength="200" class="input mt-1 w-full" :disabled="busy || !!pendingKey" /></label>
      <label class="text-sm">{{ text.claimed }}<input v-model="claimed" required maxlength="200" class="input mt-1 w-full" :disabled="busy || !!pendingKey" /></label>
      <label class="text-sm">{{ text.tier }}<select v-model="tier" class="input mt-1 w-full" :disabled="busy || !!pendingKey"><option value="low">Low</option><option value="medium">Medium</option><option value="high">High</option></select></label>
      <div class="flex items-end"><button class="btn btn-secondary" :disabled="busy || !!pendingKey || !channel">{{ text.preview }}</button></div>
    </form>
    <div v-if="plan" class="space-y-3 border-t border-gray-200 pt-3 dark:border-dark-500">
      <p>{{ text.requests }}: {{ plan.planned_requests }} · {{ text.expires }}: {{ new Date(plan.expires_at).toLocaleString() }}</p>
      <p class="text-sm text-gray-500">{{ text.costUnknown }}</p>
      <label class="flex items-center gap-2 text-sm"><input v-model="confirmed" type="checkbox" :disabled="busy" />{{ text.confirm }}</label>
      <button type="button" class="btn btn-primary" :disabled="busy || !confirmed" @click="create">{{ pendingKey ? text.retry : text.create }}</button>
    </div>
    <div class="flex max-w-3xl flex-wrap items-end gap-2 border-t border-gray-200 pt-4 dark:border-dark-500">
      <label class="min-w-0 flex-1 text-sm">{{ text.job }}<input v-model="jobID" class="input mt-1 w-full" maxlength="36" /></label>
      <button type="button" class="btn btn-secondary p-2" :disabled="busy || !jobID" :title="text.refresh" @click="refresh"><Icon name="refresh" size="sm" /></button>
    </div>
    <div v-if="job" class="space-y-2 text-sm">
      <p class="break-all">{{ job.id }} · {{ job.state }}</p>
      <p>{{ text.requests }}: {{ job.planned_requests }} · {{ text.dispatched }}: {{ job.dispatched }} · {{ text.completed }}: {{ job.completed }}</p>
      <div v-for="(report, index) in job.reports" :key="index" class="border-t border-gray-200 py-2">
        <p>{{ report.benchmark.id }} · {{ report.benchmark.version }}</p>
        <p>{{ report.verdict }} · {{ report.contract_status }}</p>
        <ul><li v-for="reason in report.reasons" :key="reason">{{ reason }}</li></ul>
      </div>
      <button v-if="!terminal" type="button" class="btn btn-secondary" :disabled="busy" @click="cancel">{{ text.cancel }}</button>
    </div>
  </section>
</template>
<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Icon } from '@/components/icons'
import { benchmarkAPI, type BenchmarkChannel } from '@/api/admin/benchmarks'
import { detectorTaskAPI, type TaskPlan, type TaskStatus } from '@/api/admin/detectorTasks'
const { locale } = useI18n()
const text = computed(() => locale.value.startsWith('zh') ? {
  title: '站内 Key 检测任务', key: '站内 Key ID', channel: '基准通道', model: '请求模型', claimed: '声明模型', tier: '采样档位', preview: '生成计划', requests: '请求数', expires: '计划到期时间', costUnknown: '实际费用未知；执行受已配置的累计预算限制。', confirm: '确认执行该计划，可能产生费用', create: '创建任务', retry: '重试同一任务', job: '任务 ID', refresh: '刷新状态', dispatched: '已派发', completed: '已完成', cancel: '取消任务', failed: '操作失败，请检查准入、权限或预算后重试。'
} : {
  title: 'Site Key Detection Tasks', key: 'Site Key ID', channel: 'Benchmark channel', model: 'Request model', claimed: 'Claimed model', tier: 'Sample tier', preview: 'Build plan', requests: 'Requests', expires: 'Plan expiry', costUnknown: 'Actual cost is unknown; configured cumulative budgets apply.', confirm: 'Confirm this plan and its potential charges', create: 'Create task', retry: 'Retry same task', job: 'Task ID', refresh: 'Refresh status', dispatched: 'Dispatched', completed: 'Completed', cancel: 'Cancel task', failed: 'Operation failed. Check admission, permissions and budget.'
})
const keyID = ref<number>()
const channels = ref<BenchmarkChannel[]>([])
const channel = ref('')
const model = ref('')
const claimed = ref('')
const tier = ref('low')
const plan = ref<TaskPlan>()
const job = ref<TaskStatus>()
const jobID = ref('')
const confirmed = ref(false)
const pendingKey = ref('')
const busy = ref(false)
const error = ref('')
const controller = new AbortController()
const terminal = computed(() => !!job.value && ['completed', 'failed', 'cancelled', 'interrupted', 'skipped'].includes(job.value.state))
watch([keyID, channel, model, claimed, tier], () => { plan.value = undefined; confirmed.value = false })
async function preview() {
  if (busy.value || pendingKey.value || !keyID.value) return
  busy.value = true; error.value = ''; plan.value = undefined
  try { plan.value = await detectorTaskAPI.plan({ site_api_key_id: keyID.value, channel: channel.value, request_model: model.value, claimed_model: claimed.value, tier: tier.value }, controller.signal) }
  catch { if (!controller.signal.aborted) error.value = text.value.failed }
  finally { busy.value = false }
}
async function create() {
  if (!plan.value || !confirmed.value || busy.value) return
  busy.value = true; error.value = ''; pendingKey.value ||= crypto.randomUUID()
  try {
    const result = await detectorTaskAPI.create(plan.value, pendingKey.value)
    jobID.value = result.id; plan.value = undefined; pendingKey.value = ''; confirmed.value = false
    await refresh()
  } catch { error.value = text.value.failed }
  finally { busy.value = false }
}
async function refresh() {
  try { job.value = await detectorTaskAPI.job(jobID.value, controller.signal) }
  catch { if (!controller.signal.aborted) error.value = text.value.failed }
}
async function cancel() {
  if (!job.value || busy.value) return
  busy.value = true
  try { await detectorTaskAPI.cancel(job.value.id); await refresh() }
  catch { error.value = text.value.failed }
  finally { busy.value = false }
}
onMounted(async () => {
  try { channels.value = (await benchmarkAPI.list(1, controller.signal)).channels.filter(c => c.state === 'approved') }
  catch { if (!controller.signal.aborted) error.value = text.value.failed }
})
onBeforeUnmount(() => controller.abort())
</script>
