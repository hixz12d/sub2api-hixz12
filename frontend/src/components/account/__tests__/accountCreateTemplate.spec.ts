import { describe, expect, it } from 'vitest'
import {
  applyAccountCreateTemplateGroups,
  buildAccountCreateTemplatePreview,
  emptyAccountCreateTemplateValues,
  filterAccountCreateTemplates,
  findDefaultAccountCreateTemplate,
  normalizeAccountCreateTemplateValues,
  type AccountCreateTemplate,
} from '../accountCreateTemplate'

describe('accountCreateTemplate helpers', () => {
  it('normalizes invalid values and keeps valid ones', () => {
    const values = normalizeAccountCreateTemplateValues({
      proxy_id: -1,
      concurrency: 0,
      load_factor: 2,
      priority: 3,
      rate_multiplier: -2,
      group_ids: [38, 38, 0, 41],
      quota_limit: 0,
      openai_ws_mode: 'ctx_pool',
      openai_compact_mode: 'maybe' as never,
      codex_fingerprint_mode: 'session',
      auto_pause_on_expired: false,
      tls_fingerprint_enabled: true,
      tls_fingerprint_profile_id: -1,
    })

    expect(values.proxy_id).toBeNull()
    expect(values.concurrency).toBe(10)
    expect(values.load_factor).toBe(2)
    expect(values.priority).toBe(3)
    expect(values.rate_multiplier).toBe(1)
    expect(values.group_ids).toEqual([38, 41])
    expect(values.quota_limit).toBeNull()
    expect(values.openai_ws_mode).toBe('ctx_pool')
    expect(values.openai_compact_mode).toBe('auto')
    expect(values.codex_fingerprint_mode).toBe('session')
    expect(values.auto_pause_on_expired).toBe(false)
    expect(values.tls_fingerprint_profile_id).toBe(-1)
  })

  it('filters templates and finds the default for a platform/type', () => {
    const items: AccountCreateTemplate[] = [
      {
        id: 'free',
        name: 'Free',
        platform: 'openai',
        type: 'oauth',
        is_default: false,
        include_groups: false,
        values: emptyAccountCreateTemplateValues(),
      },
      {
        id: 'team',
        name: 'Team',
        platform: 'openai',
        type: 'oauth',
        is_default: true,
        include_groups: true,
        values: emptyAccountCreateTemplateValues(),
      },
      {
        id: 'anthropic',
        name: 'Claude',
        platform: 'anthropic',
        type: 'oauth',
        is_default: true,
        include_groups: false,
        values: emptyAccountCreateTemplateValues(),
      },
    ]

    expect(filterAccountCreateTemplates(items, 'openai', 'oauth').map((item) => item.id)).toEqual(['free', 'team'])
    expect(findDefaultAccountCreateTemplate(items, 'openai', 'oauth')?.id).toBe('team')
  })

  it('applies groups only when requested', () => {
    const values = {
      ...emptyAccountCreateTemplateValues(),
      group_ids: [38, 41],
    }
    expect(applyAccountCreateTemplateGroups([1], values, false)).toEqual([1])
    expect(applyAccountCreateTemplateGroups([1], values, true)).toEqual([38, 41])
  })

  it('builds a preview that can omit groups', () => {
    const values = {
      ...emptyAccountCreateTemplateValues(),
      proxy_id: 7,
      concurrency: 3,
      openai_ws_mode: 'ctx_pool' as const,
      codex_fingerprint_mode: 'session' as const,
      group_ids: [38],
    }
    expect(buildAccountCreateTemplatePreview(values, {
      includeGroups: true,
      proxies: [{ id: 7, name: 'US-1' } as never],
      groups: [{ id: 38, name: 'Team A' } as never],
    })).toBe('代理=US-1 · WS=ctx_pool · 指纹=session · 并发=3 · 分组=Team A')
    expect(buildAccountCreateTemplatePreview(values, { includeGroups: false })).toBe(
      '代理=#7 · WS=ctx_pool · 指纹=session · 并发=3',
    )
  })
})
