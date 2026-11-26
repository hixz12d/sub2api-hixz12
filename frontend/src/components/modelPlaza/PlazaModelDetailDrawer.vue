<template>
  <BaseDrawer
    :show="show"
    width="w-full md:w-[660px] md:max-w-[94vw]"
    :close-label="t('modelPlaza.common.close')"
    @close="emit('close')"
  >
    <!-- 1. Header bar: ModelIcon + mono model ID + copy ID + share direct link -->
    <template #title>
      <div class="flex min-w-0 items-start gap-2.5">
        <ModelIcon
          v-if="modelName"
          :model="modelName"
          size="24px"
          class="mt-0.5 shrink-0"
        />
        <div class="min-w-0 flex-1">
          <h2 class="break-all font-mono text-base font-semibold leading-snug text-ink">
            {{ modelName || t('modelPlaza.drawer.titleFallback') }}
          </h2>
          <div class="mt-1 flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-ink-3">
            <span
              v-if="displayName && displayName !== modelName"
              class="text-ink-2"
            >
              {{ displayName }}
            </span>
            <span
              v-if="effectiveGroupName"
              class="inline-flex items-center gap-1.5 text-ink-2"
            >
              <i class="inline-block h-2.5 w-2.5 shrink-0" :class="groupSeriesClass(effectiveGroupName)" />
              <span>{{ effectiveGroupName }}</span>
            </span>
            <span v-if="updatedLabel">
              {{ t('modelPlaza.drawer.updatedAt', { time: updatedLabel }) }}
            </span>
          </div>
        </div>
      </div>
    </template>

    <template #header-actions>
      <button
        v-if="modelName"
        type="button"
        class="btn btn-sm h-8 px-2.5"
        :class="copiedId ? 'bg-ok text-white hover:bg-ok' : 'btn-secondary'"
        :title="t('modelPlaza.drawer.copyModelId')"
        @click="handleCopyModelId"
      >
        <Icon :name="copiedId ? 'check' : 'copy'" size="xs" />
        <span class="hidden sm:inline">
          {{ copiedId ? t('modelPlaza.drawer.copiedModelId') : t('modelPlaza.drawer.copyModelId') }}
        </span>
      </button>
      <button
        v-if="modelName"
        type="button"
        class="btn btn-sm h-8 px-2.5"
        :class="copiedShare ? 'bg-ok text-white hover:bg-ok' : 'btn-secondary'"
        :title="t('modelPlaza.share.tooltip')"
        @click="handleCopyShareLink"
      >
        <Icon :name="copiedShare ? 'check' : 'link'" size="xs" />
        <span class="hidden sm:inline">
          {{ copiedShare ? t('modelPlaza.share.copiedLink') : t('modelPlaza.share.copyLink') }}
        </span>
      </button>
    </template>

    <!-- Drawer body: 5 flat sections -->
    <div v-if="modelName" class="space-y-6 pb-4">
      <!-- Detail loading / degraded fallback status strip -->
      <div
        v-if="loading && !detail"
        class="flex items-center gap-2 bg-surface-tile px-3.5 py-2.5 text-xs text-ink-2"
      >
        <span class="spinner h-3.5 w-3.5 text-money-text" />
        <span>{{ t('modelPlaza.drawer.loadingDetail') }}</span>
      </div>

      <div
        v-else-if="loadError"
        class="flex flex-wrap items-center justify-between gap-2 bg-surface-tile px-3.5 py-2.5 text-xs"
      >
        <div class="flex items-center gap-2 text-warn">
          <Icon name="exclamationTriangle" size="sm" class="shrink-0" />
          <span>{{ t('modelPlaza.drawer.loadDetailFailed') }}</span>
        </div>
        <button
          type="button"
          class="btn btn-sm btn-secondary h-7 px-2.5"
          @click="retryLoad"
        >
          <Icon name="refresh" size="xs" />
          <span>{{ t('modelPlaza.drawer.retryLoad') }}</span>
        </button>
      </div>

      <!-- 2. Overview & Capabilities -->
      <section class="space-y-3">
        <h3 class="text-sm font-semibold text-ink">
          {{ t('modelPlaza.drawer.sectionOverview') }}
        </h3>

        <p class="text-sm leading-relaxed text-ink-2">
          {{ overviewDescription }}
        </p>

        <div class="grid grid-cols-2 gap-tile-gap sm:grid-cols-4">
          <div class="tile p-3">
            <div class="text-xs text-ink-3">{{ t('modelPlaza.drawer.contextWindow') }}</div>
            <div class="mt-1 font-mono text-sm font-semibold tabular-nums text-ink">
              {{ contextWindowText }}
            </div>
          </div>
          <div class="tile p-3">
            <div class="text-xs text-ink-3">{{ t('modelPlaza.drawer.maxOutput') }}</div>
            <div class="mt-1 font-mono text-sm font-semibold tabular-nums text-ink">
              {{ maxOutputText }}
            </div>
          </div>
          <div class="tile p-3">
            <div class="text-xs text-ink-3">{{ t('modelPlaza.drawer.vendor') }}</div>
            <div class="mt-1 truncate text-sm font-medium text-ink">
              {{ vendorText }}
            </div>
          </div>
          <div class="tile p-3">
            <div class="text-xs text-ink-3">{{ t('modelPlaza.drawer.platform') }}</div>
            <div class="mt-1 truncate text-sm font-medium text-ink">
              {{ platformText }}
            </div>
          </div>
        </div>

        <div class="space-y-1.5">
          <div class="text-xs text-ink-3">
            {{ t('modelPlaza.drawer.capabilitiesLabel') }}
          </div>
          <div v-if="capabilityLabels.length > 0" class="flex flex-wrap gap-1.5">
            <span
              v-for="cap in capabilityLabels"
              :key="cap.key"
              class="badge badge-gray"
            >
              {{ cap.label }}
            </span>
          </div>
          <p v-else class="text-xs text-ink-3">
            {{ t('modelPlaza.drawer.noCapabilities') }}
          </p>
        </div>
      </section>

      <!-- 3. Performance, Reliability & Consistency -->
      <section class="space-y-3 border-t border-rule pt-5">
        <h3 class="text-sm font-semibold text-ink">
          {{ t('modelPlaza.drawer.sectionPerformance') }}
        </h3>

        <PlazaDetailTrends
          :windows="detail?.windows ?? null"
          :heartbeat72="detail?.heartbeat_72 ?? []"
          :trend24h="detail?.trend_24h ?? []"
          :loading="loading"
        />

        <!-- Model fidelity & silent-downgrade circuit breaker card -->
        <div class="tile p-4">
          <div class="flex flex-wrap items-center justify-between gap-2">
            <div class="flex items-center gap-1.5 text-xs font-semibold text-ink">
              <Icon name="shield" size="sm" :class="consistencyBadge.toneClass" />
              <span>{{ t('modelPlaza.consistency.cardTitle') }}</span>
            </div>
            <span
              class="inline-flex items-center gap-1 text-xs font-medium"
              :class="consistencyBadge.toneClass"
            >
              <Icon :name="consistencyBadge.icon" size="xs" />
              <span>{{ consistencyBadge.label }}</span>
            </span>
          </div>

          <dl class="mt-3 space-y-2 border-t border-rule pt-2.5 text-xs">
            <div class="flex flex-wrap items-baseline justify-between gap-2">
              <dt class="text-ink-3">{{ t('modelPlaza.consistency.rateLabel') }}</dt>
              <dd
                class="font-mono font-medium tabular-nums"
                :class="plazaToneTextClass(consistencyMetric.tone)"
              >
                {{ consistencyMetric.text }}
              </dd>
            </div>
            <p v-if="consistencyState === 'low_sample'" class="text-[11px] text-ink-3">
              {{ t('modelPlaza.consistency.lowSampleDesc') }}
            </p>

            <div class="grid grid-cols-1 gap-0.5 pt-1">
              <dt class="text-ink-3">{{ t('modelPlaza.consistency.methodLabel') }}</dt>
              <dd class="leading-relaxed text-ink-2">
                {{ t('modelPlaza.consistency.methodDesc') }}
              </dd>
            </div>

            <div class="grid grid-cols-1 gap-0.5 pt-1">
              <dt class="text-ink-3">{{ t('modelPlaza.consistency.autoPauseLabel') }}</dt>
              <dd class="space-y-0.5">
                <div
                  class="inline-flex items-center gap-1 font-medium"
                  :class="autoPauseEnabled ? 'text-ok' : 'text-ink-2'"
                >
                  <Icon
                    :name="autoPauseEnabled ? 'checkCircle' : 'infoCircle'"
                    size="xs"
                  />
                  <span>
                    {{
                      autoPauseEnabled
                        ? t('modelPlaza.consistency.autoPauseActive')
                        : t('modelPlaza.consistency.autoPauseInactive')
                    }}
                  </span>
                </div>
                <p v-if="autoPauseEnabled" class="leading-relaxed text-ink-3">
                  {{ t('modelPlaza.consistency.autoPauseDesc') }}
                </p>
              </dd>
            </div>

            <div class="grid grid-cols-1 gap-0.5 pt-1">
              <dt class="text-ink-3">{{ t('modelPlaza.consistency.capabilityProbeLabel') }}</dt>
              <dd class="text-ink-3">
                {{ t('modelPlaza.consistency.capabilityProbeReserved') }}
              </dd>
            </div>
          </dl>
        </div>
      </section>

      <!-- 4. Full Pricing, Official Comparison, Tiers, Time/Peak, Cache Savings & Group Comparison -->
      <section class="space-y-4 border-t border-rule pt-5">
        <div class="flex flex-wrap items-baseline justify-between gap-2">
          <h3 class="text-sm font-semibold text-ink">
            {{ t('modelPlaza.drawer.sectionPricing') }}
          </h3>
          <div class="flex items-center gap-2 text-xs text-ink-3">
            <span v-if="effectiveBillingMode === BILLING_MODE_TOKEN">
              {{ t('modelPlaza.table.unitPerMillion') }}
            </span>
            <span class="font-mono tabular-nums text-ink-2">
              <span v-if="hasCustomRate" class="mr-1 text-ink-3 line-through">
                {{ formatRate(defaultGroupRate) }}×
              </span>
              <span class="font-semibold text-money-text">
                {{ formatRate(activeEffectiveRate) }}×
              </span>
            </span>
          </div>
        </div>

        <!-- Paid vs Official table -->
        <div v-if="!effectivePricing && !effectiveOfficial" class="tile p-4 text-xs text-ink-3">
          {{ t('modelPlaza.detail.noPricing') }}
        </div>

        <div v-else-if="effectiveBillingMode === BILLING_MODE_TOKEN" class="overflow-x-auto">
          <table class="w-full border-separate border-spacing-y-1 text-xs tabular-nums">
            <thead>
              <tr class="text-ink-3">
                <th class="px-3 py-1.5 text-left font-normal">
                  {{ t('modelPlaza.drawer.pricingItem') }}
                </th>
                <th class="px-3 py-1.5 text-right font-normal">
                  {{ t('modelPlaza.drawer.pricingPaid') }}
                </th>
                <th class="px-3 py-1.5 text-right font-normal">
                  {{ t('modelPlaza.drawer.pricingOfficial') }}
                </th>
                <th class="px-3 py-1.5 text-right font-normal">
                  {{ t('modelPlaza.drawer.pricingSavings') }}
                </th>
              </tr>
            </thead>
            <tbody>
              <tr
                v-for="row in tokenPriceRows"
                :key="row.key"
                class="bg-surface-tile transition-colors duration-150 hover:bg-surface-tile-2"
              >
                <td class="px-3 py-2 text-ink-2">{{ row.label }}</td>
                <td class="px-3 py-2 text-right font-mono font-semibold text-ink">
                  {{ row.paidText }}
                </td>
                <td class="px-3 py-2 text-right font-mono text-ink-3">
                  {{ row.officialText }}
                </td>
                <td class="px-3 py-2 text-right font-mono">
                  <span v-if="row.savingsPercent != null" class="font-medium text-ok">
                    {{ t('modelPlaza.badges.savings', { percent: row.savingsPercent }) }}
                  </span>
                  <span v-else class="text-ink-3">{{ PLAZA_EMPTY_TEXT }}</span>
                </td>
              </tr>
            </tbody>
          </table>

          <!-- Reasoning effort multipliers (if configured) -->
          <div
            v-if="reasoningMultipliers.length > 0"
            class="mt-2 flex flex-wrap gap-1.5"
          >
            <span
              v-for="[effort, multiplier] in reasoningMultipliers"
              :key="effort"
              class="badge badge-gray font-mono"
              :title="t('modelPlaza.table.reasoningMultiplierHint', { effort, multiplier })"
            >
              {{ t('modelPlaza.table.reasoningMultiplierBadge', { effort, multiplier }) }}
            </span>
          </div>
        </div>

        <!-- Non-token pricing (per request / image / video) -->
        <div v-else class="tile p-4">
          <div class="flex flex-wrap items-center justify-between gap-2 text-xs">
            <span class="text-ink-2">{{ nonTokenItemLabel }}</span>
            <div v-if="requestIntervals.length > 0" class="flex flex-wrap gap-1.5">
              <span
                v-for="(iv, idx) in requestIntervals"
                :key="idx"
                class="badge badge-gray font-mono tabular-nums"
              >
                <span class="font-sans text-ink-3">{{ tierLabel(iv) }}</span>
                <span class="text-ink">
                  {{ formatRequestPrice(iv.per_request_price) }}{{ perUnitSuffix }}
                </span>
              </span>
            </div>
            <span
              v-else-if="effectivePricing?.per_request_price != null"
              class="font-mono text-sm font-semibold tabular-nums text-ink"
            >
              {{ formatRequestPrice(effectivePricing.per_request_price) }}
              <span class="font-sans text-xs font-normal text-ink-3">{{ perUnitSuffix }}</span>
            </span>
            <span v-else class="text-ink-3">{{ PLAZA_EMPTY_TEXT }}</span>
          </div>
        </div>

        <!-- Long-context disabled note -->
        <div
          v-if="showLongContextDisabledNote"
          class="flex items-start gap-2 bg-surface-tile px-3.5 py-2.5 text-xs text-ink-2"
        >
          <Icon name="infoCircle" size="sm" class="mt-0.5 shrink-0 text-ink-3" />
          <span>{{ t('modelPlaza.detail.longContextDisabledNote') }}</span>
        </div>

        <!-- Long-context tiered pricing table -->
        <div v-if="resolvedTokenIntervals.length > 1" class="space-y-2">
          <div class="flex flex-wrap items-center justify-between gap-2">
            <h4 class="text-xs font-semibold text-ink-2">
              {{ t('modelPlaza.drawer.tieredTitle') }}
            </h4>
            <span class="text-[11px] text-ink-3">
              {{
                effectiveLongContextBasis === 'marginal'
                  ? t('modelPlaza.table.tierHintMarginal')
                  : t('modelPlaza.table.tierHint')
              }}
            </span>
          </div>
          <div class="overflow-x-auto">
            <table class="w-full border-separate border-spacing-y-1 text-xs tabular-nums">
              <thead>
                <tr class="text-ink-3">
                  <th class="px-3 py-1 text-left font-normal">
                    {{ t('modelPlaza.drawer.tieredRange') }}
                  </th>
                  <th class="px-3 py-1 text-right font-normal">
                    {{ t('modelPlaza.drawer.itemInput') }}
                  </th>
                  <th class="px-3 py-1 text-right font-normal">
                    {{ t('modelPlaza.drawer.itemOutput') }}
                  </th>
                  <th v-if="hasTierCache" class="px-3 py-1 text-right font-normal">
                    {{ t('modelPlaza.drawer.itemCacheRead') }}
                  </th>
                </tr>
              </thead>
              <tbody>
                <tr
                  v-for="(iv, idx) in resolvedTokenIntervals"
                  :key="idx"
                  class="bg-surface-tile"
                >
                  <td class="px-3 py-2 font-mono text-ink-2">{{ tierLabel(iv) }}</td>
                  <td class="px-3 py-2 text-right font-mono font-semibold text-ink">
                    {{ formatTokenPrice(iv.input_price, activeEffectiveRate) }}
                  </td>
                  <td class="px-3 py-2 text-right font-mono font-semibold text-ink">
                    {{ formatTokenPrice(iv.output_price, activeEffectiveRate) }}
                  </td>
                  <td v-if="hasTierCache" class="px-3 py-2 text-right font-mono text-ink-2">
                    {{ formatTokenPrice(iv.cache_read_price, activeEffectiveRate) }}
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>

        <!-- Time-of-day pricing schedule -->
        <div v-if="timePricingPeriods.length > 0" class="space-y-2">
          <div class="flex flex-wrap items-center justify-between gap-2">
            <h4 class="text-xs font-semibold text-ink-2">
              {{ t('modelPlaza.drawer.timePricingTitle') }}
            </h4>
            <div class="flex items-center gap-1.5 text-[11px] text-ink-3">
              <span v-if="effectiveTimePricing?.weekdays_only" class="badge badge-gray">
                {{ t('modelPlaza.table.timePricingWeekdays') }}
              </span>
              <span v-if="effectiveTimePricing?.timezone">
                {{ t('modelPlaza.drawer.timePricingTimezone', { timezone: effectiveTimePricing.timezone }) }}
              </span>
            </div>
          </div>
          <div class="overflow-x-auto">
            <table class="w-full border-separate border-spacing-y-1 text-xs tabular-nums">
              <thead>
                <tr class="text-ink-3">
                  <th class="px-3 py-1 text-left font-normal">
                    {{ t('modelPlaza.drawer.timePricingWindow') }}
                  </th>
                  <th class="px-3 py-1 text-right font-normal">
                    {{ t('modelPlaza.drawer.timePricingMultiplier') }}
                  </th>
                  <th
                    v-if="effectiveBillingMode === BILLING_MODE_TOKEN"
                    class="px-3 py-1 text-right font-normal"
                  >
                    {{ t('modelPlaza.drawer.groupComparePrice') }}
                  </th>
                </tr>
              </thead>
              <tbody>
                <tr
                  v-for="(period, idx) in timePricingPeriods"
                  :key="idx"
                  class="bg-surface-tile"
                >
                  <td class="px-3 py-2 font-mono text-ink-2">
                    {{ formatTimeWindow(period) }}
                  </td>
                  <td class="px-3 py-2 text-right font-mono font-semibold text-money-text">
                    ×{{ formatRate(period.multiplier) }}
                  </td>
                  <td
                    v-if="effectiveBillingMode === BILLING_MODE_TOKEN"
                    class="px-3 py-2 text-right font-mono text-ink"
                  >
                    {{
                      formatTokenPrice(baseTokenPrices.input_price, activeEffectiveRate * period.multiplier)
                    }}
                    /
                    {{
                      formatTokenPrice(baseTokenPrices.output_price, activeEffectiveRate * period.multiplier)
                    }}
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>

        <!-- Peak-hour multiplier note -->
        <div v-if="peakNote" class="tile p-3.5">
          <div class="text-xs font-semibold text-ink-2">
            {{ t('modelPlaza.drawer.peakPricingTitle') }}
          </div>
          <div class="mt-1.5 flex items-center gap-1.5 text-xs text-warn">
            <Icon name="clock" size="xs" class="shrink-0" />
            <span>{{ peakNote }}</span>
          </div>
        </div>

        <!-- 1M context live cache savings simulator -->
        <div v-if="cacheSavingsSim" class="tile p-4">
          <div class="flex flex-wrap items-center justify-between gap-2">
            <span class="text-xs font-semibold text-ink">
              {{ t('modelPlaza.drawer.savingsSimTitle') }}
            </span>
            <span class="font-mono text-xs font-semibold text-ok">
              {{ t('modelPlaza.drawer.savingsSimSaved', { percent: cacheSavingsSim.savedPercent }) }}
            </span>
          </div>
          <p class="mt-1 text-[11px] text-ink-3">
            {{
              t('modelPlaza.drawer.savingsSimDesc', {
                cacheRate: cacheSavingsSim.cacheRateText,
                multiplier: cacheSavingsSim.multiplierText
              })
            }}
          </p>
          <div class="mt-3 grid grid-cols-2 gap-3 text-xs">
            <div>
              <div class="text-ink-3">{{ t('modelPlaza.drawer.savingsSimUncached') }}</div>
              <div class="mt-0.5 font-mono text-sm tabular-nums text-ink-3 line-through">
                {{ cacheSavingsSim.uncachedText }}
              </div>
            </div>
            <div class="text-right">
              <div class="text-ink-3">{{ t('modelPlaza.drawer.savingsSimWithCache') }}</div>
              <div class="mt-0.5 font-mono text-sm font-semibold tabular-nums text-ok">
                {{ cacheSavingsSim.cachedText }}
              </div>
            </div>
          </div>
          <div class="mt-2.5 h-2 w-full bg-surface-tile-2">
            <div
              class="h-full bg-ok"
              :style="{ width: `${cacheSavingsSim.barPercent}%` }"
            />
          </div>
        </div>

        <!-- Cross-group comparison table (never displays numeric group ID) -->
        <div class="space-y-2">
          <h4 class="text-xs font-semibold text-ink-2">
            {{ t('modelPlaza.drawer.groupCompareTitle') }}
          </h4>
          <div v-if="comparisonRows.length > 0" class="overflow-x-auto">
            <table class="w-full border-separate border-spacing-y-1 text-xs tabular-nums">
              <thead>
                <tr class="text-ink-3">
                  <th class="px-3 py-1 text-left font-normal">
                    {{ t('modelPlaza.drawer.groupCompareGroup') }}
                  </th>
                  <th class="px-3 py-1 text-right font-normal">
                    {{ t('modelPlaza.drawer.groupCompareRate') }}
                  </th>
                  <th class="px-3 py-1 text-right font-normal">
                    {{ t('modelPlaza.drawer.groupComparePrice') }}
                  </th>
                  <th class="px-3 py-1 text-right font-normal">
                    {{ t('modelPlaza.drawer.groupCompareCache') }}
                  </th>
                </tr>
              </thead>
              <tbody>
                <tr
                  v-for="row in comparisonRows"
                  :key="row.groupName"
                  :tabindex="row.isCurrent ? undefined : 0"
                  :role="row.isCurrent ? undefined : 'button'"
                  :aria-current="row.isCurrent ? 'true' : undefined"
                  :title="row.isCurrent ? undefined : t('modelPlaza.drawer.groupCompareSwitchHint')"
                  class="transition-colors duration-150"
                  :class="
                    row.isCurrent
                      ? 'bg-surface-tile-2'
                      : 'cursor-pointer bg-surface-tile hover:bg-surface-tile-2'
                  "
                  @click="!row.isCurrent && emit('switch-group', row.groupName)"
                  @keydown.enter.prevent="!row.isCurrent && emit('switch-group', row.groupName)"
                  @keydown.space.prevent="!row.isCurrent && emit('switch-group', row.groupName)"
                >
                  <td
                    class="px-3 py-2"
                    :class="row.isCurrent ? 'border-l-2 border-money' : ''"
                  >
                    <div class="flex flex-wrap items-center gap-1.5">
                      <i
                        class="inline-block h-2.5 w-2.5 shrink-0"
                        :class="groupSeriesClass(row.groupName)"
                      />
                      <span class="font-medium text-ink">{{ row.groupName }}</span>
                      <span v-if="row.isCurrent" class="badge badge-primary">
                        {{ t('modelPlaza.drawer.groupCompareCurrent') }}
                      </span>
                      <span v-if="row.isExclusive" class="badge badge-gray">
                        {{ t('modelPlaza.badges.exclusive') }}
                      </span>
                      <span
                        v-if="row.subscriptionType === 'subscription'"
                        class="badge badge-gray"
                      >
                        {{ t('modelPlaza.badges.subscription') }}
                      </span>
                    </div>
                  </td>
                  <td class="px-3 py-2 text-right font-mono font-semibold text-ink">
                    {{ formatRate(row.effectiveRate) }}×
                  </td>
                  <td class="px-3 py-2 text-right font-mono text-ink-2">
                    {{ row.priceText }}
                  </td>
                  <td
                    class="px-3 py-2 text-right font-mono"
                    :class="plazaToneTextClass(row.cache.tone)"
                  >
                    {{ row.cache.text }}
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
          <p v-if="comparisonRows.length <= 1" class="text-xs text-ink-3">
            {{ t('modelPlaza.drawer.groupCompareEmpty') }}
          </p>
        </div>
      </section>

      <!-- 5. Quick Connect -->
      <section class="space-y-3 border-t border-rule pt-5">
        <h3 class="text-sm font-semibold text-ink">
          {{ t('modelPlaza.drawer.sectionQuickConnect') }}
        </h3>

        <PlazaQuickConnect
          :platform="effectivePlatform"
          :model-name="modelName"
          :group-name="effectiveGroupName"
          :group-id="currentGroup?.id ?? null"
          :active="show"
        />
      </section>
    </div>
  </BaseDrawer>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDrawer from '@/components/common/BaseDrawer.vue'
import ModelIcon from '@/components/common/ModelIcon.vue'
import Icon from '@/components/icons/Icon.vue'
import PlazaDetailTrends from './PlazaDetailTrends.vue'
import PlazaQuickConnect from './PlazaQuickConnect.vue'
import {
  getModelPlazaDetail,
  type ModelPlazaDetailResponse,
  type ModelPlazaGroup,
  type PlazaConsistencyState,
  type PlazaGroupComparison,
  type PlazaModel,
  type PlazaTimePricingPeriod
} from '@/api/modelPlaza'
import type { UserPricingInterval, UserSupportedModelPricing } from '@/api/channels'
import {
  PLAZA_EMPTY_TEXT,
  buildFallbackSummary,
  computeOfficialSavingsPercent,
  formatCacheRate,
  formatConsistencyPercent,
  formatContextWindow,
  formatRefMultiplier,
  formatRelativeUpdatedAt,
  plazaToneTextClass,
  type PlazaFormattedMetric
} from '@/utils/plazaFormat'
import { formatScaled, resolveIntervalPrices } from '@/utils/pricing'
import { formatPeakRateWindow, hasPeakRate, serverTimezoneLabel } from '@/utils/peak-rate'
import { platformLabel } from '@/utils/platformColors'
import {
  BILLING_MODE_IMAGE,
  BILLING_MODE_TOKEN,
  BILLING_MODE_VIDEO,
  REASONING_EFFORT_LEVELS,
  type BillingMode
} from '@/constants/channel'
import { useAppStore } from '@/stores/app'
import { useClipboard } from '@/composables/useClipboard'

const props = defineProps<{
  show: boolean
  model: PlazaModel | null
  groupName: string
  allGroups: ModelPlazaGroup[]
}>()

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'switch-group', groupName: string): void
}>()

const { t } = useI18n()
const appStore = useAppStore()
const { copyToClipboard } = useClipboard()

const PER_MILLION = 1_000_000
const MIN_DECIMALS = 2

const SERIES_BG_CLASSES = [
  'bg-series-1',
  'bg-series-2',
  'bg-series-3',
  'bg-series-4',
  'bg-series-5'
] as const

const KNOWN_CAPABILITY_KEYS = new Set([
  'reasoning',
  'tools',
  'vision',
  'audio',
  'image',
  'video',
  'prompt_caching',
  'web_search',
  'long_context',
  'structured_outputs',
  'code',
  'embedding',
  'realtime'
])

const KNOWN_VENDOR_KEYS = new Set([
  'openai',
  'anthropic',
  'google',
  'xai',
  'deepseek',
  'qwen',
  'zhipu',
  'meta',
  'mistral',
  'moonshot',
  'minimax',
  'doubao',
  'cohere',
  'other'
])

// Lazy-loaded detail state with AbortController race protection
const detail = ref<ModelPlazaDetailResponse | null>(null)
const loading = ref(false)
const loadError = ref(false)
const copiedId = ref(false)
const copiedShare = ref(false)

let abortController: AbortController | null = null
let copiedIdTimer: ReturnType<typeof setTimeout> | null = null
let copiedShareTimer: ReturnType<typeof setTimeout> | null = null

const modelName = computed(() => props.model?.name ?? detail.value?.model ?? '')
const effectiveGroupName = computed(() => props.groupName || detail.value?.group_name || '')
const effectivePlatform = computed(
  () => detail.value?.platform || props.model?.platform || currentGroup.value?.platform || ''
)

const currentGroup = computed<ModelPlazaGroup | null>(
  () => props.allGroups.find((g) => g.name === effectiveGroupName.value) ?? null
)

function isAbortError(err: unknown): boolean {
  if (!err || typeof err !== 'object') return false
  const candidate = err as { name?: string; code?: string }
  return candidate.name === 'CanceledError' || candidate.name === 'AbortError' || candidate.code === 'ERR_CANCELED'
}

async function fetchDetail(targetModel: string, targetGroup: string) {
  abortController?.abort()
  const controller = new AbortController()
  abortController = controller

  detail.value = null
  loadError.value = false
  loading.value = true

  try {
    const res = await getModelPlazaDetail(
      { model: targetModel, group: targetGroup || undefined },
      { signal: controller.signal }
    )
    if (controller.signal.aborted) return
    detail.value = res
  } catch (err) {
    if (controller.signal.aborted || isAbortError(err)) return
    loadError.value = true
  } finally {
    if (!controller.signal.aborted) {
      loading.value = false
    }
  }
}

watch(
  () => [props.show, props.model?.name ?? '', props.groupName] as const,
  ([isOpen, nextModel, nextGroup]) => {
    if (!isOpen || !nextModel) {
      abortController?.abort()
      abortController = null
      loading.value = false
      return
    }
    void fetchDetail(nextModel, nextGroup)
  },
  { immediate: true }
)

function retryLoad() {
  if (!modelName.value) return
  void fetchDetail(modelName.value, effectiveGroupName.value)
}

function groupSeriesClass(name: string): string {
  const idx = props.allGroups.findIndex((g) => g.name === name)
  if (idx >= 0) {
    return SERIES_BG_CLASSES[idx % SERIES_BG_CLASSES.length]
  }
  let hash = 0
  for (let i = 0; i < name.length; i++) {
    hash = (hash * 31 + name.charCodeAt(i)) >>> 0
  }
  return SERIES_BG_CLASSES[hash % SERIES_BG_CLASSES.length]
}

// Header actions: Copy Model ID & Share Direct Link (no numeric ID)
async function handleCopyModelId() {
  if (!modelName.value) return
  const ok = await copyToClipboard(modelName.value, t('modelPlaza.drawer.copiedModelId'))
  if (ok) {
    copiedId.value = true
    if (copiedIdTimer) clearTimeout(copiedIdTimer)
    copiedIdTimer = setTimeout(() => {
      copiedId.value = false
    }, 2000)
  }
}

function buildDirectShareUrl(): string {
  const origin = typeof window !== 'undefined' ? window.location.origin : ''
  const queryParts: string[] = []
  if (effectiveGroupName.value) {
    queryParts.push(`group=${encodeURIComponent(effectiveGroupName.value)}`)
  }
  if (modelName.value) {
    queryParts.push(`model=${encodeURIComponent(modelName.value)}`)
  }
  return queryParts.length > 0
    ? `${origin}/model-plaza?${queryParts.join('&')}`
    : `${origin}/model-plaza`
}

async function handleCopyShareLink() {
  if (!modelName.value) return
  const url = buildDirectShareUrl()
  const ok = await copyToClipboard(url, t('modelPlaza.share.copiedLink'))
  if (ok) {
    copiedShare.value = true
    if (copiedShareTimer) clearTimeout(copiedShareTimer)
    copiedShareTimer = setTimeout(() => {
      copiedShare.value = false
    }, 2000)
  }
}

// Section 2: Metadata & Capabilities
const effectiveMetadata = computed(() => detail.value?.metadata ?? props.model?.metadata ?? null)
const displayName = computed(() => effectiveMetadata.value?.display_name?.trim() || '')
const updatedLabel = computed(() => formatRelativeUpdatedAt(detail.value?.updated_at, t))

const overviewDescription = computed(() => buildFallbackSummary(effectiveMetadata.value, t))

const contextWindowText = computed(
  () => formatContextWindow(effectiveMetadata.value?.context_window) ?? PLAZA_EMPTY_TEXT
)

const maxOutputText = computed(
  () => formatContextWindow(effectiveMetadata.value?.max_output_tokens) ?? PLAZA_EMPTY_TEXT
)

const vendorText = computed(() => {
  const raw = effectiveMetadata.value?.vendor?.trim().toLowerCase()
  if (!raw) return PLAZA_EMPTY_TEXT
  if (KNOWN_VENDOR_KEYS.has(raw)) {
    return t(`modelPlaza.vendors.${raw}`)
  }
  return effectiveMetadata.value?.vendor || PLAZA_EMPTY_TEXT
})

const platformText = computed(() =>
  effectivePlatform.value ? platformLabel(effectivePlatform.value) : PLAZA_EMPTY_TEXT
)

const capabilityLabels = computed(() => {
  const caps = effectiveMetadata.value?.capabilities ?? []
  const seen = new Set<string>()
  const result: Array<{ key: string; label: string }> = []
  for (const raw of caps) {
    const normalized = raw.trim().toLowerCase()
    if (!normalized || seen.has(normalized)) continue
    seen.add(normalized)
    result.push({
      key: normalized,
      label: KNOWN_CAPABILITY_KEYS.has(normalized)
        ? t(`modelPlaza.capabilities.${normalized}`)
        : raw.trim()
    })
  }
  return result
})

// Section 3: Model fidelity & protection card
const consistencyState = computed<PlazaConsistencyState>(
  () =>
    detail.value?.windows?.h24?.consistency_state ??
    props.model?.metrics?.consistency_state ??
    'no_data'
)

const consistencyRate = computed<number | null>(
  () =>
    detail.value?.windows?.h24?.consistency_rate ??
    props.model?.metrics?.consistency_rate ??
    null
)

const consistencyMetric = computed(() =>
  formatConsistencyPercent(consistencyRate.value, consistencyState.value)
)

const consistencyBadge = computed((): {
  label: string
  toneClass: string
  icon: 'checkCircle' | 'exclamationTriangle' | 'infoCircle'
} => {
  if (consistencyState.value === 'verified') {
    return {
      label: t('modelPlaza.consistency.verified'),
      toneClass: 'text-ok',
      icon: 'checkCircle'
    }
  }
  if (consistencyState.value === 'mismatch_detected') {
    return {
      label: t('modelPlaza.consistency.mismatch'),
      toneClass: 'text-warn',
      icon: 'exclamationTriangle'
    }
  }
  if (consistencyState.value === 'low_sample') {
    return {
      label: t('modelPlaza.consistency.lowSample'),
      toneClass: 'text-ink-3',
      icon: 'infoCircle'
    }
  }
  return {
    label: t('modelPlaza.consistency.noData'),
    toneClass: 'text-ink-3',
    icon: 'infoCircle'
  }
})

const autoPauseEnabled = computed(
  () =>
    detail.value?.protection?.auto_pause_enabled ??
    props.model?.metrics?.auto_pause_protected ??
    false
)

// Section 4: Pricing, Official Comparison, Tiers, Time/Peak, Savings Sim & Group Comparison
const effectivePricing = computed<UserSupportedModelPricing | null>(
  () => detail.value?.pricing ?? props.model?.pricing ?? null
)
const effectiveOfficial = computed(
  () => detail.value?.official_pricing ?? props.model?.official_pricing ?? null
)
const effectiveLongContextBasis = computed(
  () => detail.value?.long_context_basis ?? props.model?.long_context_basis
)
const effectiveTimePricing = computed(
  () => detail.value?.time_pricing ?? props.model?.time_pricing ?? null
)

const effectiveBillingMode = computed<BillingMode>(
  () => (effectivePricing.value?.billing_mode || BILLING_MODE_TOKEN) as BillingMode
)

const detailCurrentGroupComparison = computed<PlazaGroupComparison | null>(
  () =>
    detail.value?.group_comparisons?.find((g) => g.group_name === effectiveGroupName.value) ?? null
)

const defaultGroupRate = computed(() => currentGroup.value?.rate_multiplier ?? 1)

const activeEffectiveRate = computed(() => {
  if (currentGroup.value) {
    return currentGroup.value.user_rate_multiplier ?? currentGroup.value.rate_multiplier
  }
  return detailCurrentGroupComparison.value?.effective_rate ?? 1
})

const hasCustomRate = computed(
  () =>
    currentGroup.value?.user_rate_multiplier != null &&
    currentGroup.value.user_rate_multiplier !== currentGroup.value.rate_multiplier
)

function resolveNonTokenRate(group: ModelPlazaGroup | null, mode: BillingMode, fallbackRate: number): number {
  if (mode === BILLING_MODE_VIDEO && group?.video_rate_independent) {
    return Math.max(0, group.video_rate_multiplier ?? 1)
  }
  if (mode === BILLING_MODE_IMAGE && group?.image_rate_independent) {
    return group.image_rate_multiplier ?? 1
  }
  return fallbackRate
}

const activeRequestRate = computed(() =>
  resolveNonTokenRate(currentGroup.value, effectiveBillingMode.value, activeEffectiveRate.value)
)

function formatRate(val: number): string {
  const rounded = Math.round(val * 1000) / 1000
  return String(rounded)
}

function formatTokenPrice(value: number | null | undefined, rate: number): string {
  if (value == null || !Number.isFinite(value)) return PLAZA_EMPTY_TEXT
  return formatScaled(value * rate, PER_MILLION, MIN_DECIMALS)
}

function formatOfficialTokenPrice(value: number | null | undefined): string {
  if (value == null || !Number.isFinite(value)) return PLAZA_EMPTY_TEXT
  return formatScaled(value, PER_MILLION, MIN_DECIMALS)
}

function formatRequestPrice(value: number | null | undefined): string {
  if (value == null || !Number.isFinite(value)) return PLAZA_EMPTY_TEXT
  return formatScaled(value * activeRequestRate.value, 1, MIN_DECIMALS)
}

const resolvedTokenIntervals = computed<UserPricingInterval[]>(() => {
  const pricing = effectivePricing.value
  const raw = pricing?.intervals ?? []
  if (!pricing || raw.length === 0) return []
  return [...raw]
    .sort((a, b) => a.min_tokens - b.min_tokens)
    .map((iv) => resolveIntervalPrices(iv, pricing))
})

const hasTierCache = computed(() =>
  resolvedTokenIntervals.value.some(
    (iv) => iv.cache_read_price != null || iv.cache_write_price != null
  )
)

const baseTokenPrices = computed(() => {
  const p = effectivePricing.value
  const first = resolvedTokenIntervals.value[0]
  return {
    input_price: p?.input_price ?? first?.input_price ?? null,
    output_price: p?.output_price ?? first?.output_price ?? null,
    cache_read_price: p?.cache_read_price ?? first?.cache_read_price ?? null,
    cache_write_price: p?.cache_write_price ?? first?.cache_write_price ?? null,
    cache_write_1h_price: p?.cache_write_1h_price ?? first?.cache_write_1h_price ?? null,
    cache_write_12h_price: p?.cache_write_12h_price ?? first?.cache_write_12h_price ?? null
  }
})

interface TokenPriceRow {
  key: string
  label: string
  paidText: string
  officialText: string
  savingsPercent: number | null
}

const tokenPriceRows = computed<TokenPriceRow[]>(() => {
  const paidBase = baseTokenPrices.value
  const off = effectiveOfficial.value
  const rate = activeEffectiveRate.value

  const defs: Array<{
    key: string
    label: string
    paidUnit: number | null | undefined
    officialUnit: number | null | undefined
    alwaysShow?: boolean
  }> = [
    {
      key: 'input',
      label: t('modelPlaza.drawer.itemInput'),
      paidUnit: paidBase.input_price,
      officialUnit: off?.input_price,
      alwaysShow: true
    },
    {
      key: 'output',
      label: t('modelPlaza.drawer.itemOutput'),
      paidUnit: paidBase.output_price,
      officialUnit: off?.output_price,
      alwaysShow: true
    },
    {
      key: 'cacheRead',
      label: t('modelPlaza.drawer.itemCacheRead'),
      paidUnit: paidBase.cache_read_price,
      officialUnit: off?.cache_read_price
    },
    {
      key: 'cacheWrite5m',
      label: t('modelPlaza.drawer.itemCacheWrite5m'),
      paidUnit: paidBase.cache_write_price,
      officialUnit: off?.cache_write_price
    },
    {
      key: 'cacheWrite1h',
      label: t('modelPlaza.drawer.itemCacheWrite1h'),
      paidUnit: paidBase.cache_write_1h_price,
      officialUnit: off?.cache_write_1h_price
    },
    {
      key: 'cacheWrite12h',
      label: t('modelPlaza.drawer.itemCacheWrite12h'),
      paidUnit: paidBase.cache_write_12h_price,
      officialUnit: off?.cache_write_12h_price
    }
  ]

  return defs
    .filter((d) => d.alwaysShow || d.paidUnit != null || d.officialUnit != null)
    .map((d) => {
      const paidVal = d.paidUnit != null ? d.paidUnit * rate : null
      return {
        key: d.key,
        label: d.label,
        paidText: formatTokenPrice(d.paidUnit, rate),
        officialText: formatOfficialTokenPrice(d.officialUnit),
        savingsPercent: computeOfficialSavingsPercent(paidVal, d.officialUnit)
      }
    })
})

const reasoningMultipliers = computed<[string, number][]>(() => {
  const map = effectivePricing.value?.reasoning_effort_multipliers
  return REASONING_EFFORT_LEVELS.flatMap((effort) => {
    const mult = map?.[effort]
    return typeof mult === 'number' && Number.isFinite(mult) && mult > 0
      ? [[effort, mult] as [string, number]]
      : []
  })
})

const nonTokenItemLabel = computed(() => {
  if (effectiveBillingMode.value === BILLING_MODE_IMAGE) {
    return t('modelPlaza.drawer.itemPerImage')
  }
  if (effectiveBillingMode.value === BILLING_MODE_VIDEO) {
    return t('modelPlaza.drawer.itemPerVideo')
  }
  return t('modelPlaza.drawer.itemPerRequest')
})

const perUnitSuffix = computed(() =>
  effectiveBillingMode.value === BILLING_MODE_IMAGE
    ? t('modelPlaza.table.perUnitImage')
    : t('modelPlaza.table.perUnitRequest')
)

const requestIntervals = computed<UserPricingInterval[]>(() =>
  (effectivePricing.value?.intervals ?? []).filter((iv) => iv.per_request_price != null)
)

const showLongContextDisabledNote = computed(
  () =>
    currentGroup.value?.long_context_pricing_enabled === false &&
    (effectiveOfficial.value?.intervals?.length ?? 0) > 1
)

function formatTokenCount(n: number): string {
  if (n >= 1_000_000) return `${Math.round((n / 1_000_000) * 100) / 100}M`
  if (n >= 1_000) return `${Math.round((n / 1_000) * 100) / 100}K`
  return String(n)
}

function tierLabel(iv: UserPricingInterval): string {
  if (iv.tier_label) return iv.tier_label
  return iv.max_tokens == null
    ? `>${formatTokenCount(iv.min_tokens)}`
    : `≤${formatTokenCount(iv.max_tokens)}`
}

const timePricingPeriods = computed<PlazaTimePricingPeriod[]>(
  () => effectiveTimePricing.value?.periods ?? []
)

function formatTimeWindow(p: PlazaTimePricingPeriod): string {
  const clock = (v: string) => v.replace(/^(\d{2}:\d{2}):00$/, '$1')
  return `${clock(p.start_time)}–${clock(p.end_time)}`
}

const peakNote = computed(() => {
  if (!currentGroup.value || !hasPeakRate(currentGroup.value)) return ''
  const win = formatPeakRateWindow(
    currentGroup.value,
    serverTimezoneLabel(appStore.cachedPublicSettings?.server_utc_offset)
  )
  if (!win) return ''
  return t('modelPlaza.detail.peakNote', {
    window: win,
    multiplier: currentGroup.value.peak_rate_multiplier
  })
})

// 1M context real cache savings simulator
const cacheSavingsSim = computed(() => {
  if (effectiveBillingMode.value !== BILLING_MODE_TOKEN) return null
  const inputUnit = baseTokenPrices.value.input_price
  if (inputUnit == null || !Number.isFinite(inputUnit) || inputUnit <= 0) return null

  const h24 = detail.value?.windows?.h24
  const refMultVal =
    h24?.reference_multiplier ?? props.model?.metrics?.reference_multiplier ?? null
  const refMultState =
    h24?.reference_multiplier_state ?? props.model?.metrics?.reference_multiplier_state ?? null
  const cacheVal = h24?.cache_read_ratio ?? props.model?.metrics?.cache_read_ratio ?? null
  const cacheState = h24?.cache_state ?? props.model?.metrics?.cache_state ?? null

  const refFormatted = formatRefMultiplier(refMultVal, refMultState)
  const cacheFormatted = formatCacheRate(cacheVal, cacheState)
  if (!refFormatted.valid || refMultVal == null || refMultVal >= 1) return null

  const uncachedPerMillion = inputUnit * activeEffectiveRate.value * PER_MILLION
  const cachedPerMillion = uncachedPerMillion * refMultVal
  const savedPercent = Math.max(1, Math.min(99, Math.round((1 - refMultVal) * 100)))
  const barPercent = Math.max(4, Math.min(100, Math.round(refMultVal * 100)))

  return {
    uncachedText: formatScaled(uncachedPerMillion, 1, MIN_DECIMALS),
    cachedText: formatScaled(cachedPerMillion, 1, MIN_DECIMALS),
    savedPercent,
    barPercent,
    cacheRateText: cacheFormatted.valid ? cacheFormatted.text : PLAZA_EMPTY_TEXT,
    multiplierText: refFormatted.text
  }
})

// Cross-group comparison rows (prefers detail.group_comparisons, falls back to allGroups)
interface GroupComparisonRow {
  groupName: string
  isCurrent: boolean
  isExclusive: boolean
  subscriptionType: string
  effectiveRate: number
  priceText: string
  cache: PlazaFormattedMetric
}

function formatComparisonPrice(
  pricing: UserSupportedModelPricing | null,
  rate: number,
  groupMeta: ModelPlazaGroup | null
): string {
  if (!pricing) return PLAZA_EMPTY_TEXT
  const mode = (pricing.billing_mode || BILLING_MODE_TOKEN) as BillingMode
  if (mode !== BILLING_MODE_TOKEN) {
    const reqRate = resolveNonTokenRate(groupMeta, mode, rate)
    const unit =
      pricing.per_request_price ??
      pricing.intervals?.find((iv) => iv.per_request_price != null)?.per_request_price ??
      null
    if (unit == null) return PLAZA_EMPTY_TEXT
    const suffix =
      mode === BILLING_MODE_IMAGE
        ? t('modelPlaza.table.perUnitImage')
        : t('modelPlaza.table.perUnitRequest')
    return `${formatScaled(unit * reqRate, 1, MIN_DECIMALS)} ${suffix}`
  }
  const firstInterval = pricing.intervals?.length
    ? resolveIntervalPrices(
        [...pricing.intervals].sort((a, b) => a.min_tokens - b.min_tokens)[0],
        pricing
      )
    : null
  const inUnit = pricing.input_price ?? firstInterval?.input_price ?? null
  const outUnit = pricing.output_price ?? firstInterval?.output_price ?? null
  if (inUnit == null && outUnit == null) return PLAZA_EMPTY_TEXT
  return `${formatTokenPrice(inUnit, rate)} / ${formatTokenPrice(outUnit, rate)}`
}

const comparisonRows = computed<GroupComparisonRow[]>(() => {
  const fromDetail = detail.value?.group_comparisons
  if (fromDetail && fromDetail.length > 0) {
    return fromDetail.map((item) => {
      const groupMeta = props.allGroups.find((g) => g.name === item.group_name) ?? null
      return {
        groupName: item.group_name,
        isCurrent: item.group_name === effectiveGroupName.value,
        isExclusive: item.is_exclusive,
        subscriptionType: item.subscription_type,
        effectiveRate: item.effective_rate,
        priceText: formatComparisonPrice(item.pricing, item.effective_rate, groupMeta),
        cache: formatCacheRate(item.metrics?.cache_read_ratio, item.metrics?.cache_state)
      }
    })
  }

  // Immediate fallback from allGroups before detail loads or on degraded failure
  if (!modelName.value) return []
  const rows: GroupComparisonRow[] = []
  for (const g of props.allGroups) {
    const matched = g.models.find((m) => m.name === modelName.value)
    if (!matched) continue
    const effRate = g.user_rate_multiplier ?? g.rate_multiplier
    rows.push({
      groupName: g.name,
      isCurrent: g.name === effectiveGroupName.value,
      isExclusive: g.is_exclusive,
      subscriptionType: g.subscription_type,
      effectiveRate: effRate,
      priceText: formatComparisonPrice(matched.pricing, effRate, g),
      cache: formatCacheRate(matched.metrics?.cache_read_ratio, matched.metrics?.cache_state)
    })
  }
  return rows
})

onBeforeUnmount(() => {
  abortController?.abort()
  if (copiedIdTimer) clearTimeout(copiedIdTimer)
  if (copiedShareTimer) clearTimeout(copiedShareTimer)
})
</script>
