import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import ModelsListConfigField from '../ModelsListConfigField.vue'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

describe('ModelsListConfigField', () => {
  it('keeps display entries when toggled without introducing an admission field', async () => {
    const config = { enabled: false, models: ['gpt-5.5'] }
    const wrapper = mount(ModelsListConfigField, { props: { modelValue: config } })
    await wrapper.get('button').trigger('click')
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual([{ enabled: true, models: ['gpt-5.5'] }])
    expect(config).toEqual({ enabled: false, models: ['gpt-5.5'] })
  })

  it('preserves multiline typing and normalizes entries on change', async () => {
    const wrapper = mount(ModelsListConfigField, {
      props: { modelValue: { enabled: true, models: ['gpt-5.5'] } }
    })
    const input = wrapper.get('textarea')
    input.element.value = 'gpt-5.5\n'
    await input.trigger('input')
    expect(input.element.value).toBe('gpt-5.5\n')
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
    input.element.value = ' gpt-5.5 \n\n gpt-5.4\ngpt-5.5'
    await input.trigger('change')
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual([
      { enabled: true, models: ['gpt-5.5', 'gpt-5.4'] }
    ])
  })
})
