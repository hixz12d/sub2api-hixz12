<template>
  <!-- 嵌入模式：上层页面 provide('appLayoutEmbedded', true) 时只渲染内容，不再套侧栏和顶栏 -->
  <slot v-if="embedded" />
  <!-- 经典风格沿用上游写法：浅灰底 + 网格渐变装饰层；新版用 bg-surface -->
  <div v-else class="min-h-screen" :class="classicUi ? 'bg-gray-50 dark:bg-dark-950' : 'bg-surface'">
    <!-- Background Decoration（仅经典风格） -->
    <div v-if="classicUi" class="pointer-events-none fixed inset-0 bg-mesh-gradient"></div>

    <!-- Sidebar -->
    <SidebarComponent />

    <!-- Main Content Area -->
    <div
      class="relative min-h-screen transition-all duration-300"
      :class="[sidebarCollapsed ? 'lg:ml-[72px]' : 'lg:ml-64']"
    >
      <!-- Header -->
      <HeaderComponent />

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
import { isClassicUi } from '@/utils/uiStyle'
import AppSidebar from './AppSidebar.vue'
import AppHeader from './AppHeader.vue'
import ModernSidebar from './modern/ModernSidebar.vue'
import ModernHeader from './modern/ModernHeader.vue'

const embedded = inject<boolean>('appLayoutEmbedded', false)

// 风格在页面加载时就定了（切换时整页刷新），用普通常量即可
// 经典风格：上游原路径的 AppSidebar / AppHeader；新版：layout/modern/ 下的组件
const classicUi = isClassicUi()
const SidebarComponent = classicUi ? AppSidebar : ModernSidebar
const HeaderComponent = classicUi ? AppHeader : ModernHeader

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
