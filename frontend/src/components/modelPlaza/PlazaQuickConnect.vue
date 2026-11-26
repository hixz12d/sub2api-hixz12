<template>
  <div class="space-y-3">
    <!-- Unauthorized exclusive / preview group guidance -->
    <div
      v-if="!isAuthorized"
      class="tile border-l-2 border-warn p-4"
    >
      <div class="flex flex-wrap items-center justify-between gap-2">
        <div class="flex items-center gap-1.5 text-xs font-semibold text-ink">
          <Icon name="lock" size="sm" class="shrink-0 text-warn" />
          <span>{{ t('modelPlaza.quickConnect.unauthorizedTitle') }}</span>
        </div>
        <span class="badge badge-gray text-[11px]">
          {{ resolvedShowcaseBadge }}
        </span>
      </div>
      <p class="mt-2 text-xs leading-relaxed text-ink-2">
        {{ t('modelPlaza.quickConnect.unauthorizedDesc') }}
      </p>
    </div>

    <!-- Authenticated API Key selector (only when authorized for this group) -->
    <div v-if="isAuthenticated && isAuthorized" class="tile p-4">
      <div class="flex flex-wrap items-center justify-between gap-2">
        <label :for="selectId" class="text-xs font-medium text-ink-2">
          {{ t('modelPlaza.quickConnect.apiKeyLabel') }}
        </label>
        <span v-if="keysLoading" class="text-xs text-ink-3">
          {{ t('modelPlaza.quickConnect.apiKeyLoading') }}
        </span>
      </div>

      <div v-if="!keysLoading && matchingKeys.length > 0" class="mt-2">
        <select
          :id="selectId"
          v-model="selectedKeyValue"
          class="input h-9 py-1.5 font-mono text-xs"
        >
          <option
            v-for="item in matchingKeys"
            :key="item.id"
            :value="item.key"
          >
            {{ formatKeyOptionLabel(item) }}
          </option>
        </select>
      </div>

      <div
        v-else-if="!keysLoading"
        class="mt-2 flex flex-wrap items-center justify-between gap-2 bg-surface-tile-2 px-3 py-2"
      >
        <span class="text-xs text-ink-2">
          {{ t('modelPlaza.quickConnect.apiKeyEmpty') }}
        </span>
        <button
          type="button"
          class="btn btn-sm btn-solid"
          @click="goToKeys"
        >
          <Icon name="key" size="xs" />
          <span>{{ t('modelPlaza.quickConnect.createApiKey') }}</span>
        </button>
      </div>
    </div>

    <!-- Client switcher tabs -->
    <div class="tabs" role="tablist">
      <button
        v-for="snippet in snippets"
        :key="snippet.id"
        type="button"
        role="tab"
        :aria-selected="activeClientId === snippet.id"
        class="tab h-9 px-3.5 py-0 text-xs"
        :class="{ 'tab-active': activeClientId === snippet.id }"
        @click="activeClientId = snippet.id"
      >
        {{ t(snippet.labelKey) }}
      </button>
    </div>

    <!-- Active snippet code block -->
    <div v-if="activeSnippet" class="bg-surface-tile">
      <div class="flex items-center justify-between gap-2 border-b border-rule px-3.5 py-2">
        <span class="truncate font-mono text-xs text-ink-3">
          {{ activeSnippet.filename }}
        </span>
        <button
          type="button"
          class="btn btn-sm shrink-0"
          :class="copied ? 'bg-ok text-white hover:bg-ok' : 'btn-secondary'"
          @click="handleCopyCode"
        >
          <Icon :name="copied ? 'check' : 'copy'" size="xs" />
          <span>
            {{
              copied
                ? t('modelPlaza.quickConnect.copiedCode')
                : t('modelPlaza.quickConnect.copyCode')
            }}
          </span>
        </button>
      </div>
      <div class="code-block p-3.5 text-xs leading-relaxed">
        <pre class="m-0 whitespace-pre"><code>{{ activeSnippet.code }}</code></pre>
      </div>
    </div>

    <p class="text-xs text-ink-3">
      {{ t('modelPlaza.quickConnect.configHint', { model: modelName }) }}
    </p>

    <!-- Guest sign-in guidance -->
    <div
      v-if="!isAuthenticated"
      class="flex flex-wrap items-center justify-between gap-3 bg-surface-tile px-4 py-3"
    >
      <div class="flex items-center gap-2 text-xs text-ink-2">
        <Icon name="key" size="sm" class="shrink-0 text-ink-3" />
        <span>{{ t('modelPlaza.quickConnect.guestHint') }}</span>
      </div>
      <div class="flex shrink-0 items-center gap-1.5">
        <button
          type="button"
          class="btn btn-sm btn-primary"
          @click="goToAuth('/login')"
        >
          {{ t('modelPlaza.quickConnect.signIn') }}
        </button>
        <button
          type="button"
          class="btn btn-sm btn-secondary"
          @click="goToAuth('/register')"
        >
          {{ t('modelPlaza.quickConnect.signUp') }}
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import Icon from '@/components/icons/Icon.vue'
import { keysAPI } from '@/api/keys'
import { useAuthStore } from '@/stores/auth'
import { useAppStore } from '@/stores/app'
import { useClipboard } from '@/composables/useClipboard'
import {
  PLACEHOLDER_API_KEY,
  buildQuickConnectSnippets,
  type QuickConnectClientId,
  type QuickConnectSnippet
} from '@/utils/plazaQuickConnect'
import type { ApiKey } from '@/types'

const props = withDefaults(
  defineProps<{
    platform: string
    modelName: string
    groupName: string
    /** Optional internal group ID for matching user API keys (never rendered in UI). */
    groupId?: number | null
    /** Only lazy-fetch user API keys while visible. */
    active?: boolean
    /** Whether the current visitor is authorized to call this group. */
    isAuthorized?: boolean
    /** Optional showcase / preview badge label for unauthorized exclusive groups. */
    showcaseBadge?: string
  }>(),
  {
    groupId: null,
    active: true,
    isAuthorized: true,
    showcaseBadge: ''
  }
)

const { t } = useI18n()
const router = useRouter()
const route = useRoute()
const authStore = useAuthStore()
const appStore = useAppStore()
const { copyToClipboard } = useClipboard()

const selectId = `plaza-qc-key-${Math.random().toString(36).slice(2, 8)}`
const isAuthenticated = computed(() => authStore.isAuthenticated)
const resolvedShowcaseBadge = computed(
  () => props.showcaseBadge?.trim() || t('modelPlaza.quickConnect.unauthorizedBadgeFallback')
)

const allActiveKeys = ref<ApiKey[]>([])
const keysLoaded = ref(false)
const keysLoading = ref(false)
const selectedKeyValue = ref<string>('')
const copied = ref(false)

let fetchController: AbortController | null = null
let copiedTimer: ReturnType<typeof setTimeout> | null = null

const baseUrl = computed(() => {
  const configured = appStore.apiBaseUrl?.trim()
  if (configured) return configured
  return typeof window !== 'undefined' ? window.location.origin : ''
})

function isExactGroupMatch(item: ApiKey): boolean {
  if (props.groupId != null && item.group_id === props.groupId) {
    return true
  }
  if (props.groupName && item.group?.name === props.groupName) {
    return true
  }
  return false
}

const matchingKeys = computed<ApiKey[]>(() => {
  if (!isAuthenticated.value || !props.isAuthorized) return []
  const activeOnly = allActiveKeys.value.filter((k) => k.status === 'active')
  const matched = activeOnly.filter((k) => isExactGroupMatch(k) || k.group_id == null)
  return matched.sort((a, b) => {
    const aExact = isExactGroupMatch(a) ? 0 : 1
    const bExact = isExactGroupMatch(b) ? 0 : 1
    if (aExact !== bExact) return aExact - bExact
    return a.name.localeCompare(b.name)
  })
})

watch(
  matchingKeys,
  (list) => {
    if (list.length === 0) {
      selectedKeyValue.value = ''
      return
    }
    if (!list.some((k) => k.key === selectedKeyValue.value)) {
      selectedKeyValue.value = list[0].key
    }
  },
  { immediate: true }
)

async function loadUserKeys() {
  if (!isAuthenticated.value || !props.isAuthorized || keysLoaded.value || keysLoading.value) return
  fetchController?.abort()
  const controller = new AbortController()
  fetchController = controller
  keysLoading.value = true
  try {
    const res = await keysAPI.list(1, 100, { status: 'active' }, { signal: controller.signal })
    if (controller.signal.aborted) return
    allActiveKeys.value = Array.isArray(res.items) ? res.items : []
    keysLoaded.value = true
  } catch {
    if (controller.signal.aborted) return
    // Silent fallback to placeholder key; avoid noisy error toasts in secondary panel.
    allActiveKeys.value = []
  } finally {
    if (!controller.signal.aborted) {
      keysLoading.value = false
    }
  }
}

watch(
  () => [props.active, isAuthenticated.value, props.isAuthorized] as const,
  ([isActive, authed, authorized]) => {
    if (!authed) {
      fetchController?.abort()
      fetchController = null
      allActiveKeys.value = []
      keysLoaded.value = false
      keysLoading.value = false
      selectedKeyValue.value = ''
      return
    }
    if (!authorized) {
      fetchController?.abort()
      fetchController = null
      keysLoading.value = false
      selectedKeyValue.value = ''
      return
    }
    if (isActive && !keysLoaded.value) {
      void loadUserKeys()
    }
  },
  { immediate: true }
)

const resolvedApiKey = computed(() => {
  if (!isAuthenticated.value || !props.isAuthorized) return PLACEHOLDER_API_KEY
  return selectedKeyValue.value || PLACEHOLDER_API_KEY
})

const snippets = computed<QuickConnectSnippet[]>(() =>
  buildQuickConnectSnippets({
    platform: props.platform,
    modelName: props.modelName,
    baseUrl: baseUrl.value,
    apiKey: resolvedApiKey.value
  })
)

const activeClientId = ref<QuickConnectClientId>('curl')

watch(
  snippets,
  (next) => {
    if (next.length === 0) return
    if (!next.some((s) => s.id === activeClientId.value)) {
      activeClientId.value = next[0].id
    }
  },
  { immediate: true }
)

const activeSnippet = computed<QuickConnectSnippet | null>(
  () => snippets.value.find((s) => s.id === activeClientId.value) ?? snippets.value[0] ?? null
)

function maskKey(raw: string): string {
  if (raw.length <= 12) return raw
  return `${raw.slice(0, 6)}...${raw.slice(-4)}`
}

function formatKeyOptionLabel(item: ApiKey): string {
  const masked = maskKey(item.key)
  const groupSuffix = item.group?.name ? ` · ${item.group.name}` : ''
  return `${item.name} (${masked})${groupSuffix}`
}

async function handleCopyCode() {
  if (!activeSnippet.value) return
  const ok = await copyToClipboard(
    activeSnippet.value.code,
    t('modelPlaza.quickConnect.copiedCode')
  )
  if (ok) {
    copied.value = true
    if (copiedTimer) clearTimeout(copiedTimer)
    copiedTimer = setTimeout(() => {
      copied.value = false
    }, 2000)
  }
}

function goToKeys() {
  void router.push('/keys')
}

function goToAuth(path: '/login' | '/register') {
  void router.push({
    path,
    query: route.fullPath ? { redirect: route.fullPath } : undefined
  })
}

onBeforeUnmount(() => {
  fetchController?.abort()
  if (copiedTimer) clearTimeout(copiedTimer)
})
</script>
