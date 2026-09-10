<template>
  <section class="space-y-4 border-t border-gray-200 py-4 dark:border-dark-500">
    <header class="flex items-center gap-3"><h2 class="mr-auto text-lg font-semibold">{{ zh ? '平台检测策略' : 'Platform policies' }}</h2><button class="btn btn-secondary p-2" :disabled="busy" :title="zh ? '刷新' : 'Refresh'" @click="load"><Icon name="refresh" size="sm" /></button><button class="btn btn-secondary" :disabled="busy" @click="createDraft">{{ zh ? '新建策略' : 'New policy' }}</button></header>
    <p v-if="error" role="alert" class="text-sm text-red-600">{{ error }}</p>
    <div class="overflow-x-auto"><table class="w-full text-left text-sm"><thead><tr><th>{{ zh ? '分组' : 'Group' }}</th><th>{{ zh ? '名称' : 'Name' }}</th><th>{{ zh ? '状态' : 'State' }}</th><th></th></tr></thead><tbody><tr v-for="policy in policies" :key="policy.id" class="border-t border-gray-200"><td class="py-2">{{ groupName(policy.group_id) }}</td><td>{{ policy.display_name || policy.primary_model }}</td><td>{{ policy.enabled ? (zh ? '启用' : 'Enabled') : (zh ? '停用' : 'Disabled') }}</td><td><button class="btn btn-secondary mr-2" :disabled="busy" @click="reportPolicy = policy; reportPage = 1; loadReports()">{{ zh ? '报告' : 'Reports' }}</button><button class="btn btn-secondary" :disabled="busy" @click="editPolicy(policy)">{{ zh ? '编辑' : 'Edit' }}</button><button class="btn btn-secondary ml-2" :disabled="busy" @click="removing = policy">{{ zh ? '删除' : 'Delete' }}</button></td></tr></tbody></table></div>
    <div class="flex justify-end gap-2"><button class="btn btn-secondary p-2" :disabled="busy || page === 1" :title="zh ? '上一页' : 'Previous'" @click="page--; load()"><Icon name="chevronLeft" size="sm" /></button><button class="btn btn-secondary p-2" :disabled="busy || !hasMore" :title="zh ? '下一页' : 'Next'" @click="page++; load()"><Icon name="chevronRight" size="sm" /></button></div>
    <section v-if="reportPolicy" class="space-y-2 border-t border-gray-200 pt-3">
      <h3 class="text-sm font-semibold">{{ groupName(reportPolicy.group_id) }} · {{ zh ? '检测报告' : 'Detection reports' }}</h3>
      <p v-if="!reports.length" class="text-sm text-gray-500">{{ zh ? '暂无报告' : 'No reports' }}</p>
      <div v-for="report in reports" :key="`${report.job_id}:${report.account_id}:${report.request_model}:${report.created_at}`" class="border-t border-gray-200 py-2 text-sm">
        <p class="break-all">{{ report.request_model }} · {{ zh ? '账号' : 'Account' }} #{{ report.account_id }} · {{ report.evidence.verdict }} · {{ report.evidence.contract_status }}</p>
        <p>{{ report.evidence.benchmark.id }} · {{ report.evidence.benchmark.version }}</p>
        <p v-for="reason in report.evidence.reasons" :key="reason" class="break-words text-gray-500">{{ reason }}</p>
      </div>
      <div class="flex justify-end gap-2"><button class="btn btn-secondary p-2" :disabled="busy || reportPage === 1" :title="zh ? '上一页' : 'Previous'" @click="reportPage--; loadReports()"><Icon name="chevronLeft" size="sm" /></button><button class="btn btn-secondary p-2" :disabled="busy || !reportsMore" :title="zh ? '下一页' : 'Next'" @click="reportPage++; loadReports()"><Icon name="chevronRight" size="sm" /></button></div>
    </section>
    <form v-if="draft" class="space-y-4 border-t border-gray-200 pt-4" @submit.prevent="save">
      <fieldset :disabled="busy" class="grid grid-cols-1 gap-3 sm:grid-cols-2">
        <label class="text-sm">{{ zh ? '分组' : 'Group' }}<select v-model.number="draft.group_id" class="input mt-1 w-full" :disabled="draft.id > 0" required><option v-for="group in groups" :key="group.id" :value="group.id">{{ group.name }}</option></select></label>
        <label class="text-sm">{{ zh ? '展示名称' : 'Display name' }}<input v-model="draft.display_name" maxlength="200" class="input mt-1 w-full" /></label>
        <label class="text-sm">{{ zh ? '主模型' : 'Primary model' }}<input v-model="draft.primary_model" maxlength="200" required class="input mt-1 w-full" /></label>
        <label class="flex items-center gap-2 text-sm"><input v-model="draft.enabled" type="checkbox" />{{ zh ? '启用策略' : 'Enable policy' }}</label>
        <label class="flex items-center gap-2 text-sm"><input v-model="draft.probe_config.enabled" type="checkbox" />{{ zh ? '主动探活' : 'Availability probe' }}</label>
        <label class="flex items-center gap-2 text-sm"><input v-model="draft.capability_config.enabled" type="checkbox" />{{ zh ? '能力检测' : 'Capability detection' }}</label>
        <label v-for="field in probeFields" :key="field.key" class="text-sm">{{ zh ? field.zh : field.en }}<input v-model.number="draft.probe_config[field.key]" type="number" min="0" step="1" required class="input mt-1 w-full" /></label>
        <label v-for="field in capabilityFields" :key="field.key" class="text-sm">{{ zh ? field.zh : field.en }}<input v-model.number="draft.capability_config[field.key]" type="number" min="0" step="1" required class="input mt-1 w-full" /></label>
        <label class="text-sm">{{ zh ? '采样档位' : 'Sample tier' }}<select v-model="draft.capability_config.tier" class="input mt-1 w-full"><option value="low">Low</option><option value="medium">Medium</option><option value="high">High</option></select></label>
        <label class="text-sm">{{ zh ? '账号选择' : 'Account selection' }}<select v-model="draft.capability_config.selection_mode" class="input mt-1 w-full"><option value="random">{{ zh ? '随机' : 'Random' }}</option><option value="fixed">{{ zh ? '固定' : 'Fixed' }}</option></select></label>
        <label v-if="draft.capability_config.selection_mode === 'fixed'" class="text-sm">{{ zh ? '固定账号 ID（逗号分隔）' : 'Fixed account IDs (comma separated)' }}<input v-model="fixedIDs" required class="input mt-1 w-full" /></label>
      </fieldset>
      <div v-for="(target, index) in draft.capability_config.targets" :key="index" class="grid grid-cols-1 gap-2 border-t border-gray-200 pt-3 sm:grid-cols-3">
        <label class="text-sm">{{ zh ? '请求模型' : 'Request model' }}<input v-model="target.request_model" required maxlength="200" class="input mt-1 w-full" :disabled="busy" /></label>
        <label class="text-sm">{{ zh ? '声明模型' : 'Claimed model' }}<input v-model="target.claimed_model" required maxlength="200" class="input mt-1 w-full" :disabled="busy" /></label>
        <label class="text-sm">{{ zh ? '已批准基准' : 'Approved benchmark' }}<select :value="releases.find(r => r.benchmark_id === target.benchmark_id && r.version === target.benchmark_version && r.sha256 === target.benchmark_sha256)?.id || ''" required class="input mt-1 w-full" :disabled="busy" @change="selectRelease(index, ($event.target as HTMLSelectElement).value)"><option value="" disabled>{{ zh ? '选择基准' : 'Select benchmark' }}</option><option v-for="release in releases" :key="release.id" :value="release.id">{{ release.benchmark_id }} · {{ release.version }}</option></select></label>
        <label class="text-sm">{{ zh ? '自动跟随基准通道' : 'Follow benchmark channel' }}<select :value="target.benchmark_channel || ''" class="input mt-1 w-full" :disabled="busy" @change="selectChannel(index, ($event.target as HTMLSelectElement).value)"><option value="">{{ zh ? '固定版本' : 'Pinned version' }}</option><option v-for="channel in channels" :key="channel.name" :value="channel.name">{{ channel.name }}</option></select></label>
      </div>
      <div class="flex flex-wrap gap-2"><button type="button" class="btn btn-secondary" :disabled="busy || draft.capability_config.targets.length >= 8" @click="addTarget">{{ zh ? '添加模型' : 'Add model' }}</button><button type="button" class="btn btn-secondary" :disabled="busy || !draft.capability_config.targets.length" @click="draft.capability_config.targets.pop()">{{ zh ? '移除末项' : 'Remove last' }}</button><button type="submit" class="btn btn-primary" :disabled="busy">{{ zh ? '保存策略' : 'Save policy' }}</button><button type="button" class="btn btn-secondary" :disabled="busy" @click="draft = undefined">{{ zh ? '取消' : 'Cancel' }}</button></div>
    </form>
    <ConfirmDialog :show="!!removing" :title="zh ? '删除策略' : 'Delete policy'" :message="zh ? '删除后停止后续派发，已发送请求不能撤销。' : 'Stop future dispatches. Requests already sent cannot be undone.'" danger @cancel="removing = undefined" @confirm="remove" />
  </section>
</template>
<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, toRaw, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Icon } from '@/components/icons'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import { getAll } from '@/api/admin/groups'
import { benchmarkAPI, type BenchmarkRelease, type BenchmarkChannel } from '@/api/admin/benchmarks'
import { monitorPolicyAPI, type PlatformReport } from '@/api/admin/monitorPolicies'
import { defaultGroupProbeConfig, defaultGroupCapabilityConfig, type ChannelMonitorGroupPolicy } from '@/api/channelMonitorGroups'
const { locale } = useI18n()
const zh = computed(() => locale.value.startsWith('zh'))
const policies = ref<ChannelMonitorGroupPolicy[]>([])
const groups = ref<{ id: number; name: string }[]>([])
const releases = ref<BenchmarkRelease[]>([])
const channels = ref<BenchmarkChannel[]>([])
const draft = ref<ChannelMonitorGroupPolicy>()
const removing = ref<ChannelMonitorGroupPolicy>()
const fixedIDs = ref('')
const reportPolicy = ref<ChannelMonitorGroupPolicy>()
const reportPage = ref(1)
const reports = ref<PlatformReport[]>([])
const reportsMore = ref(false)
async function loadReports() {
  if (!reportPolicy.value) return
  busy.value = true; reports.value = []
  try { const result = await monitorPolicyAPI.reports(reportPolicy.value.id, reportPage.value, controller.signal); if (!controller.signal.aborted) { reports.value = result.items; reportsMore.value = result.has_more } }
  catch { if (!controller.signal.aborted) fail() }
  finally { busy.value = false }
}
const page = ref(1)
const hasMore = ref(false)
const busy = ref(false)
const error = ref('')
const controller = new AbortController()
const probeFields = [{ key: 'interval_seconds', zh: '探活间隔（秒）', en: 'Probe interval (s)' }, { key: 'daily_request_limit', zh: '探活日请求上限', en: 'Probe daily request limit' }] as const
const capabilityFields = [{ key: 'interval_seconds', zh: '检测间隔（秒）', en: 'Detection interval (s)' }, { key: 'sample_size', zh: '抽样账号数（1–3）', en: 'Account sample size (1-3)' }, { key: 'daily_request_limit', zh: '检测日请求上限', en: 'Detection daily request limit' }, { key: 'execution_timeout_seconds', zh: '执行超时（秒）', en: 'Execution timeout (s)' }, { key: 'result_ttl_seconds', zh: '结果有效期（秒）', en: 'Result TTL (s)' }] as const
const groupName = (id: number) => groups.value.find(g => g.id === id)?.name || (zh.value ? '不可用分组' : 'Unavailable group')
watch(draft, value => { fixedIDs.value = value?.capability_config.fixed_account_ids.join(',') || '' })
function editPolicy(policy: ChannelMonitorGroupPolicy) { draft.value = structuredClone(toRaw(policy)) }
function createDraft() {
  draft.value = { id: 0, group_id: groups.value[0]?.id || 0, display_name: '', primary_model: '', extra_models: [], enabled: false, probe_config: defaultGroupProbeConfig(), capability_config: defaultGroupCapabilityConfig(), version: 0, evaluation_revision: '', next_probe_at: null, next_capability_at: null, created_by: 0, updated_by: 0, created_at: '', updated_at: '' }
}
function addTarget() { draft.value?.capability_config.targets.push({ request_model: draft.value.primary_model, claimed_model: '', benchmark_id: '', benchmark_version: '', benchmark_sha256: '' }) }
function selectRelease(index: number, id: string) { const release = releases.value.find(r => r.id === id); const target = draft.value?.capability_config.targets[index]; if (release && target) Object.assign(target, { benchmark_id: release.benchmark_id, benchmark_version: release.version, benchmark_sha256: release.sha256 }) }
function selectChannel(index: number, name: string) { const target = draft.value?.capability_config.targets[index]; if (!target) return; target.benchmark_channel = name; const channel = channels.value.find(c => c.name === name); if (channel) selectRelease(index, channel.release_id) }
function fail() { error.value = zh.value ? '操作未完成，请刷新核对版本、配置和执行准入。' : 'Operation not completed. Refresh and check revision, configuration and admission.' }
async function load() {
  busy.value = true; error.value = ''
  try { const result = await monitorPolicyAPI.list(page.value, controller.signal); if (!controller.signal.aborted) { policies.value = result.items; hasMore.value = result.has_more } }
  catch { if (!controller.signal.aborted) fail() }
  finally { busy.value = false }
}
async function save() {
  if (!draft.value || busy.value) return
  busy.value = true; error.value = ''
  try {
    const value = structuredClone(toRaw(draft.value))
    value.capability_config.fixed_account_ids = value.capability_config.selection_mode === 'fixed' ? fixedIDs.value.split(',').map(s => Number(s.trim())) : []
    value.extra_models = [...new Set([...value.extra_models, ...value.capability_config.targets.map(t => t.request_model)])].filter(m => m !== value.primary_model)
    await monitorPolicyAPI.save(value); draft.value = undefined; await load()
  } catch { fail() }
  finally { busy.value = false }
}
async function remove() {
  if (!removing.value || busy.value) return
  const target = removing.value; removing.value = undefined; busy.value = true
  try { await monitorPolicyAPI.remove(target); await load() } catch { fail() } finally { busy.value = false }
}
onMounted(async () => {
  void load()
  try { groups.value = await getAll('openai'); const result = await benchmarkAPI.list(1, controller.signal); releases.value = result.items.filter(r => r.state === 'approved' && r.mode === 'gpt'); channels.value = result.channels.filter(c => c.state === 'approved' && releases.value.some(r => r.id === c.release_id)) } catch { if (!controller.signal.aborted) fail() }
})
onBeforeUnmount(() => controller.abort())
</script>
