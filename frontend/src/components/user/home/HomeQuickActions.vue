<template>
  <!-- 有 Key：三块快捷磁贴；还没有 Key：换成三步引导，同样可点 -->
  <template v-if="!onboarding">
    <button
      v-if="canRecharge"
      type="button"
      class="action-tile col-span-2 bg-money text-money-ink hover:bg-money-hover lg:col-span-1"
      @click="router.push(walletPath)"
    >
      <Icon name="plus" size="lg" :stroke-width="1.5" />
      <span>
        <span class="block text-[17px] font-semibold">{{ t('userHome.actions.recharge') }}</span>
        <small class="block text-[13px] opacity-80">{{ t('userHome.actions.rechargeHint') }}</small>
      </span>
    </button>
    <button
      type="button"
      class="action-tile bg-gray-950 text-white hover:bg-gray-800 dark:bg-white dark:text-black dark:hover:bg-gray-200"
      @click="router.push('/keys')"
    >
      <Icon name="key" size="lg" :stroke-width="1.5" />
      <span>
        <span class="block text-[17px] font-semibold">{{ t('userHome.actions.createKey') }}</span>
        <small class="block text-[13px] opacity-80">{{ t('userHome.actions.createKeyHint') }}</small>
      </span>
    </button>
    <button
      type="button"
      class="action-tile bg-surface-tile text-ink hover:bg-surface-tile-2"
      :class="{ '!bg-ok !text-white': copied }"
      @click="copyEndpoint"
    >
      <Icon :name="copied ? 'check' : 'link'" size="lg" :stroke-width="1.5" />
      <span class="min-w-0">
        <span class="block text-[17px] font-semibold">{{ copied ? t('userHome.actions.endpointCopied') : t('userHome.actions.copyEndpoint') }}</span>
        <small class="block truncate text-[13px] opacity-80">{{ endpointHost }}</small>
      </span>
    </button>
  </template>

  <template v-else>
    <button
      v-for="(step, i) in steps"
      :key="step.key"
      type="button"
      class="action-tile items-start bg-surface-tile text-ink hover:bg-surface-tile-2"
      :class="{ 'col-span-2 lg:col-span-1': i === 0 && steps.length === 3 }"
      @click="step.run()"
    >
      <span class="text-[40px] font-light leading-none text-money-text">{{ i + 1 }}</span>
      <span>
        <span class="block text-base font-semibold">{{ t(`userHome.onboarding.${step.key}`) }}</span>
        <small class="block text-[13px] text-ink-2">{{ t(`userHome.onboarding.${step.key}Hint`) }}</small>
      </span>
    </button>
  </template>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import Icon from '@/components/icons/Icon.vue'
import { useClipboard } from '@/composables/useClipboard'

const props = defineProps<{
  onboarding: boolean
  canRecharge: boolean
  walletPath: string
  endpoint: string
}>()

const { t } = useI18n()
const router = useRouter()
const { copied, copyToClipboard } = useClipboard()

const endpointHost = computed(() => props.endpoint.replace(/^https?:\/\//, ''))

function copyEndpoint() {
  void copyToClipboard(props.endpoint, t('userHome.actions.endpointCopied'))
}

const steps = computed(() => {
  const list = [
    { key: 'step1', run: () => router.push('/keys') },
    { key: 'step2', run: copyEndpoint }
  ]
  if (props.canRecharge) list.push({ key: 'step3', run: () => router.push(props.walletPath) })
  return list
})
</script>

<style scoped>
.action-tile {
  display: flex;
  min-height: 104px;
  flex-direction: column;
  justify-content: space-between;
  gap: 12px;
  padding: 18px 20px;
  text-align: left;
  transition:
    background-color 0.15s ease,
    transform 0.18s cubic-bezier(0.2, 0.8, 0.2, 1);
}

.action-tile:active {
  transform: scale(0.985);
}
</style>
