import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import KeyTestModal from '../KeyTestModal.vue'
import { listKeyTestModels, runKeyQuestion } from '@/api/keyTest'
import type { ApiKey } from '@/types'

vi.mock('@/api/keyTest', async importOriginal => ({
  ...await importOriginal<typeof import('@/api/keyTest')>(),
  listKeyTestModels: vi.fn(), runKeyQuestion: vi.fn()
}))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/composables/useClipboard', () => ({ useClipboard: () => ({ copyToClipboard: vi.fn().mockResolvedValue(true) }) }))

let wrapper: VueWrapper
const key = { id: 1, key: 'sk-user-private', name: 'My key', status: 'active', group_id: 3, group: { name: 'My group' } } as ApiKey
function render() {
  wrapper = mount(KeyTestModal, {
    props: { apiKey: key },
    global: { stubs: {
      BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' },
      Select: {
        props: ['modelValue', 'options'], emits: ['update:modelValue'],
        template: '<select :value="modelValue" @change="$emit(\'update:modelValue\', $event.target.value)"><option v-for="option in options" :value="option.value">{{ option.label }}</option></select>'
      }
    } }
  })
  return wrapper
}
function button(key: string) {
  return wrapper.findAll('button').find(b => b.text() === key)!
}

beforeEach(() => {
  vi.mocked(listKeyTestModels).mockReset().mockResolvedValue(['gpt-future', 'gpt-next'])
  vi.mocked(runKeyQuestion).mockReset()
})
afterEach(() => { wrapper?.unmount(); vi.useRealTimers() })

describe('user manual Q&A dialog', () => {
  it('does not send a question on open and only submits the chosen model and question', async () => {
    render()
    await flushPromises()
    expect(runKeyQuestion).not.toHaveBeenCalled()
    expect(wrapper.text()).not.toContain(key.key)
    await wrapper.get('select').setValue('gpt-next')
    await wrapper.get('textarea').setValue('  My exact question\n')
    vi.mocked(runKeyQuestion).mockImplementation(async (_key, _model, _prompt, _signal, onText) => {
      onText('<img src=x onerror=alert(1)>')
      return { truncated: false }
    })
    await button('keys.questionTest.start').trigger('click')
    await flushPromises()
    expect(runKeyQuestion).toHaveBeenCalledWith(key.key, 'gpt-next', '  My exact question\n', expect.any(AbortSignal), expect.any(Function))
    expect(wrapper.get('[data-testid="key-test-answer"]').text()).toBe('<img src=x onerror=alert(1)>')
    expect(wrapper.find('img').exists()).toBe(false)
    expect(wrapper.text()).toContain('keys.questionTest.status.complete')
  })

  it('stops, preserves partial answers and ignores stale callbacks after restarting', async () => {
    render()
    await flushPromises()
    const calls: { signal: AbortSignal; onText: (text: string) => void; resolve: (value: { truncated: boolean }) => void }[] = []
    vi.mocked(runKeyQuestion).mockImplementation((_key, _model, _prompt, signal, onText) => new Promise(resolve => { calls.push({ signal, onText, resolve }) }))
    await button('keys.questionTest.start').trigger('click')
    calls[0].onText('partial')
    await button('keys.questionTest.stop').trigger('click')
    expect(calls[0].signal.aborted).toBe(true)
    expect(wrapper.text()).toContain('partial')
    expect(wrapper.text()).toContain('keys.questionTest.status.stopped')
    await button('keys.questionTest.start').trigger('click')
    calls[0].onText('old answer')
    calls[0].resolve({ truncated: false })
    calls[1].onText('new answer')
    await flushPromises()
    expect(wrapper.get('[data-testid="key-test-answer"]').text()).toBe('new answer')
    expect(wrapper.text()).toContain('keys.questionTest.status.running')
    await button('common.close').trigger('click')
    expect(calls[1].signal.aborted).toBe(true)
    expect(wrapper.emitted('close')).toHaveLength(1)
    calls[1].resolve({ truncated: false })
  })

  it('prevents empty, oversized and inactive-key requests', async () => {
    render()
    await flushPromises()
    await wrapper.get('textarea').setValue('   ')
    expect(button('keys.questionTest.start').attributes('disabled')).toBeDefined()
    await wrapper.get('textarea').setValue('中'.repeat(1400))
    expect(button('keys.questionTest.start').attributes('disabled')).toBeDefined()
    await wrapper.get('textarea').setValue('question')
    await wrapper.setProps({ apiKey: { ...key, status: 'inactive' } })
    expect(button('keys.questionTest.start').attributes('disabled')).toBeDefined()
    expect(runKeyQuestion).not.toHaveBeenCalled()
  })

  it('cancels discovery when the dialog closes', async () => {
    vi.mocked(listKeyTestModels).mockImplementation(() => new Promise(() => {}))
    render()
    const discoverySignal = vi.mocked(listKeyTestModels).mock.calls[0][1]
    await button('common.close').trigger('click')
    expect(discoverySignal.aborted).toBe(true)
  })

  it('shows an actionable empty-model state', async () => {
    vi.mocked(listKeyTestModels).mockResolvedValue([])
    render()
    await flushPromises()
    expect(wrapper.text()).toContain('keys.questionTest.noModels')
    expect(button('keys.questionTest.start').attributes('disabled')).toBeDefined()
    vi.mocked(listKeyTestModels).mockResolvedValue(['new-model'])
    await button('keys.questionTest.retry').trigger('click')
    await flushPromises()
    expect(button('keys.questionTest.start').attributes('disabled')).toBeUndefined()
  })
})
