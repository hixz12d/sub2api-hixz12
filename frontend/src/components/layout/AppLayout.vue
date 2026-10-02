<template>
  <!-- 嵌入模式：上层页面 provide('appLayoutEmbedded', true) 时只渲染内容，不再套侧栏和顶栏 -->
  <slot v-if="embedded" />
  <div v-else class="min-h-screen bg-surface">
    <!-- Sidebar -->
    <AppSidebar />

    <!-- Main Content Area -->
    <div
      class="relative min-h-screen transition-all duration-300"
      :class="[sidebarCollapsed ? 'lg:ml-[72px]' : 'lg:ml-64']"
    >
      <!-- Header -->
      <AppHeader />

      <!-- Main Content -->
      <main class="p-4 md:p-6 lg:p-8">
        <slot />
      </main>
    </div>
  </div>
</template>

<script setup lang="ts">
import '@/styles/onboarding.css'
import { computed, inject, onMounted } from 'vue'
import { useAppStore } from '@/stores'
import { useAuthStore } from '@/stores/auth'
import { useOnboardingTour } from '@/composables/useOnboardingTour'
import { useOnboardingStore } from '@/stores/onboarding'
import AppSidebar from './AppSidebar.vue'
import AppHeader from './AppHeader.vue'

const embedded = inject<boolean>('appLayoutEmbedded', false)

const appStore = useAppStore()
const authStore = useAuthStore()
const sidebarCollapsed = computed(() => appStore.sidebarCollapsed)
const isAdmin = computed(() => authStore.user?.role === 'admin')

// 嵌入时外层 AppLayout 已经启动了引导，这里不再重复启动
const { replayTour } = useOnboardingTour({
  storageKey: isAdmin.value ? 'admin_guide' : 'user_guide',
  autoStart: !embedded
})

const onboardingStore = useOnboardingStore()

onMounted(() => {
  if (!embedded) onboardingStore.setReplayCallback(replayTour)
})

defineExpose({ replayTour })
</script>
