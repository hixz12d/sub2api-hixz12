<template>
  <!-- 每项一块小磁贴，选中项钱绿灌满；手机上可横向滑动 -->
  <nav class="-mx-4 overflow-x-auto px-4 scrollbar-hide sm:mx-0 sm:px-0" :aria-label="label">
    <div class="flex w-max gap-1">
      <router-link
        v-for="tab in tabs"
        :key="tab.path"
        :to="{ path: tab.path, query: currentPath === tab.path ? query : undefined }"
        class="tab whitespace-nowrap"
        :class="{ 'tab-active': currentPath.startsWith(tab.path) }"
        :aria-current="currentPath.startsWith(tab.path) ? 'page' : undefined"
      >
        {{ tab.label }}
      </router-link>
    </div>
  </nav>
</template>

<script setup lang="ts">
import type { LocationQuery } from 'vue-router'

export interface WalletTab {
  path: string
  label: string
}

defineProps<{
  tabs: WalletTab[]
  currentPath: string
  query: LocationQuery
  label: string
}>()
</script>
