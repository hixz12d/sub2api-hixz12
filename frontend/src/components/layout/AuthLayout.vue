<template>
  <div class="flex min-h-screen bg-surface">
    <!-- 品牌区：宽屏左侧整块钱绿磁贴，手机上隐藏 -->
    <aside class="relative hidden w-[44%] max-w-[640px] flex-col justify-between bg-money p-12 text-money-ink lg:flex xl:p-16">
      <router-link to="/home" class="flex items-center gap-3">
        <span v-if="settingsLoaded" class="flex h-10 w-10 items-center justify-center overflow-hidden bg-white/10">
          <img :src="siteLogo || '/logo.svg'" alt="Logo" class="h-full w-full object-contain" />
        </span>
        <span class="text-lg font-semibold tracking-tight">{{ siteName }}</span>
      </router-link>

      <div class="auth-glide">
        <p class="max-w-[12em] text-[54px] font-light leading-[1.08] tracking-[-0.035em]">
          {{ siteName }}
        </p>
        <p class="mt-5 max-w-[28em] text-[17px] font-light leading-relaxed text-money-dim">
          {{ siteSubtitle }}
        </p>
      </div>

      <div class="grid grid-cols-3 gap-2 text-sm">
        <div v-for="point in points" :key="point" class="bg-black/15 px-4 py-3.5 text-money-ink/90">
          {{ point }}
        </div>
      </div>
    </aside>

    <!-- 表单区 -->
    <main class="flex flex-1 flex-col">
      <div class="flex items-center justify-between px-5 py-5 lg:px-10">
        <router-link to="/home" class="flex items-center gap-2.5 lg:invisible">
          <span v-if="settingsLoaded" class="flex h-8 w-8 items-center justify-center overflow-hidden">
            <img :src="siteLogo || '/logo.svg'" alt="Logo" class="h-full w-full object-contain" />
          </span>
          <span class="text-base font-semibold tracking-tight text-ink">{{ siteName }}</span>
        </router-link>
        <LocaleSwitcher />
      </div>

      <div class="flex flex-1 items-center justify-center px-5 pb-10 lg:px-10">
        <div class="auth-glide w-full max-w-[420px]">
          <slot />

          <!-- Footer Links -->
          <div class="mt-8 text-sm">
            <slot name="footer" />
          </div>
        </div>
      </div>

      <div class="px-5 pb-6 text-xs text-ink-3 lg:px-10">
        &copy; {{ currentYear }} {{ siteName }}. All rights reserved.
      </div>
    </main>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores'
import { sanitizeUrl } from '@/utils/url'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'

const { t } = useI18n()
const appStore = useAppStore()

const siteName = computed(() => appStore.siteName || 'Sub2API')
const siteLogo = computed(() => sanitizeUrl(appStore.siteLogo || '', { allowRelative: true, allowDataUrl: true }))
const siteSubtitle = computed(() => appStore.cachedPublicSettings?.site_subtitle || t('authPages.defaultSubtitle'))
const settingsLoaded = computed(() => appStore.publicSettingsLoaded)

const points = computed(() => [t('authPages.points.unified'), t('authPages.points.transparent'), t('authPages.points.topup')])

const currentYear = computed(() => new Date().getFullYear())

onMounted(() => {
  appStore.fetchPublicSettings()
})
</script>

<style scoped>
.auth-glide {
  animation: auth-glide 0.5s cubic-bezier(0.2, 0.9, 0.2, 1) both;
}

@keyframes auth-glide {
  from {
    opacity: 0;
    transform: translateX(28px);
  }
}

@media (prefers-reduced-motion: reduce) {
  .auth-glide {
    animation: none;
  }
}
</style>
