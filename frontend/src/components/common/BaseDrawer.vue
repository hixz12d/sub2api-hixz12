<template>
  <Teleport to="body">
    <Transition name="drawer">
      <div
        v-if="show"
        class="drawer-overlay fixed inset-0 flex justify-end bg-black/45"
        :style="zIndexStyle"
        :aria-labelledby="drawerTitleId"
        role="dialog"
        aria-modal="true"
        @click.self="handleOverlayClick"
      >
        <div
          ref="panelRef"
          tabindex="-1"
          :class="[
            'drawer-panel flex h-full flex-col rounded-none border-l border-rule bg-surface text-ink focus:outline-none',
            widthClass
          ]"
          @click.stop
        >
          <!-- Header -->
          <div class="flex shrink-0 items-start justify-between gap-3 border-b border-rule px-5 py-4">
            <div class="min-w-0 flex-1">
              <slot name="title">
                <h2 :id="drawerTitleId" class="truncate text-lg font-semibold leading-snug text-ink">
                  {{ title }}
                </h2>
              </slot>
              <p v-if="subtitle" class="mt-0.5 truncate text-[13px] text-ink-3">
                {{ subtitle }}
              </p>
            </div>

            <div class="flex shrink-0 items-center gap-1.5">
              <slot name="header-actions" />
              <button
                v-if="showCloseButton"
                type="button"
                class="inline-flex h-8 w-8 shrink-0 items-center justify-center rounded-none text-ink-2 transition-colors duration-150 hover:bg-surface-tile hover:text-ink"
                :aria-label="closeLabel"
                @click="emit('close')"
              >
                <Icon name="x" size="md" />
              </button>
            </div>
          </div>

          <!-- Body -->
          <div ref="bodyRef" class="flex-1 overflow-y-auto px-5 py-4">
            <slot />
          </div>

          <!-- Footer -->
          <div
            v-if="$slots.footer"
            class="flex shrink-0 items-center justify-end gap-3 border-t border-rule bg-surface-tile px-5 py-3.5"
          >
            <slot name="footer" />
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<script lang="ts">
let drawerIdCounter = 0
const openDrawers = new Set<string>()
</script>

<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import Icon from '@/components/icons/Icon.vue'

const drawerId = `drawer-${++drawerIdCounter}`
const drawerTitleId = `drawer-title-${drawerIdCounter}`

const panelRef = ref<HTMLElement | null>(null)
const bodyRef = ref<HTMLElement | null>(null)
let previousActiveElement: HTMLElement | null = null

interface Props {
  show: boolean
  title?: string
  subtitle?: string
  /** Optional Tailwind width utility override; defaults to full width on mobile and 560px / 92vw on md+. */
  width?: string
  closeOnEscape?: boolean
  closeOnClickOutside?: boolean
  showCloseButton?: boolean
  closeLabel?: string
  zIndex?: number
}

interface Emits {
  (e: 'close'): void
}

const props = withDefaults(defineProps<Props>(), {
  title: '',
  subtitle: '',
  width: '',
  closeOnEscape: true,
  closeOnClickOutside: true,
  showCloseButton: true,
  closeLabel: 'Close',
  zIndex: 50
})

const emit = defineEmits<Emits>()

const zIndexStyle = computed(() => ({ zIndex: props.zIndex }))

const widthClass = computed(() => props.width || 'w-full md:w-[560px] md:max-w-[92vw]')

const handleOverlayClick = () => {
  if (props.closeOnClickOutside) {
    emit('close')
  }
}

const handleEscape = (event: KeyboardEvent) => {
  if (
    props.show &&
    props.closeOnEscape &&
    event.key === 'Escape' &&
    [...openDrawers].pop() === drawerId
  ) {
    emit('close')
  }
}

const updateScrollLock = (isOpen: boolean) => {
  if (typeof document === 'undefined') return
  if (isOpen) openDrawers.add(drawerId)
  else openDrawers.delete(drawerId)
  document.body.classList.toggle('drawer-open', openDrawers.size > 0)
}

watch(
  () => props.show,
  async (isOpen) => {
    if (isOpen) {
      if (typeof document !== 'undefined') {
        previousActiveElement = document.activeElement as HTMLElement | null
      }
      updateScrollLock(true)

      await nextTick()
      if (bodyRef.value) {
        bodyRef.value.scrollTop = 0
      }
      if (panelRef.value) {
        const firstFocusable = panelRef.value.querySelector<HTMLElement>(
          'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])'
        )
        ;(firstFocusable ?? panelRef.value).focus()
      }
    } else {
      updateScrollLock(false)
      if (previousActiveElement && typeof previousActiveElement.focus === 'function') {
        previousActiveElement.focus()
      }
      previousActiveElement = null
    }
  },
  { immediate: true }
)

onMounted(() => {
  document.addEventListener('keydown', handleEscape)
})

onUnmounted(() => {
  document.removeEventListener('keydown', handleEscape)
  updateScrollLock(false)
})
</script>

<style>
body.drawer-open {
  overflow: hidden;
}
</style>

<style scoped>
.drawer-enter-active {
  transition: opacity 250ms ease-out;
}

.drawer-leave-active {
  transition: opacity 200ms ease-in;
}

.drawer-enter-from,
.drawer-leave-to {
  opacity: 0;
}

.drawer-enter-active .drawer-panel {
  transition: transform 250ms cubic-bezier(0.2, 0.9, 0.2, 1);
}

.drawer-leave-active .drawer-panel {
  transition: transform 200ms ease-in;
}

.drawer-enter-from .drawer-panel,
.drawer-leave-to .drawer-panel {
  transform: translateX(100%);
}

.drawer-enter-to .drawer-panel,
.drawer-leave-from .drawer-panel {
  transform: translateX(0);
}

@media (prefers-reduced-motion: reduce) {
  .drawer-enter-active,
  .drawer-leave-active,
  .drawer-enter-active .drawer-panel,
  .drawer-leave-active .drawer-panel {
    transition-duration: 0.01ms !important;
    transition-delay: 0ms !important;
  }

  .drawer-enter-from .drawer-panel,
  .drawer-leave-to .drawer-panel {
    transform: none !important;
  }
}
</style>
