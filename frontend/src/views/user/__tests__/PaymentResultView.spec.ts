import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import PaymentResultView from '../PaymentResultView.vue'

const { pollOrderStatus, verifyOrderPublic } = vi.hoisted(() => ({
  pollOrderStatus: vi.fn(),
  verifyOrderPublic: vi.fn(),
}))

const mockRoute = {
  query: {
    order_id: '12',
    out_trade_no: 'sub2_20260913McR9IPNs',
    status: 'success',
  },
}

const mockRouter = {
  push: vi.fn(),
}

vi.mock('vue-router', () => ({
  useRoute: () => mockRoute,
  useRouter: () => mockRouter,
}))

vi.mock('@/stores/payment', () => ({
  usePaymentStore: () => ({
    pollOrderStatus,
  }),
}))

vi.mock('@/api/payment', () => ({
  paymentAPI: {
    verifyOrderPublic,
  },
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, defaultVal?: string) => defaultVal || key,
  }),
}))

describe('PaymentResultView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders numeric baseAmount without NaN when fee_rate is missing or 0', async () => {
    pollOrderStatus.mockResolvedValue({
      id: 12,
      out_trade_no: 'sub2_20260913McR9IPNs',
      pay_amount: 100,
      amount: 100,
      status: 'COMPLETED',
      payment_type: 'nowpayments',
      fee_rate: 0,
    })

    const wrapper = mount(PaymentResultView)
    await flushPromises()

    expect(wrapper.text()).not.toContain('NaN')
    expect(wrapper.text()).toContain('$100.00')
    expect(wrapper.text()).toContain('#12')
    expect(wrapper.text()).toContain('sub2_20260913McR9IPNs')
  })

  it('renders numeric baseAmount correctly when fee_rate is undefined', async () => {
    pollOrderStatus.mockResolvedValue({
      id: 12,
      out_trade_no: 'sub2_20260913McR9IPNs',
      pay_amount: 100,
      amount: 100,
      status: 'PAID',
      payment_type: 'nowpayments',
      fee_rate: undefined,
    })

    const wrapper = mount(PaymentResultView)
    await flushPromises()

    expect(wrapper.text()).not.toContain('NaN')
    expect(wrapper.text()).toContain('$100.00')
  })
})
