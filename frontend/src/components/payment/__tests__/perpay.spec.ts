import { describe, expect, it } from 'vitest'
import { PROVIDER_CONFIG_FIELDS, PROVIDER_SUPPORTED_TYPES, PROVIDER_CALLBACK_PATHS } from '../providerConfig'
import { decidePaymentLaunch } from '../paymentFlow'

describe('PerPay hosted checkout', () => {
  it('supports Alipay and keeps both service credentials sensitive', () => {
    expect(PROVIDER_SUPPORTED_TYPES.perpay).toEqual(['alipay'])
    expect(PROVIDER_CONFIG_FIELDS.perpay.filter(field => field.sensitive).map(field => field.key)).toEqual(['apiSecret', 'webhookSecret'])
    expect(PROVIDER_CALLBACK_PATHS.perpay.notifyUrl).toBe('/api/v1/payment/webhook/perpay')
  })
  it('opens the hosted checkout on desktop and mobile without exposing the static payment code', () => {
    for (const isMobile of [false, true]) {
      const decision = decidePaymentLaunch({
        order_id: 1, amount: 10, pay_amount: 10, fee_rate: 0,
        pay_url: 'https://pay.example.com/checkout/token', payment_mode: '',
        expires_at: new Date(Date.now() + 300000).toISOString(),
      }, { visibleMethod: 'alipay', orderType: 'balance', isMobile })
      expect(decision.kind).toBe('redirect_waiting')
      expect(decision.paymentState.qrCode).toBe('')
      expect(decision.paymentState.payUrl).toBe('https://pay.example.com/checkout/token')
    }
  })
})
