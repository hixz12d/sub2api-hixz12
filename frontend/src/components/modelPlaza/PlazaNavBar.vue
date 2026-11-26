<template>
  <header class="sticky top-0 z-30 border-b border-rule bg-surface text-ink">
    <div
      class="mx-auto flex h-16 max-w-[1320px] items-center justify-between gap-3 px-4 md:px-6 lg:h-[76px] lg:px-8"
    >
      <!-- Left: Site Logo + Name -->
      <RouterLink to="/home" class="flex min-w-0 items-center gap-3">
        <template v-if="settings">
          <span
            class="flex h-9 w-9 shrink-0 items-center justify-center overflow-hidden rounded-none bg-surface-tile"
          >
            <img :src="siteLogo || '/logo.svg'" alt="Logo" class="h-full w-full object-contain" />
          </span>
          <span class="truncate text-base font-semibold text-ink">
            {{ siteName }}
          </span>
        </template>
        <template v-else>
          <span
            class="h-9 w-9 shrink-0 animate-pulse rounded-none bg-surface-tile"
            aria-hidden="true"
          />
          <span
            class="h-5 w-28 animate-pulse rounded-none bg-surface-tile"
            aria-hidden="true"
          />
        </template>
      </RouterLink>

      <!-- Right: Monitor shortcut (if available) + Theme Toggle + Locale Switcher + Login / Console -->
      <div class="flex shrink-0 items-center gap-1.5 sm:gap-2">
        <RouterLink
          v-if="canOpenMonitor"
          to="/monitor"
          class="btn btn-ghost hidden h-10 rounded-none px-3 text-sm sm:inline-flex"
        >
          <Icon name="chart" size="sm" aria-hidden="true" />
          <span>{{ t('modelPlaza.nav.monitor') }}</span>
        </RouterLink>

        <button
          type="button"
          class="inline-flex h-10 w-10 shrink-0 items-center justify-center rounded-none text-ink-2 transition-colors duration-150 hover:bg-surface-tile hover:text-ink"
          :title="t('modelPlaza.nav.switchTheme')"
          :aria-label="t('modelPlaza.nav.switchTheme')"
          @click="toggleTheme"
        >
          <Icon :name="isDark ? 'sun' : 'moon'" size="md" aria-hidden="true" />
        </button>

        <LocaleSwitcher />

        <RouterLink
          v-if="isAuthenticated"
          :to="backTarget"
          class="btn btn-primary h-10 shrink-0 rounded-none px-4 text-sm"
        >
          {{ t('modelPlaza.nav.backToDashboard') }}
        </RouterLink>
        <RouterLink
          v-else
          :to="{ path: '/login', query: { redirect: loginRedirect } }"
          class="btn btn-primary h-10 shrink-0 rounded-none px-4 text-sm"
        >
          {{ t('modelPlaza.nav.login') }}
        </RouterLink>
      </div>
    </div>
  </header>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'
import { sanitizeUrl } from '@/utils/url'
import { isChannelMonitorRouteEnabled } from '@/utils/featureFlags'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'

const { t } = useI18n()
const route = useRoute()
const appStore = useAppStore()
const authStore = useAuthStore()

const settings = computed(() => appStore.cachedPublicSettings)
const siteName = computed(() => settings.value?.site_name || 'Sub2API')
const siteLogo = computed(() =>
  sanitizeUrl(settings.value?.site_logo || '', { allowRelative: true, allowDataUrl: true })
)
const isAuthenticated = computed(() => authStore.isAuthenticated)
const backTarget = computed(() => (authStore.isAdmin ? '/admin/dashboard' : '/dashboard'))
const canOpenMonitor = computed(() => isAuthenticated.value && isChannelMonitorRouteEnabled())
const loginRedirect = computed(() => route.fullPath || '/model-plaza')

const isDark = ref(
  typeof document !== 'undefined' && document.documentElement.classList.contains('dark')
)

function toggleTheme() {
  isDark.value = !isDark.value
  document.documentElement.classList.toggle('dark', isDark.value)
  try {
    localStorage.setItem('theme', isDark.value ? 'dark' : 'light')
  } catch {
    // ignore storage errors in restricted browsing modes
  }
}

onMounted(() => {
  isDark.value = document.documentElement.classList.contains('dark')
})
</script>
