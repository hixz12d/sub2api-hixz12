import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { PublicSettings } from '@/types'

const store = vi.hoisted(() => ({ cachedPublicSettings: {} as Partial<PublicSettings> }))
vi.mock('@/stores/app', () => ({ useAppStore: () => store }))

import { defaultGroupCapabilityConfig, defaultGroupProbeConfig } from '../channelMonitorGroups'
import { FeatureFlags, isFeatureFlagEnabled, isChannelMonitorGroupViewEnabled, isLLMDetectorUserTestingEnabled } from '@/utils/featureFlags'

describe('group monitor foundation', () => {
  beforeEach(() => { store.cachedPublicSettings = {} })

  it('defaults to disabled and returns independent configuration arrays', () => {
    const probe = defaultGroupProbeConfig()
    const capability = defaultGroupCapabilityConfig()
    expect(probe.enabled).toBe(false)
    expect(probe.include_extra_models).toBe(false)
    expect(capability.enabled).toBe(false)
    expect(capability.targets).toEqual([])
    probe.fixed_account_ids.push(1)
    expect(defaultGroupProbeConfig().fixed_account_ids).toEqual([])
  })

  it('keeps new flags closed on old public-settings payloads', () => {
    for (const flag of [FeatureFlags.channelMonitorGroups, FeatureFlags.llmDetector, FeatureFlags.llmDetectorUserTesting, FeatureFlags.channelMonitorOutputTPS]) {
      expect(isFeatureFlagEnabled(flag)).toBe(false)
    }
    expect(isChannelMonitorGroupViewEnabled()).toBe(false)
    expect(isLLMDetectorUserTestingEnabled()).toBe(false)
  })

  it('requires V2 for group view but not for private detection', () => {
    store.cachedPublicSettings = {
      channel_monitor_enabled: false, channel_monitor_mode: 'v1',
      channel_monitor_group_view_enabled: true,
      llm_detector_enabled: true, llm_detector_user_testing_enabled: true,
    }
    expect(isChannelMonitorGroupViewEnabled()).toBe(false)
    expect(isLLMDetectorUserTestingEnabled()).toBe(true)
    store.cachedPublicSettings.channel_monitor_enabled = true
    store.cachedPublicSettings.channel_monitor_mode = 'v2'
    expect(isChannelMonitorGroupViewEnabled()).toBe(true)
  })
})
