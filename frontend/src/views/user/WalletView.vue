<template>
  <AppLayout>
    <div class="mx-auto max-w-[1320px]">
      <h1 class="tile-glide text-[40px] font-light leading-[1.08] tracking-[-0.035em] text-ink sm:text-[54px]" style="--n: 0">
        {{ t('wallet.title') }}
      </h1>

      <WalletBalanceCard
        class="tile-glide mt-6"
        style="--n: 1"
        :balance="balance"
        :frozen="frozen"
        :rebate="rebate"
        :can-recharge="showRecharge"
        @recharge="router.push('/wallet/recharge')"
      />

      <WalletTabs
        class="mt-8"
        :tabs="visibleTabs"
        :current-path="route.path"
        :query="route.query"
        :label="t('wallet.title')"
      />

      <!-- 子页面（旧的充值、兑换、订单、返利页）在嵌入模式下只渲染内容 -->
      <div class="wallet-panel mt-6">
        <WalletEmbedScope>
          <router-view />
        </WalletEmbedScope>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import WalletBalanceCard from '@/components/user/wallet/WalletBalanceCard.vue'
import WalletEmbedScope from '@/components/user/wallet/WalletEmbedScope.vue'
import WalletTabs, { type WalletTab } from '@/components/user/wallet/WalletTabs.vue'
import userAPI from '@/api/user'
import { useAppStore, useAuthStore } from '@/stores'
import { usePaymentStore } from '@/stores/payment'
import { FeatureFlags, makeSidebarFlag } from '@/utils/featureFlags'
import { getExternalRechargeEntries } from '@/utils/externalRecharge'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const appStore = useAppStore()
const authStore = useAuthStore()
const paymentStore = usePaymentStore()

const balance = computed(() => Number(authStore.user?.balance || 0))
const frozen = computed(() => Number(authStore.user?.frozen_balance || 0))

// 各页签的显示条件沿用侧栏原有开关：充值 flagPurchaseEntry、订单 flagPayment、兑换始终可见、返利 flagAffiliate
const flagPayment = makeSidebarFlag(FeatureFlags.payment)
const flagAffiliate = makeSidebarFlag(FeatureFlags.affiliate)
const hasExternalRecharge = computed(() => getExternalRechargeEntries(appStore.cachedPublicSettings?.custom_menu_items).length > 0)
const showRecharge = computed(() => (flagPayment() !== false && paymentStore.purchaseEntryAvailable) || hasExternalRecharge.value)
const showOrders = computed(() => flagPayment() !== false && !authStore.isSimpleMode)
const showRedeem = computed(() => !authStore.isSimpleMode)
const showInvite = computed(() => flagAffiliate() !== false && !authStore.isSimpleMode)

const visibleTabs = computed((): WalletTab[] => {
  const tabs: WalletTab[] = []
  if (showRecharge.value) tabs.push({ path: '/wallet/recharge', label: t('wallet.tabs.recharge') })
  if (showRedeem.value) tabs.push({ path: '/wallet/redeem', label: t('wallet.tabs.redeem') })
  if (showOrders.value) tabs.push({ path: '/wallet/orders', label: t('wallet.tabs.orders') })
  if (showInvite.value) tabs.push({ path: '/wallet/invite', label: t('wallet.tabs.invite') })
  return tabs
})

// /wallet 或不可见的页签 → 跳到第一个可见页签。
// 公共设置没加载完时开关还是未知态，只处理 /wallet 本身，避免把 /wallet/invite 等直达链接误跳走
function ensureVisibleTab() {
  const first = visibleTabs.value[0]
  if (!first) return
  if (route.path === '/wallet' || route.path === '/wallet/') {
    void router.replace({ path: first.path, query: route.query })
    return
  }
  if (!appStore.publicSettingsLoaded) return
  const onVisible = visibleTabs.value.some((tab) => route.path.startsWith(tab.path))
  if (!onVisible) void router.replace({ path: first.path, query: route.query })
}
watch([() => route.path, visibleTabs, () => appStore.publicSettingsLoaded], ensureVisibleTab)

// 返利余额：返利功能开启时取一次（与“邀请返利”页同一个接口）；设置可能稍后才加载完，所以用 watch
const rebate = ref<number | null>(null)
let rebateRequested = false
async function loadRebate() {
  if (rebateRequested || !showInvite.value || flagAffiliate() !== true) return
  rebateRequested = true
  try {
    const detail = await userAPI.getAffiliateDetail()
    rebate.value = Number(detail.aff_quota || 0)
  } catch {
    rebate.value = null
  }
}
watch(showInvite, () => void loadRebate())

onMounted(async () => {
  void authStore.refreshUser().catch(() => undefined)
  void loadRebate()
  if (flagPayment() !== false) await paymentStore.fetchConfig()
  ensureVisibleTab()
})
</script>

<style scoped>
/* 嵌入的旧页面自带 max-w-* mx-auto 外框，在钱包里改为占满内容宽度，四个页签对齐 */
.wallet-panel :deep(> div.mx-auto) {
  max-width: none;
}
</style>
