<template>
  <!-- 入口组件：按界面风格挑选登录注册外框。经典版见 classic/AuthLayoutClassic.vue，新版见 modern/ModernAuthLayout.vue -->
  <component :is="LayoutComponent">
    <slot />
    <template v-if="$slots.footer" #footer>
      <slot name="footer" />
    </template>
  </component>
</template>

<script setup lang="ts">
import { isClassicUi } from '@/utils/uiStyle'
import AuthLayoutClassic from './classic/AuthLayoutClassic.vue'
import ModernAuthLayout from './modern/ModernAuthLayout.vue'

// 风格在页面加载时就定了（切换时整页刷新），用普通常量即可
const LayoutComponent = isClassicUi() ? AuthLayoutClassic : ModernAuthLayout
</script>
