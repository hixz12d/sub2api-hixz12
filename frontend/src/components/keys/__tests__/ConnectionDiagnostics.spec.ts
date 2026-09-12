import { afterEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

const { models, stream, copy } = vi.hoisted(() => ({ models: vi.fn(), stream: vi.fn(), copy: vi.fn() }))
vi.mock('@/utils/connectionDiagnostics', async importOriginal => ({ ...await importOriginal<typeof import('@/utils/connectionDiagnostics')>(), probeDiagnosticModels: models, probeDiagnosticStream: stream }))
vi.mock('@/composables/useClipboard', () => ({ useClipboard: () => ({ copyToClipboard: copy }) }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
import ConnectionDiagnostics from '../ConnectionDiagnostics.vue'

async function render() {
  const wrapper = mount(ConnectionDiagnostics, { props: { baseUrl: 'https://example.com', apiKey: 'sk-user-secret123', initialModel: 'gpt-5', windows: true, authMode: 'api-key' } })
  wrapper.get('details').element.open = true
  await wrapper.get('details').trigger('toggle')
  return wrapper
}
afterEach(() => vi.clearAllMocks())

describe('ConnectionDiagnostics', () => {
  it('sends no automatic probe and requires separate consent for a billable stream', async () => {
    models.mockResolvedValue(['gpt-5']); stream.mockResolvedValue(undefined)
    const wrapper = await render()
    expect(models).not.toHaveBeenCalled(); expect(stream).not.toHaveBeenCalled()
    expect(wrapper.get('[data-testid="check-stream"]').attributes('disabled')).toBeDefined()
    await wrapper.get('[data-testid="check-models"]').trigger('click'); await flushPromises()
    expect(wrapper.text()).toContain('connectionDiagnostics.available')
    expect(stream).not.toHaveBeenCalled()
    await wrapper.get('[data-testid="allow-paid-test"]').setValue(true)
    await wrapper.get('[data-testid="check-stream"]').trigger('click'); await flushPromises()
    expect(stream).toHaveBeenCalledOnce()
    const reportButton = wrapper.findAll('button').find(button => button.text() === 'connectionDiagnostics.copyReport')!
    await reportButton.trigger('click')
    const report = copy.mock.calls.at(-1)![0]
    expect(report).not.toContain('sk-user-secret123')
    expect(JSON.parse(report).websocket).toBe('not_tested_in_browser')
    wrapper.unmount()
  })
  it('discards pending results when the key changes', async () => {
    let resolve!: (models: string[]) => void
    let signal!: AbortSignal
    models.mockImplementation((_url, _key, requestSignal) => { signal = requestSignal; return new Promise(done => { resolve = done }) })
    const wrapper = await render()
    await wrapper.get('[data-testid="check-models"]').trigger('click')
    await wrapper.setProps({ apiKey: 'sk-new-secret456' })
    expect(signal.aborted).toBe(true)
    resolve(['gpt-5']); await flushPromises()
    expect(wrapper.text()).not.toContain('connectionDiagnostics.available')
    expect(wrapper.get('[data-testid="check-stream"]').attributes('disabled')).toBeDefined()
    wrapper.unmount()
  })
})
