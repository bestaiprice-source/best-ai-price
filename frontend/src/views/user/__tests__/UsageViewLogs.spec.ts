import { describe, expect, it, vi, beforeEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import UsageView from '../UsageView.vue'

const {
  query,
  getStatsByDateRange,
  getDashboardStats,
  listErrors,
  list,
  getAvailable,
  getLedger,
  showError,
  showWarning,
  showSuccess,
  showInfo,
} = vi.hoisted(() => ({
  query: vi.fn(),
  getStatsByDateRange: vi.fn(),
  getDashboardStats: vi.fn(),
  listErrors: vi.fn(),
  list: vi.fn(),
  getAvailable: vi.fn(),
  getLedger: vi.fn(),
  showError: vi.fn(),
  showWarning: vi.fn(),
  showSuccess: vi.fn(),
  showInfo: vi.fn(),
}))

const messages: Record<string, string> = {
  'usage.tabRequests': 'Requests',
  'usage.tabErrors': 'Errors',
  'usage.tabOperations': 'Operations',
  'usage.rpm': 'RPM',
  'usage.tpm': 'TPM',
  'usage.liveHint': 'live · last 5 min',
  'usage.usedQuota': 'Used quota',
  'usage.usedQuotaHint': 'spent today',
  'usage.estimatedDaily': 'Estimated daily',
  'usage.estimatedDailyHint': 'projected from today',
  'usage.operation': 'Operation',
  'usage.operationTopUp': 'Top up',
  'usage.operationConsume': 'Consume',
  'usage.operationRefund': 'Refund',
  'usage.operationAdjustment': 'Adjustment',
  'usage.amount': 'Amount',
  'usage.reference': 'Reference',
  'usage.status': 'Status',
  'usage.description': 'Description',
  'usage.time': 'Time',
  'usage.noOperations': 'No money operations in this range',
  'usage.filterModel': 'Model',
  'usage.filterAllModels': 'All models',
  'usage.filterGroup': 'Group',
  'usage.filterAllGroups': 'All groups',
  'usage.filterType': 'Type',
  'usage.filterAllTypes': 'All types',
  'usage.stream': 'Stream',
  'usage.sync': 'Sync',
  'usage.model': 'Model',
  'usage.tokens': 'Tokens',
  'usage.cost': 'Cost',
  'usage.duration': 'Duration',
  'usage.userAgent': 'User Agent',
}

vi.mock('@/api', () => ({
  usageAPI: { query, getStatsByDateRange, getDashboardStats, listErrors },
  keysAPI: { list },
  userGroupsAPI: { getAvailable },
}))

vi.mock('@/api/billing', () => ({ getLedger }))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError, showWarning, showSuccess, showInfo }),
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => messages[key] ?? key }),
  }
})

// Lightweight stand-in for the real (virtual-scroll) table: it renders the
// scoped cell slots so the badges/amounts can be asserted directly.
const DataTableStub = {
  props: {
    columns: { type: Array, default: () => [] },
    data: { type: Array, default: () => [] },
    loading: Boolean,
  },
  template: `
    <div class="dt">
      <div v-if="data.length === 0" class="dt-empty"><slot name="empty" /></div>
      <div v-for="(row, i) in data" :key="i" class="dt-row">
        <span v-for="col in columns" :key="col.key" class="dt-cell">
          <slot :name="'cell-' + col.key" :row="row" :value="row[col.key]">{{ row[col.key] }}</slot>
        </span>
      </div>
    </div>
  `,
}

const mountView = () =>
  mount(UsageView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        TablePageLayout: {
          template:
            '<div><slot name="actions" /><slot name="filters" /><slot name="table" /><slot name="pagination" /><slot /></div>',
        },
        DataTable: DataTableStub,
        Pagination: true,
        EmptyState: true,
        Select: true,
        DateRangePicker: true,
        Icon: true,
        Teleport: true,
      },
    },
  })

const setupOf = (wrapper: ReturnType<typeof mountView>) => (wrapper.vm as any).$?.setupState

describe('user UsageView logs screen', () => {
  beforeEach(() => {
    vi.clearAllMocks()

    query.mockResolvedValue({ items: [], total: 0, pages: 1 })
    listErrors.mockResolvedValue({ items: [], total: 0, pages: 1 })
    getStatsByDateRange.mockResolvedValue({
      total_requests: 0,
      total_tokens: 0,
      total_cost: 0,
      avg_duration_ms: 0,
    })
    getDashboardStats.mockResolvedValue({ rpm: 12.5, tpm: 3456, today_actual_cost: 1.2 })
    list.mockResolvedValue({ items: [] })
    getAvailable.mockResolvedValue([
      { id: 7, name: 'yunwu' },
      { id: 9, name: 'premium' },
    ])
    getLedger.mockResolvedValue({
      items: [
        {
          id: 'dep_1',
          type: 'deposit',
          amount_usd: 50,
          currency: 'USD',
          network: 'usdttrc20',
          status: 'paid',
          description: 'USDT top-up',
          reference: 'sub2api_order_1',
          created_at: '2026-10-23T10:00:00Z',
          confirmed_at: '2026-10-23T10:01:00Z',
        },
        {
          id: 'use_1',
          type: 'usage',
          amount_usd: -1.2,
          currency: 'USD',
          network: '',
          status: 'completed',
          description: 'API usage',
          reference: 'req_8f21c4a0b7',
          created_at: '2026-10-23T11:00:00Z',
          confirmed_at: null,
        },
      ],
      total: 2,
      page: 1,
      page_size: 20,
      total_deposit_usd: 50,
      total_usage_usd: 1.2,
      total_refund_usd: 0,
      total_adjustment_usd: 0,
    })

    ;(globalThis as any).ResizeObserver = class {
      observe() {}
      disconnect() {}
    }
  })

  it('shows live RPM/TPM and the used / estimated daily quota', async () => {
    const wrapper = mountView()
    await flushPromises()

    const text = wrapper.text()
    expect(text).toContain('RPM')
    expect(text).toContain('12.5')
    expect(text).toContain('TPM')
    expect(text).toContain('live · last 5 min')
    expect(text).toContain('Used quota')
    expect(text).toContain('$1.2000')
    expect(text).toContain('Estimated daily')

    // The projection is derived from today's spend and the hours elapsed, so it
    // can only ever be at or above what has actually been spent today.
    const setup = setupOf(wrapper)
    expect(setup.estimatedDailyCost).toBeGreaterThanOrEqual(1.2)
  })

  it('lists money operations with Consume / Top up badges in their own tab', async () => {
    const wrapper = mountView()
    await flushPromises()

    const setup = setupOf(wrapper)
    setup.switchUsageTab('operations')
    await flushPromises()

    const text = wrapper.text()
    expect(text).toContain('Top up')
    expect(text).toContain('Consume')
    expect(text).toContain('+$50.0000')
    expect(text).toContain('-$1.2000')
    expect(text).toContain('sub2api_order_1')

    expect(getLedger).toHaveBeenCalled()
  })

  it('sends the model, group and request-type filters to the usage query', async () => {
    const wrapper = mountView()
    await flushPromises()
    query.mockClear()

    const setup = setupOf(wrapper)
    setup.filters.model = 'gpt-5.2'
    setup.filters.group_id = 7
    setup.filters.stream = true
    setup.applyFilters()
    await flushPromises()

    const params = query.mock.calls[0][0]
    expect(params.model).toBe('gpt-5.2')
    expect(params.group_id).toBe(7)
    expect(params.stream).toBe(true)
  })

  it('maps the group select to the numeric group id and back to "all"', async () => {
    const wrapper = mountView()
    await flushPromises()

    const setup = setupOf(wrapper)
    expect(setup.groupFilter).toBe('')
    expect(setup.groupOptions).toEqual([
      { value: '', label: 'All groups' },
      { value: 7, label: 'yunwu' },
      { value: 9, label: 'premium' },
    ])

    setup.groupFilter = '9'
    await flushPromises()
    expect(setup.filters.group_id).toBe(9)

    setup.groupFilter = ''
    await flushPromises()
    expect(setup.filters.group_id).toBeUndefined()
  })

  it('clears the new filters on reset', async () => {
    const wrapper = mountView()
    await flushPromises()

    const setup = setupOf(wrapper)
    setup.filters.model = 'gpt-5.2'
    setup.filters.group_id = 7
    setup.filters.stream = false

    setup.resetFilters()
    await flushPromises()

    expect(setup.filters.model).toBeUndefined()
    expect(setup.filters.group_id).toBeUndefined()
    expect(setup.filters.stream).toBeUndefined()
  })
})
