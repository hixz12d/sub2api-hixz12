<template>
  <section
    v-if="hasContent"
    class="rounded-none bg-surface-tile text-ink"
    :aria-label="t('modelPlaza.poolBar.title')"
  >
    <!-- Unauthorized / Preview Group Banner (never exposes numeric group_id) -->
    <div
      v-if="isUnauthorized"
      class="flex flex-col gap-2 border-l-2 border-warn bg-surface-tile-2/70 px-4 py-3 sm:flex-row sm:items-center sm:justify-between md:px-5"
      :class="hasDescription ? 'border-b border-rule' : ''"
      role="status"
    >
      <div class="flex min-w-0 items-start gap-2.5">
        <Icon
          name="lock"
          size="sm"
          class="mt-0.5 h-4 w-4 shrink-0 text-warn"
          aria-hidden="true"
        />
        <div class="min-w-0 space-y-0.5">
          <div class="flex flex-wrap items-center gap-2">
            <span class="text-xs font-semibold text-ink sm:text-[13px]">
              {{ t('modelPlaza.poolBar.unauthorizedTitle') }}
            </span>
            <span class="inline-flex items-center bg-warn/15 px-2 py-0.5 text-[11px] font-semibold text-warn">
              {{ showcaseBadgeText }}
            </span>
          </div>
          <p class="text-xs leading-relaxed text-ink-2">
            {{ t('modelPlaza.poolBar.unauthorizedHint') }}
          </p>
        </div>
      </div>

      <span class="inline-flex shrink-0 items-center gap-1.5 self-start bg-surface px-2.5 py-1 text-xs font-medium text-ink-2 sm:self-center">
        <Icon name="infoCircle" size="xs" class="h-3.5 w-3.5 text-warn" aria-hidden="true" />
        <span>{{ t('modelPlaza.badges.contactAdmin') }}</span>
      </span>
    </div>

    <!-- Group Description + Pool Status Row -->
    <div
      v-if="hasDescription"
      class="flex flex-col gap-3 px-4 py-3 md:flex-row md:items-center md:justify-between md:gap-5 md:px-5"
    >
      <!-- Left: Optional showcase badge + group description -->
      <div class="flex min-w-0 flex-1 flex-wrap items-start gap-2">
        <span
          v-if="!isUnauthorized && group.showcase_badge"
          class="inline-flex shrink-0 items-center bg-surface-tile-2 px-1.5 py-0.5 text-[11px] font-medium text-ink-2"
        >
          {{ group.showcase_badge }}
        </span>
        <p
          class="min-w-0 flex-1 whitespace-pre-line break-words text-xs leading-relaxed text-ink-2"
        >
          {{ trimmedDescription }}
        </p>
      </div>

      <!-- Right: Pool 24h SLA + 12-slot heartbeat + last probe relative time (desktop md+ only) -->
      <div
        v-if="group.pool_status"
        class="hidden shrink-0 flex-wrap items-center gap-x-4 gap-y-2 md:flex"
      >
        <div class="flex items-center gap-2 text-xs">
          <span class="whitespace-nowrap text-ink-3">
            {{ t('modelPlaza.poolBar.sla24h') }}
          </span>
          <span
            class="inline-flex items-center gap-1 font-semibold tabular-nums"
            :class="poolSla.valid ? plazaToneTextClass(poolSla.tone) : 'text-ink-3'"
          >
            <Icon
              v-if="poolSla.valid"
              :name="poolSlaIcon"
              size="xs"
              class="h-3.5 w-3.5 shrink-0"
              aria-hidden="true"
            />
            <span>{{ poolSla.text }}</span>
          </span>
        </div>

        <div class="w-[120px] shrink-0 sm:w-[144px]">
          <PlazaHeartbeatBar
            :slots="group.pool_status.heartbeat_12 ?? []"
            variant="compact"
            class="!h-[14px]"
          />
        </div>

        <span class="whitespace-nowrap text-xs text-ink-3 tabular-nums">
          {{ lastProbeLabel }}
        </span>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import PlazaHeartbeatBar from './PlazaHeartbeatBar.vue'
import {
  formatRelativeUpdatedAt,
  formatSlaPercent,
  plazaToneTextClass
} from '@/utils/plazaFormat'
import type { ModelPlazaGroup } from '@/api/modelPlaza'

interface Props {
  group: ModelPlazaGroup
  orderedGroupIds?: number[]
}

const props = withDefaults(defineProps<Props>(), {
  orderedGroupIds: () => []
})

const { t } = useI18n()

const isUnauthorized = computed(() => props.group.is_authorized === false)
const trimmedDescription = computed(() => props.group.description?.trim() ?? '')
const hasDescription = computed(() => trimmedDescription.value.length > 0)
const hasContent = computed(() => isUnauthorized.value || hasDescription.value)

const showcaseBadgeText = computed(
  () => props.group.showcase_badge?.trim() || t('modelPlaza.badges.showcase')
)

const poolSla = computed(() =>
  formatSlaPercent(
    props.group.pool_status?.sla_24h,
    props.group.pool_status?.sla_state,
    2
  )
)

const poolSlaIcon = computed(() => {
  if (poolSla.value.tone === 'ok') return 'checkCircle' as const
  if (poolSla.value.tone === 'warn') return 'exclamationTriangle' as const
  return 'xCircle' as const
})

const lastProbeLabel = computed(() => {
  const rel = formatRelativeUpdatedAt(props.group.pool_status?.last_probe_at, t)
  if (!rel) return t('modelPlaza.poolBar.noProbeYet')
  return t('modelPlaza.poolBar.lastProbeAt', { time: rel })
})
</script>
