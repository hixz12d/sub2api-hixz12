import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import AccountTestModal from '../AccountTestModal.vue'

const { getAvailableModels, copyToClipboard } = vi.hoisted(() => ({
  getAvailableModels: vi.fn(),
  copyToClipboard: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      getAvailableModels
    }
  }
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({
    copyToClipboard
  })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  const messages: Record<string, string> = {
    'admin.accounts.imagePromptDefault': 'Generate a cute orange cat astronaut sticker on a clean pastel background.'
  }
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, string | number>) => {
        if (key === 'admin.accounts.imageReceived' && params?.count) {
          return `received-${params.count}`
        }
        if (key === 'admin.accounts.imagePreviewAlt' && params?.index) {
          return `test-image-${params.index}`
        }
        return messages[key] || key
      }
    })
  }
})

function createStreamResponse(lines: string[]) {
  const encoder = new TextEncoder()
  const chunks = lines.map((line) => encoder.encode(line))
  let index = 0

  return {
    ok: true,
    body: {
      getReader: () => ({
        read: vi.fn().mockImplementation(async () => {
          if (index < chunks.length) {
            return { done: false, value: chunks[index++] }
          }
          return { done: true, value: undefined }
        })
      })
    }
  } as Response
}

function mountModal(account: Record<string, unknown> = {
  id: 42,
  name: 'Gemini Image Test',
  platform: 'gemini',
  type: 'apikey',
  status: 'active'
}) {
  return mount(AccountTestModal, {
    props: {
      show: false,
      account
    } as any,
    global: {
      stubs: {
        BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' },
        Select: { template: '<div class="select-stub"></div>' },
        TextArea: {
          props: ['modelValue'],
          emits: ['update:modelValue'],
          template: '<textarea class="textarea-stub" :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" />'
        },
        Icon: true
      }
    }
  })
}

describe('AccountTestModal', () => {
  beforeEach(() => {
    getAvailableModels.mockResolvedValue([
      { id: 'gemini-2.0-flash', display_name: 'Gemini 2.0 Flash' },
      { id: 'gemini-2.5-flash-image', display_name: 'Gemini 2.5 Flash Image' },
      { id: 'gemini-3.1-flash-image', display_name: 'Gemini 3.1 Flash Image' }
    ])
    copyToClipboard.mockReset()
    Object.defineProperty(globalThis, 'localStorage', {
      value: {
        getItem: vi.fn((key: string) => (key === 'auth_token' ? 'test-token' : null)),
        setItem: vi.fn(),
        removeItem: vi.fn(),
        clear: vi.fn()
      },
      configurable: true
    })
    global.fetch = vi.fn().mockResolvedValue(
      createStreamResponse([
        'data: {"type":"test_start","model":"gemini-2.5-flash-image"}\n',
        'data: {"type":"image","image_url":"data:image/png;base64,QUJD","mime_type":"image/png"}\n',
        'data: {"type":"test_complete","success":true}\n'
      ])
    ) as any
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('gemini 图片模型测试会携带提示词并渲染图片预览', async () => {
    const wrapper = mountModal()
    await wrapper.setProps({ show: true })
    await flushPromises()

    const promptInput = wrapper.find('textarea.textarea-stub')
    expect(promptInput.exists()).toBe(true)
    await promptInput.setValue('draw a tiny orange cat astronaut')

    const buttons = wrapper.findAll('button')
    const startButton = buttons.find((button) => button.text().includes('admin.accounts.startTest'))
    expect(startButton).toBeTruthy()

    await startButton!.trigger('click')
    await flushPromises()
    await flushPromises()

    expect(global.fetch).toHaveBeenCalledTimes(1)
    const [, request] = (global.fetch as any).mock.calls[0]
    expect(JSON.parse(request.body)).toEqual({
      model_id: 'gemini-3.1-flash-image',
      prompt: 'draw a tiny orange cat astronaut'
    })

    const preview = wrapper.find('img[alt="test-image-1"]')
    expect(preview.exists()).toBe(true)
    expect(preview.attributes('src')).toBe('data:image/png;base64,QUJD')
  })

  it('grok 账号测试默认选择 Grok 模型', async () => {
    getAvailableModels.mockResolvedValue([
      { id: 'grok-4.3', display_name: 'Grok 4.3' },
      { id: 'grok-build-0.1', display_name: 'Grok Build 0.1' }
    ])
    global.fetch = vi.fn().mockResolvedValue(
      createStreamResponse([
        'data: {"type":"test_start","model":"grok-4.3"}\n',
        'data: {"type":"content","text":"ok"}\n',
        'data: {"type":"test_complete","success":true}\n'
      ])
    ) as any

    const wrapper = mountModal({
      id: 13,
      name: 'Grok Account',
      platform: 'grok',
      type: 'oauth',
      status: 'active'
    })
    await wrapper.setProps({ show: true })
    await flushPromises()

    const buttons = wrapper.findAll('button')
    const startButton = buttons.find((button) => button.text().includes('admin.accounts.startTest'))
    expect(startButton).toBeTruthy()

    await startButton!.trigger('click')
    await flushPromises()

    expect(global.fetch).toHaveBeenCalledTimes(1)
    const [, request] = (global.fetch as any).mock.calls[0]
    expect(JSON.parse(request.body)).toEqual({
      model_id: 'grok-4.3',
      prompt: '',
      mode: 'text'
    })
  })

  it('OpenAI Compact 探测会携带 compact 测试模式', async () => {
    getAvailableModels.mockResolvedValue([
      { id: 'gpt-5.4', display_name: 'GPT-5.4' }
    ])
    global.fetch = vi.fn().mockResolvedValue(
      createStreamResponse([
        'data: {"type":"test_complete","success":true}\n'
      ])
    ) as any

    const wrapper = mountModal({
      id: 42,
      name: 'OpenAI OAuth',
      platform: 'openai',
      type: 'oauth',
      status: 'active'
    })
    await wrapper.setProps({ show: true })
    await flushPromises()

    ;(wrapper.vm as any).selectedModelId = 'gpt-5.4'
    ;(wrapper.vm as any).testMode = 'compact'
    await (wrapper.vm as any).startTest()
    await flushPromises()

    expect(global.fetch).toHaveBeenCalledTimes(1)
    const [, request] = (global.fetch as any).mock.calls[0]
    expect(JSON.parse(request.body)).toMatchObject({
      model_id: 'gpt-5.4',
      prompt: '',
      mode: 'compact'
    })
  })

  it('sends a manual question to the selected account without assigning a verdict', async () => {
    getAvailableModels.mockResolvedValue([{ id: 'gpt-5.4', display_name: 'GPT-5.4' }])
    global.fetch = vi.fn().mockResolvedValue(createStreamResponse([
      'data: {"type":"content","text":"fixture answer"}\n',
      'data: {"type":"test_complete","success":true}\n'
    ])) as any
    const wrapper = mountModal({ id: 42, name: 'Question account', platform: 'openai', type: 'oauth', status: 'active' })
    await wrapper.setProps({ show: true })
    await flushPromises()
    ;(wrapper.vm as any).testMode = 'question'
    await flushPromises()
    const input = wrapper.get('textarea.textarea-stub')
    expect((input.element as HTMLTextAreaElement).value).toBe("don't search the internet, who is Thibault Sottiaux on X")
    const question = '  custom question\n'
    await input.setValue(question)
    await (wrapper.vm as any).startTest()
    await flushPromises()
    expect(global.fetch).toHaveBeenCalledTimes(1)
    const [url, request] = (global.fetch as any).mock.calls[0]
    expect(url).toContain('/admin/accounts/42/test')
    expect(JSON.parse(request.body)).toEqual({ model_id: 'gpt-5.4', prompt: question, mode: 'question' })
    expect(wrapper.text()).toContain('fixture answer')
    expect(wrapper.text()).toContain('admin.accounts.openai.questionReceived')
    expect(wrapper.text()).not.toContain('admin.accounts.testCompleted')
    await wrapper.setProps({ show: false })
    await wrapper.setProps({ show: true, account: { id: 43, name: 'Next account', platform: 'openai', type: 'oauth', status: 'active' } as any })
    await flushPromises()
    expect((wrapper.get('textarea.textarea-stub').element as HTMLTextAreaElement).value).toBe(question)
    expect(wrapper.text()).not.toContain('fixture answer')
    expect(global.fetch).toHaveBeenCalledTimes(1)
    await (wrapper.vm as any).startTest()
    await flushPromises()
    expect(global.fetch).toHaveBeenCalledTimes(2)
    expect((global.fetch as any).mock.calls[1][0]).toContain('/admin/accounts/43/test')
    wrapper.unmount()
  })

  it('ignores late failures from the previous account test', async () => {
    getAvailableModels.mockResolvedValue([{ id: 'gpt-5.4', display_name: 'GPT-5.4' }])
    let rejectPrevious!: (reason: Error) => void
    global.fetch = vi.fn()
      .mockImplementationOnce(() => new Promise((_resolve, reject) => { rejectPrevious = reject }))
      .mockResolvedValueOnce(createStreamResponse([
        'data: {"type":"content","text":"current account answer"}\n',
        'data: {"type":"test_complete","success":true}\n'
      ])) as any
    const wrapper = mountModal({ id: 42, name: 'First', platform: 'openai', type: 'oauth', status: 'active' })
    await wrapper.setProps({ show: true })
    await flushPromises()
    ;(wrapper.vm as any).testMode = 'question'
    await flushPromises()
    const previous = (wrapper.vm as any).startTest()
    await wrapper.setProps({ show: false })
    await wrapper.setProps({ show: true, account: { id: 43, name: 'Second', platform: 'openai', type: 'oauth', status: 'active' } as any })
    await flushPromises()
    await (wrapper.vm as any).startTest()
    rejectPrevious(new Error('previous account failure'))
    await previous
    await flushPromises()
    expect(wrapper.text()).toContain('current account answer')
    expect(wrapper.text()).not.toContain('previous account failure')
    expect((wrapper.vm as any).status).toBe('success')
    expect((global.fetch as any).mock.calls[0][1].signal.aborted).toBe(true)
    wrapper.unmount()
  })

  it('does not send empty manual questions or image-model questions', async () => {
    const wrapper = mountModal({ id: 42, name: 'Question account', platform: 'openai', type: 'apikey', status: 'active' })
    ;(wrapper.vm as any).testMode = 'question'
    ;(wrapper.vm as any).selectedModelId = 'gpt-5.4'
    await flushPromises()
    ;(wrapper.vm as any).testPrompt = '  '
    await (wrapper.vm as any).startTest()
    expect(global.fetch).not.toHaveBeenCalled()
    ;(wrapper.vm as any).testPrompt = 'question'
    ;(wrapper.vm as any).selectedModelId = 'gpt-image-1'
    await (wrapper.vm as any).startTest()
    expect(global.fetch).not.toHaveBeenCalled()
    wrapper.unmount()
  })
})
