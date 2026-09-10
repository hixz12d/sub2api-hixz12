import { onBeforeUnmount, onMounted, ref, watch, type Ref } from 'vue'
import { apiClient } from '@/api/client'

export interface HumanAssessment { id: number; normal: number; degraded: number; unlabeled: number }
export function useHumanAssessments(scope: 'account' | 'group', ids: Ref<number[]>) {
  const summaries = ref<Record<number, HumanAssessment>>({})
  const failed = ref(false)
  let controller: AbortController | undefined
  async function reload() {
    controller?.abort()
    const request = new AbortController(); controller = request
    summaries.value = {}; failed.value = false
    const selected = [...new Set(ids.value)].filter(id => Number.isSafeInteger(id) && id > 0)
    try {
      const next: Record<number, HumanAssessment> = {}
      for (let offset = 0; offset < selected.length; offset += 100) {
        const result = await apiClient.get<{ items: HumanAssessment[] }>('/admin/question-assessments', {
          params: { scope, ids: selected.slice(offset, offset + 100).join(',') }, signal: request.signal
        })
        if (request.signal.aborted) return
        for (const item of result.data.items) next[item.id] = item
      }
      summaries.value = next
    } catch { if (!request.signal.aborted) failed.value = true }
  }
  watch(ids, () => { void reload() }, { immediate: true })
  const refresh = () => { void reload() }
  onMounted(() => window.addEventListener('question-assessment-changed', refresh))
  onBeforeUnmount(() => { controller?.abort(); window.removeEventListener('question-assessment-changed', refresh) })
  return { summaries, failed }
}
