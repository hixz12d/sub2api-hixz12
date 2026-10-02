<template>
  <slot />
</template>

<script setup lang="ts">
import { provide } from 'vue'

// 只对插槽里的子页面生效：让旧的充值、兑换、订单、返利页自带的 AppLayout 只渲染内容，
// 钱包页自己的 AppLayout（在本组件之外）照常显示侧栏和顶栏
provide('appLayoutEmbedded', true)
</script>
