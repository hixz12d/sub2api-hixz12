<template>
  <!-- 后台内嵌形态:?embedded=1 且已登录,套完整后台布局 -->
  <AppLayout v-if="isEmbedded">
    <ModelPlazaContent
      :response="data"
      :loading="loading"
      :error="loadFailed"
      :selected-group="urlGroup"
      :active-model="urlModel"
      :preferred-view="urlView"
      embedded
      @update:selected-group="handleGroupChange"
      @update:active-model="handleModelChange"
      @update:view-mode="handleViewChange"
      @retry="fetchPlaza"
    />
  </AppLayout>

  <!-- 独立形态:自带新版顶栏(logo/站名 + 主题/语言 + 登录/回后台) -->
  <div v-else class="min-h-screen bg-surface text-ink">
    <PlazaNavBar />
    <main class="mx-auto w-full max-w-[1320px] px-4 py-6 md:px-6 lg:px-8 lg:py-8">
      <ModelPlazaContent
        :response="data"
        :loading="loading"
        :error="loadFailed"
        :selected-group="urlGroup"
        :active-model="urlModel"
        :preferred-view="urlView"
        @update:selected-group="handleGroupChange"
        @update:active-model="handleModelChange"
        @update:view-mode="handleViewChange"
        @retry="fetchPlaza"
      />
    </main>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter, type LocationQueryRaw, type LocationQueryValue } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import PlazaNavBar from '@/components/modelPlaza/PlazaNavBar.vue'
import ModelPlazaContent from '@/components/modelPlaza/ModelPlazaContent.vue'
import type { PlazaViewMode } from '@/components/modelPlaza/PlazaToolbar.vue'
import { getModelPlaza, type ModelPlazaResponse } from '@/api/modelPlaza'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'

const route = useRoute()
const router = useRouter()
const appStore = useAppStore()
const authStore = useAuthStore()

// embedded=1 但未登录(如转发的链接)自动降级为独立形态。
const isEmbedded = computed(() => route.query.embedded === '1' && authStore.isAuthenticated)

function readQueryString(val: LocationQueryValue | LocationQueryValue[] | undefined): string {
  if (Array.isArray(val)) {
    return typeof val[0] === 'string' ? val[0].trim() : ''
  }
  return typeof val === 'string' ? val.trim() : ''
}

// URL query parameters (group is always group name, never numeric group ID)
const urlGroup = computed(() => readQueryString(route.query.group))
const urlModel = computed(() => readQueryString(route.query.model))
const urlView = computed<PlazaViewMode | null>(() => {
  const raw = readQueryString(route.query.view)
  return raw === 'card' || raw === 'table' ? raw : null
})

// Batch same-tick query updates so simultaneous group + model updates never clobber each other
interface QueryPatch {
  group?: string
  model?: string
  view?: PlazaViewMode | null
}

let pendingPatch: QueryPatch | null = null
let flushScheduled = false

function scheduleQueryPatch(patch: QueryPatch) {
  pendingPatch = { ...(pendingPatch ?? {}), ...patch }
  if (flushScheduled) return
  flushScheduled = true
  queueMicrotask(() => {
    flushScheduled = false
    const nextPatch = pendingPatch
    pendingPatch = null
    if (!nextPatch) return

    const nextQuery: LocationQueryRaw = { ...route.query }
    let changed = false

    const applyKey = (key: 'group' | 'model' | 'view', value: string | null | undefined) => {
      if (value === undefined) return
      const normalized = value ? value.trim() : ''
      if (normalized) {
        if (route.query[key] !== normalized) {
          nextQuery[key] = normalized
          changed = true
        }
      } else if (key in nextQuery) {
        delete nextQuery[key]
        changed = true
      }
    }

    applyKey('group', nextPatch.group)
    applyKey('model', nextPatch.model)
    applyKey('view', nextPatch.view)

    if (changed) {
      void router.replace({ query: nextQuery })
    }
  })
}

function handleGroupChange(groupName: string) {
  scheduleQueryPatch({ group: groupName })
}

function handleModelChange(modelName: string) {
  scheduleQueryPatch({ model: modelName })
}

function handleViewChange(mode: PlazaViewMode) {
  scheduleQueryPatch({ view: mode })
}

const data = ref<ModelPlazaResponse | null>(null)
const loading = ref(true)
const loadFailed = ref(false)
let abortController: AbortController | null = null

function isAbortError(err: unknown): boolean {
  if (!err || typeof err !== 'object') return false
  const candidate = err as { name?: string; code?: string }
  return (
    candidate.name === 'CanceledError' ||
    candidate.name === 'AbortError' ||
    candidate.code === 'ERR_CANCELED'
  )
}

async function fetchPlaza() {
  abortController?.abort()
  const controller = new AbortController()
  abortController = controller

  loading.value = true
  loadFailed.value = false
  try {
    const res = await getModelPlaza({ signal: controller.signal })
    if (controller.signal.aborted) return
    data.value = res
  } catch (err) {
    if (controller.signal.aborted || isAbortError(err)) return
    loadFailed.value = true
  } finally {
    if (!controller.signal.aborted) {
      loading.value = false
    }
  }
}

onMounted(() => {
  // 独立形态导航条需要站点名/Logo;有 __APP_CONFIG__ 注入时同步命中缓存。
  void appStore.fetchPublicSettings()
  void fetchPlaza()
})

// 登录态切换时重新拉取对应可见分组与专属倍率
watch(
  () => authStore.isAuthenticated,
  () => {
    void fetchPlaza()
  }
)

onBeforeUnmount(() => {
  abortController?.abort()
  abortController = null
})
</script>
