<template>
  <!--
    到账弹窗 / 用卡确认。两种用法：
    - 不传 card-id：挂在 App 上，自动弹出新到账的卡（每张卡只弹一次）；
    - 传 card-id：钱包列表点"使用"时打开同一张确认卡面，null 表示关闭，关闭时触发 close。
    手机端（< sm）从底部弹出、全宽。
  -->
  <Teleport to="body">
    <Transition name="rc-sheet">
      <div
        v-if="current"
        class="fixed inset-0 z-[115] flex items-end justify-center bg-black/45 sm:items-center sm:p-4"
        @click.self="close"
      >
        <div
          ref="panelRef"
          role="dialog"
          aria-modal="true"
          :aria-labelledby="titleId"
          tabindex="-1"
          class="rc-panel flex max-h-[92dvh] w-full flex-col overflow-y-auto border-t border-rule bg-surface outline-none sm:max-w-[460px] sm:border"
        >
          <h2 :id="titleId" class="border-b border-rule px-5 py-4 text-lg font-semibold text-ink" aria-live="polite">
            {{ title }}
          </h2>

          <div class="px-5 pt-4">
            <ResetCardTile :card="current" :result="result" :nothing="nothing" />
            <p v-if="moreCount > 0 && !result" class="mt-3 text-[13px] text-ink-3">
              {{ t('resetCards.popup.moreCards', { n: moreCount }) }}
              <router-link
                to="/wallet/reset-cards"
                class="ml-1 font-medium text-money-text underline-offset-2 hover:underline"
                @click="close"
              >
                {{ t('resetCards.popup.viewAll') }}
              </router-link>
            </p>
          </div>

          <div
            class="mt-4 flex justify-end gap-2 bg-surface-tile px-5 pt-3 pb-[max(12px,env(safe-area-inset-bottom))]"
          >
            <button v-if="result" ref="primaryRef" type="button" class="btn btn-primary min-w-[96px]" @click="close">
              {{ t('resetCards.popup.done') }}
            </button>
            <template v-else>
              <button ref="laterRef" type="button" class="btn btn-ghost" :disabled="busy" @click="close">
                {{ manual ? t('resetCards.popup.cancel') : t('resetCards.popup.later') }}
              </button>
              <button
                ref="primaryRef"
                type="button"
                class="btn btn-primary min-w-[112px]"
                :disabled="busy || nothing"
                :aria-busy="busy"
                @click="useNow"
              >
                <span v-if="busy" class="spinner h-4 w-4" aria-hidden="true"></span>
                {{ t('resetCards.popup.useNow') }}
              </button>
            </template>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import ResetCardTile from './ResetCardTile.vue'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'
import { useAnnouncementStore } from '@/stores/announcements'
import { resetCardErrorReason, useResetCardsStore } from '@/stores/resetCards'
import type { ResetCard, ResetCardUseResponse } from '@/types/resetCard'

const props = withDefaults(
  defineProps<{
    /** 手动模式：要确认使用的卡 id；null 为关闭。不传则为自动到账弹窗 */
    cardId?: number | null
  }>(),
  { cardId: undefined }
)
const emit = defineEmits<{ close: [] }>()

const { t } = useI18n()
const route = useRoute()
const appStore = useAppStore()
const authStore = useAuthStore()
const announcementStore = useAnnouncementStore()
const store = useResetCardsStore()

const titleId = `reset-card-popup-title-${Math.random().toString(36).slice(2, 10)}`

const manual = computed(() => props.cardId !== undefined)

// 打开时对卡做一份快照：用卡后列表会刷新，卡面要保持本次弹窗的内容直到关闭
const current = ref<ResetCard | null>(null)
const moreCount = ref(0)
const result = ref<ResetCardUseResponse | null>(null)
const nothingFromServer = ref(false)
const busy = ref(false)

const panelRef = ref<HTMLElement | null>(null)
const primaryRef = ref<HTMLButtonElement | null>(null)
const laterRef = ref<HTMLButtonElement | null>(null)
let previousFocus: HTMLElement | null = null

const nothing = computed(() => {
  if (result.value || !current.value) return false
  if (nothingFromServer.value) return true
  return current.value.status === 'unused' && (current.value.estimate?.amount ?? 0) <= 0
})

const title = computed(() => {
  if (result.value) return t('resetCards.popup.titleDone')
  if (manual.value) return t('resetCards.popup.titleManual')
  const total = moreCount.value + 1
  return total > 1 ? t('resetCards.popup.titleMany', { n: total }) : t('resetCards.popup.titleOne')
})

function snapshot(card: ResetCard): ResetCard {
  return {
    ...card,
    groups: card.groups.map((g) => ({ ...g })),
    estimate: card.estimate ? { ...card.estimate, daily: [...card.estimate.daily] } : null,
    credit: card.credit ? { ...card.credit } : null
  }
}

function open(card: ResetCard, others: number) {
  previousFocus = document.activeElement as HTMLElement | null
  current.value = snapshot(card)
  moreCount.value = others
  result.value = null
  nothingFromServer.value = false
  busy.value = false
  document.body.classList.add('modal-open')
  void nextTick(focusAction)
  void refreshEstimate(card.id)
}

// 自动弹出时焦点放在"稍后"，避免误按回车直接用掉卡；手动模式用户已点过"使用"，焦点放在确认按钮
function focusAction() {
  const primaryUsable = primaryRef.value && !primaryRef.value.disabled
  const target = result.value
    ? primaryRef.value
    : manual.value && primaryUsable
      ? primaryRef.value
      : laterRef.value
  ;(target ?? panelRef.value)?.focus()
}

// 按钮变成不可点（预估为 0）后焦点会掉到 body，挪回弹窗内
watch(nothing, (isNothing) => {
  if (isNothing && current.value) void nextTick(focusAction)
})

// 打开时静默重算一次预估（卡面数字每天会变），失败不打扰用户
async function refreshEstimate(id: number) {
  try {
    const estimate = await store.preview(id)
    if (current.value?.id === id && !result.value) {
      current.value = { ...current.value, estimate }
    }
  } catch {
    // 用卡时还会再算一次，这里忽略
  }
}

function close() {
  if (busy.value || !current.value) return
  current.value = null
  result.value = null
  nothingFromServer.value = false
  moreCount.value = 0
  // 别的模态框（BaseDialog）还开着时，保留它的滚动锁定
  void nextTick(() => {
    if (!document.querySelector('.modal-overlay')) document.body.classList.remove('modal-open')
  })
  // 列表里的"使用"按钮在用卡后会消失，那时由父组件接管焦点
  if (previousFocus?.isConnected) previousFocus.focus()
  previousFocus = null
  if (manual.value) emit('close')
}

async function useNow() {
  const card = current.value
  if (!card || busy.value || nothing.value || result.value) return
  busy.value = true
  try {
    // 先拿最新预估，再真正使用；预估为 0 时不消耗卡
    const estimate = await store.preview(card.id)
    if (current.value?.id !== card.id) return
    current.value = { ...current.value, estimate }
    if (estimate.amount <= 0) {
      nothingFromServer.value = true
      return
    }
    const res = await store.use(card.id)
    if (current.value?.id === card.id) {
      result.value = res
      void nextTick(() => primaryRef.value?.focus())
    }
  } catch (err) {
    const reason = resetCardErrorReason(err)
    if (reason === 'RESET_CARD_NOTHING_TO_REFUND') {
      nothingFromServer.value = true
    } else if (
      reason === 'RESET_CARD_EXPIRED' ||
      reason === 'RESET_CARD_NOT_AVAILABLE' ||
      reason === 'RESET_CARD_DISABLED'
    ) {
      // 重试也不会成功：提示原因后关掉弹窗，并刷新列表
      appStore.showError(errorMessage(reason, err))
      void store.fetch(true)
      busy.value = false
      close()
    } else {
      // 网络等临时错误：弹窗保持打开，可以再点
      appStore.showError(errorMessage(reason, err))
    }
  } finally {
    busy.value = false
    if (current.value && !result.value) void nextTick(focusAction)
  }
}

function errorMessage(reason: string | undefined, err: unknown): string {
  switch (reason) {
    case 'RESET_CARD_EXPIRED':
      return t('resetCards.errors.expired')
    case 'RESET_CARD_NOT_AVAILABLE':
      return t('resetCards.errors.notAvailable')
    case 'RESET_CARD_DISABLED':
      return t('resetCards.errors.disabled')
    // 429：同一用户正在用另一张卡（服务端加锁），稍后可以再点
    case 'RESET_CARD_BUSY':
      return t('resetCards.errors.busy')
    default: {
      const message = (err as { message?: unknown } | null)?.message
      return typeof message === 'string' && message ? message : t('resetCards.errors.failed')
    }
  }
}

// ---------- 自动模式：新到账的卡 ----------
// 只在控制台页面（需登录的路由）弹；公告弹窗还在时先等它关掉；一次只弹一个窗口
const autoCandidates = computed<ResetCard[] | null>(() => {
  if (manual.value || !store.enabled || !authStore.isAuthenticated) return null
  if (route.meta.requiresAuth !== true) return null
  if (announcementStore.currentPopup) return null
  const ids = store.popupCardIds
  if (ids.length === 0) return null
  const cards = store.cards.filter((c) => ids.includes(c.id) && c.status === 'unused')
  return cards.length > 0 ? cards : null
})

watch(
  autoCandidates,
  (cards) => {
    if (!cards || current.value) return
    // 多张时展示预计返还最高的那张
    const best = cards.reduce((a, b) => ((b.estimate?.amount ?? 0) > (a.estimate?.amount ?? 0) ? b : a))
    open(best, cards.length - 1)
    // 弹出即已读：窗口里涉及的卡都标记，保证同一张卡只弹一次
    void store.markPopupSeen(cards.map((c) => c.id))
  },
  { immediate: true }
)

// ---------- 手动模式：列表点"使用" ----------
watch(
  () => props.cardId,
  (id) => {
    if (!manual.value) return
    if (id == null) {
      if (current.value) close()
      return
    }
    const card = store.cards.find((c) => c.id === id)
    if (card) open(card, 0)
    else emit('close')
  },
  { immediate: true }
)

function onKeydown(e: KeyboardEvent) {
  if (!current.value) return
  if (e.key === 'Escape') {
    close()
    return
  }
  // 焦点留在弹窗内
  if (e.key === 'Tab' && panelRef.value) {
    const focusables = Array.from(
      panelRef.value.querySelectorAll<HTMLElement>('button:not([disabled]), [href]')
    )
    if (focusables.length === 0) return
    const first = focusables[0]
    const last = focusables[focusables.length - 1]
    const active = document.activeElement
    if (e.shiftKey && (active === first || !panelRef.value.contains(active))) {
      e.preventDefault()
      last.focus()
    } else if (!e.shiftKey && (active === last || !panelRef.value.contains(active))) {
      e.preventDefault()
      first.focus()
    }
  }
}

onMounted(() => document.addEventListener('keydown', onKeydown))
onBeforeUnmount(() => {
  document.removeEventListener('keydown', onKeydown)
  if (current.value && !document.querySelector('.modal-overlay')) document.body.classList.remove('modal-open')
})
</script>

<style scoped>
/* 遮罩淡入；面板桌面端从下方 12px 滑上，手机端从屏幕底部滑出 */
.rc-sheet-enter-active {
  transition: opacity 250ms ease-out;
}

.rc-sheet-leave-active {
  transition: opacity 200ms ease-in;
}

.rc-sheet-enter-from,
.rc-sheet-leave-to {
  opacity: 0;
}

.rc-sheet-enter-active .rc-panel {
  transition: transform 300ms cubic-bezier(0.2, 0.9, 0.2, 1);
}

.rc-sheet-leave-active .rc-panel {
  transition: transform 200ms ease-in;
}

.rc-sheet-enter-from .rc-panel,
.rc-sheet-leave-to .rc-panel {
  transform: translateY(100%);
}

@media (min-width: 640px) {
  .rc-sheet-enter-active .rc-panel {
    transition-duration: 250ms;
  }

  .rc-sheet-enter-from .rc-panel,
  .rc-sheet-leave-to .rc-panel {
    transform: translateY(12px);
  }
}

@media (prefers-reduced-motion: reduce) {
  .rc-sheet-enter-active,
  .rc-sheet-leave-active,
  .rc-sheet-enter-active .rc-panel,
  .rc-sheet-leave-active .rc-panel {
    transition-duration: 1ms;
  }

  .rc-sheet-enter-from .rc-panel,
  .rc-sheet-leave-to .rc-panel {
    transform: none;
  }
}
</style>
