<template>
  <ChannelStatusV1Page v-if="isV1" />
  <ChannelStatusV2View v-else />
</template>

<script setup lang="ts">
import { computed, defineAsyncComponent } from 'vue'
import { isChannelMonitorV1Mode } from '@/utils/featureFlags'
import { isClassicUi } from '@/utils/uiStyle'
import ChannelStatusV2View from './ChannelStatusV2View.vue'

// V1 page follows the UI style: classic = upstream layout, modern = status board
const ChannelStatusV1Page = isClassicUi()
  ? defineAsyncComponent(() => import('./ChannelStatusV1View.vue'))
  : defineAsyncComponent(() => import('./modern/ChannelStatusV1ModernView.vue'))

const isV1 = computed(() => isChannelMonitorV1Mode())
</script>
