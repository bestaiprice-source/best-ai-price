import { describe, it, expect, beforeEach } from 'vitest'
import { openSupportWidget, useSupportWidget } from '../useSupportWidget'

const { open, prefill, topic, amount } = useSupportWidget()

describe('useSupportWidget', () => {
  beforeEach(() => {
    open.value = false
    prefill.value = ''
    topic.value = ''
    amount.value = ''
  })

  it('opens the widget without pre-filling when no message is given', () => {
    openSupportWidget()
    expect(open.value).toBe(true)
    expect(prefill.value).toBe('')
    expect(topic.value).toBe('')
    expect(amount.value).toBe('')
  })

  it('opens the widget and pre-fills the message box', () => {
    openSupportWidget('Hello! Please issue an invoice.')
    expect(open.value).toBe(true)
    expect(prefill.value).toBe('Hello! Please issue an invoice.')
  })

  it('carries the request tag for the admin alert', () => {
    openSupportWidget('Hello!', { topic: 'manual_invoice', amount: '250.00' })
    expect(topic.value).toBe('manual_invoice')
    expect(amount.value).toBe('250.00')
  })

  it('leaves an existing tag alone when opened without options', () => {
    openSupportWidget('Hello!', { topic: 'manual_invoice', amount: '250.00' })
    open.value = false
    openSupportWidget()
    expect(topic.value).toBe('manual_invoice')
    expect(amount.value).toBe('250.00')
  })

  it('opens without a message and keeps the existing pre-fill', () => {
    openSupportWidget('first')
    open.value = false
    openSupportWidget()
    expect(open.value).toBe(true)
    expect(prefill.value).toBe('first')
  })
})
